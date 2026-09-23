// Package session 将应用配置接入 SCS，应用直接使用原生管理器和 Store。
package session

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	sessionpb "github.com/Servora-Kit/plateau/api/gen/go/plateau/security/session/v1"
	"github.com/alexedwards/scs/v2"
	"google.golang.org/protobuf/proto"
)

// New 构造独立的管理器；Store 的命名空间和关闭由注入它的应用负责。
func New(config *sessionpb.Session, store scs.Store) (*scs.SessionManager, error) {
	if config == nil || store == nil {
		return nil, fmt.Errorf("session: config and store are required")
	}
	c := proto.Clone(config).(*sessionpb.Session)
	if err := c.Apply(); err != nil {
		return nil, fmt.Errorf("session: config: %w", err)
	}
	if err := c.GetLifetime().CheckValid(); err != nil || c.GetLifetime().AsDuration() <= 0 {
		return nil, fmt.Errorf("session: lifetime must be a positive duration")
	}
	if idle := c.GetIdleTimeout(); idle != nil {
		if err := idle.CheckValid(); err != nil || idle.AsDuration() < 0 || idle.AsDuration() > c.GetLifetime().AsDuration() {
			return nil, fmt.Errorf("session: idle_timeout must be between zero and lifetime")
		}
	}
	cookie := c.GetCookie()
	secure := cookie.Secure == nil || cookie.GetSecure()
	sameSite := http.SameSiteLaxMode
	switch cookie.GetSameSite() {
	case "lax":
	case "strict":
		sameSite = http.SameSiteStrictMode
	case "none":
		if !secure {
			return nil, fmt.Errorf("session: SameSite=None requires Secure")
		}
		sameSite = http.SameSiteNoneMode
	default:
		return nil, fmt.Errorf("session: unsupported same_site")
	}
	value := &http.Cookie{Name: cookie.GetName(), Path: cookie.GetPath(), Domain: cookie.GetDomain(), Secure: secure, HttpOnly: cookie.HttpOnly == nil || cookie.GetHttpOnly(), SameSite: sameSite}
	if err := value.Valid(); err != nil {
		return nil, fmt.Errorf("session: cookie: %w", err)
	}
	if !strings.HasPrefix(value.Path, "/") {
		return nil, fmt.Errorf("session: cookie path must be absolute")
	}
	if strings.HasPrefix(value.Name, "__Host-") && (!value.Secure || value.Domain != "" || value.Path != "/") {
		return nil, fmt.Errorf("session: __Host- cookie requires Secure, Path=/ and no Domain")
	}
	if strings.HasPrefix(value.Name, "__Secure-") && !value.Secure {
		return nil, fmt.Errorf("session: __Secure- cookie requires Secure")
	}
	manager := scs.New()
	manager.Store = store
	manager.Lifetime = c.GetLifetime().AsDuration()
	manager.IdleTimeout = c.GetIdleTimeout().AsDuration()
	manager.HashTokenInStore = true
	manager.Cookie.Name = value.Name
	manager.Cookie.Path = value.Path
	manager.Cookie.Domain = value.Domain
	manager.Cookie.Secure = value.Secure
	manager.Cookie.HttpOnly = value.HttpOnly
	manager.Cookie.SameSite = value.SameSite
	manager.Cookie.Persist = cookie.Persist == nil || cookie.GetPersist()
	manager.ErrorFunc = func(w http.ResponseWriter, _ *http.Request, _ error) {
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		// 终止后续业务正文写入，避免保存失败后继续输出成功响应。
		panic(responseAborted{})
	}
	return manager, nil
}

type loadedKey struct{ manager *scs.SessionManager }
type responseAborted struct{}

// LoadAndSave 使用 SCS 原生生命周期，并标记所属管理器已装载上下文。
func LoadAndSave(manager *scs.SessionManager) func(http.Handler) http.Handler {
	if manager == nil {
		panic("session: manager is nil")
	}
	return func(next http.Handler) http.Handler {
		handler := manager.LoadAndSave(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := context.WithValue(r.Context(), loadedKey{manager}, true)
			next.ServeHTTP(w, r.WithContext(ctx))
		}))
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if value := recover(); value != nil {
					if _, ok := value.(responseAborted); !ok {
						panic(value)
					}
				}
			}()
			handler.ServeHTTP(w, r)
		})
	}
}

// Loaded 区分缺失 HTTP 接线与已加载的匿名会话，不解析传输字段。
func Loaded(ctx context.Context, manager *scs.SessionManager) bool {
	return ctx != nil && manager != nil && ctx.Value(loadedKey{manager}) == true
}
