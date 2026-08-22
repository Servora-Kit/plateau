package biz

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
)

const opaqueSecretBytes = 32

// NewOpaqueSecret returns a high-entropy URL-safe secret and its irreversible storage hash.
func NewOpaqueSecret() (plaintext, hash string, err error) {
	secret := make([]byte, opaqueSecretBytes)
	if _, err := rand.Read(secret); err != nil {
		return "", "", fmt.Errorf("generate opaque secret: %w", err)
	}
	plaintext = base64.RawURLEncoding.EncodeToString(secret)
	return plaintext, HashOpaqueSecret(plaintext), nil
}

// HashOpaqueSecret returns a deterministic SHA-256 digest suitable for database lookup.
func HashOpaqueSecret(plaintext string) string {
	digest := sha256.Sum256([]byte(plaintext))
	return base64.RawURLEncoding.EncodeToString(digest[:])
}
