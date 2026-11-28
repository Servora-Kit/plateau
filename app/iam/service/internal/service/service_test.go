package service

import (
	"context"
	"errors"
	crudpb "github.com/Servora-Kit/plateau/api/gen/go/plateau/crud/v1"
	accountpb "github.com/Servora-Kit/plateau/api/gen/go/plateau/iam/account/v1"
	authnpb "github.com/Servora-Kit/plateau/api/gen/go/plateau/iam/authn/v1"
	iamconfpb "github.com/Servora-Kit/plateau/api/gen/go/plateau/iam/conf/v1"
	oidcconfpb "github.com/Servora-Kit/plateau/api/gen/go/plateau/iam/oidc/conf/v1"
	userpb "github.com/Servora-Kit/plateau/api/gen/go/plateau/iam/user/v1"
	securityerrorspb "github.com/Servora-Kit/plateau/api/gen/go/plateau/security/errors/v1"
	iamauthn "github.com/Servora-Kit/plateau/app/iam/service/internal/authn"
	"github.com/Servora-Kit/plateau/app/iam/service/internal/biz"
	corecrud "github.com/Servora-Kit/plateau/crud"
	"github.com/Servora-Kit/plateau/security"
	sessionauthn "github.com/Servora-Kit/plateau/security/authn/session"
	"github.com/Servora-Kit/plateau/security/password"
	sessions "github.com/Servora-Kit/plateau/security/session"
	corepb "github.com/Servora-Kit/servora/api/gen/go/servora/core/v1"
	"github.com/alexedwards/scs/v2"
	"github.com/alexedwards/scs/v2/memstore"
	"github.com/go-kratos/kratos/v3/transport"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/fieldmaskpb"
	"google.golang.org/protobuf/types/known/timestamppb"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"testing"
	"time"
)

type stubUserRepo struct {
	user     *userpb.User
	listErr  error
	lastShow bool
}

func (r *stubUserRepo) Create(_ context.Context, u *userpb.User, _, _ string) (*userpb.User, error) {
	if u.Etag == "" {
		u.Etag = "etag-1"
	}
	r.user = u
	return u, nil
}
func (r *stubUserRepo) FindByEmail(context.Context, string) (*userpb.User, error) {
	if r.user == nil || r.user.DeleteTime != nil {
		return nil, biz.ErrNotFound
	}
	return r.user, nil
}
func (r *stubUserRepo) FindByEmailIncludingDeleted(context.Context, string) (*userpb.User, error) {
	if r.user == nil {
		return nil, biz.ErrNotFound
	}
	return r.user, nil
}
func (r *stubUserRepo) Get(context.Context, string) (*userpb.User, error) {
	if r.user == nil || r.user.DeleteTime != nil {
		return nil, biz.ErrNotFound
	}
	return r.user, nil
}
func (r *stubUserRepo) UpdateProfile(_ context.Context, _, _ string, p *userpb.UserProfile) (*userpb.User, error) {
	if r.user == nil || r.user.DeleteTime != nil {
		return nil, biz.ErrNotFound
	}
	r.user.Profile = p
	return r.user, nil
}
func (r *stubUserRepo) UpdateStatus(_ context.Context, _, _ string, st userpb.UserStatus, _ time.Time) (*userpb.User, error) {
	if r.user == nil || r.user.DeleteTime != nil {
		return nil, biz.ErrNotFound
	}
	r.user.Status = st
	return r.user, nil
}
func (r *stubUserRepo) GetUser(context.Context, userpb.UserName) (*userpb.User, error) {
	if r.user == nil {
		return nil, biz.ErrNotFound
	}
	return r.user, nil
}
func (r *stubUserRepo) ListUsers(_ context.Context, q corecrud.ListQuery, showDeleted bool) (corecrud.ListResult[*userpb.User], error) {
	r.lastShow = showDeleted
	if r.listErr != nil {
		return corecrud.ListResult[*userpb.User]{}, r.listErr
	}
	if r.user == nil || (!showDeleted && r.user.DeleteTime != nil) {
		return corecrud.NewListResult[*userpb.User](q, nil, "", nil)
	}
	return corecrud.NewListResult[*userpb.User](q, []*userpb.User{r.user}, "", nil)
}
func (r *stubUserRepo) UpdateUser(context.Context, userpb.UserName, *userpb.User, *fieldmaskpb.FieldMask, string) (*userpb.User, error) {
	if r.user == nil || r.user.DeleteTime != nil {
		return nil, biz.ErrNotFound
	}
	return r.user, nil
}
func (r *stubUserRepo) SoftDelete(_ context.Context, _ string, _ *biz.DeleteAccountProof, now time.Time) (*userpb.User, error) {
	if r.user == nil || r.user.DeleteTime != nil {
		return nil, biz.ErrNotFound
	}
	r.user.DeleteTime = timestamppb.New(now)
	return r.user, nil
}
func (r *stubUserRepo) Undelete(context.Context, string, time.Time) (*userpb.User, error) {
	if r.user == nil {
		return nil, biz.ErrNotFound
	}
	if r.user.DeleteTime == nil {
		return nil, biz.ErrAlreadyExists
	}
	r.user.DeleteTime = nil
	return r.user, nil
}
func (r *stubUserRepo) Expunge(context.Context, string) error {
	if r.user == nil {
		return biz.ErrNotFound
	}
	if r.user.DeleteTime == nil {
		return biz.ErrUserInvalidState
	}
	r.user = nil
	return nil
}

