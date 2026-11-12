package oidc

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	oidcv1 "github.com/Servora-Kit/plateau/api/gen/go/plateau/security/oidc/v1"
	"github.com/go-jose/go-jose/v4"
	"github.com/go-kratos/kratos/v3/transport"
	"github.com/zitadel/oidc/v3/pkg/oidc"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/durationpb"
)

const (
	testClientSecret = "unit-test-oidc-client-secret-32b!"
	testCookieSecret = "unit-test-cookie-secret-at-least-32-bytes"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return fn(r)
}

type fakeClientTransport struct {
	header headerMap
}

type headerMap map[string][]string

func (h headerMap) Get(key string) string {
	vals := h[http.CanonicalHeaderKey(key)]
	if len(vals) == 0 {
		return ""
	}
	return vals[0]
}
func (h headerMap) Set(key, value string) { h[http.CanonicalHeaderKey(key)] = []string{value} }
func (h headerMap) Add(key, value string) {
	k := http.CanonicalHeaderKey(key)
	h[k] = append(h[k], value)
}
func (h headerMap) Keys() []string {
	out := make([]string, 0, len(h))
	for k := range h {
		out = append(out, k)
	}
	return out
}
func (h headerMap) Values(key string) []string { return h[http.CanonicalHeaderKey(key)] }

func (t *fakeClientTransport) Kind() transport.Kind            { return transport.KindGRPC }
func (t *fakeClientTransport) Endpoint() string                { return "127.0.0.1:10001" }
func (t *fakeClientTransport) Operation() string               { return "/iam.user.v1.UserService/ListUsers" }
func (t *fakeClientTransport) RequestHeader() transport.Header { return t.header }
func (t *fakeClientTransport) ReplyHeader() transport.Header   { return t.header }

type mockOP struct {
	server           *httptest.Server
	signer           jose.Signer
	jwk              jose.JSONWebKey
	discoveryHits    atomic.Int32
	lastChallenge    string
	lastChallengeAlg string
	lastNonce        string
	overrideNonce    string
	userinfoSub      string
}

func newMockOP(t *testing.T) *mockOP {
	t.Helper()
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	jwk := jose.JSONWebKey{Key: &priv.PublicKey, KeyID: "kid-1", Algorithm: string(jose.RS256), Use: "sig"}
	signer, err := jose.NewSigner(jose.SigningKey{Algorithm: jose.RS256, Key: priv}, (&jose.SignerOptions{}).WithType("JWT").WithHeader("kid", "kid-1"))
	if err != nil {
		t.Fatal(err)
	}

	op := &mockOP{signer: signer, jwk: jwk, userinfoSub: "user-123"}
	mux := http.NewServeMux()
	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, _ *http.Request) {
		op.discoveryHits.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"issuer":                                op.server.URL,
			"authorization_endpoint":                op.server.URL + "/authorize",
			"token_endpoint":                        op.server.URL + "/oauth/token",
			"userinfo_endpoint":                     op.server.URL + "/userinfo",
			"jwks_uri":                              op.server.URL + "/jwks.json",
			"response_types_supported":              []string{"code"},
			"subject_types_supported":               []string{"public"},
			"id_token_signing_alg_values_supported": []string{"RS256"},
			"code_challenge_methods_supported":      []string{"S256"},
			"token_endpoint_auth_methods_supported": []string{"client_secret_basic"},
		})
	})
	mux.HandleFunc("/jwks.json", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(jose.JSONWebKeySet{Keys: []jose.JSONWebKey{op.jwk}})
	})
	mux.HandleFunc("/oauth/token", func(w http.ResponseWriter, r *http.Request) {
		user, pass, ok := r.BasicAuth()
		pass, _ = url.QueryUnescape(pass)
		if !ok || user != "admin" || pass != testClientSecret {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"error":"invalid_client"}`))
			return
		}
		if err := r.ParseForm(); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		verifier := r.FormValue("code_verifier")
		sum := sha256.Sum256([]byte(verifier))
		gotChallenge := base64.RawURLEncoding.EncodeToString(sum[:])
		if gotChallenge != op.lastChallenge || r.FormValue("code") != "good-code" {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":"invalid_grant"}`))
			return
		}

		accessToken := "access-token-abc"
		atHash, err := oidc.ClaimHash(accessToken, jose.RS256)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		nonce := op.lastNonce
		if op.overrideNonce != "" {
			nonce = op.overrideNonce
		}
		now := time.Now().UTC().Truncate(time.Second)
		claims := map[string]any{
			"iss":       op.server.URL,
			"sub":       "user-123",
			"aud":       "admin",
			"exp":       now.Add(5 * time.Minute).Unix(),
			"iat":       now.Unix(),
			"auth_time": now.Add(-time.Minute).Unix(),
			"nonce":     nonce,
			"at_hash":   atHash,
		}
		payload, _ := json.Marshal(claims)
		jws, err := op.signer.Sign(payload)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		idToken, _ := jws.CompactSerialize()

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token": accessToken,
			"token_type":   "Bearer",
			"expires_in":   300,
			"id_token":     idToken,
		})
	})
	mux.HandleFunc("/userinfo", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer access-token-abc" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"sub":            op.userinfoSub,
			"email":          "admin@plateau.local",
			"email_verified": true,
			"name":           "Platform Admin",
			"picture":        "https://cdn.plateau.local/avatar.png",
		})
	})

	op.server = httptest.NewServer(mux)
	t.Cleanup(op.server.Close)
	return op
}

