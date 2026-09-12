package jwt

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	jwtpb "github.com/Servora-Kit/plateau/api/gen/go/plateau/security/jwt/v1"
	"github.com/go-jose/go-jose/v4"
	jwtlib "github.com/golang-jwt/jwt/v5"
)

func TestJWKSCacheRefreshAndCleanup(t *testing.T) {
	first, second := signerFromKey(t, newRSAKey(t)), signerFromKey(t, newRSAKey(t))
	var mu sync.Mutex
	current, failed := first, false
	var calls atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		// Close idle network goroutines before advancing the test's virtual clock.
		w.Header().Set("Connection", "close")
		calls.Add(1)
		mu.Lock()
		defer mu.Unlock()
		if failed {
			w.WriteHeader(503)
			return
		}
		json.NewEncoder(w).Encode(jose.JSONWebKeySet{Keys: []jose.JSONWebKey{{Key: current.PublicKey(), KeyID: current.KID(), Algorithm: "RS256", Use: "sig"}}})
	}))
	t.Cleanup(server.Close)
	synctest.Test(t, func(t *testing.T) {
		v, cleanup, err := NewJWKS(t.Context(), &jwtpb.JWKS{Uri: server.URL})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(cleanup)
		firstToken, err := first.Sign(jwtlib.MapClaims{})
		if err != nil {
			t.Fatal(err)
		}
		secondToken, err := second.Sign(jwtlib.MapClaims{})
		if err != nil {
			t.Fatal(err)
		}
		for range 10 {
			if _, err := v.VerifySignature(t.Context(), firstToken, jwtlib.MapClaims{}); err != nil {
				t.Fatal(err)
			}
		}
		if calls.Load() != 1 {
			t.Fatalf("known kid caused HTTP calls: %d", calls.Load())
		}
		mu.Lock()
		current = second
		mu.Unlock()
		if _, err := v.VerifySignature(t.Context(), secondToken, jwtlib.MapClaims{}); err != nil {
			t.Fatalf("unknown kid refresh: %v", err)
		}
		if _, err := v.VerifySignature(t.Context(), firstToken, jwtlib.MapClaims{}); err == nil {
			t.Fatal("successful refresh retained removed key")
		}
		count := calls.Load()
		for range 5 {
			_, _ = v.VerifySignature(t.Context(), firstToken, jwtlib.MapClaims{})
		}
		if calls.Load() != count {
			t.Fatal("unknown kid was not rate limited")
		}
		mu.Lock()
		failed = true
		mu.Unlock()
		time.Sleep(time.Hour)
		synctest.Wait()
		if calls.Load() == count {
			t.Fatal("background refresh did not run")
		}
		if _, err := v.VerifySignature(t.Context(), secondToken, jwtlib.MapClaims{}); err != nil {
			t.Fatalf("failed refresh discarded known key: %v", err)
		}
		cleanup()
		synctest.Wait()
		count = calls.Load()
		time.Sleep(2 * time.Hour)
		synctest.Wait()
		if calls.Load() != count {
			t.Fatal("refresh continued after cleanup")
		}
	})
}

func TestJWKSInitialFailureIsFatal(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(503) }))
	defer server.Close()
	if _, cleanup, err := NewJWKS(t.Context(), &jwtpb.JWKS{Uri: server.URL}); err == nil {
		cleanup()
		t.Fatal("unavailable initial JWKS accepted")
	}
}