type stubCredentialRepo struct {
	cred *biz.PasswordCredential
}

func (r *stubCredentialRepo) FindActivePassword(context.Context, string) (*biz.PasswordCredential, error) {
	if r.cred == nil {
		return nil, biz.ErrNotFound
	}
	c := *r.cred
	return &c, nil
}
func (r *stubCredentialRepo) ReplacePassword(context.Context, string, string, string, string, string, time.Time) error {
	return nil
}
func (r *stubCredentialRepo) SetPasswordAndRevokeSessions(_ context.Context, _, hash string, _ time.Time) error {
	if r.cred != nil {
		r.cred.PasswordHash = hash
	}
	return nil
}

type stubSessionRepo struct {
	session *biz.LoginSession
}

func (r *stubSessionRepo) Find(_ context.Context, id string) (*biz.LoginSession, error) {
	if r.session == nil || r.session.ID != id {
		return nil, biz.ErrNotFound
	}
	return r.session, nil
}
func (r *stubSessionRepo) Revoke(context.Context, string, time.Time) error {
	r.session = nil
	return nil
}
func (r *stubSessionRepo) RevokeAll(context.Context, string, time.Time) error {
	r.session = nil
	return nil
}

type stubOIDCReq struct {
	clientID    string
	redirectURI string
	state       string
}

type stubAuthFlowRepo struct {
	users    *stubUserRepo
	creds    *stubCredentialRepo
	sessions *stubSessionRepo
	flows    map[string]*biz.AuthFlowSnapshot
	oidcReqs map[string]*stubOIDCReq
}

func newStubAuthFlowRepo(users *stubUserRepo, creds *stubCredentialRepo, sessions *stubSessionRepo) *stubAuthFlowRepo {
	return &stubAuthFlowRepo{
		users:    users,
		creds:    creds,
		sessions: sessions,
		flows:    make(map[string]*biz.AuthFlowSnapshot),
		oidcReqs: make(map[string]*stubOIDCReq),
	}
}

func (r *stubAuthFlowRepo) FindPendingOIDCRequest(_ context.Context, id string, _ time.Time) (*authnpb.AuthFlowOidcContext, time.Time, error) {
	req, ok := r.oidcReqs[id]
	if !ok {
		return nil, time.Time{}, biz.ErrNotFound
	}
	return &authnpb.AuthFlowOidcContext{
		ClientId:   req.clientID,
		ClientName: req.clientID,
	}, time.Now().Add(20 * time.Minute).UTC(), nil
}

func (r *stubAuthFlowRepo) Create(_ context.Context, seed *authnpb.AuthFlow, ownerHash, oidcReqID string, _ time.Time) (*authnpb.AuthFlow, error) {
	cloned := proto.CloneOf(seed)
	r.flows[cloned.GetFlowId()] = &biz.AuthFlowSnapshot{
		Flow:          proto.CloneOf(cloned),
		OwnerHash:     ownerHash,
		Status:        biz.AuthFlowStatusActive,
		OIDCRequestID: oidcReqID,
	}
	return cloned, nil
}

