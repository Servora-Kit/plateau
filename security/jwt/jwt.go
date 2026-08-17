// Package jwt provides claims-neutral RS256 signing and verification primitives.
package jwt

import (
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"reflect"

	"github.com/golang-jwt/jwt/v5"
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

	key, pkcs1Err := x509.ParsePKCS1PrivateKey(block.Bytes)
	if pkcs1Err != nil {
		pkcs8Key, pkcs8Err := x509.ParsePKCS8PrivateKey(block.Bytes)
		if pkcs8Err != nil {
			return nil, fmt.Errorf("jwt: failed to parse private key as PKCS#1 or PKCS#8: %w", errors.Join(pkcs1Err, pkcs8Err))
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
func (signer *Signer) Sign(claims jwt.Claims) (string, error) {
	if signer == nil || signer.key == nil {
		return "", fmt.Errorf("jwt: signer is nil")
	}
	if claims == nil || nilValue(claims) {
		return "", fmt.Errorf("jwt: claims are nil")
	}
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	token.Header["kid"] = signer.kid
	return token.SignedString(signer.key)
}

// PublicKey returns a detached copy of this signer's RSA public key.
func (signer *Signer) PublicKey() *rsa.PublicKey {
	if signer == nil || signer.key == nil || signer.key.PublicKey.N == nil {
		return nil
	}
	return &rsa.PublicKey{N: new(big.Int).Set(signer.key.PublicKey.N), E: signer.key.PublicKey.E}
}

// KID returns the stable public-key-derived key ID.
func (signer *Signer) KID() string {
	if signer == nil {
		return ""
	}
	return signer.kid
}

// Verifier selects registered RSA public keys by KID and verifies signatures.
type Verifier struct {
	publicKeys map[string]*rsa.PublicKey
	parser     *jwt.Parser
}

// New validates and snapshots an immutable KID-to-public-key set.
func New(publicKeys map[string]*rsa.PublicKey) (*Verifier, error) {
	if len(publicKeys) == 0 {
		return nil, fmt.Errorf("jwt: public key set is empty")
	}
	cloned := make(map[string]*rsa.PublicKey, len(publicKeys))
	for kid, publicKey := range publicKeys {
		if kid == "" {
			return nil, fmt.Errorf("jwt: public key KID is empty")
		}
		if publicKey == nil || publicKey.N == nil || publicKey.N.Sign() <= 0 || publicKey.E <= 1 {
			return nil, fmt.Errorf("jwt: public key for KID %q is invalid", kid)
		}
		cloned[kid] = &rsa.PublicKey{N: new(big.Int).Set(publicKey.N), E: publicKey.E}
	}
	return &Verifier{
		publicKeys: cloned,
		parser: jwt.NewParser(
			jwt.WithValidMethods([]string{jwt.SigningMethodRS256.Alg()}),
			jwt.WithStrictDecoding(),
			jwt.WithoutClaimsValidation(),
		),
	}, nil
}

// VerifySignature verifies one RS256 signature into caller-owned claims.
// Registered and custom claims policy belongs to the concrete token consumer.
func (verifier *Verifier) VerifySignature(tokenString string, claims jwt.Claims) (*jwt.Token, error) {
	if verifier == nil || len(verifier.publicKeys) == 0 || verifier.parser == nil {
		return nil, fmt.Errorf("jwt: verifier is invalid")
	}
	if tokenString == "" {
		return nil, fmt.Errorf("jwt: token is empty")
	}
	if claims == nil || nilValue(claims) {
		return nil, fmt.Errorf("jwt: claims are nil")
	}
	return verifier.parser.ParseWithClaims(tokenString, claims, verifier.keyFunc)
}

func nilValue(value any) bool {
	reflected := reflect.ValueOf(value)
	switch reflected.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return reflected.IsNil()
	default:
		return false
	}
}

func (verifier *Verifier) keyFunc(token *jwt.Token) (any, error) {
	if token.Method != jwt.SigningMethodRS256 {
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
