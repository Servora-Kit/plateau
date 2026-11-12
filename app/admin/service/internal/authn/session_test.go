package authn

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	adminauthpb "github.com/Servora-Kit/plateau/api/gen/go/admin/auth/v1"
	sessionpb "github.com/Servora-Kit/plateau/api/gen/go/plateau/security/session/v1"
	"github.com/Servora-Kit/plateau/security"
	authnruntime "github.com/Servora-Kit/plateau/security/authn"
	authnsession "github.com/Servora-Kit/plateau/security/authn/session"
	httpsession "github.com/Servora-Kit/plateau/security/session"
	"github.com/alexedwards/scs/v2/memstore"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func TestSessionAuthenticatorRoundTripAndRotation(t *testing.T) {
	store := memstore.NewWithCleanupInterval(0)
	manager, err := httpsession.New(&sessionpb.Session{
		Lifetime:    durationpb.New(time.Hour),
		IdleTimeout: durationpb.New(10 * time.Minute),
		Cookie: &sessionpb.Cookie{
			Name:   proto.String("admin_session"),
			Secure: proto.Bool(false),
		},
	}, store)
	if err != nil {
		t.Fatal(err)
	}

	authenticator, err := NewSessionAuthenticator(manager)
	if err != nil {
		t.Fatal(err)
	}

	preRec := httptest.NewRecorder()
	httpsession.LoadAndSave(manager)(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		manager.Put(r.Context(), "pre", "1")
	})).ServeHTTP(preRec, httptest.NewRequest(http.MethodGet, "/", nil))
	preCookie := preRec.Result().Cookies()[0]

	authTime := time.Date(2026, 11, 12, 8, 0, 0, 0, time.UTC)
	loginRec := httptest.NewRecorder()
	loginReq := httptest.NewRequest(http.MethodGet, "/", nil)
	loginReq.AddCookie(preCookie)
	httpsession.LoadAndSave(manager)(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		if err := SavePrincipal(r.Context(), manager, &adminauthpb.AdminPrincipal{
			UserId:   "admin-1",
			Email:    "admin@plateau.local",
			Name:     "Admin",
			Picture:  "https://cdn.example/a.png",
			AuthTime: timestamppb.New(authTime),
		}); err != nil {
			t.Fatal(err)
		}
	})).ServeHTTP(loginRec, loginReq)

	rotatedCookie := loginRec.Result().Cookies()[0]
	if rotatedCookie.Value == preCookie.Value {
		t.Fatal("SavePrincipal did not rotate session token")
	}

	mw := authnsession.Server(authenticator, authnruntime.WithRulesFuncs(adminauthpb.AuthnRules))
	_ = mw

	verifyRec := httptest.NewRecorder()
	verifyReq := httptest.NewRequest(http.MethodGet, "/", nil)
	verifyReq.AddCookie(rotatedCookie)
	httpsession.LoadAndSave(manager)(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		raw := manager.GetBytes(r.Context(), SessionKey)
		got := new(adminauthpb.AdminPrincipal)
		if err := proto.Unmarshal(raw, got); err != nil {
			t.Fatal(err)
		}
		if got.GetUserId() != "admin-1" || got.GetEmail() != "admin@plateau.local" || !got.GetAuthTime().AsTime().Equal(authTime) {
			t.Fatalf("unexpected principal: %+v", got)
		}
		actor := security.Actor{Type: security.ActorTypeHuman, ID: got.GetUserId()}
		if !actor.Valid() {
			t.Fatal("actor should be valid")
		}
	})).ServeHTTP(verifyRec, verifyReq)
}
