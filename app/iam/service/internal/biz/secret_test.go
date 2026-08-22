package biz

import (
	"encoding/base64"
	"testing"
)

func TestNewOpaqueSecret(t *testing.T) {
	t.Parallel()

	first, firstHash, err := NewOpaqueSecret()
	if err != nil {
		t.Fatalf("first NewOpaqueSecret() error = %v", err)
	}
	second, secondHash, err := NewOpaqueSecret()
	if err != nil {
		t.Fatalf("second NewOpaqueSecret() error = %v", err)
	}
	if first == second || firstHash == secondHash {
		t.Fatal("NewOpaqueSecret() reused secret material")
	}
	decoded, err := base64.RawURLEncoding.DecodeString(first)
	if err != nil {
		t.Fatalf("decode secret: %v", err)
	}
	if len(decoded) != opaqueSecretBytes {
		t.Fatalf("secret length = %d bytes, want %d", len(decoded), opaqueSecretBytes)
	}
	if firstHash != HashOpaqueSecret(first) {
		t.Fatal("stored hash does not match plaintext secret")
	}
	if firstHash == first {
		t.Fatal("stored hash exposes plaintext secret")
	}
}

func TestHashOpaqueSecretIsDeterministicAndFixedLength(t *testing.T) {
	t.Parallel()

	first := HashOpaqueSecret("opaque credential")
	second := HashOpaqueSecret("opaque credential")
	if first != second {
		t.Fatal("HashOpaqueSecret() is not deterministic")
	}
	decoded, err := base64.RawURLEncoding.DecodeString(first)
	if err != nil {
		t.Fatalf("decode hash: %v", err)
	}
	if len(decoded) != 32 {
		t.Fatalf("hash length = %d bytes, want SHA-256", len(decoded))
	}
}
