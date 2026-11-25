package audit

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"

	security "github.com/Servora-Kit/plateau/security"
	cloudevents "github.com/cloudevents/sdk-go/v2"
	kratos "github.com/go-kratos/kratos/v3"
)

type captureAuditor struct {
	mu     sync.Mutex
	events []cloudevents.Event
	err    error
}

func (c *captureAuditor) Emit(_ context.Context, ev cloudevents.Event) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.err != nil {
		return c.err
	}
	c.events = append(c.events, ev)
	return nil
}

func (c *captureAuditor) snapshot() []cloudevents.Event {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]cloudevents.Event, len(c.events))
	copy(out, c.events)
	return out
}

func TestContextHelpersAreNoopWithoutWrapper(t *testing.T) {
	ctx := context.Background()
	SetActor(ctx, security.Actor{Type: security.ActorTypeHuman, ID: "u-1"})
	SetResource(ctx, "iam.plateau.dev/User", "users/u-1")
	SetDetail(ctx, map[string]any{"client_id": "admin"})
	SetError(ctx, "ERR", "msg")
	SetEventOverride(ctx, AuthzDeniedEventType, CategoryAuthz)
	Skip(ctx)
}

func TestHTTPHandlerSuccessSkipOverrideAndTraceHeader(t *testing.T) {
	app := kratos.New(kratos.Name("iam.service"))
	baseCtx := kratos.NewContext(context.Background(), app)

	t.Run("success enriches actor, resource, merged detail, client meta, and W3C traceparent", func(t *testing.T) {
		capAud := &captureAuditor{}
		h := HTTPHandler(capAud, "plateau.iam.oidc.authorize.v1", CategoryAuthn)(
			http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				SetActor(r.Context(), security.Actor{Type: security.ActorTypeHuman, ID: "user-42"})
				SetResource(r.Context(), "iam.plateau.dev/User", "users/user-42")
				SetDetail(r.Context(), map[string]any{"client_id": "admin"})
				SetDetail(r.Context(), map[string]any{"scopes": []string{"openid", "profile"}})
				http.Redirect(w, r, "https://admin.local/callback?code=secret-code", http.StatusFound)
			}),
		)

		req := httptest.NewRequest(http.MethodGet, "http://iam.local/authorize/callback?id=req-1", nil).WithContext(baseCtx)
		req.Header.Set("X-Forwarded-For", "198.51.100.24, 10.0.0.1")
		req.Header.Set("User-Agent", "PlateauBrowser/1.0")
		req.Header.Set("X-Request-Id", "req-oidc-1")
		req.Header.Set("Traceparent", "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01")
		req.Header.Set("Tracestate", "vendor=alpha")

		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)

		if rec.Code != http.StatusFound {
			t.Fatalf("status = %d, want 302", rec.Code)
		}
		events := capAud.snapshot()
		if len(events) != 1 {
			t.Fatalf("len(events) = %d, want 1", len(events))
		}
		ev := events[0]
		if ev.Source() != "//iam.service" || ev.Type() != "plateau.iam.oidc.authorize.v1" || ev.Subject() != "GET /authorize/callback" {
			t.Fatalf("envelope = (%q, %q, %q)", ev.Source(), ev.Type(), ev.Subject())
		}
		exts := ev.Extensions()
		if exts[ExtCategory] != CategoryAuthn || exts[ExtOutcome] != OutcomeSuccess {
			t.Fatalf("category/outcome = (%v, %v)", exts[ExtCategory], exts[ExtOutcome])
		}
		if exts[ExtActorType] != "human" || exts[ExtActorID] != "user-42" {
			t.Fatalf("actor = (%v, %v)", exts[ExtActorType], exts[ExtActorID])
		}
		if exts[ExtResourceType] != "iam.plateau.dev/User" || exts[ExtResourceName] != "users/user-42" {
			t.Fatalf("resource = (%v, %v)", exts[ExtResourceType], exts[ExtResourceName])
		}
		if exts[ExtClientIP] != "198.51.100.24" || exts[ExtUserAgent] != "PlateauBrowser/1.0" || exts[ExtRequestID] != "req-oidc-1" {
			t.Fatalf("client meta = ip:%v ua:%v reqid:%v", exts[ExtClientIP], exts[ExtUserAgent], exts[ExtRequestID])
		}
		if exts[ExtTraceParent] != "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01" || exts[ExtTraceState] != "vendor=alpha" {
			t.Fatalf("trace = (%v, %v)", exts[ExtTraceParent], exts[ExtTraceState])
		}

		var detail map[string]any
		if err := json.Unmarshal(ev.Data(), &detail); err != nil {
			t.Fatalf("unmarshal detail: %v", err)
		}
		if detail["client_id"] != "admin" {
			t.Fatalf("detail = %v, want client_id=admin", detail)
		}
		if strings.Contains(string(ev.Data()), "secret-code") {
			t.Fatalf("detail leaked authorization code: %s", string(ev.Data()))
		}
	})

	t.Run("Skip suppresses unauthenticated 302 redirect noise", func(t *testing.T) {
		capAud := &captureAuditor{}
		h := HTTPHandler(capAud, "plateau.iam.oidc.authorize.v1", CategoryAuthn)(
			http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				Skip(r.Context())
				http.Redirect(w, r, "/login?auth_request_id=req-1", http.StatusFound)
			}),
		)

		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "http://iam.local/authorize/callback?id=req-1", nil))
		if len(capAud.snapshot()) != 0 {
			t.Fatalf("expected 0 events when Skip called, got %d", len(capAud.snapshot()))
		}
	})

	t.Run("redirect with SetError and SetEventOverride records authz denial failure", func(t *testing.T) {
		capAud := &captureAuditor{}
		h := HTTPHandler(capAud, "plateau.admin.auth.callback.v1", CategoryAuthn)(
			http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				SetEventOverride(r.Context(), AuthzDeniedEventType, CategoryAuthz)
				SetActor(r.Context(), security.Actor{Type: security.ActorTypeHuman, ID: "u-non-admin"})
				SetResource(r.Context(), "admin", "admin/global")
				SetError(r.Context(), "SECURITY_ERROR_REASON_PERMISSION_DENIED", "admin access denied")
				http.Redirect(w, r, "/login?error=access_denied", http.StatusFound)
			}),
		)

		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "http://admin.local/v1/admin/auth/callback", nil))
		events := capAud.snapshot()
		if len(events) != 1 {
			t.Fatalf("len(events) = %d, want 1", len(events))
		}
		ev := events[0]
		exts := ev.Extensions()
		if ev.Type() != AuthzDeniedEventType || exts[ExtCategory] != CategoryAuthz {
			t.Fatalf("override = (%q, %v)", ev.Type(), exts[ExtCategory])
		}
		if exts[ExtOutcome] != OutcomeFailure || exts[ExtErrorCode] != "SECURITY_ERROR_REASON_PERMISSION_DENIED" {
			t.Fatalf("failure = (%v, %v)", exts[ExtOutcome], exts[ExtErrorCode])
		}
	})
}

