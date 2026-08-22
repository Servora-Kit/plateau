package oidc

import (
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"os"
	"strings"

	"github.com/go-jose/go-jose/v4"
)

type signingKey struct {
	id  string
	key *rsa.PrivateKey
}

func (key *signingKey) SignatureAlgorithm() jose.SignatureAlgorithm { return jose.RS256 }

func (key *signingKey) Key() any { return key.key }

func (key *signingKey) ID() string { return key.id }

type publicKey struct {
	id  string
	key any
}

func (key *publicKey) ID() string                         { return key.id }
func (key *publicKey) Algorithm() jose.SignatureAlgorithm { return jose.RS256 }
func (key *publicKey) Use() string                        { return "sig" }
func (key *publicKey) Key() any                           { return key.key }

func loadSigningKey(path string) (*rsa.PrivateKey, string, string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, "", "", fmt.Errorf("OIDC signing key path is empty")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, "", "", fmt.Errorf("read OIDC signing key: %w", err)
	}
	block, _ := pem.Decode(data)
	if block == nil {
		return nil, "", "", fmt.Errorf("decode OIDC signing key: PEM block is missing")
	}
	var privateKey *rsa.PrivateKey
	switch block.Type {
	case "RSA PRIVATE KEY":
		privateKey, err = x509.ParsePKCS1PrivateKey(block.Bytes)
	case "PRIVATE KEY":
		var parsed any
		parsed, err = x509.ParsePKCS8PrivateKey(block.Bytes)
		if err == nil {
			var ok bool
			privateKey, ok = parsed.(*rsa.PrivateKey)
			if !ok {
				return nil, "", "", fmt.Errorf("OIDC signing key is not RSA")
			}
		}
	default:
		return nil, "", "", fmt.Errorf("unsupported OIDC signing PEM type %q", block.Type)
	}
	if err != nil {
		return nil, "", "", fmt.Errorf("parse OIDC signing key: %w", err)
	}
	if privateKey.N.BitLen() < 2048 {
		return nil, "", "", fmt.Errorf("OIDC RSA signing key must be at least 2048 bits")
	}
	if err := privateKey.Validate(); err != nil {
		return nil, "", "", fmt.Errorf("validate OIDC signing key: %w", err)
	}
	publicDER, err := x509.MarshalPKIXPublicKey(&privateKey.PublicKey)
	if err != nil {
		return nil, "", "", fmt.Errorf("marshal OIDC public key: %w", err)
	}
	digest := sha256.Sum256(publicDER)
	keyID := base64.RawURLEncoding.EncodeToString(digest[:])
	jwkBytes, err := json.Marshal(jose.JSONWebKey{
		Key:       &privateKey.PublicKey,
		KeyID:     keyID,
		Algorithm: string(jose.RS256),
		Use:       "sig",
	})
	if err != nil {
		return nil, "", "", fmt.Errorf("marshal OIDC public JWK: %w", err)
	}
	return privateKey, keyID, string(jwkBytes), nil
}

func loadCryptoKey(path string) ([32]byte, string, error) {
	var key [32]byte
	path = strings.TrimSpace(path)
	if path == "" {
		return key, "", fmt.Errorf("OIDC crypto key path is empty")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return key, "", fmt.Errorf("read OIDC crypto key: %w", err)
	}
	material := data
	trimmed := strings.TrimSpace(string(data))
	if decoded, decodeErr := hex.DecodeString(trimmed); decodeErr == nil && len(decoded) == len(key) {
		material = decoded
	} else if decoded, decodeErr := base64.RawURLEncoding.DecodeString(trimmed); decodeErr == nil && len(decoded) == len(key) {
		material = decoded
	}
	if len(material) != len(key) {
		return key, "", fmt.Errorf("OIDC crypto key must contain exactly %d bytes", len(key))
	}
	copy(key[:], material)
	digest := sha256.Sum256(material)
	return key, base64.RawURLEncoding.EncodeToString(digest[:]), nil
}
