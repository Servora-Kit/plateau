package service

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	adminauditpb "github.com/Servora-Kit/plateau/api/gen/go/plateau/admin/audit/v1"
	adminauthpb "github.com/Servora-Kit/plateau/api/gen/go/plateau/admin/auth/v1"
	auditeventpb "github.com/Servora-Kit/plateau/api/gen/go/plateau/audit/event/v1"
	userpb "github.com/Servora-Kit/plateau/api/gen/go/plateau/iam/user/v1"
	securityerrorspb "github.com/Servora-Kit/plateau/api/gen/go/plateau/security/errors/v1"
	oidcpb "github.com/Servora-Kit/plateau/api/gen/go/plateau/security/oidc/v1"
	"github.com/Servora-Kit/plateau/app/admin/service/internal/biz"
	"github.com/Servora-Kit/plateau/security"
	"github.com/Servora-Kit/plateau/security/audit"
	authnsession "github.com/Servora-Kit/plateau/security/authn/session"
	securityoidc "github.com/Servora-Kit/plateau/security/oidc"
	httpsession "github.com/Servora-Kit/plateau/security/session"
	"github.com/alexedwards/scs/v2"
	"github.com/alexedwards/scs/v2/memstore"
	cloudevents "github.com/cloudevents/sdk-go/v2"
	kerrors "github.com/go-kratos/kratos/v3/errors"
	khttp "github.com/go-kratos/kratos/v3/transport/http"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type fakeAuditRepo struct {
	listFn     func(context.Context, *auditeventpb.ListEventsRequest) (*auditeventpb.ListEventsResponse, error)
	getFn      func(context.Context, *auditeventpb.GetEventRequest) (*auditeventpb.Event, error)
	servicesFn func(context.Context, *auditeventpb.ListServicesRequest) (*auditeventpb.ListServicesResponse, error)
}

func (f *fakeAuditRepo) ListEvents(ctx context.Context, req *auditeventpb.ListEventsRequest) (*auditeventpb.ListEventsResponse, error) {
	if f.listFn != nil {
		return f.listFn(ctx, req)
	}
	return &auditeventpb.ListEventsResponse{}, nil
}

func (f *fakeAuditRepo) GetEvent(ctx context.Context, req *auditeventpb.GetEventRequest) (*auditeventpb.Event, error) {
	if f.getFn != nil {
		return f.getFn(ctx, req)
	}
	return &auditeventpb.Event{}, nil
}

func (f *fakeAuditRepo) ListServices(ctx context.Context, req *auditeventpb.ListServicesRequest) (*auditeventpb.ListServicesResponse, error) {
	if f.servicesFn != nil {
		return f.servicesFn(ctx, req)
	}
	return &auditeventpb.ListServicesResponse{Services: []string{"//admin.service", "//iam.service"}}, nil
}