func (r *stubAuthFlowRepo) Get(_ context.Context, flowID string) (*biz.AuthFlowSnapshot, error) {
	snap, ok := r.flows[flowID]
	if !ok {
		return nil, biz.ErrNotFound
	}
	cp := *snap
	cp.Flow = proto.CloneOf(snap.Flow)
	return &cp, nil
}

func (r *stubAuthFlowRepo) Invalidate(_ context.Context, flowID string, _ time.Time) error {
	if snap, ok := r.flows[flowID]; ok {
		snap.Status = biz.AuthFlowStatusInvalidated
	}
	return nil
}

func (r *stubAuthFlowRepo) AdvanceAfterPassword(_ context.Context, flowID, _ string, cred biz.PasswordCredential, _ string, amr []string, stepTTL time.Duration, now time.Time) (*authnpb.AuthFlow, *biz.LoginSession, error) {
	snap := r.flows[flowID]
	u := r.users.user
	next := biz.EvaluateFlowProgress(amr, biz.DefaultAuthPolicy(), u.GetRequiredActions())
	next.FlowId = flowID
	next.Email = u.GetEmail()
	next.ExpiresTime = proto.CloneOf(snap.Flow.GetExpiresTime())
	if next.GetState() == authnpb.AuthFlowState_AUTH_FLOW_STATE_REQUIRED_ACTION {
		next.StepExpiresTime = timestamppb.New(now.Add(stepTTL))
		changed := biz.NormalizeTimestamp(cred.ChangedTime)
		snap.Flow = proto.CloneOf(next)
		snap.UserID = u.GetUserId()
		snap.UserEtag = u.GetEtag()
		snap.AuthenticatorID = cred.AuthenticatorID
		snap.PasswordAuthenticatorID = cred.PasswordAuthenticatorID
		snap.PasswordChangedTime = &changed
		return next, nil, nil
	}
	login := &biz.LoginSession{ID: "login-1", UserID: u.GetUserId(), AMR: biz.NormalizeAMR(amr), AuthTime: now}
	r.sessions.session = login
	next.ContinueUrl = biz.BuildContinueURL(snap.OIDCRequestID)
	next.User = proto.CloneOf(u)
	snap.Status = biz.AuthFlowStatusCompleted
	snap.Flow = proto.CloneOf(next)
	return next, login, nil
}

func (r *stubAuthFlowRepo) CompleteUpdatePassword(_ context.Context, flowID, _, newHash string, _ time.Duration, now time.Time) (*authnpb.AuthFlow, *biz.LoginSession, error) {
	snap := r.flows[flowID]
	u := r.users.user
	r.creds.cred.PasswordHash = newHash
	r.creds.cred.ChangedTime = biz.NormalizeTimestamp(now)
	u.RequiredActions = nil
	u.Etag = "etag-2"
	amr := biz.NormalizeAMR(snap.Flow.GetCompletedAmr())
	login := &biz.LoginSession{ID: "new-login-id", UserID: u.GetUserId(), AMR: amr, AuthTime: now}
	r.sessions.session = login
	done := &authnpb.AuthFlow{
		FlowId:       flowID,
		State:        authnpb.AuthFlowState_AUTH_FLOW_STATE_COMPLETED,
		CompletedAmr: amr,
		Email:        u.GetEmail(),
		ExpiresTime:  proto.CloneOf(snap.Flow.GetExpiresTime()),
		ContinueUrl:  biz.BuildContinueURL(snap.OIDCRequestID),
		User:         proto.CloneOf(u),
	}
	snap.Status = biz.AuthFlowStatusCompleted
	snap.Flow = proto.CloneOf(done)
	return done, login, nil
}

func (r *stubAuthFlowRepo) Reset(_ context.Context, flowID, newFlowID, ownerHash string, _ time.Time) (*authnpb.AuthFlow, string, error) {
	old := r.flows[flowID]
	old.Status = biz.AuthFlowStatusReset
	fresh := biz.EvaluateFlowProgress(nil, biz.DefaultAuthPolicy(), nil)
	fresh.FlowId = newFlowID
	fresh.ExpiresTime = proto.CloneOf(old.Flow.GetExpiresTime())
	r.flows[newFlowID] = &biz.AuthFlowSnapshot{
		Flow:          proto.CloneOf(fresh),
		OwnerHash:     ownerHash,
		Status:        biz.AuthFlowStatusActive,
		OIDCRequestID: old.OIDCRequestID,
	}
	return fresh, old.UserID, nil
}

