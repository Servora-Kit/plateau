package session

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	userpb "github.com/Servora-Kit/plateau/api/gen/go/iam/user/v1"
	authnpb "github.com/Servora-Kit/plateau/api/gen/go/plateau/security/authn/v1"
	securitypb "github.com/Servora-Kit/plateau/api/gen/go/plateau/security/errors/v1"
	sessionpb "github.com/Servora-Kit/plateau/api/gen/go/plateau/security/session/v1"
	"github.com/Servora-Kit/plateau/security"
	httpsession "github.com/Servora-Kit/plateau/security/session"
	"github.com/alexedwards/scs/v2"
	"github.com/alexedwards/scs/v2/memstore"
	"github.com/go-kratos/kratos/v3/transport"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/durationpb"
)

func newTestSessionManager(t *testing.T) *scs.SessionManager {
	t.Helper()
	store := memstore.NewWithCleanupInterval(0)
	manager, err := httpsession.New(&sessionpb.Session{
		Lifetime:    durationpb.New(time.Hour),
		IdleTimeout: durationpb.New(10 * time.Minute),
		Cookie: &sessionpb.Cookie{
			Name:   new("test_session"),
			Secure: new(false),
		},
	}, store)
	if err != nil {
		t.Fatal(err)
	}
	return manager
}

func TestUserSessionSaveRotationAuthAndDefensiveClone(t *testing.T) {
	manager := newTestSessionManager(t)
	userAuthn, err := NewUserAuthenticator(manager)
	if err != nil {
		t.Fatal(err)
	}
	requiredRules := security.WithRulesFuncs(func() map[string]*authnpb.AuthnRule {
		return map[string]*authnpb.AuthnRule{"/test/Get": {Mode: authnpb.AuthnMode_AUTHN_MODE_REQUIRED}}
	})
	mw := Server(userAuthn, requiredRules)

	// Missing session credentials -> 401 Unauthenticated.
	emptyCtx := transport.NewServerContext(loadedContext(manager), testTransport{})
	if _, err := mw(func(context.Context, any) (any, error) {
		t.Fatal("handler should not be called without session snapshot")
		return nil, nil
	})(emptyCtx, nil); !securitypb.IsSecurityErrorReasonUnauthenticated(err) {
		t.Fatalf("missing session snapshot err = %v, want Unauthenticated", err)
	}

	// SaveUser rejects unloaded context and empty user_id.
	validUser := &userpb.User{
		Name:          "users/user-101",
		UserId:        "user-101",
		Email:         new("alice@plateau.local"),
		EmailVerified: true,
		Profile: &userpb.UserProfile{
			Name:   new("Alice"),
			Locale: new("zh-CN"),
		},
	}
	if err := SaveUser(context.Background(), manager, validUser); err == nil {
		t.Fatal("expected SaveUser to reject unloaded session context")
	}
	if err := SaveUser(loadedContext(manager), manager, &userpb.User{}); err == nil {
		t.Fatal("expected SaveUser to reject empty user_id")
	}

	// Establish initial anonymous cookie, then SaveUser and verify SCS token rotation.
	preRec := httptest.NewRecorder()
	httpsession.LoadAndSave(manager)(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		manager.Put(r.Context(), "anon_flag", "1")
	})).ServeHTTP(preRec, httptest.NewRequest(http.MethodGet, "/", nil))
	preCookie := preRec.Result().Cookies()[0]

	loginRec := httptest.NewRecorder()
	loginReq := httptest.NewRequest(http.MethodGet, "/", nil)
	loginReq.AddCookie(preCookie)
	httpsession.LoadAndSave(manager)(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		if err := SaveUser(r.Context(), manager, validUser); err != nil {
			t.Fatal(err)
		}
	})).ServeHTTP(loginRec, loginReq)
	rotatedCookie := loginRec.Result().Cookies()[0]
	if rotatedCookie.Value == preCookie.Value {
		t.Fatal("SaveUser did not rotate SCS session token")
	}

	// Mutating original struct after SaveUser does not affect stored session.
	validUser.Profile.Name = new("MutatedInput")

	// Authenticate request using rotated cookie and verify defensive clone isolation in UserFrom.
	verifyRec := httptest.NewRecorder()
	verifyReq := httptest.NewRequest(http.MethodGet, "/", nil)
	verifyReq.AddCookie(rotatedCookie)
	httpsession.LoadAndSave(manager)(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		srvCtx := transport.NewServerContext(r.Context(), testTransport{})
		_, err := mw(func(ctx context.Context, _ any) (any, error) {
			actor, ok := security.ActorFrom(ctx)
			if !ok || actor != (security.Actor{Type: security.ActorTypeHuman, ID: "user-101"}) {
				t.Fatalf("unexpected actor: %+v (ok=%v)", actor, ok)
			}
			first, ok := UserFrom(ctx)
			if !ok || first.GetProfile().GetName() != "Alice" || first.GetEmail() != "alice@plateau.local" {
				t.Fatalf("unexpected first UserFrom: %+v (ok=%v)", first, ok)
			}
			// Mutate returned snapshot and confirm subsequent UserFrom(ctx) stays pristine.
			first.UserId = "tampered-id"
			first.Profile.Name = new("TamperedName")

			second, ok := UserFrom(ctx)
			if !ok || second.GetUserId() != "user-101" || second.GetProfile().GetName() != "Alice" {
				t.Fatalf("UserFrom failed defensive clone isolation: %+v", second)
			}
			return nil, nil
		})(srvCtx, nil)
		if err != nil {
			t.Fatalf("middleware error: %v", err)
		}
	})).ServeHTTP(verifyRec, verifyReq)

	// Corrupt protobuf wire bytes in session -> 401 Unauthenticated.
	corruptRec := httptest.NewRecorder()
	httpsession.LoadAndSave(manager)(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		manager.Put(r.Context(), "plateau_session_user", []byte{0xff, 0xff, 0xff, 0x01})
		srvCtx := transport.NewServerContext(r.Context(), testTransport{})
		_, err := mw(func(context.Context, any) (any, error) {
			t.Fatal("handler should not execute with corrupt protobuf bytes")
			return nil, nil
		})(srvCtx, nil)
		if !securitypb.IsSecurityErrorReasonUnauthenticated(err) {
			t.Fatalf("corrupt protobuf err = %v, want Unauthenticated", err)
		}
	})).ServeHTTP(corruptRec, httptest.NewRequest(http.MethodGet, "/", nil))
}