func TestRelyingPartyValidationAndLazyDiscovery(t *testing.T) {
	if _, err := NewRelyingParty(&oidcv1.OIDCClient{
		Issuer:       proto.String("https://iam.example.com"),
		ClientId:     proto.String("admin"),
		ClientSecret: proto.String(testClientSecret),
		RedirectUri:  "https://admin.example.com/v1/admin/auth/callback",
		CookieSecret: "too-short",
	}); err == nil {
		t.Fatal("expected short cookie_secret to be rejected")
	}

	var networkCalls atomic.Int32
	client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		networkCalls.Add(1)
		return nil, errors.New("unexpected startup network call")
	})}
	relyingParty, err := NewRelyingParty(&oidcv1.OIDCClient{
		Issuer:       proto.String("https://iam.example.com"),
		ClientId:     proto.String("admin"),
		ClientSecret: proto.String(testClientSecret),
		RedirectUri:  "https://admin.example.com/v1/admin/auth/callback",
		CookieSecret: testCookieSecret,
	}, WithHTTPClient(client))
	if err != nil {
		t.Fatalf("NewRelyingParty() error = %v", err)
	}
	if networkCalls.Load() != 0 {
		t.Fatalf("NewRelyingParty() made %d network calls during construction", networkCalls.Load())
	}
	if relyingParty.cookieName != "__Host-admin_oidc_handshake" {
		t.Fatalf("https cookieName = %q, want __Host-admin_oidc_handshake", relyingParty.cookieName)
	}
}

