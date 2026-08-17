package jwt

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	jwtconfpb "github.com/Servora-Kit/plateau/api/gen/go/plateau/security/authn/jwt/v1"
	jwtkeypb "github.com/Servora-Kit/plateau/api/gen/go/plateau/security/jwt/v1"
	security "github.com/Servora-Kit/plateau/security"
	securityjwt "github.com/Servora-Kit/plateau/security/jwt"
	jwtlib "github.com/golang-jwt/jwt/v5"
)

type testClaims struct {
	ActorType string `json:"actor_type"`
	jwtlib.RegisteredClaims
}

type pointerClaims struct {
	ActorType string `json:"actor_type"`
	*jwtlib.RegisteredClaims
}

func newPointerClaims() *pointerClaims {
	return &pointerClaims{RegisteredClaims: &jwtlib.RegisteredClaims{}}
}

func newTestClaims() *testClaims { return &testClaims{} }

func TestAuthenticateMapsHumanAndServiceActors(t *testing.T) {
	signer, _, authenticator := newAuthenticator(t)
	for _, want := range []struct {
		name      string
		actorType security.ActorType
		id        string
	}{
		{name: "human", actorType: security.ActorTypeHuman, id: "user-1"},
		{name: "service", actorType: security.ActorTypeService, id: "worker-1"},
	} {
		t.Run(want.name, func(t *testing.T) {
			token := mustToken(t, signer, validTestClaims(string(want.actorType), want.id))
			actor, err := Authenticate(context.Background(), authenticator, "Bearer "+token, newTestClaims, func(claims *testClaims) (security.Actor, error) {
				return security.Actor{Type: security.ActorType(claims.ActorType), ID: claims.Subject}, nil
			})
			if err != nil || actor != (security.Actor{Type: want.actorType, ID: want.id}) {
				t.Fatalf("actor=%+v error=%v", actor, err)
			}
		})
	}
}

func TestAuthenticateSupportsInitializedPointerEmbeddedClaims(t *testing.T) {
	signer, _, authenticator := newAuthenticator(t)
	claims := &pointerClaims{
		ActorType: "human",
		RegisteredClaims: &jwtlib.RegisteredClaims{
			Issuer: "issuer-a", Audience: jwtlib.ClaimStrings{"audience-a"}, Subject: "user-1",
			ExpiresAt: jwtlib.NewNumericDate(time.Now().Add(time.Hour)),
		},
	}
	token, err := signer.Sign(claims)
	if err != nil {
		t.Fatal(err)
	}
	actor, err := Authenticate(context.Background(), authenticator, "Bearer "+token, newPointerClaims, func(claims *pointerClaims) (security.Actor, error) {
		return security.Actor{Type: security.ActorType(claims.ActorType), ID: claims.Subject}, nil
	})
	if err != nil || actor != (security.Actor{Type: security.ActorTypeHuman, ID: "user-1"}) {
		t.Fatalf("actor=%+v error=%v", actor, err)
	}
	if _, err := Authenticate(context.Background(), authenticator, "Bearer "+token, func() *pointerClaims {
		return &pointerClaims{}
	}, func(*pointerClaims) (security.Actor, error) {
		return security.Actor{}, nil
	}); err == nil || !strings.Contains(err.Error(), "uninitialized claims") {
		t.Fatalf("uninitialized factory error=%v", err)
	}
}