func TestAuditServiceErrorMappingShieldsHumanSession(t *testing.T) {
	repo := &fakeAuditRepo{}
	uc, err := biz.NewAuditUsecase(repo)
	if err != nil {
		t.Fatal(err)
	}
	svc, err := NewAuditService(uc)
	if err != nil {
		t.Fatal(err)
	}

	adminCtx := security.WithActor(context.Background(), security.Actor{Type: security.ActorTypeHuman, ID: "admin-uid-1"})

	// 1. Invalid resource names and invalid query arguments -> 400 ADMIN_AUDIT_ERROR_REASON_INVALID_ARGUMENT.
	for _, badName := range []string{"", "users/123", "events/", "events/a/b"} {
		_, err := svc.GetEvent(adminCtx, &adminauditpb.GetEventRequest{Name: badName})
		if !adminauditpb.IsAdminAuditErrorReasonInvalidArgument(err) {
			t.Fatalf("GetEvent(%q) err = %v, want INVALID_ARGUMENT (400)", badName, err)
		}
	}
	now := time.Now().UTC()
	if _, err := svc.ListEvents(adminCtx, &adminauditpb.ListEventsRequest{
		StartTime: timestamppb.New(now),
		EndTime:   timestamppb.New(now.Add(-time.Hour)),
	}); !adminauditpb.IsAdminAuditErrorReasonInvalidArgument(err) {
		t.Fatalf("ListEvents(start > end) err = %v, want INVALID_ARGUMENT (400)", err)
	}

	// 2. Downstream NOT_FOUND -> 404 EVENT_NOT_FOUND; INVALID_PAGE_TOKEN -> 400 INVALID_PAGE_TOKEN.
	repo.getFn = func(context.Context, *auditeventpb.GetEventRequest) (*auditeventpb.Event, error) {
		return nil, auditeventpb.ErrorEventErrorReasonNotFound("not found")
	}
	if _, err := svc.GetEvent(adminCtx, &adminauditpb.GetEventRequest{Name: "events/missing"}); !adminauditpb.IsAdminAuditErrorReasonEventNotFound(err) {
		t.Fatalf("GetEvent(missing) err = %v, want EVENT_NOT_FOUND (404)", err)
	}

	repo.listFn = func(context.Context, *auditeventpb.ListEventsRequest) (*auditeventpb.ListEventsResponse, error) {
		return nil, auditeventpb.ErrorEventErrorReasonInvalidPageToken("bad cursor")
	}
	if _, err := svc.ListEvents(adminCtx, &adminauditpb.ListEventsRequest{PageToken: "bad"}); !adminauditpb.IsAdminAuditErrorReasonInvalidPageToken(err) {
		t.Fatalf("ListEvents(bad token) err = %v, want INVALID_PAGE_TOKEN (400)", err)
	}

	// 3. Downstream 401/403/503/timeout/token errors MUST all become 503 DOWNSTREAM_UNAVAILABLE (never 401/403!).
	downstreamFailures := []struct {
		name string
		err  error
	}{
		{"storage_unavailable", auditeventpb.ErrorEventErrorReasonStorageUnavailable("clickhouse down")},
		{"service_jwt_unauthenticated", securityerrorspb.ErrorSecurityErrorReasonUnauthenticated("bad service token")},
		{"grpc_unauthenticated", status.Error(codes.Unauthenticated, "expired service token")},
		{"openfga_permission_denied", securityerrorspb.ErrorSecurityErrorReasonPermissionDenied("missing service:admin read_events")},
		{"grpc_permission_denied", status.Error(codes.PermissionDenied, "denied")},
		{"grpc_unavailable", status.Error(codes.Unavailable, "connection refused")},
		{"downstream_rpc_timeout_with_live_caller_ctx", context.DeadlineExceeded},
		{"oidc_token_fetch_failure", errors.New("oauth2: cannot fetch client_credentials token")},
	}
	for _, tc := range downstreamFailures {
		t.Run(tc.name, func(t *testing.T) {
			repo.listFn = func(context.Context, *auditeventpb.ListEventsRequest) (*auditeventpb.ListEventsResponse, error) {
				return nil, tc.err
			}
			_, gotErr := svc.ListEvents(adminCtx, &adminauditpb.ListEventsRequest{})
			if !adminauditpb.IsAdminAuditErrorReasonDownstreamUnavailable(gotErr) {
				t.Fatalf("ListEvents(%s) err = %v (kratos=%+v), want 503 ADMIN_AUDIT_ERROR_REASON_DOWNSTREAM_UNAVAILABLE", tc.name, gotErr, kerrors.FromError(gotErr))
			}
			repo.servicesFn = func(context.Context, *auditeventpb.ListServicesRequest) (*auditeventpb.ListServicesResponse, error) {
				return nil, tc.err
			}
			_, gotSvcErr := svc.ListServices(adminCtx, &adminauditpb.ListServicesRequest{})
			if !adminauditpb.IsAdminAuditErrorReasonDownstreamUnavailable(gotSvcErr) {
				t.Fatalf("ListServices(%s) err = %v, want 503 ADMIN_AUDIT_ERROR_REASON_DOWNSTREAM_UNAVAILABLE", tc.name, gotSvcErr)
			}
		})
	}

	// 4. Caller's own canceled or deadline-exceeded context maps to 499 / 504.
	canceledCtx, cancel := context.WithCancel(adminCtx)
	cancel()
	repo.listFn = func(ctx context.Context, _ *auditeventpb.ListEventsRequest) (*auditeventpb.ListEventsResponse, error) {
		return nil, ctx.Err()
	}
	if _, err := svc.ListEvents(canceledCtx, &adminauditpb.ListEventsRequest{}); kerrors.Code(err) != 499 {
		t.Fatalf("ListEvents(caller canceled) code = %d, want 499", kerrors.Code(err))
	}

	expiredCtx, cancelTimeout := context.WithDeadline(adminCtx, time.Now().Add(-time.Second))
	defer cancelTimeout()
	if _, err := svc.ListEvents(expiredCtx, &adminauditpb.ListEventsRequest{}); kerrors.Code(err) != 504 {
		t.Fatalf("ListEvents(caller deadline exceeded) code = %d, want 504", kerrors.Code(err))
	}
}

