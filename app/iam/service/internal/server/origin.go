package server

import (
	"context"
	"net/http"
	"net/url"

	accountpb "github.com/Servora-Kit/plateau/api/gen/go/iam/account/v1"
	authnpb "github.com/Servora-Kit/plateau/api/gen/go/iam/authn/v1"
	sessionpb "github.com/Servora-Kit/plateau/api/gen/go/iam/session/v1"
	securityerrorspb "github.com/Servora-Kit/plateau/api/gen/go/plateau/security/errors/v1"
	"github.com/go-kratos/kratos/v3/middleware"
	"github.com/go-kratos/kratos/v3/transport"
	khttp "github.com/go-kratos/kratos/v3/transport/http"
)

func isCookieMutationOperation(operation string) bool {
	switch operation {
	case authnpb.OperationAuthnServiceLogin,
		accountpb.OperationAccountServiceUpdateProfile,
		accountpb.OperationAccountServiceChangePassword,
		sessionpb.OperationSessionServiceLogout:
		return true
	default:
		return false
	}
}

func cookieOriginProtection(publicOrigin string) middleware.Middleware {
	return func(handler middleware.Handler) middleware.Handler {
		return func(ctx context.Context, request any) (any, error) {
			operation := httpOperation(ctx)
			if !isCookieMutationOperation(operation) {
				return handler(ctx, request)
			}
			httpRequest, ok := khttp.RequestFromServerContext(ctx)
			if ok && httpRequest != nil && requestOriginAllowed(httpRequest, publicOrigin) {
				return handler(ctx, request)
			}
			return nil, securityerrorspb.ErrorSecurityErrorReasonPermissionDenied("请求来源验证失败：%s", operation)
		}
	}
}

func httpOperation(ctx context.Context) string {
	if serverTransport, ok := transport.FromServerContext(ctx); ok {
		return serverTransport.Operation()
	}
	return ""
}

func requestOriginAllowed(request *http.Request, publicOrigin string) bool {
	origins := request.Header.Values("Origin")
	if len(origins) != 0 {
		return len(origins) == 1 && validOrigin(origins[0]) && origins[0] == publicOrigin
	}

	referers := request.Header.Values("Referer")
	if len(referers) != 1 {
		return false
	}
	parsed, err := url.Parse(referers[0])
	if err != nil || parsed.Scheme == "" || parsed.Host == "" || parsed.User != nil || parsed.Opaque != "" || parsed.Fragment != "" {
		return false
	}
	return parsed.Scheme+"://"+parsed.Host == publicOrigin
}

func validOrigin(raw string) bool {
	parsed, err := url.Parse(raw)
	return err == nil && parsed.Scheme != "" && parsed.Host != "" && parsed.User == nil && parsed.Opaque == "" && parsed.Path == "" && parsed.RawQuery == "" && !parsed.ForceQuery && parsed.Fragment == "" && parsed.String() == raw
}