func (r *stubAuthFlowRepo) Cancel(_ context.Context, flowID, _ string, _ time.Time) (string, string, string, error) {
	snap := r.flows[flowID]
	snap.Status = biz.AuthFlowStatusCancelled
	var redirectURI, state string
	if snap.OIDCRequestID != "" {
		if req := r.oidcReqs[snap.OIDCRequestID]; req != nil {
			redirectURI = req.redirectURI
			state = req.state
		}
		delete(r.oidcReqs, snap.OIDCRequestID)
	}
	return redirectURI, state, snap.UserID, nil
}

type stubVerifyRepo struct{ err error }

func (r stubVerifyRepo) Create(context.Context, string, string, string, time.Time) error {
	return r.err
}
func (r stubVerifyRepo) ConsumeAndActivate(context.Context, string, time.Time) (string, error) {
	return "", nil
}

type stubResetRepo struct{}

func (stubResetRepo) Create(context.Context, string, string, time.Time) error { return nil }
func (stubResetRepo) Peek(context.Context, string, time.Time) (*biz.PasswordResetProof, error) {
	return nil, biz.ErrInvalidToken
}
func (stubResetRepo) ConsumeAndReplacePassword(context.Context, biz.PasswordResetProof, string, time.Time) (string, error) {
	return "", nil
}

type stubCAP struct{}

func (stubCAP) ValidateToken(context.Context, string) (bool, error) { return true, nil }

type stubMailer struct{ err error }

func (m stubMailer) SendVerification(context.Context, string, string, int) error  { return m.err }
func (m stubMailer) SendPasswordReset(context.Context, string, string, int) error { return m.err }

type stubConsentPending struct {
	view        *authnpb.ConsentView
	ownerHash   string
	redirectURI string
	state       string
}

type stubConsentRepo struct {
	pending *stubConsentPending
	granted map[string][]string
	apps    []*accountpb.AuthorizedApp
	revoked []string
}

func (r *stubConsentRepo) GetConsentView(_ context.Context, userID, ownerHash, id string, _ time.Time) (*authnpb.ConsentView, error) {
	if r.pending == nil || r.pending.view.GetRequestId() != id || r.pending.ownerHash != ownerHash {
		return nil, biz.ErrOIDCRequestInvalid
	}
	cp := proto.CloneOf(r.pending.view)
	if r.granted != nil {
		cp.GrantedScopes = slices.Clone(r.granted[userID+":"+cp.GetClientId()])
	}
	return cp, nil
}

func (r *stubConsentRepo) ApproveConsent(_ context.Context, userID, ownerHash, id string, _ time.Time) (string, error) {
	if r.pending == nil || r.pending.view.GetRequestId() != id || r.pending.ownerHash != ownerHash {
		return "", biz.ErrOIDCRequestInvalid
	}
	if r.granted == nil {
		r.granted = make(map[string][]string)
	}
	cid := r.pending.view.GetClientId()
	key := userID + ":" + cid
	r.granted[key] = biz.MergeScopes(r.granted[key], r.pending.view.GetRequestedScopes())
	return cid, nil
}

func (r *stubConsentRepo) DenyConsent(_ context.Context, ownerHash, id string, _ time.Time) (string, string, string, error) {
	if r.pending == nil || r.pending.view.GetRequestId() != id || r.pending.ownerHash != ownerHash {
		return "", "", "", biz.ErrOIDCRequestInvalid
	}
	p := r.pending
	r.pending = nil
	return p.redirectURI, p.state, p.view.GetClientId(), nil
}

func (r *stubConsentRepo) ListAuthorizedApps(context.Context, string) ([]*accountpb.AuthorizedApp, error) {
	return r.apps, nil
}

func (r *stubConsentRepo) RevokeAuthorizedApp(_ context.Context, _, clientID string, _ time.Time) error {
	r.revoked = append(r.revoked, clientID)
	return nil
}

type failingSCSStore struct {
	scs.Store
	failCommit bool
}