func TestRelyingPartyHandshakePKCENonceAndOpenRedirect(t *testing.T) {
	op := newMockOP(t)
	rpClient, err := NewRelyingParty(&oidcv1.OIDCClient{
		Issuer:       proto.String(op.server.URL),
		ClientId:     proto.String("admin"),
		ClientSecret: proto.String(testClientSecret),
		RedirectUri:  "http://localhost:10012/v1/admin/auth/callback",
		CookieSecret: testCookieSecret,
	})
	if err != nil {
		t.Fatalf("NewRelyingParty() error = %v", err)
	}

	// Open-redirect attempts must all fall back to defaultReturnTo ("/dashboard").
	for _, unsafeReturnTo := range []string{
		"https://evil.example.com/steal",
		"//evil.example.com/steal",
		`/\evil.example.com`,
		"/valid\r\nSet-Cookie: hacked=1",
		"javascript:alert(1)",
	} {
		req := httptest.NewRequest(http.MethodGet, "/v1/admin/auth/login?return_to="+url.QueryEscape(unsafeReturnTo), nil)
		rec := httptest.NewRecorder()
		if err := rpClient.BeginLogin(rec, req, "/dashboard"); err != nil {
			t.Fatalf("BeginLogin(%q) error = %v", unsafeReturnTo, err)
		}
		if rec.Code != http.StatusFound {
			t.Fatalf("BeginLogin status = %d", rec.Code)
		}
		cookie := rec.Result().Cookies()[0]
		loc, err := url.Parse(rec.Header().Get("Location"))
		if err != nil {
			t.Fatal(err)
		}
		op.lastChallenge = loc.Query().Get("code_challenge")
		op.lastChallengeAlg = loc.Query().Get("code_challenge_method")
		op.lastNonce = loc.Query().Get("nonce")
		if op.lastChallenge == "" || op.lastChallengeAlg != "S256" || op.lastNonce == "" {
			t.Fatalf("missing PKCE S256 or nonce in authorize URL: %s", loc.RawQuery)
		}

		cbReq := httptest.NewRequest(http.MethodGet, "/v1/admin/auth/callback?code=good-code&state="+url.QueryEscape(loc.Query().Get("state")), nil)
		cbReq.AddCookie(cookie)
		cbRec := httptest.NewRecorder()
		id, returnTo, err := rpClient.CompleteLogin(cbRec, cbReq)
		if err != nil {
			t.Fatalf("CompleteLogin() error = %v", err)
		}
		if returnTo != "/dashboard" {
			t.Fatalf("unsafe return_to %q resolved to %q, want /dashboard", unsafeReturnTo, returnTo)
		}
		if id.Subject != "user-123" || id.Email != "admin@plateau.local" || !id.EmailVerified || id.Name != "Platform Admin" {
			t.Fatalf("unexpected identity: %+v", id)
		}
		// Handshake cookie must be deleted immediately on callback.
		cleared := cbRec.Result().Cookies()
		if len(cleared) != 1 || cleared[0].MaxAge >= 0 || cleared[0].Value != "" {
			t.Fatalf("handshake cookie was not cleared: %+v", cleared)
		}
	}

	// Safe internal relative path with query is preserved, and discovery was cached (only 1 hit across logins).
	loginReq := httptest.NewRequest(http.MethodGet, "/v1/admin/auth/login?return_to="+url.QueryEscape("/users?page=2"), nil)
	loginRec := httptest.NewRecorder()
	if err := rpClient.BeginLogin(loginRec, loginReq, "/"); err != nil {
		t.Fatal(err)
	}
	if op.discoveryHits.Load() != 1 {
		t.Fatalf("discoveryHits = %d, want 1", op.discoveryHits.Load())
	}
	cookie := loginRec.Result().Cookies()[0]
	loc, _ := url.Parse(loginRec.Header().Get("Location"))
	op.lastChallenge = loc.Query().Get("code_challenge")
	op.lastNonce = loc.Query().Get("nonce")
	state := loc.Query().Get("state")

	// Tampered cookie is rejected.
	tamperedReq := httptest.NewRequest(http.MethodGet, "/v1/admin/auth/callback?code=good-code&state="+url.QueryEscape(state), nil)
	tamperedReq.AddCookie(&http.Cookie{Name: cookie.Name, Value: cookie.Value + "tamper"})
	if _, _, err := rpClient.CompleteLogin(httptest.NewRecorder(), tamperedReq); !errors.Is(err, ErrHandshakeInvalid) {
		t.Fatalf("tampered cookie err = %v, want ErrHandshakeInvalid", err)
	}

	// Wrong state is rejected.
	badStateReq := httptest.NewRequest(http.MethodGet, "/v1/admin/auth/callback?code=good-code&state=wrong-state", nil)
	badStateReq.AddCookie(cookie)
	if _, _, err := rpClient.CompleteLogin(httptest.NewRecorder(), badStateReq); !errors.Is(err, ErrHandshakeInvalid) {
		t.Fatalf("mismatched state err = %v, want ErrHandshakeInvalid", err)
	}

	// Wrong nonce in ID token is rejected.
	op.overrideNonce = "wrong-nonce"
	badNonceReq := httptest.NewRequest(http.MethodGet, "/v1/admin/auth/callback?code=good-code&state="+url.QueryEscape(state), nil)
	badNonceReq.AddCookie(cookie)
	if _, _, err := rpClient.CompleteLogin(httptest.NewRecorder(), badNonceReq); !errors.Is(err, ErrHandshakeInvalid) {
		t.Fatalf("mismatched nonce err = %v, want ErrHandshakeInvalid", err)
	}
	op.overrideNonce = ""

	// Valid completion preserves safe relative return_to.
	okReq := httptest.NewRequest(http.MethodGet, "/v1/admin/auth/callback?code=good-code&state="+url.QueryEscape(state), nil)
	okReq.AddCookie(cookie)
	_, returnTo, err := rpClient.CompleteLogin(httptest.NewRecorder(), okReq)
	if err != nil || returnTo != "/users?page=2" {
		t.Fatalf("CompleteLogin() returnTo=%q err=%v", returnTo, err)
	}
}

