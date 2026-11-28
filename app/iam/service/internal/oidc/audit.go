package oidc

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"unicode/utf8"

	"github.com/Servora-Kit/plateau/security/audit"
	httptransport "github.com/go-kratos/kratos/v3/transport/http"
)

const maxAuditDetailFieldBytes = 256

var oidcRoutePaths = []string{
	"/.well-known/openid-configuration",
	"/authorize",
	"/authorize/callback",
	"/oauth/token",
	"/oauth/introspect",
	"/userinfo",
	"/revoke",
	"/end_session",
	"/keys",
}

func RegisterHTTPServer(server *httptransport.Server, provider *IAMProvider, auditor audit.Auditor) {
	for _, path := range oidcRoutePaths {
		var handler http.Handler = provider
		switch path {
		case "/authorize/callback":
			handler = audit.HTTPHandler(auditor, "plateau.iam.oidc.authorize.v1", audit.CategoryAuthn)(provider)
		case "/end_session":
			handler = audit.HTTPHandler(auditor, "plateau.iam.oidc.end_session.v1", audit.CategoryAuthn)(provider)
		case "/oauth/token":
			handler = audit.HTTPHandler(
				auditor,
				"plateau.iam.oidc.token.v1",
				audit.CategoryAuthn,
				audit.WithHTTPOnlyErrors(),
				audit.WithHTTPPostHook(enrichFailedTokenAudit),
			)(provider)
		}
		server.Handle(path, handler)
	}
}

func enrichFailedTokenAudit(ctx context.Context, r *http.Request, statusCode int) {
	if statusCode < http.StatusBadRequest {
		return
	}
	clientID, _, ok := r.BasicAuth()
	if ok && clientID != "" {
		if decoded, err := url.QueryUnescape(clientID); err == nil && decoded != "" {
			clientID = decoded
		}
	} else {
		clientID = strings.TrimSpace(r.PostForm.Get("client_id"))
	}
	grantType := strings.TrimSpace(r.PostForm.Get("grant_type"))
	clientID = truncateUTF8Bytes(strings.TrimSpace(clientID), maxAuditDetailFieldBytes)
	grantType = truncateUTF8Bytes(grantType, maxAuditDetailFieldBytes)
	detail := make(map[string]any, 2)
	if grantType != "" {
		detail["grant_type"] = grantType
	}
	if clientID != "" {
		detail["client_id"] = clientID
	}
	if len(detail) > 0 {
		audit.SetDetail(ctx, detail)
	}
}

func truncateUTF8Bytes(s string, maxBytes int) string {
	if len(s) <= maxBytes {
		return s
	}
	s = s[:maxBytes]
	for len(s) > 0 && !utf8.ValidString(s) {
		s = s[:len(s)-1]
	}
	return s
}

func redirectOAuthErrorCode(location string) string {
	if location == "" {
		return ""
	}
	parsed, err := url.Parse(location)
	if err != nil {
		return ""
	}
	errParam := strings.TrimSpace(parsed.Query().Get("error"))
	if errParam == "" {
		return ""
	}
	return "OAUTH_" + strings.ToUpper(strings.ReplaceAll(truncateUTF8Bytes(errParam, 64), "-", "_"))
}