type authCaptureAuditor struct {
	events []cloudevents.Event
}

func (a *authCaptureAuditor) Emit(_ context.Context, ev cloudevents.Event) error {
	a.events = append(a.events, ev.Clone())
	return nil
}

func TestAuthServiceCallbackAndLogoutAuditEnrichment(t *testing.T) {
	mgr := scs.New()
	mgr.Store = memstore.NewWithCleanupInterval(0)

	oidcCfg := &oidcpb.OIDCClient{
		Issuer:       proto.String("http://127.0.0.1:1"),
		ClientId:     proto.String("admin"),
		ClientSecret: proto.String("{{SECRET_0006testsecret}}"),
		RedirectUri:  "http://localhost:10012/v1/admin/auth/callback",
		CookieSecret: "0123456789abcdef0123456789abcdef",
		Scopes:       []string{"openid", "profile", "email"},
	}
	rp, err := securityoidc.NewRelyingParty(oidcCfg)
	if err != nil {
		t.Fatal(err)
	}
	authSvc := &AuthService{
		oidcRP:  rp,
		manager: mgr,
	}

	userAuthn, err := authnsession.NewUserAuthenticator(mgr)
	if err != nil {
		t.Fatal(err)
	}
	recAuditor := &authCaptureAuditor{}
	browserFilter, err := httpsession.NewFilter(mgr, httpsession.FilterConfig{
		PublicOrigin: "http://localhost:10012",
		JSONPrefixes: []string{"/v1/admin/"},
	})
	if err != nil {
		t.Fatal(err)
	}
	srv := khttp.NewServer(
		khttp.Filter(browserFilter),
		khttp.Middleware(
			authnsession.Server(userAuthn, security.WithRulesFuncs(adminauthpb.AuthnRules)),
			audit.Server(recAuditor, security.WithRulesFuncs(adminauthpb.AuditRules)),
		),
	)
	authSvc.RegisterOIDCHTTP(srv, recAuditor)
	adminauthpb.RegisterAuthServiceHTTPServer(srv, authSvc)

	// 1. Callback without handshake cookie -> redirects ?error=login_failed and emits OIDC_LOGIN_FAILED without leaking code.
	const leakedCallbackCode = "{{SECRET_0007cbcode}}"
	req := httptest.NewRequest(http.MethodGet, "http://localhost:10012/v1/admin/auth/callback?code="+leakedCallbackCode+"&state=bad-state", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	if w.Code != http.StatusFound {
		t.Fatalf("callback status = %d, want 302", w.Code)
	}
	if len(recAuditor.events) != 1 {
		t.Fatalf("expected 1 callback audit event, got %d", len(recAuditor.events))
	}
	ev := recAuditor.events[0]
	if ev.Type() != "plateau.admin.auth.callback.v1" ||
		ev.Subject() != "GET /v1/admin/auth/callback" ||
		ev.Extensions()[audit.ExtOutcome] != audit.OutcomeFailure ||
		ev.Extensions()[audit.ExtErrorCode] != "OIDC_LOGIN_FAILED" ||
		ev.Extensions()[audit.ExtActorType] != "anonymous" {
		t.Fatalf("unexpected failed callback audit event: %+v", ev)
	}
	if strings.Contains(ev.Subject(), leakedCallbackCode) {
		t.Fatalf("callback code leaked into subject: %q", ev.Subject())
	}

	// 2. Logout enriches resource = users/<actor_id> and records human actor.
	recAuditor.events = nil
	email := "admin42@plateau.local"
	seedW := httptest.NewRecorder()
	httpsession.LoadAndSave(mgr)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if saveErr := authnsession.SaveUser(r.Context(), mgr, &userpb.User{
			Name:   "users/admin-42",
			UserId: "admin-42",
			Email:  &email,
		}); saveErr != nil {
			t.Errorf("SaveUser: %v", saveErr)
		}
		w.WriteHeader(http.StatusNoContent)
	})).ServeHTTP(seedW, httptest.NewRequest(http.MethodGet, "http://localhost:10012/", nil))
	var adminCookie *http.Cookie
	for _, ck := range seedW.Result().Cookies() {
		if ck.Name == mgr.Cookie.Name {
			adminCookie = ck
		}
	}
	if adminCookie == nil {
		t.Fatal("failed to issue test session cookie")
	}

	logoutReq := httptest.NewRequest(http.MethodPost, "http://localhost:10012/v1/admin/auth/logout", strings.NewReader(`{}`))
	logoutReq.Header.Set("Content-Type", "application/json")
	logoutReq.Header.Set("Origin", "http://localhost:10012")
	logoutReq.AddCookie(adminCookie)
	logoutW := httptest.NewRecorder()
	srv.ServeHTTP(logoutW, logoutReq)
	if logoutW.Code != http.StatusOK {
		t.Fatalf("logout status = %d body = %s", logoutW.Code, logoutW.Body.String())
	}
	if len(recAuditor.events) != 1 {
		t.Fatalf("expected 1 logout audit event, got %d", len(recAuditor.events))
	}
	logoutEv := recAuditor.events[0]
	if logoutEv.Subject() != adminauthpb.OperationAuthServiceLogout ||
		logoutEv.Extensions()[audit.ExtActorType] != "human" ||
		logoutEv.Extensions()[audit.ExtActorID] != "admin-42" ||
		logoutEv.Extensions()[audit.ExtResourceType] != "iam.plateau.dev/User" ||
		logoutEv.Extensions()[audit.ExtResourceName] != "users/admin-42" {
		t.Fatalf("unexpected logout audit event: %+v", logoutEv)
	}
}

