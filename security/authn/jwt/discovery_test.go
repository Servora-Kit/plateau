package jwt

import (
	"crypto/rsa"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	jwtconfpb "github.com/Servora-Kit/plateau/api/gen/go/plateau/security/authn/jwt/v1"
	jwtkeypb "github.com/Servora-Kit/plateau/api/gen/go/plateau/security/jwt/v1"
	"github.com/Servora-Kit/plateau/security"
	securityjwt "github.com/Servora-Kit/plateau/security/jwt"
	"github.com/go-jose/go-jose/v4"
	jwtlib "github.com/golang-jwt/jwt/v5"
)

func TestDiscoveryAndExplicitSources(t *testing.T) {
	signer := signerFromKey(t, newRSAKey(t))
	var issuer string
	badIssuer := false
	discoveries := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/keys" {
			if err := json.NewEncoder(w).Encode(jose.JSONWebKeySet{Keys: []jose.JSONWebKey{{Key: signer.PublicKey(), KeyID: signer.KID(), Algorithm: "RS256", Use: "sig"}}}); err != nil {
				t.Error(err)
			}
			return
		}
		discoveries++
		value := issuer
		if badIssuer {
			value += "/wrong"
		}
		if err := json.NewEncoder(w).Encode(map[string]string{"issuer": value, "jwks_uri": issuer + "/keys"}); err != nil {
			t.Error(err)
		}
	}))
	defer server.Close()
	issuer = server.URL
	config := &jwtconfpb.JwtAuthnConfig{Issuer: issuer, Audience: "iam"}
	authenticator, cleanup, err := New(t.Context(), config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(cleanup)
	claims := validTestClaims("service", "admin")
	claims.Issuer, claims.Audience = issuer, jwtlib.ClaimStrings{"iam"}
	token := mustToken(t, signer, claims)
	if _, err := Authenticate(t.Context(), authenticator, "Bearer "+token, newTestClaims, mapTestActor); err != nil {
		t.Fatal(err)
	}
	badIssuer = true
	if _, _, err := New(t.Context(), config); err == nil {
		t.Fatal("mismatched discovery issuer accepted")
	}
	config.Jwks = &jwtkeypb.JWKS{Uri: issuer + "/keys"}
	_, cleanup, err = New(t.Context(), config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(cleanup)
	local, err := securityjwt.New(map[string]*rsa.PublicKey{signer.KID(): signer.PublicKey()})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := New(t.Context(), config, WithVerifier(local)); err == nil {
		t.Fatal("multiple key sources accepted")
	}
	config.Jwks = nil
	if _, cleanup, err := New(t.Context(), config, WithVerifier(local)); err != nil {
		t.Fatal(err)
	} else {
		cleanup()
	}
	if discoveries != 2 {
		t.Fatalf("explicit source performed discovery: %d", discoveries)
	}
}

type applicationClaims struct {
	jwtlib.RegisteredClaims
	ClientID string `json:"client_id"`
	Purpose  string `json:"purpose"`
}

func (claims *applicationClaims) Validate() error {
	if claims.ClientID != claims.Subject || claims.Purpose != "access" {
		return errors.New("invalid application claims")
	}
	return nil
}

func TestClaimsValidationAndConcurrentIsolation(t *testing.T) {
	signer, _, authenticator := newAuthenticator(t)
	mapActor := func(claims *applicationClaims) (security.Actor, error) {
		return security.Actor{Type: security.ActorTypeService, ID: claims.ClientID}, nil
	}
	factory := func() *applicationClaims { return &applicationClaims{} }
	claims := &applicationClaims{ClientID: "admin", Purpose: "access", RegisteredClaims: jwtlib.RegisteredClaims{
		Issuer: "issuer-a", Audience: jwtlib.ClaimStrings{"audience-a"}, Subject: "admin",
		ExpiresAt: jwtlib.NewNumericDate(time.Now().Add(time.Hour)),
	}}
	for _, invalid := range []string{"purpose", "subject", "issuer", "expiry"} {
		copy := *claims
		switch invalid {
		case "purpose":
			copy.Purpose = "id"
		case "subject":
			copy.Subject = "other"
		case "issuer":
			copy.Issuer = "other"
		case "expiry":
			copy.ExpiresAt = jwtlib.NewNumericDate(time.Now().Add(-time.Hour))
		}
		token, err := signer.Sign(&copy)
		if err != nil {
			t.Fatal(err)
		}
		called := false
		_, err = Authenticate(t.Context(), authenticator, "Bearer "+token, factory, func(c *applicationClaims) (security.Actor, error) { called = true; return mapActor(c) })
		if !errors.Is(err, ErrInvalidToken) || called {
			t.Fatalf("%s: error=%v mapper=%v", invalid, err, called)
		}
	}
	var workers sync.WaitGroup
	for i := range 24 {
		copy := *claims
		copy.ClientID, copy.Subject = fmt.Sprint(i), fmt.Sprint(i)
		token, err := signer.Sign(&copy)
		if err != nil {
			t.Fatal(err)
		}
		workers.Go(func() {
			actor, err := Authenticate(t.Context(), authenticator, "Bearer "+token, factory, mapActor)
			if err != nil || actor.ID != copy.ClientID {
				t.Errorf("concurrent claims: actor=%v error=%v", actor, err)
			}
		})
	}
	workers.Wait()
}