func (s *failingSCSStore) Commit(token string, b []byte, expiry time.Time) error {
	if s.failCommit {
		return errors.New("scs commit failed")
	}
	return s.Store.Commit(token, b, expiry)
}

type stubHeader struct{}

func (stubHeader) Get(string) string      { return "" }
func (stubHeader) Set(string, string)     {}
func (stubHeader) Add(string, string)     {}
func (stubHeader) Keys() []string         { return nil }
func (stubHeader) Values(string) []string { return nil }

type stubServerTransport struct{ op string }

func (t stubServerTransport) Kind() transport.Kind            { return transport.KindHTTP }
func (t stubServerTransport) Endpoint() string                { return "" }
func (t stubServerTransport) Operation() string               { return t.op }
func (t stubServerTransport) RequestHeader() transport.Header { return stubHeader{} }
func (t stubServerTransport) ReplyHeader() transport.Header   { return stubHeader{} }

func TestAuthnAndAccountServiceInitialPasswordAndDeleteFlows(t *testing.T) {
	const initPass = "correct horse battery staple"
	const nextPass = "updated horse battery staple"
	hash, err := password.Hash(initPass)
	if err != nil {
		t.Fatal(err)
	}
	userID := "01912345-6789-7abc-8def-0123456789ab"
	email := "alice@example.com"
	users := &stubUserRepo{user: &userpb.User{
		Name:            "users/" + userID,
		UserId:          userID,
		Email:           &email,
		Status:          userpb.UserStatus_USER_STATUS_ACTIVE,
		EmailVerified:   true,
		Etag:            "etag-1",
		RequiredActions: []userpb.UserRequiredAction{userpb.UserRequiredAction_USER_REQUIRED_ACTION_UPDATE_PASSWORD},
	}}
	now := time.Now().UTC()
	creds := &stubCredentialRepo{cred: &biz.PasswordCredential{
		UserID: userID, AuthenticatorID: "auth-1", PasswordAuthenticatorID: "pa-1",
		PasswordHash: hash, ChangedTime: biz.NormalizeTimestamp(now),
	}}
	sessRepo := &stubSessionRepo{}
	flowRepo := newStubAuthFlowRepo(users, creds, sessRepo)
	flowRepo.oidcReqs["oidc-req-1"] = &stubOIDCReq{
		clientID:    "admin",
		redirectURI: "https://admin.example/callback",
		state:       "st-abc",
	}
	sessUC, err := biz.NewSessionUsecase(users, sessRepo)
	if err != nil {
		t.Fatal(err)
	}
	authnUC, err := biz.NewAuthenticationUsecase(&iamconfpb.IAM{}, &oidcconfpb.OIDC{Issuer: proto.String("https://iam.example")}, users, creds, sessUC, flowRepo, new(stubConsentRepo))
	if err != nil {
		t.Fatal(err)
	}
	app := &corepb.App{ExternalUrl: "https://iam.example"}
	accUC, err := biz.NewAccountUsecase(users, creds, stubVerifyRepo{}, stubResetRepo{}, new(stubConsentRepo), stubCAP{}, stubMailer{}, app)
	if err != nil {
		t.Fatal(err)
	}
	manager := scs.New()
	store := &failingSCSStore{Store: memstore.New()}
	manager.Store = store

	authnSvc, err := NewAuthnService(authnUC, manager)
	if err != nil {
		t.Fatal(err)
	}
	accSvc, err := NewAccountService(accUC, manager)
	if err != nil {
		t.Fatal(err)
	}

	// Unowned probe on GetAuthFlow returns 404 FLOW_NOT_FOUND and emits NO Set-Cookie!
	probeRec := httptest.NewRecorder()
	sessions.LoadAndSave(manager)(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		_, err := authnSvc.GetAuthFlow(r.Context(), &authnpb.GetAuthFlowRequest{FlowId: "nonexistent"})
		if !authnpb.IsAuthnErrorReasonFlowNotFound(err) {
			t.Fatalf("unowned GetAuthFlow = %v, want FLOW_NOT_FOUND", err)
		}
	})).ServeHTTP(probeRec, httptest.NewRequest(http.MethodGet, "/v1/iam/authn/flows/nonexistent", nil))
	if len(probeRec.Result().Cookies()) != 0 {
		t.Fatalf("unowned GetAuthFlow unexpectedly set cookies: %+v", probeRec.Result().Cookies())
	}

	// Browser A creates flow, submits initial password (enters REQUIRED_ACTION and clears stale LoginReferenceKey while keeping AuthFlowOwnerKey), then completes update password.
	var ctx context.Context
	sessions.LoadAndSave(manager)(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		ctx = r.Context()
	})).ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/v1/iam/authn/flows", nil))
	manager.Put(ctx, iamauthn.LoginReferenceKey, "stale-login-id")

	createResp, err := authnSvc.CreateAuthFlow(ctx, &authnpb.CreateAuthFlowRequest{OidcRequestId: "oidc-req-1"})
	if err != nil {
		t.Fatalf("CreateAuthFlow() error = %v", err)
	}
	flowID := createResp.GetFlow().GetFlowId()
	ownerToken := manager.GetString(ctx, iamauthn.AuthFlowOwnerKey)
	if flowID == "" || ownerToken == "" {
		t.Fatalf("CreateAuthFlow flowID=%q ownerToken=%q", flowID, ownerToken)
	}

	submitResp, err := authnSvc.SubmitPassword(ctx, &authnpb.SubmitPasswordRequest{
		FlowId:   flowID,
		Email:    email,
		Password: initPass,
	})
	if err != nil {
		t.Fatalf("SubmitPassword(required_actions) error = %v", err)
	}
	if got := manager.GetString(ctx, iamauthn.LoginReferenceKey); got != "" {
		t.Fatalf("SubmitPassword(REQUIRED_ACTION) retained LoginReferenceKey = %q, want empty", got)
	}
	if got := manager.GetString(ctx, iamauthn.AuthFlowOwnerKey); got != ownerToken {
		t.Fatalf("SubmitPassword(REQUIRED_ACTION) lost AuthFlowOwnerKey = %q, want %q", got, ownerToken)
	}
	if submitResp.GetFlow().GetState() != authnpb.AuthFlowState_AUTH_FLOW_STATE_REQUIRED_ACTION {
		t.Fatalf("SubmitPassword state = %v, want REQUIRED_ACTION", submitResp.GetFlow().GetState())
	}

	// Cross-browser caller (even if signed in!) cannot inspect or hijack Browser A's flow -> 404 FLOW_NOT_FOUND.
	var otherBrowserCtx context.Context
	sessions.LoadAndSave(manager)(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		otherBrowserCtx = r.Context()
	})).ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/v1/iam/authn/flows/"+flowID, nil))
	sessRepo.session = &biz.LoginSession{ID: "other-login", UserID: userID, AMR: []string{"pwd"}, AuthTime: now}
	manager.Put(otherBrowserCtx, iamauthn.LoginReferenceKey, "other-login")
	if _, err := authnSvc.GetAuthFlow(otherBrowserCtx, &authnpb.GetAuthFlowRequest{FlowId: flowID}); !authnpb.IsAuthnErrorReasonFlowNotFound(err) {
		t.Fatalf("cross-browser signed-in GetAuthFlow = %v, want FLOW_NOT_FOUND", err)
	}
	sessRepo.session = nil

	// Unchanged password returns 400 INVALID_PASSWORD while keeping flow retryable.
	if _, err := authnSvc.CompleteUpdatePassword(ctx, &authnpb.CompleteUpdatePasswordRequest{
		FlowId:      flowID,
		NewPassword: initPass,
	}); !authnpb.IsAuthnErrorReasonInvalidPassword(err) {
		t.Fatalf("CompleteUpdatePassword(same password) = %v, want INVALID_PASSWORD", err)
	}

	completeResp, err := authnSvc.CompleteUpdatePassword(ctx, &authnpb.CompleteUpdatePasswordRequest{
		FlowId:      flowID,
		NewPassword: nextPass,
	})
	if err != nil {
		t.Fatalf("CompleteUpdatePassword() error = %v", err)
	}
	if got := manager.GetString(ctx, iamauthn.LoginReferenceKey); got != "new-login-id" {
		t.Fatalf("CompleteUpdatePassword LoginReferenceKey = %q, want new-login-id", got)
	}
	if completeResp.GetFlow().GetContinueUrl() != "/authorize/callback?id=oidc-req-1" {
		t.Fatalf("CompleteUpdatePassword continue_url = %q", completeResp.GetFlow().GetContinueUrl())
	}

	// Re-submitting CompleteUpdatePassword on completed flow -> 404 FLOW_NOT_FOUND.
	if _, err := authnSvc.CompleteUpdatePassword(ctx, &authnpb.CompleteUpdatePasswordRequest{
		FlowId:      flowID,
		NewPassword: "third horse battery staple",
	}); !authnpb.IsAuthnErrorReasonFlowNotFound(err) {
		t.Fatalf("CompleteUpdatePassword(completed) = %v, want FLOW_NOT_FOUND", err)
	}

	// CancelAuthFlow on active OIDC flow returns RFC 6749/9207 redirect URI.
	flowRepo.oidcReqs["oidc-req-cancel"] = &stubOIDCReq{
		clientID:    "admin",
		redirectURI: "https://admin.example/callback",
		state:       "st-cancel",
	}
	manager.Remove(ctx, iamauthn.LoginReferenceKey)
	cancelTarget, err := authnSvc.CreateAuthFlow(ctx, &authnpb.CreateAuthFlowRequest{OidcRequestId: "oidc-req-cancel"})
	if err != nil {
		t.Fatal(err)
	}
	cancelResp, err := authnSvc.CancelAuthFlow(ctx, &authnpb.CancelAuthFlowRequest{FlowId: cancelTarget.GetFlow().GetFlowId()})
	if err != nil {
		t.Fatalf("CancelAuthFlow() error = %v", err)
	}
	parsedCancel, err := url.Parse(cancelResp.GetRedirectUri())
	if err != nil || parsedCancel.Query().Get("error") != "access_denied" || parsedCancel.Query().Get("state") != "st-cancel" || parsedCancel.Query().Get("iss") != "https://iam.example" {
		t.Fatalf("CancelAuthFlow() redirect_uri = %q", cancelResp.GetRedirectUri())
	}
	manager.Put(ctx, iamauthn.LoginReferenceKey, "new-login-id")
	// Authenticated DeleteAccount verifies password, soft-deletes, and destroys SCS session.
	sessionAuthn, err := iamauthn.NewSessionAuthenticator(sessUC, manager)
	if err != nil {
		t.Fatal(err)
	}
	authnMW := sessionauthn.Server(sessionAuthn, security.WithRulesFuncs(accountpb.AuthnRules))
	var capturedAuthedCtx context.Context
	deleteCtx := transport.NewServerContext(ctx, stubServerTransport{op: accountpb.OperationAccountServiceDeleteAccount})
	if _, err := authnMW(func(authedCtx context.Context, req any) (any, error) {
		capturedAuthedCtx = authedCtx
		return accSvc.DeleteAccount(authedCtx, req.(*accountpb.DeleteAccountRequest))
	})(deleteCtx, &accountpb.DeleteAccountRequest{CurrentPassword: nextPass}); err != nil {
		t.Fatalf("DeleteAccount() error = %v", err)
	}
	if got := manager.GetString(ctx, iamauthn.LoginReferenceKey); got != "" {
		t.Fatalf("DeleteAccount did not destroy SCS LoginReferenceKey: %q", got)
	}
	if _, err := authnMW(func(authedCtx context.Context, req any) (any, error) {
		return accSvc.GetProfile(authedCtx, req.(*accountpb.GetProfileRequest))
	})(transport.NewServerContext(ctx, stubServerTransport{op: accountpb.OperationAccountServiceGetProfile}), &accountpb.GetProfileRequest{}); !securityerrorspb.IsSecurityErrorReasonUnauthenticated(err) {
		t.Fatalf("GetProfile after session destroy = %v, want SecurityErrorReasonUnauthenticated", err)
	}

	// Concurrent in-flight authenticated call on deleted user maps ErrNotFound -> 401 UNAUTHENTICATED.
	if _, err := accSvc.UpdateProfile(capturedAuthedCtx, &accountpb.UpdateProfileRequest{Profile: &userpb.UserProfile{}}); !accountpb.IsAccountErrorReasonUnauthenticated(err) {
		t.Fatalf("UpdateProfile on deleted user = %v, want UNAUTHENTICATED", err)
	}
}

