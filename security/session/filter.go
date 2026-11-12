package session

import (
	"fmt"
	"mime"
	"net/http"
	"net/url"
	"slices"
	"strings"

	"github.com/alexedwards/scs/v2"
)

const (
	unsupportedJSONMediaBody = `{"code":415,"reason":"UNSUPPORTED_MEDIA_TYPE","message":"Content-Type must be application/json"}` + "\n"
	forbiddenOriginBody      = `{"code":403,"reason":"FORBIDDEN","message":"Origin is not allowed"}` + "\n"
)

// FilterConfig 声明应用公开入口及三类浏览器会话路径范围：
//
//   - JSONPrefixes：浏览器 JSON RPC 路由前缀（须以 "/" 开头且以 "/" 结尾，如 "/v1/iam/"、"/v1/admin/"）。
//     非安全方法要求 Content-Type 为 application/json（否则 415），并拒绝跨站 Sec-Fetch-Site 或不匹配
//     PublicOrigin 的 Origin 头（否则 403），校验通过后再执行 LoadAndSave。
//   - SameOriginPaths：精确匹配的应用自有导航或回调路径（须以 "/" 开头，如 "/authorize/callback"）。
//     非安全方法拒绝跨站 Sec-Fetch-Site 或不匹配 PublicOrigin 的 Origin 头（否则 403），不强制 JSON 请求体，
//     校验通过后执行 LoadAndSave。
//   - CrossOriginPaths：精确匹配的标准跨站协议入口（须以 "/" 开头，如 OIDC "/authorize"、"/end_session"）。
//     允许外部依赖方发起导航或协议表单请求，仅执行 LoadAndSave，跨站安全性由协议层（redirect_uri、state、id_token_hint）保证。
//   - 未命中上述范围的路径（如 Bearer 令牌端点、JWKS、健康检查）原样透传，不读写会话存储。
type FilterConfig struct {
	PublicOrigin     string
	JSONPrefixes     []string
	SameOriginPaths  []string
	CrossOriginPaths []string
}

// NewFilter 按应用声明的公开入口与路径分类构造浏览器会话 HTTP 过滤器。
func NewFilter(manager *scs.SessionManager, cfg FilterConfig) (func(http.Handler) http.Handler, error) {
	if manager == nil {
		return nil, fmt.Errorf("session: manager is required")
	}
	publicOrigin, err := parseOrigin(cfg.PublicOrigin)
	if err != nil {
		return nil, err
	}
	jsonPrefixes := make([]string, 0, len(cfg.JSONPrefixes))
	for _, raw := range cfg.JSONPrefixes {
		prefix := strings.TrimSpace(raw)
		if !strings.HasPrefix(prefix, "/") || !strings.HasSuffix(prefix, "/") {
			return nil, fmt.Errorf("session: JSON prefix %q must start and end with '/'", raw)
		}
		if !slices.Contains(jsonPrefixes, prefix) {
			jsonPrefixes = append(jsonPrefixes, prefix)
		}
	}
	sameOriginPaths, err := normalizeExactPaths("same-origin", cfg.SameOriginPaths, jsonPrefixes)
	if err != nil {
		return nil, err
	}
	crossOriginPaths, err := normalizeExactPaths("cross-origin", cfg.CrossOriginPaths, jsonPrefixes)
	if err != nil {
		return nil, err
	}
	for _, path := range crossOriginPaths {
		if slices.Contains(sameOriginPaths, path) {
			return nil, fmt.Errorf("session: path %q cannot appear in both SameOriginPaths and CrossOriginPaths", path)
		}
	}
	if len(jsonPrefixes) == 0 && len(sameOriginPaths) == 0 && len(crossOriginPaths) == 0 {
		return nil, fmt.Errorf("session: filter requires at least one route")
	}

	return func(next http.Handler) http.Handler {
		if next == nil {
			panic("session: next handler is nil")
		}
		browser := LoadAndSave(manager)(next)
		sameOriginBrowser := sameOriginGuard(browser, publicOrigin)
		jsonBrowser := jsonWriteGuard(browser, publicOrigin)
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			path := r.URL.Path
			for _, prefix := range jsonPrefixes {
				if strings.HasPrefix(path, prefix) {
					jsonBrowser.ServeHTTP(w, r)
					return
				}
			}
			if slices.Contains(sameOriginPaths, path) {
				sameOriginBrowser.ServeHTTP(w, r)
				return
			}
			if slices.Contains(crossOriginPaths, path) {
				browser.ServeHTTP(w, r)
				return
			}
			next.ServeHTTP(w, r)
		})
	}, nil
}

func normalizeExactPaths(kind string, rawPaths []string, jsonPrefixes []string) ([]string, error) {
	paths := make([]string, 0, len(rawPaths))
	for _, raw := range rawPaths {
		path := strings.TrimSpace(raw)
		if !strings.HasPrefix(path, "/") {
			return nil, fmt.Errorf("session: %s path %q must start with '/'", kind, raw)
		}
		for _, prefix := range jsonPrefixes {
			if strings.HasPrefix(path, prefix) {
				return nil, fmt.Errorf("session: %s path %q overlaps JSON prefix %q", kind, path, prefix)
			}
		}
		if !slices.Contains(paths, path) {
			paths = append(paths, path)
		}
	}
	return paths, nil
}

func parseOrigin(rawOrigin string) (string, error) {
	raw := strings.TrimRight(strings.TrimSpace(rawOrigin), "/")
	if raw == "" {
		return "", fmt.Errorf("session: public origin is empty")
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("session: invalid public origin: %w", err)
	}
	scheme := strings.ToLower(parsed.Scheme)
	if (scheme != "http" && scheme != "https") || parsed.Host == "" || parsed.User != nil || parsed.Path != "" || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", fmt.Errorf("session: public origin must be a bare http(s) origin")
	}
	return scheme + "://" + strings.ToLower(parsed.Host), nil
}

func jsonWriteGuard(next http.Handler, publicOrigin string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if isSafeMethod(r.Method) {
			next.ServeHTTP(w, r)
			return
		}
		mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
		if err != nil || mediaType != "application/json" {
			writeFilterError(w, http.StatusUnsupportedMediaType, unsupportedJSONMediaBody)
			return
		}
		if !isAllowedOrigin(r, publicOrigin) {
			writeFilterError(w, http.StatusForbidden, forbiddenOriginBody)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func sameOriginGuard(next http.Handler, publicOrigin string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if isSafeMethod(r.Method) {
			next.ServeHTTP(w, r)
			return
		}
		if !isAllowedOrigin(r, publicOrigin) {
			writeFilterError(w, http.StatusForbidden, forbiddenOriginBody)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func isSafeMethod(method string) bool {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return true
	default:
		return false
	}
}

func isAllowedOrigin(r *http.Request, publicOrigin string) bool {
	if fetchSite := r.Header.Get("Sec-Fetch-Site"); fetchSite != "" && fetchSite != "same-origin" && fetchSite != "none" {
		return false
	}
	if origins := r.Header.Values("Origin"); len(origins) > 1 || (len(origins) == 1 && origins[0] != "" && origins[0] != publicOrigin) {
		return false
	}
	return true
}

func writeFilterError(w http.ResponseWriter, statusCode int, body string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(statusCode)
	_, _ = w.Write([]byte(body))
}