func TestAuthServiceCallbackCancelledVsForgedError(t *testing.T) {
	var opURL string
	opSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/.well-known/openid-configuration" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"issuer":"` + opURL + `","authorization_endpoint":"` + opURL + `/authorize","token_endpoint":"` + opURL + `/oauth/token","jwks_uri":"` + opURL + `/keys"}`))
			return
		}
		http.NotFound(w, r)
	}))
	defer opSrv.Close()
	opURL = opSrv.URL

	mgr := scs.New()
	mgr.Store = memstore.NewWithCleanupInterval(0)
	rp, err := securityoidc.NewRelyingParty(&oidcpb.OIDCClient{
		Issuer:       proto.String(opURL),
		ClientId:     proto.String("admin"),
		ClientSecret: proto.String("admin-client-secret-for-callback-test"),
		RedirectUri:  "http://localhost:10012/v1/admin/auth/callback",
		CookieSecret: "0123456789abcdef0123456789abcdef",
		Scopes:       []string{"openid", "profile", "email"},
	})
	if err != nil {
		t.Fatal(err)
	}
	authSvc := &AuthService{oidcRP: rp, manager: mgr}
	recAuditor := &authCaptureAuditor{}
	browserFilter, err := httpsession.NewFilter(mgr, httpsession.FilterConfig{
		PublicOrigin: "http://localhost:10012",
		JSONPrefixes: []string{"/v1/admin/"},
	})
	if err != nil {
		t.Fatal(err)
	}
	srv := khttp.NewServer(khttp.Filter(browserFilter))
	authSvc.RegisterOIDCHTTP(srv, recAuditor)

	// 1. Forged ?error=access_denied WITHOUT handshake cookie must NOT map to login_cancelled.
	forgedW := httptest.NewRecorder()
	srv.ServeHTTP(forgedW, httptest.NewRequest(http.MethodGet, "http://localhost:10012/v1/admin/auth/callback?error=access_denied&state=fake&iss="+url.QueryEscape(opURL), nil))
	if forgedW.Code != http.StatusFound || forgedW.Header().Get("Location") != "/auth/login?error=login_failed" || forgedW.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("forged cancel = (%d, %q, cc=%q), want 302 /auth/login?error=login_failed", forgedW.Code, forgedW.Header().Get("Location"), forgedW.Header().Get("Cache-Control"))
	}
	if len(recAuditor.events) != 1 || recAuditor.events[0].Extensions()[audit.ExtErrorCode] != "OIDC_LOGIN_FAILED" {
		t.Fatalf("forged cancel audit = %+v, want OIDC_LOGIN_FAILED", recAuditor.events)
	}

	// 2. Legitimate OIDC cancel with valid handshake cookie + matching state + iss -> login_cancelled + OIDC_LOGIN_CANCELLED.
	recAuditor.events = nil
	beginW := httptest.NewRecorder()
	srv.ServeHTTP(beginW, httptest.NewRequest(http.MethodGet, "http://localhost:10012/v1/admin/auth/login", nil))
	if beginW.Code != http.StatusFound || beginW.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("BeginLogin = (%d, cc=%q)", beginW.Code, beginW.Header().Get("Cache-Control"))
	}
	var handshake *http.Cookie
	for _, ck := range beginW.Result().Cookies() {
		if ck.Name == "admin_oidc_handshake" {
			handshake = ck
		}
	}
	authLoc, err := url.Parse(beginW.Header().Get("Location"))
	if err != nil || handshake == nil {
		t.Fatalf("BeginLogin redirect/cookie err=%v handshake=%v", err, handshake)
	}
	stateParam := authLoc.Query().Get("state")

	cancelReq := httptest.NewRequest(http.MethodGet, "http://localhost:10012/v1/admin/auth/callback?error=access_denied&state="+url.QueryEscape(stateParam)+"&iss="+url.QueryEscape(opURL), nil)
	cancelReq.AddCookie(handshake)
	cancelW := httptest.NewRecorder()
	srv.ServeHTTP(cancelW, cancelReq)
	if cancelW.Code != http.StatusFound || cancelW.Header().Get("Location") != "/auth/login?error=login_cancelled" || cancelW.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("legit cancel = (%d, %q, cc=%q), want 302 /auth/login?error=login_cancelled", cancelW.Code, cancelW.Header().Get("Location"), cancelW.Header().Get("Cache-Control"))
	}
	if len(recAuditor.events) != 1 || recAuditor.events[0].Extensions()[audit.ExtErrorCode] != "OIDC_LOGIN_CANCELLED" {
		t.Fatalf("legit cancel audit = %+v, want OIDC_LOGIN_CANCELLED", recAuditor.events)
	}
}