func TestUserSessionEdgeCases(t *testing.T) {
	if _, err := NewUserAuthenticator(nil); err == nil {
		t.Fatal("expected NewUserAuthenticator(nil) to fail")
	}
	if u, ok := UserFrom(context.Background()); ok || u != nil {
		t.Fatalf("UserFrom(empty ctx) = (%+v, %v), want (nil, false)", u, ok)
	}

	manager := newTestSessionManager(t)
	userAuthn, err := NewUserAuthenticator(manager)
	if err != nil {
		t.Fatal(err)
	}
	requiredRules := security.WithRulesFuncs(func() map[string]*authnpb.AuthnRule {
		return map[string]*authnpb.AuthnRule{"/test/Get": {Mode: authnpb.AuthnMode_AUTHN_MODE_REQUIRED}}
	})
	mw := Server(userAuthn, requiredRules)

	// Stored protobuf User without user_id -> 401 Unauthenticated via ActorMapper rejection.
	encodedNoID, err := proto.Marshal(&userpb.User{Email: new("noid@plateau.local")})
	if err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	httpsession.LoadAndSave(manager)(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		manager.Put(r.Context(), userSessionKey, encodedNoID)
		srvCtx := transport.NewServerContext(r.Context(), testTransport{})
		_, err := mw(func(context.Context, any) (any, error) {
			t.Fatal("handler should not execute when stored User has empty user_id")
			return nil, nil
		})(srvCtx, nil)
		if !securitypb.IsSecurityErrorReasonUnauthenticated(err) {
			t.Fatalf("empty user_id snapshot err = %v, want Unauthenticated", err)
		}
	})).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
}
