package session

import (
	"context"
	"errors"
	"fmt"

	authnpb "github.com/Servora-Kit/plateau/api/gen/go/plateau/security/authn/v1"
	securityerrorspb "github.com/Servora-Kit/plateau/api/gen/go/plateau/security/errors/v1"
	security "github.com/Servora-Kit/plateau/security"
	authnruntime "github.com/Servora-Kit/plateau/security/authn"
	"github.com/go-kratos/kratos/v3/middleware"
	"github.com/go-kratos/kratos/v3/transport"
)

// Server constructs session-specific route authentication middleware.
func Server[T any](authenticator *Authenticator[T], opts ...authnruntime.Option) middleware.Middleware {
	if !validAuthenticator(authenticator) {
		panic("session authn: authenticator is invalid")
	}
	rules := authnruntime.NewRules(opts...)
	return func(handler middleware.Handler) middleware.Handler {
		return func(ctx context.Context, request any) (any, error) {
			if ctx == nil {
				return nil, apiError(fmt.Errorf("session authn: context is nil"))
			}
			serverTransport, ok := transport.FromServerContext(ctx)
			if !ok || serverTransport == nil {
				return nil, apiError(fmt.Errorf("session authn: server transport is missing"))
			}
			operation := serverTransport.Operation()
			rule := rules[operation]
			if rule == nil {
				return nil, apiError(fmt.Errorf("session authn: no authentication rule for operation %q", operation))
			}
			switch rule.GetMode() {
			case authnpb.AuthnMode_AUTHN_MODE_PUBLIC:
				return handler(security.WithActor(ctx, security.Actor{Type: security.ActorTypeAnonymous}), request)
			case authnpb.AuthnMode_AUTHN_MODE_REQUIRED:
			default:
				return nil, apiError(fmt.Errorf("session authn: unsupported mode %s", rule.GetMode()))
			}
			header := serverTransport.RequestHeader()
			if header == nil {
				return nil, apiError(fmt.Errorf("session authn: request header is missing"))
			}
			credential, err := cookieCredential(header.Get("Cookie"), authenticator.cookieName)
			if err != nil {
				return nil, apiError(err)
			}
			trusted, err := authenticator.authenticate(ctx, credential)
			if err != nil {
				return nil, apiError(err)
			}
			return handler(trusted, request)
		}
	}
}

func apiError(err error) error {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	switch {
	case errors.Is(err, ErrMissingCredentials), errors.Is(err, ErrInvalidCredentials), errors.Is(err, ErrActorMapping):
		return securityerrorspb.ErrorSecurityErrorReasonUnauthenticated("authentication failed").WithCause(err)
	case errors.Is(err, ErrDependencyUnavailable):
		return securityerrorspb.ErrorSecurityErrorReasonUnavailable("authentication service unavailable").WithCause(err)
	default:
		return securityerrorspb.ErrorSecurityErrorReasonInternal("internal authentication error").WithCause(err)
	}
}
