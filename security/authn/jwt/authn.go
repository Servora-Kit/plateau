package jwt

import (
	"context"
	"fmt"
	"net/http"
	"reflect"
	"strings"
	"time"

	jwtconfpb "github.com/Servora-Kit/plateau/api/gen/go/plateau/security/authn/jwt/v1"
	jwtkeypb "github.com/Servora-Kit/plateau/api/gen/go/plateau/security/jwt/v1"
	security "github.com/Servora-Kit/plateau/security"
	securityjwt "github.com/Servora-Kit/plateau/security/jwt"
	"github.com/golang-jwt/jwt/v5"
	"github.com/zitadel/oidc/v3/pkg/client"
)

const expectedTokenType = "JWT"

// Authenticator verifies Bearer JWTs for one immutable Resource Server profile.
type Authenticator struct {
	verifier        *securityjwt.Verifier
	claimsValidator *jwt.Validator
}

// Option configures an injected, application-owned verifier.
type Option func(*options)
type options struct {
	verifier *securityjwt.Verifier
	injected int
}

// WithVerifier skips discovery and uses the supplied trusted key source.
func WithVerifier(verifier *securityjwt.Verifier) Option {
	return func(o *options) { o.verifier = verifier; o.injected++ }
}

// New discovers or loads a verifier. Cleanup releases owned refresh workers.
func New(ctx context.Context, config *jwtconfpb.JwtAuthnConfig, opts ...Option) (*Authenticator, func(), error) {
	if ctx == nil || config == nil {
		return nil, nil, fmt.Errorf("jwt authn: context and config are required")
	}
	issuer, audience := config.GetIssuer(), config.GetAudience()
	if issuer == "" || strings.TrimSpace(issuer) != issuer || audience == "" || strings.TrimSpace(audience) != audience {
		return nil, nil, fmt.Errorf("jwt authn: issuer and audience must be non-empty without surrounding whitespace")
	}
	o := options{}
	for _, option := range opts {
		if option != nil {
			option(&o)
		}
	}
	count := o.injected
	if len(config.GetVerificationKeys()) != 0 {
		count++
	}
	if config.GetJwks() != nil {
		count++
	}
	if count > 1 || o.injected > 0 && o.verifier == nil {
		return nil, nil, fmt.Errorf("jwt authn: exactly one key source may be selected")
	}
	verifier := o.verifier
	cleanup := func() {}
	var err error
	switch {
	case o.injected > 0:
	case len(config.GetVerificationKeys()) > 0:
		verifier, err = securityjwt.NewFromConfig(config.GetVerificationKeys())
	default:
		jwks := config.GetJwks()
		if jwks == nil {
			metadata, discoveryErr := client.Discover(ctx, issuer, &http.Client{Timeout: 5 * time.Second})
			if discoveryErr != nil {
				return nil, nil, fmt.Errorf("jwt authn: discover issuer: %w", discoveryErr)
			}
			jwks = &jwtkeypb.JWKS{Uri: metadata.JwksURI}
		}
		verifier, cleanup, err = securityjwt.NewJWKS(ctx, jwks)
	}
	if err != nil {
		return nil, nil, err
	}
	return &Authenticator{
		verifier:        verifier,
		claimsValidator: jwt.NewValidator(jwt.WithIssuer(issuer), jwt.WithAudience(audience), jwt.WithExpirationRequired(), jwt.WithIssuedAt()),
	}, cleanup, nil
}

// Authenticate validates one Authorization header and maps fresh verified claims to one stable Actor.
func Authenticate[T jwt.Claims](ctx context.Context, authenticator *Authenticator, authorization string, newClaims func() T, mapActor func(T) (security.Actor, error)) (security.Actor, error) {
	if ctx == nil {
		return security.Actor{}, fmt.Errorf("jwt authn: context is nil")
	}
	if err := ctx.Err(); err != nil {
		return security.Actor{}, err
	}
	if !validAuthenticator(authenticator) {
		return security.Actor{}, fmt.Errorf("jwt authn: authenticator is invalid")
	}
	if newClaims == nil {
		return security.Actor{}, fmt.Errorf("jwt authn: claims factory is nil")
	}
	if mapActor == nil {
		return security.Actor{}, fmt.Errorf("jwt authn: actor mapper is nil")
	}
	tokenString, err := bearerToken(authorization)
	if err != nil {
		return security.Actor{}, err
	}
	claims, err := claimsFromFactory(newClaims)
	if err != nil {
		return security.Actor{}, err
	}
	token, err := authenticator.verifier.VerifySignature(ctx, tokenString, claims)
	if err != nil {
		return security.Actor{}, fmt.Errorf("%w: %w", ErrInvalidToken, err)
	}
	if err := validateTokenType(token); err != nil {
		return security.Actor{}, err
	}
	if err := authenticator.claimsValidator.Validate(claims); err != nil {
		return security.Actor{}, fmt.Errorf("%w: %w", ErrInvalidToken, err)
	}
	actor, err := mapActor(claims)
	if err != nil {
		return security.Actor{}, fmt.Errorf("%w: %w", ErrActorMapping, err)
	}
	if !actor.Valid() || actor.Type == security.ActorTypeAnonymous {
		return security.Actor{}, fmt.Errorf("%w: mapper returned invalid authenticated Actor", ErrActorMapping)
	}
	return actor, nil
}

func claimsFromFactory[T jwt.Claims](factory func() T) (claims T, err error) {
	var zero T
	defer func() {
		if recover() != nil {
			claims = zero
			err = fmt.Errorf("jwt authn: claims factory returned uninitialized claims")
		}
	}()
	claims = factory()
	value := reflect.ValueOf(claims)
	if !value.IsValid() || value.Kind() != reflect.Pointer {
		return zero, fmt.Errorf("jwt authn: claims factory must return a non-nil pointer")
	}
	if value.IsNil() {
		return zero, fmt.Errorf("jwt authn: claims factory returned nil")
	}
	_, _ = claims.GetExpirationTime()
	_, _ = claims.GetIssuedAt()
	_, _ = claims.GetNotBefore()
	_, _ = claims.GetIssuer()
	_, _ = claims.GetSubject()
	_, _ = claims.GetAudience()
	return claims, nil
}

func validateTokenType(token *jwt.Token) error {
	if token == nil {
		return fmt.Errorf("%w: token is nil", ErrInvalidToken)
	}
	tokenType, ok := token.Header["typ"].(string)
	if !ok || tokenType != expectedTokenType {
		return fmt.Errorf("%w: unexpected token type", ErrInvalidToken)
	}
	return nil
}

func validAuthenticator(authenticator *Authenticator) bool {
	return authenticator != nil && authenticator.verifier != nil && authenticator.claimsValidator != nil
}