func TestHTTPHandlerOnlyErrorsAndPostHookZeroSecretLeak(t *testing.T) {
	capAud := &captureAuditor{}
	wrap := HTTPHandler(
		capAud,
		"plateau.iam.oidc.token.v1",
		CategoryAuthn,
		WithHTTPOnlyErrors(),
		WithHTTPPostHook(func(ctx context.Context, r *http.Request, statusCode int) {
			if statusCode < http.StatusBadRequest {
				return
			}
			detail := make(map[string]any, 2)
			if clientID, _, ok := r.BasicAuth(); ok && clientID != "" {
				detail["client_id"] = clientID
			} else if cid := strings.TrimSpace(r.PostForm.Get("client_id")); cid != "" {
				detail["client_id"] = cid
			}
			if gt := strings.TrimSpace(r.PostForm.Get("grant_type")); gt != "" {
				detail["grant_type"] = gt
			}
			SetDetail(ctx, detail)
		}),
	)

	tokenHandler := wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		if r.PostForm.Get("grant_type") == "client_credentials" && r.PostForm.Get("fail") == "" {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"access_token":"jwt-secret"}`))
			return
		}
		http.Error(w, "invalid_grant", http.StatusBadRequest)
	}))

	// 1. Successful token exchange produces zero events under WithHTTPOnlyErrors.
	okForm := url.Values{
		"grant_type":    {"client_credentials"},
		"client_id":     {"admin"},
		"client_secret": {"top-secret-value"},
	}
	okReq := httptest.NewRequest(http.MethodPost, "http://iam.local/oauth/token", strings.NewReader(okForm.Encode()))
	okReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	tokenHandler.ServeHTTP(httptest.NewRecorder(), okReq)
	if len(capAud.snapshot()) != 0 {
		t.Fatalf("expected 0 events on 200 /oauth/token, got %d", len(capAud.snapshot()))
	}

	// 2. Failed token exchange emits failure with grant_type + client_id and zero secrets.
	badForm := url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {"authz-code-secret"},
		"refresh_token": {"refresh-token-secret"},
		"code_verifier": {"pkce-verifier-secret"},
		"client_secret": {"body-client-secret"},
	}
	badReq := httptest.NewRequest(http.MethodPost, "http://iam.local/oauth/token", strings.NewReader(badForm.Encode()))
	badReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	badReq.SetBasicAuth("admin", "basic-client-secret")
	badRec := httptest.NewRecorder()
	tokenHandler.ServeHTTP(badRec, badReq)

	events := capAud.snapshot()
	if len(events) != 1 {
		t.Fatalf("len(events) = %d, want 1", len(events))
	}
	ev := events[0]
	exts := ev.Extensions()
	if ev.Subject() != "POST /oauth/token" || exts[ExtOutcome] != OutcomeFailure || exts[ExtErrorCode] != "BAD_REQUEST" {
		t.Fatalf("token failure event = (%q, %v, %v)", ev.Subject(), exts[ExtOutcome], exts[ExtErrorCode])
	}
	rawData := string(ev.Data())
	for _, secret := range []string{"authz-code-secret", "refresh-token-secret", "pkce-verifier-secret", "body-client-secret", "basic-client-secret"} {
		if strings.Contains(rawData, secret) {
			t.Fatalf("detail leaked secret %q: %s", secret, rawData)
		}
	}
	var detail map[string]any
	if err := json.Unmarshal(ev.Data(), &detail); err != nil {
		t.Fatalf("unmarshal detail: %v", err)
	}
	if detail["client_id"] != "admin" || detail["grant_type"] != "authorization_code" {
		t.Fatalf("detail = %v, want client_id=admin & grant_type=authorization_code", detail)
	}

	// 3. Emit failure never breaks HTTP response.
	failAud := &captureAuditor{err: errors.New("broker down")}
	failHandler := HTTPHandler(failAud, "plateau.iam.oidc.token.v1", CategoryAuthn)(
		http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}),
	)
	rec500 := httptest.NewRecorder()
	failHandler.ServeHTTP(rec500, httptest.NewRequest(http.MethodPost, "http://iam.local/oauth/token", nil))
	if rec500.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec500.Code)
	}
}

type abortOnCommitWriter struct {
	http.ResponseWriter
}

type simulatedCommitAbort struct{}

func (w *abortOnCommitWriter) WriteHeader(statusCode int) {
	if statusCode == http.StatusFound {
		w.ResponseWriter.WriteHeader(http.StatusInternalServerError)
		panic(simulatedCommitAbort{})
	}
	w.ResponseWriter.WriteHeader(statusCode)
}

func TestHTTPHandlerEmitsFailureOnSessionCommitAbort(t *testing.T) {
	t.Run("outer session commit panic during 302 WriteHeader emits 500 failure and re-panics", func(t *testing.T) {
		capAud := &captureAuditor{}
		inner := HTTPHandler(capAud, "plateau.admin.auth.callback.v1", CategoryAuthn)(
			http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				SetActor(r.Context(), security.Actor{Type: security.ActorTypeHuman, ID: "admin-1"})
				SetResource(r.Context(), "iam.plateau.dev/User", "users/admin-1")
				http.Redirect(w, r, "/dashboard", http.StatusFound)
			}),
		)

		rec := httptest.NewRecorder()
		recovered := false
		func() {
			defer func() {
				if v := recover(); v != nil {
					if _, ok := v.(simulatedCommitAbort); !ok {
						t.Fatalf("unexpected panic value: %T (%v)", v, v)
					}
					recovered = true
				}
			}()
			inner.ServeHTTP(&abortOnCommitWriter{ResponseWriter: rec}, httptest.NewRequest(http.MethodGet, "http://admin.local/v1/admin/auth/callback", nil))
		}()

		if !recovered {
			t.Fatal("expected simulatedCommitAbort panic to propagate to outer LoadAndSave guard")
		}
		if rec.Code != http.StatusInternalServerError {
			t.Fatalf("outer response status = %d, want 500", rec.Code)
		}
		events := capAud.snapshot()
		if len(events) != 1 {
			t.Fatalf("len(events) = %d, want 1 failure event on aborted commit", len(events))
		}
		exts := events[0].Extensions()
		if exts[ExtOutcome] != OutcomeFailure || exts[ExtErrorCode] != "INTERNAL_SERVER_ERROR" || exts[ExtErrorMessage] != "internal server error" {
			t.Fatalf("aborted commit event extensions = %+v", exts)
		}
		if exts[ExtActorType] != "human" || exts[ExtActorID] != "admin-1" || exts[ExtResourceName] != "users/admin-1" {
			t.Fatalf("aborted commit event lost actor/resource attribution: %+v", exts)
		}
	})
}
