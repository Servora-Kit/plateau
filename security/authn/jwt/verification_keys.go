package jwt

import (
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"os"
	"strings"

	jwtkeypb "github.com/Servora-Kit/plateau/api/gen/go/plateau/security/jwt/v1"
)

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
