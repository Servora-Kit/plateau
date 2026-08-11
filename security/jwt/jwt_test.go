package jwt

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"strings"
	"testing"
	"time"

	jwtv1 "github.com/Servora-Kit/servora-platform/api/gen/go/platform/security/jwt/v1"
	jwtlib "github.com/golang-jwt/jwt/v5"
)

type customClaims struct {
	Tenant string `json:"tenant"`
	jwtlib.RegisteredClaims
}

func TestSignerAcceptsPKCS1AndPKCS8WithStableKID(t *testing.T) {
	key := newRSAKey(t)
	pkcs1 := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
	pkcs8DER, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	pkcs8 := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: pkcs8DER})

	first, err := NewSigner(pkcs1)
	if err != nil {
		t.Fatalf("NewSigner PKCS#1: %v", err)
	}
	second, err := NewSigner(pkcs8)
	if err != nil {
		t.Fatalf("NewSigner PKCS#8: %v", err)
	}
	if first.KID() != second.KID() || len(first.KID()) != 8 {
		t.Fatalf("KIDs = %q and %q", first.KID(), second.KID())
	}

	tokenString, err := first.Sign(&customClaims{
		Tenant: "tenant-a",
		RegisteredClaims: jwtlib.RegisteredClaims{
			ExpiresAt: jwtlib.NewNumericDate(time.Now().Add(time.Hour)),
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	parsed, _, err := jwtlib.NewParser().ParseUnverified(tokenString, &customClaims{})
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Method.Alg() != jwtlib.SigningMethodRS256.Alg() || parsed.Header["kid"] != first.KID() {
		t.Fatalf("alg=%q kid=%v", parsed.Method.Alg(), parsed.Header["kid"])
	}
}

func TestSignerRejectsInvalidAndNonRSAKeys(t *testing.T) {
	if _, err := NewSigner([]byte("not pem")); err == nil {
		t.Fatal("invalid PEM accepted")
	}
	ecdsaKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(ecdsaKey)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := NewSigner(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})); err == nil || !strings.Contains(err.Error(), "not RSA") {
		t.Fatalf("non-RSA error = %v", err)
	}
}

func TestVerifierAcceptsOnlyRS256AndKnownStringKID(t *testing.T) {
	key := newRSAKey(t)
	signer := signerFromKey(t, key)
	claims := func() *customClaims {
		return &customClaims{RegisteredClaims: jwtlib.RegisteredClaims{ExpiresAt: jwtlib.NewNumericDate(time.Now().Add(time.Hour))}}
	}
	valid, err := signer.Sign(claims())
	if err != nil {
		t.Fatal(err)
	}
	verifier := NewVerifier()
	verifier.AddKey(signer.KID(), signer.PublicKey())
	if err := verifier.Verify(valid, claims()); err != nil {
		t.Fatalf("verify RS256: %v", err)
	}

	unknownVerifier := NewVerifier()
	if err := unknownVerifier.Verify(valid, claims()); err == nil || !strings.Contains(err.Error(), "unknown kid") {
		t.Fatalf("unknown KID error = %v", err)
	}

	missingKID := jwtlib.NewWithClaims(jwtlib.SigningMethodRS256, claims())
	missingToken, err := missingKID.SignedString(key)
	if err != nil {
		t.Fatal(err)
	}
	if err := verifier.Verify(missingToken, claims()); err == nil || !strings.Contains(err.Error(), "missing kid") {
		t.Fatalf("missing KID error = %v", err)
	}

	nonStringKID := jwtlib.NewWithClaims(jwtlib.SigningMethodRS256, claims())
	nonStringKID.Header["kid"] = 7
	nonStringToken, err := nonStringKID.SignedString(key)
	if err != nil {
		t.Fatal(err)
	}
	if err := verifier.Verify(nonStringToken, claims()); err == nil || !strings.Contains(err.Error(), "missing kid") {
		t.Fatalf("non-string KID error = %v", err)
	}

	rs512 := jwtlib.NewWithClaims(jwtlib.SigningMethodRS512, claims())
	rs512.Header["kid"] = signer.KID()
	rs512Token, err := rs512.SignedString(key)
	if err != nil {
		t.Fatal(err)
	}
	if err := verifier.Verify(rs512Token, claims()); err == nil {
		t.Fatal("RS512 token accepted")
	}

	hs256 := jwtlib.NewWithClaims(jwtlib.SigningMethodHS256, claims())
	hs256.Header["kid"] = signer.KID()
	hs256Token, err := hs256.SignedString([]byte("secret"))
	if err != nil {
		t.Fatal(err)
	}
	if err := verifier.Verify(hs256Token, claims()); err == nil {
		t.Fatal("HS256 token accepted")
	}
}

func TestVerifierRejectsInvalidSignature(t *testing.T) {
	first := signerFromKey(t, newRSAKey(t))
	second := signerFromKey(t, newRSAKey(t))
	claims := &customClaims{RegisteredClaims: jwtlib.RegisteredClaims{ExpiresAt: jwtlib.NewNumericDate(time.Now().Add(time.Hour))}}
	token, err := first.Sign(claims)
	if err != nil {
		t.Fatal(err)
	}
	verifier := NewVerifier()
	verifier.AddKey(first.KID(), second.PublicKey())
	if err := verifier.Verify(token, &customClaims{}); err == nil {
		t.Fatal("invalid signature accepted")
	}
}

func TestTypedClaimsContextAndGeneratedDefaults(t *testing.T) {
	claims := &customClaims{Tenant: "tenant-a"}
	ctx := NewContext(context.Background(), claims)
	got, ok := FromContext[customClaims](ctx)
	if !ok || got != claims {
		t.Fatalf("FromContext = (%p, %v), want %p", got, ok, claims)
	}
	if _, ok := FromContext[customClaims](context.Background()); ok {
		t.Fatal("empty context returned claims")
	}

	config := new(jwtv1.Jwt)
	config.ApplyDefaults()
	if config.GetAccessExpire() != 3600 || config.GetRefreshExpire() != 604800 || config.SectionKey() != "jwt" || !config.SectionOptional() {
		t.Fatalf("config defaults = %#v", config)
	}
}

func newRSAKey(t *testing.T) *rsa.PrivateKey {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	return key
}

func signerFromKey(t *testing.T, key *rsa.PrivateKey) *Signer {
	t.Helper()
	pemBytes := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
	signer, err := NewSigner(pemBytes)
	if err != nil {
		t.Fatal(err)
	}
	return signer
}
