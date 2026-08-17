package jwt

import (
	"context"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"os"
	"reflect"
	"strings"

	jwtconfpb "github.com/Servora-Kit/servora-platform/api/gen/go/platform/security/authn/jwt/v1"
	jwtkeypb "github.com/Servora-Kit/servora-platform/api/gen/go/platform/security/jwt/v1"
	security "github.com/Servora-Kit/servora-platform/security"
	securityjwt "github.com/Servora-Kit/servora-platform/security/jwt"
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

// Authenticate validates one Authorization header and maps fresh verified claims
// to one stable human or service Actor. newClaims must return a fresh, non-nil,
// mutable pointer on every call and initialize any pointer-embedded registered claims.
func Authenticate[T jwt.Claims](
	ctx context.Context,
	authenticator *Authenticator,
	authorization string,
	newClaims func() T,
	mapActor func(T) (security.Actor, error),
) (security.Actor, error) {
	if ctx == nil {
		return security.Actor{}, fmt.Errorf("jwt authn: context is nil")
	}
	if err := ctx.Err(); err != nil {
		return security.Actor{}, err
	}
	if authenticator == nil || authenticator.verifier == nil || authenticator.claimsValidator == nil {
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

func publicKeyData(config *jwtkeypb.VerificationKey) ([]byte, error) {
	switch source := config.GetSource().(type) {
	case *jwtkeypb.VerificationKey_PublicKeyPem:
		if source == nil || strings.TrimSpace(source.PublicKeyPem) == "" {
			return nil, fmt.Errorf("public_key_pem is empty")
		}
		return []byte(source.PublicKeyPem), nil
	case *jwtkeypb.VerificationKey_PublicKeyPath:
		if source == nil || strings.TrimSpace(source.PublicKeyPath) == "" {
			return nil, fmt.Errorf("public_key_path is empty")
		}
		data, err := os.ReadFile(source.PublicKeyPath)
		if err != nil {
			return nil, fmt.Errorf("read public_key_path: %w", err)
		}
		return data, nil
	default:
		return nil, fmt.Errorf("exactly one public key source is required")
	}
}

func parsePublicKey(data []byte) (*rsa.PublicKey, error) {
	block, _ := pem.Decode(data)
	if block == nil {
		return nil, fmt.Errorf("failed to decode public key PEM")
	}
	if publicKey, err := x509.ParsePKIXPublicKey(block.Bytes); err == nil {
		rsaKey, ok := publicKey.(*rsa.PublicKey)
		if !ok {
			return nil, fmt.Errorf("public key is not RSA")
		}
		return rsaKey, nil
	}
	publicKey, err := x509.ParsePKCS1PublicKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse RSA public key: %w", err)
	}
	return publicKey, nil
}

func bearerToken(authorization string) (string, error) {
	if authorization == "" {
		return "", ErrMissingCredentials
	}
	parts := strings.Fields(authorization)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") || parts[1] == "" {
		return "", ErrMalformedCredentials
	}
	return parts[1], nil
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
