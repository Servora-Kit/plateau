package jwt

import (
	"context"
	"crypto/rsa"
	"fmt"
	"reflect"
	"strings"

	jwtconfpb "github.com/Servora-Kit/plateau/api/gen/go/plateau/security/authn/jwt/v1"
	security "github.com/Servora-Kit/plateau/security"
	securityjwt "github.com/Servora-Kit/plateau/security/jwt"
	"github.com/golang-jwt/jwt/v5"
)

const expectedTokenType = "JWT"

// Authenticator verifies Bearer JWTs for one immutable Resource Server profile.
type Authenticator struct {
	verifier        *securityjwt.Verifier
	claimsValidator *jwt.Validator
}

// New constructs one immutable JWT Authenticator from generated Resource Server config.
func New(config *jwtconfpb.JwtAuthnConfig) (*Authenticator, error) {
	if config == nil {
		return nil, fmt.Errorf("jwt authn: config is nil")
	}
	issuer := strings.TrimSpace(config.GetIssuer())
	if issuer == "" || issuer != config.GetIssuer() {
		return nil, fmt.Errorf("jwt authn: issuer must be non-empty without surrounding whitespace")
	}
	audience := strings.TrimSpace(config.GetAudience())
	if audience == "" || audience != config.GetAudience() {
		return nil, fmt.Errorf("jwt authn: audience must be non-empty without surrounding whitespace")
	}
	keyConfigs := config.GetVerificationKeys()
	if len(keyConfigs) == 0 {
		return nil, fmt.Errorf("jwt authn: verification key set is empty")
	}
	keys := make(map[string]*rsa.PublicKey, len(keyConfigs))
	for index, keyConfig := range keyConfigs {
		if keyConfig == nil {
			return nil, fmt.Errorf("jwt authn: verification_keys[%d] is nil", index)
		}
		kid := strings.TrimSpace(keyConfig.GetKid())
		if kid == "" || kid != keyConfig.GetKid() {
			return nil, fmt.Errorf("jwt authn: verification_keys[%d].kid must be non-empty without surrounding whitespace", index)
		}
		if _, exists := keys[kid]; exists {
			return nil, fmt.Errorf("jwt authn: duplicate verification KID %q", kid)
		}
		data, err := publicKeyData(keyConfig)
		if err != nil {
			return nil, fmt.Errorf("jwt authn: verification_keys[%d] KID %q: %w", index, kid, err)
		}
		publicKey, err := parsePublicKey(data)
		if err != nil {
			return nil, fmt.Errorf("jwt authn: verification_keys[%d] KID %q: %w", index, kid, err)
		}
		keys[kid] = publicKey
	}
	verifier, err := securityjwt.New(keys)
	if err != nil {
		return nil, fmt.Errorf("jwt authn: verifier config: %w", err)
	}
	return &Authenticator{
		verifier: verifier,
		claimsValidator: jwt.NewValidator(
			jwt.WithIssuer(issuer),
			jwt.WithAudience(audience),
			jwt.WithExpirationRequired(),
			jwt.WithIssuedAt(),
		),
	}, nil
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
	token, err := authenticator.verifier.VerifySignature(tokenString, claims)
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