func TestServiceTokenMiddlewareSingleflightCancelAndSecretRedaction(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var exchanges atomic.Int32
		client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			user, pass, ok := r.BasicAuth()
			pass, _ = url.QueryUnescape(pass)
			if !ok || user != "admin" || pass != testClientSecret {
				return &http.Response{
					StatusCode: http.StatusUnauthorized,
					Header:     http.Header{"Content-Type": []string{"application/json"}},
					Body:       io.NopCloser(strings.NewReader(`{"error":"invalid_client","secret_echo":"` + testClientSecret + `"}`)),
				}, nil
			}
			n := exchanges.Add(1)
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"application/json"}},
				Body:       io.NopCloser(strings.NewReader(fmt.Sprintf(`{"access_token":"svc-tok-%d","token_type":"Bearer","expires_in":12}`, n))),
			}, nil
		})}

		mw, cleanup, err := NewServiceTokenMiddleware(&oidcv1.OIDCClient{
			Issuer:       proto.String("https://iam.example.com"),
			ClientId:     proto.String("admin"),
			ClientSecret: proto.String(testClientSecret),
			HttpTimeout:  durationpb.New(5 * time.Second),
		}, WithHTTPClient(client))
		if err != nil {
			t.Fatal(err)
		}
		defer cleanup()

		call := func(ctx context.Context) (string, error) {
			tr := &fakeClientTransport{header: make(headerMap)}
			ctx = transport.NewClientContext(ctx, tr)
			_, err := mw(func(_ context.Context, _ any) (any, error) {
				return nil, nil
			})(ctx, nil)
			return tr.header.Get("Authorization"), err
		}

		for range 16 {
			hdr, err := call(t.Context())
			if err != nil || hdr != "Bearer svc-tok-1" {
				t.Fatalf("cached call hdr=%q err=%v", hdr, err)
			}
		}
		if exchanges.Load() != 1 {
			t.Fatalf("exchanges = %d, want 1", exchanges.Load())
		}

		// Advance past x/oauth2's 10s expiry delta so concurrent callers renew once.
		time.Sleep(3 * time.Second)
		var wg sync.WaitGroup
		for range 24 {
			wg.Go(func() {
				hdr, err := call(t.Context())
				if err != nil || hdr != "Bearer svc-tok-2" {
					t.Errorf("renewed hdr=%q err=%v", hdr, err)
				}
			})
		}
		wg.Wait()
		if exchanges.Load() != 2 {
			t.Fatalf("concurrent renewal exchanges = %d, want 2", exchanges.Load())
		}
	})

	// Request cancellation must abort immediately even while upstream token endpoint hangs.
	release := make(chan struct{})
	defer close(release)
	hangingClient := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		<-release
		return nil, errors.New("aborted")
	})}
	mwHang, cleanupHang, err := NewServiceTokenMiddleware(&oidcv1.OIDCClient{
		Issuer:       proto.String("https://iam.example.com"),
		ClientId:     proto.String("admin"),
		ClientSecret: proto.String(testClientSecret),
		HttpTimeout:  durationpb.New(5 * time.Second),
	}, WithHTTPClient(hangingClient))
	if err != nil {
		t.Fatal(err)
	}
	defer cleanupHang()

	reqCtx, cancel := context.WithTimeout(t.Context(), 30*time.Millisecond)
	defer cancel()
	tr := &fakeClientTransport{header: make(headerMap)}
	_, err = mwHang(func(context.Context, any) (any, error) { return nil, nil })(transport.NewClientContext(reqCtx, tr), nil)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("hanging token endpoint err = %v, want context.DeadlineExceeded", err)
	}

	// Bad credentials return ErrTokenSourceUnavailable without leaking secret or raw response body.
	badClient := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusUnauthorized,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(`{"error":"invalid_client","echo":"` + testClientSecret + `"}`)),
		}, nil
	})}
	mwBad, cleanupBad, err := NewServiceTokenMiddleware(&oidcv1.OIDCClient{
		Issuer:       proto.String("https://iam.example.com"),
		ClientId:     proto.String("admin"),
		ClientSecret: proto.String(testClientSecret),
	}, WithHTTPClient(badClient))
	if err != nil {
		t.Fatal(err)
	}
	defer cleanupBad()

	_, err = mwBad(func(context.Context, any) (any, error) { return nil, nil })(transport.NewClientContext(t.Context(), &fakeClientTransport{header: make(headerMap)}), nil)
	if !errors.Is(err, ErrTokenSourceUnavailable) {
		t.Fatalf("bad client secret err = %v, want ErrTokenSourceUnavailable", err)
	}
	if strings.Contains(err.Error(), testClientSecret) || strings.Contains(err.Error(), "invalid_client") {
		t.Fatalf("token error leaked sensitive material: %v", err)
	}
}
