// Package jwt provides claims-neutral RS256 signing, verification, KID, and
// typed claims context helpers.
package jwt

import (
	"context"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"fmt"

	jwtlib "github.com/golang-jwt/jwt/v5"
)

// Signer signs arbitrary JWT claims with one RSA private key.
type Signer struct {
	key *rsa.PrivateKey
	kid string
}

// NewSigner accepts PEM-encoded PKCS#1 or PKCS#8 RSA private keys.
func NewSigner(privateKeyPEM []byte) (*Signer, error) {
	block, _ := pem.Decode(privateKeyPEM)
	if block == nil {
		return nil, fmt.Errorf("jwt: failed to decode PEM block")
	}

	key, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	if err != nil {
		pkcs8Key, pkcs8Err := x509.ParsePKCS8PrivateKey(block.Bytes)
		if pkcs8Err != nil {
			return nil, fmt.Errorf("jwt: failed to parse private key: %w", err)
		}
		var ok bool
		key, ok = pkcs8Key.(*rsa.PrivateKey)
		if !ok {
			return nil, fmt.Errorf("jwt: PKCS#8 key is not RSA")
		}
	}
	if err := key.Validate(); err != nil {
		return nil, fmt.Errorf("jwt: invalid RSA private key: %w", err)
	}

	der, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	if err != nil {
		return nil, fmt.Errorf("jwt: failed to marshal public key: %w", err)
	}
	hash := sha256.Sum256(der)
	kid := hex.EncodeToString(hash[:])[:8]

	return &Signer{key: key, kid: kid}, nil
}

// Sign creates an RS256 token and writes this signer's stable KID header.
func (signer *Signer) Sign(claims jwtlib.Claims) (string, error) {
	token := jwtlib.NewWithClaims(jwtlib.SigningMethodRS256, claims)
	token.Header["kid"] = signer.kid
	return token.SignedString(signer.key)
}

// PrivateKey returns the configured RSA private key.
func (signer *Signer) PrivateKey() *rsa.PrivateKey {
	return signer.key
}

// PublicKey returns the configured RSA public key.
func (signer *Signer) PublicKey() *rsa.PublicKey {
	return &signer.key.PublicKey
}

// KID returns the stable public-key-derived key ID.
func (signer *Signer) KID() string {
	return signer.kid
}

// Verifier selects registered RSA public keys by KID.
type Verifier struct {
	publicKeys map[string]*rsa.PublicKey
}

// NewVerifier creates an empty verifier.
func NewVerifier() *Verifier {
	return &Verifier{publicKeys: make(map[string]*rsa.PublicKey)}
}

// AddKey registers one RSA public key under an exact KID.
func (verifier *Verifier) AddKey(kid string, publicKey *rsa.PublicKey) {
	verifier.publicKeys[kid] = publicKey
}

// Verify parses and validates one RS256 token into caller-owned claims.
func (verifier *Verifier) Verify(tokenString string, claims jwtlib.Claims) error {
	_, err := jwtlib.ParseWithClaims(
		tokenString,
		claims,
		verifier.keyFunc,
		jwtlib.WithValidMethods([]string{jwtlib.SigningMethodRS256.Alg()}),
	)
	return err
}

func (verifier *Verifier) keyFunc(token *jwtlib.Token) (any, error) {
	if token.Method != jwtlib.SigningMethodRS256 {
		return nil, fmt.Errorf("jwt: unexpected signing method: %v", token.Header["alg"])
	}

	kid, ok := token.Header["kid"].(string)
	if !ok || kid == "" {
		return nil, fmt.Errorf("jwt: missing kid in token header")
	}
	publicKey, exists := verifier.publicKeys[kid]
	if !exists {
		return nil, fmt.Errorf("jwt: unknown kid: %s", kid)
	}
	return publicKey, nil
}

type claimsKey struct{}

// NewContext stores caller-owned typed claims without interpreting them.
func NewContext[T any](ctx context.Context, claims *T) context.Context {
	return context.WithValue(ctx, claimsKey{}, claims)
}

// FromContext reads caller-owned typed claims.
func FromContext[T any](ctx context.Context) (*T, bool) {
	claims, ok := ctx.Value(claimsKey{}).(*T)
	return claims, ok
}
