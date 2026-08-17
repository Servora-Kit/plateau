package jwt

import (
	"context"
	"errors"
	"fmt"

	authnpb "github.com/Servora-Kit/servora-platform/api/gen/go/platform/security/authn/v1"
	securityerrorspb "github.com/Servora-Kit/servora-platform/api/gen/go/platform/security/errors/v1"
	security "github.com/Servora-Kit/servora-platform/security"
	"github.com/go-kratos/kratos/v3/middleware"
	"github.com/go-kratos/kratos/v3/transport"
	jwtlib "github.com/golang-jwt/jwt/v5"
)

// Server constructs JWT-specific route authentication middleware.
func Server[T jwtlib.Claims](
	authenticator *Authenticator,
	newClaims func() T,
	mapActor func(T) (security.Actor, error),
	rules map[string]*authnpb.AuthnRule,
) middleware.Middleware {
	if authenticator == nil || authenticator.verifier == nil || authenticator.claimsValidator == nil {
		panic("jwt authn: authenticator is invalid")
	}
	if newClaims == nil {
		panic("jwt authn: claims factory is nil")
	}
	if mapActor == nil {
		panic("jwt authn: actor mapper is nil")
	}
	// newClaims must return a fresh mutable pointer on every invocation.
	return func(handler middleware.Handler) middleware.Handler {
		return func(ctx context.Context, request any) (any, error) {
			if ctx == nil {
				return nil, apiError(fmt.Errorf("jwt authn: context is nil"))
			}
			serverTransport, ok := transport.FromServerContext(ctx)
			if !ok || serverTransport == nil {
				return nil, apiError(fmt.Errorf("jwt authn: server transport is missing"))
			}
			operation := serverTransport.Operation()
			rule := rules[operation]
			if rule == nil {
				return nil, apiError(fmt.Errorf("jwt authn: no authentication rule for operation %q", operation))
			}
			mode := rule.GetMode()
			switch mode {
			case authnpb.AuthnMode_AUTHN_MODE_PUBLIC:
				return handler(security.WithActor(ctx, security.Actor{Type: security.ActorTypeAnonymous}), request)
			case authnpb.AuthnMode_AUTHN_MODE_REQUIRED:
			default:
				return nil, apiError(fmt.Errorf("jwt authn: unsupported mode %s", mode))
			}

			header := serverTransport.RequestHeader()
			if header == nil {
				return nil, apiError(fmt.Errorf("jwt authn: request header is missing"))
			}
			actor, err := Authenticate(ctx, authenticator, header.Get("Authorization"), newClaims, mapActor)
			if err != nil {
				return nil, apiError(err)
			}
			return handler(security.WithActor(ctx, actor), request)
		}
	}
}

func apiError(err error) error {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	switch {
	case errors.Is(err, ErrMissingCredentials),
		errors.Is(err, ErrMalformedCredentials),
		errors.Is(err, ErrInvalidToken),
		errors.Is(err, ErrActorMapping):
		return securityerrorspb.ErrorSecurityErrorReasonUnauthenticated("authentication failed").WithCause(err)
	default:
		return securityerrorspb.ErrorSecurityErrorReasonInternal("internal authentication error").WithCause(err)
	}
}