func TestUserServiceLifecycleAndErrorMappings(t *testing.T) {
	users := new(stubUserRepo)
	creds := new(stubCredentialRepo)
	sessRepo := &stubSessionRepo{}
	app := &corepb.App{ExternalUrl: "https://iam.example"}
	mailer := &stubMailer{err: errors.New("smtp unavailable")}
	accUC, err := biz.NewAccountUsecase(users, creds, stubVerifyRepo{}, stubResetRepo{}, new(stubConsentRepo), stubCAP{}, mailer, app)
	if err != nil {
		t.Fatal(err)
	}
	userUC, err := biz.NewUserUsecase(accUC, users, creds, sessRepo)
	if err != nil {
		t.Fatal(err)
	}
	svc, err := NewUserService(userUC)
	if err != nil {
		t.Fatal(err)
	}

	email := "managed@example.com"
	created, err := svc.CreateUser(t.Context(), &userpb.CreateUserRequest{
		User:     &userpb.User{Email: &email},
		Password: "correct horse battery staple",
	})
	if err != nil {
		t.Fatalf("CreateUser() error = %v", err)
	}
	if len(created.GetRequiredActions()) != 1 || created.GetRequiredActions()[0] != userpb.UserRequiredAction_USER_REQUIRED_ACTION_UPDATE_PASSWORD {
		t.Fatalf("CreateUser default required_actions = %v", created.GetRequiredActions())
	}

	if _, err := svc.SendUserVerificationEmail(t.Context(), &userpb.SendUserVerificationEmailRequest{Name: created.GetName()}); !userpb.IsUserErrorReasonVerificationEmailFailed(err) {
		t.Fatalf("SendUserVerificationEmail(smtp err) = %v, want VERIFICATION_EMAIL_FAILED", err)
	}

	if _, err := svc.UndeleteUser(t.Context(), &userpb.UndeleteUserRequest{Name: created.GetName()}); !userpb.IsUserErrorReasonAlreadyExists(err) || strings.Contains(err.Error(), "email") {
		t.Fatalf("UndeleteUser(live) = %v, want ALREADY_EXISTS distinct from email collision", err)
	}
	if _, err := svc.ExpungeUser(t.Context(), &userpb.ExpungeUserRequest{Name: created.GetName()}); !userpb.IsUserErrorReasonInvalidState(err) {
		t.Fatalf("ExpungeUser(live) = %v, want INVALID_STATE", err)
	}

	deleted, err := svc.DeleteUser(t.Context(), &userpb.DeleteUserRequest{Name: created.GetName()})
	if err != nil || deleted.GetDeleteTime() == nil {
		t.Fatalf("DeleteUser() = %#v, err=%v", deleted, err)
	}
	if _, err := svc.DeleteUser(t.Context(), &userpb.DeleteUserRequest{Name: created.GetName()}); !userpb.IsUserErrorReasonNotFound(err) {
		t.Fatalf("second DeleteUser = %v, want NOT_FOUND", err)
	}

	// ListUsers forwards show_deleted and preserves CRUD_ERROR_REASON_INVALID_PAGE_TOKEN.
	if resp, err := svc.ListUsers(t.Context(), &userpb.ListUsersRequest{ShowDeleted: true}); err != nil || !users.lastShow || len(resp.GetUsers()) != 1 {
		t.Fatalf("ListUsers(show_deleted=true) resp=%#v err=%v", resp, err)
	}
	users.listErr = crudpb.ErrorCrudErrorReasonInvalidPageToken("scope mismatch")
	if _, err := svc.ListUsers(t.Context(), &userpb.ListUsersRequest{}); !crudpb.IsCrudErrorReasonInvalidPageToken(err) {
		t.Fatalf("ListUsers(invalid page token) = %v, want INVALID_PAGE_TOKEN", err)
	}
	users.listErr = nil

	if _, err := svc.UndeleteUser(t.Context(), &userpb.UndeleteUserRequest{Name: created.GetName()}); err != nil {
		t.Fatalf("UndeleteUser() error = %v", err)
	}
	if _, err := svc.DeleteUser(t.Context(), &userpb.DeleteUserRequest{Name: created.GetName()}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ExpungeUser(t.Context(), &userpb.ExpungeUserRequest{Name: created.GetName()}); err != nil {
		t.Fatalf("ExpungeUser() error = %v", err)
	}
}