func TestAuthenticateRejectsInvalidCredentialsAndProfiles(t *testing.T) {
	signer, signerKey, authenticator := newAuthenticator(t)
	validToken := mustToken(t, signer, validTestClaims("human", "user-1"))
	otherKey := newRSAKey(t)
	otherSigner := signerFromKey(t, otherKey)
	unknownToken := mustToken(t, otherSigner, validTestClaims("human", "user-1"))
	invalidSignature := jwtlib.NewWithClaims(jwtlib.SigningMethodRS256, validTestClaims("human", "user-1"))
	invalidSignature.Header["kid"] = signer.KID()
	invalidToken, err := invalidSignature.SignedString(otherKey)
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name   string
		header string
		want   error
	}{
		{name: "missing", want: ErrMissingCredentials},
		{name: "malformed", header: "Basic token", want: ErrMalformedCredentials},
		{name: "unknown kid", header: "Bearer " + unknownToken, want: ErrInvalidToken},
		{name: "invalid signature", header: "Bearer " + invalidToken, want: ErrInvalidToken},
		{name: "wrong issuer", header: "Bearer " + mustToken(t, signer, claimsWithProfile("issuer-b", "audience-a", time.Now().Add(time.Hour))), want: ErrInvalidToken},
		{name: "wrong audience", header: "Bearer " + mustToken(t, signer, claimsWithProfile("issuer-a", "audience-b", time.Now().Add(time.Hour))), want: ErrInvalidToken},
		{name: "expired", header: "Bearer " + mustToken(t, signer, claimsWithProfile("issuer-a", "audience-a", time.Now().Add(-time.Minute))), want: ErrInvalidToken},
		{name: "missing signed profile", header: "Bearer " + mustToken(t, signer, &testClaims{ActorType: "human", RegisteredClaims: jwtlib.RegisteredClaims{Subject: "user-1"}}), want: ErrInvalidToken},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := Authenticate(context.Background(), authenticator, test.header, newTestClaims, func(claims *testClaims) (security.Actor, error) {
				return security.Actor{Type: security.ActorTypeHuman, ID: claims.Subject}, nil
			})
			if !errors.Is(err, test.want) {
				t.Fatalf("error = %v, want %v", err, test.want)
			}
		})
	}

	parsed, _, err := jwtlib.NewParser().ParseUnverified(validToken, &testClaims{})
	if err != nil {
		t.Fatal(err)
	}
	parsed.Header["typ"] = "at+jwt"
	wrongTypeToken, err := parsed.SignedString(signerKey)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Authenticate(context.Background(), authenticator, "Bearer "+wrongTypeToken, newTestClaims, func(*testClaims) (security.Actor, error) {
		return security.Actor{Type: security.ActorTypeHuman, ID: "user-1"}, nil
	}); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("wrong token type error = %v", err)
	}

	if _, err := Authenticate(context.Background(), authenticator, "Bearer "+validToken, newTestClaims, func(*testClaims) (security.Actor, error) {
		return security.Actor{}, errors.New("missing subject")
	}); !errors.Is(err, ErrActorMapping) {
		t.Fatalf("mapping error = %v", err)
	}
}

func TestNewRejectsTypedNilKeySource(t *testing.T) {
	config := &jwtconfpb.JwtAuthnConfig{
		Issuer: "issuer-a", Audience: "audience-a",
		VerificationKeys: []*jwtkeypb.VerificationKey{{
			Kid: "key-1", Source: (*jwtkeypb.VerificationKey_PublicKeyPem)(nil),
		}},
	}
	if _, err := New(config); err == nil {
		t.Fatal("New accepted typed-nil key source")
	}
}

func TestLocalErrorsPreserveCause(t *testing.T) {
	cause := errors.New("provider detail")
	err := fmt.Errorf("%w: %w", ErrInvalidToken, cause)
	if !errors.Is(err, ErrInvalidToken) || !errors.Is(err, cause) {
		t.Fatalf("error chain = %v", err)
	}
}

func validTestClaims(actorType, subject string) *testClaims {
	return &testClaims{ActorType: actorType, RegisteredClaims: jwtlib.RegisteredClaims{
		Issuer: "issuer-a", Audience: jwtlib.ClaimStrings{"audience-a"}, Subject: subject,
		ExpiresAt: jwtlib.NewNumericDate(time.Now().Add(time.Hour)),
	}}
}

func claimsWithProfile(issuer, audience string, expiresAt time.Time) *testClaims {
	claims := validTestClaims("human", "user-1")
	claims.Issuer = issuer
	claims.Audience = jwtlib.ClaimStrings{audience}
	claims.ExpiresAt = jwtlib.NewNumericDate(expiresAt)
	return claims
}

func newAuthenticator(t *testing.T) (*securityjwt.Signer, *rsa.PrivateKey, *Authenticator) {
	t.Helper()
	key := newRSAKey(t)
	signer := signerFromKey(t, key)
	publicDER, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	if err != nil {
		t.Fatal(err)
	}
	authenticator, err := New(&jwtconfpb.JwtAuthnConfig{
		Issuer: "issuer-a", Audience: "audience-a",
		VerificationKeys: []*jwtkeypb.VerificationKey{{
			Kid:    signer.KID(),
			Source: &jwtkeypb.VerificationKey_PublicKeyPem{PublicKeyPem: string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: publicDER}))},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	return signer, key, authenticator
}

func mustToken(t *testing.T, signer *securityjwt.Signer, claims *testClaims) string {
	t.Helper()
	token, err := signer.Sign(claims)
	if err != nil {
		t.Fatal(err)
	}
	return token
}

func newRSAKey(t *testing.T) *rsa.PrivateKey {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	return key
}

func signerFromKey(t *testing.T, key *rsa.PrivateKey) *securityjwt.Signer {
	t.Helper()
	pemBytes := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
	signer, err := securityjwt.NewSigner(pemBytes)
	if err != nil {
		t.Fatal(err)
	}
	return signer
}
