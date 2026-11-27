package biz

import (
	"context"
	"crypto/subtle"
	"errors"
	"net/url"
	"slices"
	"testing"
	"time"

	authnpb "github.com/Servora-Kit/plateau/api/gen/go/plateau/iam/authn/v1"
	iamconfpb "github.com/Servora-Kit/plateau/api/gen/go/plateau/iam/conf/v1"
	oidcconfpb "github.com/Servora-Kit/plateau/api/gen/go/plateau/iam/oidc/conf/v1"
	userpb "github.com/Servora-Kit/plateau/api/gen/go/plateau/iam/user/v1"
	"github.com/Servora-Kit/plateau/security/password"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type fakeCredentials struct {
	credential    *PasswordCredential
	replaced      bool
	keepLoginID   string
	adminSetCalls int
}

func (repo *fakeCredentials) FindActivePassword(context.Context, string) (*PasswordCredential, error) {
	if repo.credential == nil {
		return nil, ErrNotFound
	}
	copyCred := *repo.credential
	return &copyCred, nil
}

func (repo *fakeCredentials) ReplacePassword(_ context.Context, _ string, _ string, _ string, passwordHash, keepID string, now time.Time) error {
	repo.credential.PasswordHash = passwordHash
	repo.credential.ChangedTime = NormalizeTimestamp(now)
	repo.replaced = true
	repo.keepLoginID = keepID
	return nil
}

func (repo *fakeCredentials) SetPasswordAndRevokeSessions(_ context.Context, userID, passwordHash string, now time.Time) error {
	repo.adminSetCalls++
	if repo.credential == nil {
		repo.credential = &PasswordCredential{UserID: userID}
	}
	repo.credential.PasswordHash = passwordHash
	repo.credential.ChangedTime = NormalizeTimestamp(now)
	return nil
}

type fakeSessions struct {
	created    *LoginSession
	revoked    bool
	revokedAll int
	findErr    error
}

func (repo *fakeSessions) Find(_ context.Context, id string) (*LoginSession, error) {
	if repo.findErr != nil {
		return nil, repo.findErr
	}
	if repo.created == nil || repo.created.ID != id {
		return nil, ErrNotFound
	}
	value := *repo.created
	value.AMR = slices.Clone(repo.created.AMR)
	if repo.revoked {
		value.RevokedAt = new(time.Now())
	}
	return &value, nil
}

func (repo *fakeSessions) Revoke(context.Context, string, time.Time) error {
	repo.revoked = true
	return nil
}

func (repo *fakeSessions) RevokeAll(context.Context, string, time.Time) error {
	repo.revoked = true
	repo.revokedAll++
	return nil
}

type fakeOIDCEntry struct {
	clientID    string
	expiresTime time.Time
	done        bool
	redirectURI string
	state       string
}

type fakeAuthFlows struct {
	users       *fakeAccountUsers
	credentials *fakeCredentials
	sessions    *fakeSessions
	flows       map[string]*AuthFlowSnapshot
	oidc        map[string]*fakeOIDCEntry
	advanceErr  error
}

func newFakeAuthFlows(users *fakeAccountUsers, credentials *fakeCredentials, sessions *fakeSessions) *fakeAuthFlows {
	return &fakeAuthFlows{
		users:       users,
		credentials: credentials,
		sessions:    sessions,
		flows:       make(map[string]*AuthFlowSnapshot),
		oidc:        make(map[string]*fakeOIDCEntry),
	}
}

func (repo *fakeAuthFlows) FindPendingOIDCRequest(_ context.Context, requestID string, now time.Time) (string, time.Time, error) {
	entry, ok := repo.oidc[requestID]
	if !ok || entry.done || !entry.expiresTime.After(now) {
		return "", time.Time{}, ErrNotFound
	}
	return entry.clientID, entry.expiresTime, nil
}

func (repo *fakeAuthFlows) Create(_ context.Context, seed *authnpb.AuthFlow, ownerHash, oidcRequestID string, now time.Time) (*authnpb.AuthFlow, error) {
	cloned := proto.CloneOf(seed)
	if oidcRequestID != "" {
		clientID, exp, err := repo.FindPendingOIDCRequest(context.Background(), oidcRequestID, now)
		if err != nil {
			return nil, ErrOIDCRequestInvalid
		}
		cloned.Oidc = &authnpb.AuthFlowOidcContext{ClientId: clientID}
		if exp.Before(cloned.GetExpiresTime().AsTime()) {
			cloned.ExpiresTime = timestamppb.New(exp)
		}
	}
	repo.flows[cloned.GetFlowId()] = &AuthFlowSnapshot{
		Flow:          proto.CloneOf(cloned),
		OwnerHash:     ownerHash,
		Status:        AuthFlowStatusActive,
		OIDCRequestID: oidcRequestID,
	}
	return cloned, nil
}

func (repo *fakeAuthFlows) Get(_ context.Context, flowID string) (*AuthFlowSnapshot, error) {
	snap, ok := repo.flows[flowID]
	if !ok {
		return nil, ErrNotFound
	}
	out := *snap
	out.Flow = proto.CloneOf(snap.Flow)
	if snap.PasswordChangedTime != nil {
		t := *snap.PasswordChangedTime
		out.PasswordChangedTime = &t
	}
	return &out, nil
}

func (repo *fakeAuthFlows) Invalidate(_ context.Context, flowID string, _ time.Time) error {
	if snap, ok := repo.flows[flowID]; ok && snap.Status == AuthFlowStatusActive {
		snap.Status = AuthFlowStatusInvalidated
	}
	return nil
}

func (repo *fakeAuthFlows) AdvanceAfterPassword(
	_ context.Context,
	flowID, ownerHash string,
	credential PasswordCredential,
	userEtag string,
	amr []string,
	stepTTL time.Duration,
	now time.Time,
) (*authnpb.AuthFlow, *LoginSession, error) {
	if repo.advanceErr != nil {
		return nil, nil, repo.advanceErr
	}
	snap, ok := repo.flows[flowID]
	if !ok || subtle.ConstantTimeCompare([]byte(ownerHash), []byte(snap.OwnerHash)) != 1 {
		return nil, nil, ErrAuthFlowNotFound
	}
	u := repo.users.user
	if u == nil || u.GetEtag() != userEtag {
		return nil, nil, ErrMutationMiss
	}
	completedAMR := NormalizeAMR(append(slices.Clone(snap.Flow.GetCompletedAmr()), amr...))
	next := EvaluateFlowProgress(completedAMR, DefaultAuthPolicy(), u.GetRequiredActions())
	next.FlowId = flowID
	next.Email = u.GetEmail()
	next.ExpiresTime = proto.CloneOf(snap.Flow.GetExpiresTime())
	if snap.OIDCRequestID != "" {
		if entry, ok := repo.oidc[snap.OIDCRequestID]; ok && !entry.done && entry.expiresTime.After(now) {
			next.Oidc = &authnpb.AuthFlowOidcContext{ClientId: entry.clientID}
		}
	}
	if next.GetState() == authnpb.AuthFlowState_AUTH_FLOW_STATE_REQUIRED_ACTION {
		stepExp := now.Add(stepTTL)
		if snap.Flow.GetExpiresTime().AsTime().Before(stepExp) {
			stepExp = snap.Flow.GetExpiresTime().AsTime()
		}
		next.StepExpiresTime = timestamppb.New(stepExp)
		changed := NormalizeTimestamp(credential.ChangedTime)
		snap.Flow = proto.CloneOf(next)
		snap.UserID = u.GetUserId()
		snap.UserEtag = u.GetEtag()
		snap.AuthenticatorID = credential.AuthenticatorID
		snap.PasswordAuthenticatorID = credential.PasswordAuthenticatorID
		snap.PasswordChangedTime = &changed
		return next, nil, nil
	}
	login := &LoginSession{ID: "session-1", UserID: u.GetUserId(), AMR: completedAMR, AuthTime: now}
	repo.sessions.created = login
	next.ContinueUrl = BuildContinueURL(snap.OIDCRequestID)
	next.User = proto.CloneOf(u)
	snap.Status = AuthFlowStatusCompleted
	snap.UserID = u.GetUserId()
	snap.Flow = proto.CloneOf(next)
	return next, login, nil
}

func (repo *fakeAuthFlows) CompleteUpdatePassword(
	_ context.Context,
	flowID, ownerHash, newPasswordHash string,
	_ time.Duration,
	now time.Time,
) (*authnpb.AuthFlow, *LoginSession, error) {
	snap, ok := repo.flows[flowID]
	if !ok || subtle.ConstantTimeCompare([]byte(ownerHash), []byte(snap.OwnerHash)) != 1 {
		return nil, nil, ErrAuthFlowNotFound
	}
	if snap.Status == AuthFlowStatusCompleted {
		return nil, nil, ErrAuthFlowNotFound
	}
	if snap.Status == AuthFlowStatusInvalidated {
		return nil, nil, ErrAuthFlowInvalidated
	}
	u := repo.users.user
	repo.credentials.credential.PasswordHash = newPasswordHash
	repo.credentials.credential.ChangedTime = NormalizeTimestamp(now)
	u.RequiredActions = nil
	u.Etag = "etag-updated"
	for id, other := range repo.flows {
		if id != flowID && other.UserID == u.GetUserId() && other.Status == AuthFlowStatusActive {
			other.Status = AuthFlowStatusInvalidated
		}
	}
	amr := NormalizeAMR(snap.Flow.GetCompletedAmr())
	login := &LoginSession{ID: "session-after-pwd", UserID: u.GetUserId(), AMR: amr, AuthTime: now}
	repo.sessions.created = login
	oidcReq := snap.OIDCRequestID
	var oidcCtx *authnpb.AuthFlowOidcContext
	if oidcReq != "" {
		if entry, ok := repo.oidc[oidcReq]; ok && !entry.done && entry.expiresTime.After(now) {
			oidcCtx = &authnpb.AuthFlowOidcContext{ClientId: entry.clientID}
		} else {
			oidcReq = ""
		}
	}
	doneFlow := &authnpb.AuthFlow{
		FlowId:       flowID,
		State:        authnpb.AuthFlowState_AUTH_FLOW_STATE_COMPLETED,
		CompletedAmr: amr,
		Email:        u.GetEmail(),
		Oidc:         oidcCtx,
		ExpiresTime:  proto.CloneOf(snap.Flow.GetExpiresTime()),
		ContinueUrl:  BuildContinueURL(oidcReq),
		User:         proto.CloneOf(u),
	}
	snap.Status = AuthFlowStatusCompleted
	snap.Flow = proto.CloneOf(doneFlow)
	return doneFlow, login, nil
}

func (repo *fakeAuthFlows) Reset(_ context.Context, flowID, newFlowID, ownerHash string, now time.Time) (*authnpb.AuthFlow, string, error) {
	snap, ok := repo.flows[flowID]
	if !ok || subtle.ConstantTimeCompare([]byte(ownerHash), []byte(snap.OwnerHash)) != 1 {
		return nil, "", ErrAuthFlowNotFound
	}
	var oidcCtx *authnpb.AuthFlowOidcContext
	if snap.OIDCRequestID != "" {
		entry, ok := repo.oidc[snap.OIDCRequestID]
		if !ok || entry.done || !entry.expiresTime.After(now) {
			return nil, snap.UserID, ErrOIDCRequestInvalid
		}
		oidcCtx = &authnpb.AuthFlowOidcContext{ClientId: entry.clientID}
	}
	snap.Status = AuthFlowStatusReset
	fresh := EvaluateFlowProgress(nil, DefaultAuthPolicy(), nil)
	fresh.FlowId = newFlowID
	fresh.Oidc = oidcCtx
	fresh.ExpiresTime = proto.CloneOf(snap.Flow.GetExpiresTime())
	repo.flows[newFlowID] = &AuthFlowSnapshot{
		Flow:          proto.CloneOf(fresh),
		OwnerHash:     ownerHash,
		Status:        AuthFlowStatusActive,
		OIDCRequestID: snap.OIDCRequestID,
	}
	return fresh, snap.UserID, nil
}

func (repo *fakeAuthFlows) Cancel(_ context.Context, flowID, ownerHash string, now time.Time) (string, string, string, error) {
	snap, ok := repo.flows[flowID]
	if !ok || subtle.ConstantTimeCompare([]byte(ownerHash), []byte(snap.OwnerHash)) != 1 {
		return "", "", "", ErrAuthFlowNotFound
	}
	snap.Status = AuthFlowStatusCancelled
	var redirectURI, state string
	if snap.OIDCRequestID != "" {
		if entry, ok := repo.oidc[snap.OIDCRequestID]; ok && !entry.done && entry.expiresTime.After(now) {
			delete(repo.oidc, snap.OIDCRequestID)
			redirectURI = entry.redirectURI
			state = entry.state
		}
	}
	return redirectURI, state, snap.UserID, nil
}

func activeUser() *userpb.User {
	return &userpb.User{
		UserId: "user-1", Email: new("person@example.com"),
		Status: userpb.UserStatus_USER_STATUS_ACTIVE, EmailVerified: true, Etag: "etag-1",
	}
}

func newSessionForTest(t *testing.T, users *fakeAccountUsers, sessions *fakeSessions) *SessionUsecase {
	t.Helper()
	usecase, err := NewSessionUsecase(users, sessions)
	if err != nil {
		t.Fatalf("NewSessionUsecase() error = %v", err)
	}
	return usecase
}

func newAuthenticationForTest(
	t *testing.T,
	users *fakeAccountUsers,
	credentials *fakeCredentials,
	sessions *fakeSessions,
	flows *fakeAuthFlows,
) (*AuthenticationUsecase, *SessionUsecase) {
	t.Helper()
	sessionUsecase := newSessionForTest(t, users, sessions)
	if flows == nil {
		flows = newFakeAuthFlows(users, credentials, sessions)
	}
	usecase, err := NewAuthenticationUsecase(&iamconfpb.IAM{}, &oidcconfpb.OIDC{Issuer: new("https://iam.example.com/")}, users, credentials, sessionUsecase, flows)
	if err != nil {
		t.Fatalf("NewAuthenticationUsecase() error = %v", err)
	}
	return usecase, sessionUsecase
}

func TestAuthenticationConfigValidation(t *testing.T) {
	users := &fakeAccountUsers{user: activeUser()}
	creds := new(fakeCredentials)
	sessions := new(fakeSessions)
	sessionUC := newSessionForTest(t, users, sessions)
	flows := newFakeAuthFlows(users, creds, sessions)

	// Default nil AuthFlow applies 30m/10m without mutating caller's config pointer.
	origCfg := &iamconfpb.IAM{BootstrapUserEmail: new("admin@example.com")}
	uc, err := NewAuthenticationUsecase(origCfg, &oidcconfpb.OIDC{Issuer: new("https://iam.example.com")}, users, creds, sessionUC, flows)
	if err != nil {
		t.Fatalf("default config error = %v", err)
	}
	if origCfg.AuthFlow != nil {
		t.Fatal("NewAuthenticationUsecase mutated caller's IAM config")
	}
	if uc.ttl != 30*time.Minute || uc.stepTTL != 10*time.Minute {
		t.Fatalf("defaults = (%s, %s), want (30m, 10m)", uc.ttl, uc.stepTTL)
	}

	for _, tc := range []struct {
		name    string
		ttl     time.Duration
		stepTTL time.Duration
	}{
		{name: "zero ttl", ttl: 0, stepTTL: time.Minute},
		{name: "negative step_ttl", ttl: 10 * time.Minute, stepTTL: -time.Second},
		{name: "step_ttl > ttl", ttl: 5 * time.Minute, stepTTL: 10 * time.Minute},
	} {
		_, err := NewAuthenticationUsecase(&iamconfpb.IAM{
			AuthFlow: &iamconfpb.AuthFlow{
				Ttl:     durationpb.New(tc.ttl),
				StepTtl: durationpb.New(tc.stepTTL),
			},
		}, &oidcconfpb.OIDC{Issuer: new("https://iam.example.com")}, users, creds, sessionUC, flows)
		if err == nil {
			t.Fatalf("%s: expected error", tc.name)
		}
	}
	if _, err := NewAuthenticationUsecase(&iamconfpb.IAM{}, &oidcconfpb.OIDC{Issuer: new("http://evil.example.com")}, users, creds, sessionUC, flows); err == nil {
		t.Fatal("expected non-HTTPS non-loopback OIDC issuer to fail")
	}
}

func TestAuthenticationDirectPasswordFlowAndShortCircuit(t *testing.T) {
	plaintext := "correct horse battery staple"
	hash, err := password.Hash(plaintext)
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	baseNow := time.Date(2026, 11, 27, 10, 0, 0, 0, time.UTC)
	u := activeUser()
	users := &fakeAccountUsers{user: u}
	creds := &fakeCredentials{credential: &PasswordCredential{
		UserID: "user-1", AuthenticatorID: "auth-1", PasswordAuthenticatorID: "pa-1",
		PasswordHash: hash, ChangedTime: baseNow,
	}}
	sessions := new(fakeSessions)
	flows := newFakeAuthFlows(users, creds, sessions)
	flows.oidc["req-1"] = &fakeOIDCEntry{
		clientID: "admin", expiresTime: baseNow.Add(15 * time.Minute),
		redirectURI: "http://admin.local/cb", state: "st-1",
	}
	usecase, _ := newAuthenticationForTest(t, users, creds, sessions, flows)
	usecase.now = func() time.Time { return baseNow }

	// Invalid oidc_request_id returns ErrOIDCRequestInvalid without generating owner token.
	if _, ownerTok, err := usecase.CreateAuthFlow(t.Context(), "", "", "missing-req"); !errors.Is(err, ErrOIDCRequestInvalid) || ownerTok != "" {
		t.Fatalf("invalid oidc_request_id = (%q, %v), want empty token & ErrOIDCRequestInvalid", ownerTok, err)
	}

	// Create flow caps expires_time to OIDC request's 15m expiry (< 30m default).
	created, ownerToken, err := usecase.CreateAuthFlow(t.Context(), "", "", "req-1")
	if err != nil || ownerToken == "" || created.GetFlowId() == "" {
		t.Fatalf("CreateAuthFlow() = (%+v, %q, %v)", created, ownerToken, err)
	}
	if !created.GetExpiresTime().AsTime().Equal(baseNow.Add(15 * time.Minute)) {
		t.Fatalf("expires_time = %v, want %v", created.GetExpiresTime().AsTime(), baseNow.Add(15*time.Minute))
	}

	// Wrong browser owner gets ErrAuthFlowNotFound.
	if _, err := usecase.GetAuthFlow(t.Context(), "other-owner", "", created.GetFlowId()); !errors.Is(err, ErrAuthFlowNotFound) {
		t.Fatalf("wrong owner GetAuthFlow err = %v, want ErrAuthFlowNotFound", err)
	}

	// SubmitPassword completes directly with AMR=["pwd"] and OIDC continue_url.
	doneFlow, loginSession, actorID, err := usecase.SubmitPassword(
		t.Context(), ownerToken, created.GetFlowId(), " PERSON@EXAMPLE.COM ", plaintext,
	)
	if err != nil || loginSession == nil || actorID != "user-1" {
		t.Fatalf("SubmitPassword() = (%+v, %+v, %q, %v)", doneFlow, loginSession, actorID, err)
	}
	if doneFlow.GetState() != authnpb.AuthFlowState_AUTH_FLOW_STATE_COMPLETED ||
		doneFlow.GetContinueUrl() != "/authorize/callback?id=req-1" ||
		!slices.Equal(loginSession.AMR, []string{"pwd"}) {
		t.Fatalf("unexpected completed flow/session: flow=%+v session=%+v", doneFlow, loginSession)
	}

	// CreateAuthFlow when already signed in with non-empty AMR short-circuits without DB row or new owner token.
	shortFlow, newTok, err := usecase.CreateAuthFlow(t.Context(), loginSession.ID, "", "req-1")
	if err != nil || newTok != "" || shortFlow.GetFlowId() != "" || shortFlow.GetState() != authnpb.AuthFlowState_AUTH_FLOW_STATE_COMPLETED {
		t.Fatalf("signed-in CreateAuthFlow short-circuit = (%+v, %q, %v)", shortFlow, newTok, err)
	}

	// Empty AMR session does NOT short-circuit on OIDC flow, preventing redirect loops.
	sessions.created.AMR = nil
	nonShort, nextTok, err := usecase.CreateAuthFlow(t.Context(), loginSession.ID, "", "req-1")
	if err != nil || nextTok == "" || nonShort.GetState() != authnpb.AuthFlowState_AUTH_FLOW_STATE_AUTHENTICATE {
		t.Fatalf("empty AMR OIDC CreateAuthFlow = (%+v, %q, %v), want AUTHENTICATE", nonShort, nextTok, err)
	}

	// CancelAuthFlow destroys pending OIDC request and returns RFC 6749 + RFC 9207 access_denied redirect URL.
	cancelRedirect, cancelActor, err := usecase.CancelAuthFlow(t.Context(), nextTok, nonShort.GetFlowId())
	if err != nil || cancelActor != "" {
		t.Fatalf("CancelAuthFlow() = (%q, %q, %v)", cancelRedirect, cancelActor, err)
	}
	parsedCancel, err := url.Parse(cancelRedirect)
	if err != nil || parsedCancel.Query().Get("error") != "access_denied" || parsedCancel.Query().Get("state") != "st-1" || parsedCancel.Query().Get("iss") != "https://iam.example.com" {
		t.Fatalf("CancelAuthFlow redirect = %q", cancelRedirect)
	}
}

func TestAuthenticationUpdatePasswordClocksRetryAndInvalidation(t *testing.T) {
	initialPlain := "correct horse battery staple"
	nextPlain := "updated horse battery staple"
	hash, err := password.Hash(initialPlain)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 11, 27, 12, 0, 0, 123456789, time.UTC)
	u := activeUser()
	u.RequiredActions = []userpb.UserRequiredAction{userpb.UserRequiredAction_USER_REQUIRED_ACTION_UPDATE_PASSWORD}
	users := &fakeAccountUsers{user: u}
	creds := &fakeCredentials{credential: &PasswordCredential{
		UserID: "user-1", AuthenticatorID: "auth-1", PasswordAuthenticatorID: "pa-1",
		PasswordHash: hash, ChangedTime: now, // sub-microsecond nanoseconds normalized on compare
	}}
	sessions := new(fakeSessions)
	flows := newFakeAuthFlows(users, creds, sessions)
	usecase, _ := newAuthenticationForTest(t, users, creds, sessions, flows)
	usecase.now = func() time.Time { return now }

	flow, ownerToken, err := usecase.CreateAuthFlow(t.Context(), "", "", "")
	if err != nil {
		t.Fatal(err)
	}

	// Wrong password returns ErrInvalidCredentials and keeps flow active for retry.
	if _, _, actor, err := usecase.SubmitPassword(
		t.Context(), ownerToken, flow.GetFlowId(), "person@example.com", "wrong-password",
	); !errors.Is(err, ErrInvalidCredentials) || actor != "" {
		t.Fatalf("wrong password = (%q, %v), want empty actor & ErrInvalidCredentials", actor, err)
	}

	// Correct initial password advances to REQUIRED_ACTION without creating LoginSession.
	reqFlow, login, actor, err := usecase.SubmitPassword(
		t.Context(), ownerToken, flow.GetFlowId(), "person@example.com", initialPlain,
	)
	if err != nil || login != nil || sessions.created != nil || actor != "user-1" {
		t.Fatalf("SubmitPassword(UPDATE_PASSWORD) = (%+v, %+v, %q, %v)", reqFlow, login, actor, err)
	}
	if reqFlow.GetState() != authnpb.AuthFlowState_AUTH_FLOW_STATE_REQUIRED_ACTION ||
		reqFlow.GetEmail() != "person@example.com" ||
		reqFlow.GetNextRequiredAction() != userpb.UserRequiredAction_USER_REQUIRED_ACTION_UPDATE_PASSWORD {
		t.Fatalf("unexpected REQUIRED_ACTION flow: %+v", reqFlow)
	}
	origStepExp := reqFlow.GetStepExpiresTime().AsTime()

	// Short password & unchanged password return 400 ErrInvalidPassword, enrich actor, and preserve retryable flow without extending step_expires_time.
	now = now.Add(2 * time.Minute)
	for _, bad := range []string{"short", initialPlain} {
		if _, _, gotActor, err := usecase.CompleteUpdatePassword(
			t.Context(), ownerToken, flow.GetFlowId(), bad,
		); !errors.Is(err, ErrInvalidPassword) || gotActor != "user-1" {
			t.Fatalf("bad newPassword %q = (actor=%q, err=%v)", bad, gotActor, err)
		}
	}
	refreshed, err := usecase.GetAuthFlow(t.Context(), ownerToken, "", flow.GetFlowId())
	if err != nil || !refreshed.GetStepExpiresTime().AsTime().Equal(origStepExp) {
		t.Fatalf("GetAuthFlow after 400 = (%+v, %v), step_expires_time must not extend", refreshed, err)
	}

	// Step TTL expiration returns ErrAuthFlowExpired, while ResetAuthFlow still succeeds within total 30m TTL.
	stepExpiredNow := origStepExp.Add(time.Second)
	usecase.now = func() time.Time { return stepExpiredNow }
	if _, _, _, err := usecase.CompleteUpdatePassword(
		t.Context(), ownerToken, flow.GetFlowId(), nextPlain,
	); !errors.Is(err, ErrAuthFlowExpired) {
		t.Fatalf("after step_ttl err = %v, want ErrAuthFlowExpired", err)
	}
	resetFlow, resetActor, err := usecase.ResetAuthFlow(t.Context(), ownerToken, flow.GetFlowId())
	if err != nil || resetActor != "user-1" || resetFlow.GetFlowId() == flow.GetFlowId() || resetFlow.GetState() != authnpb.AuthFlowState_AUTH_FLOW_STATE_AUTHENTICATE {
		t.Fatalf("ResetAuthFlow() = (%+v, %q, %v)", resetFlow, resetActor, err)
	}

	// Advance new flow to REQUIRED_ACTION, then simulate concurrent admin password change -> 409 ErrAuthFlowInvalidated.
	if _, _, _, err := usecase.SubmitPassword(
		t.Context(), ownerToken, resetFlow.GetFlowId(), "person@example.com", initialPlain,
	); err != nil {
		t.Fatal(err)
	}
	creds.credential.ChangedTime = stepExpiredNow.Add(time.Minute)
	if _, _, gotActor, err := usecase.CompleteUpdatePassword(
		t.Context(), ownerToken, resetFlow.GetFlowId(), nextPlain,
	); !errors.Is(err, ErrAuthFlowInvalidated) || gotActor != "user-1" {
		t.Fatalf("drifted credential = (actor=%q, err=%v), want ErrAuthFlowInvalidated", gotActor, err)
	}

	// Fresh flow completes update password cleanly.
	flow3, _, err := usecase.CreateAuthFlow(t.Context(), "", ownerToken, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := usecase.SubmitPassword(
		t.Context(), ownerToken, flow3.GetFlowId(), "person@example.com", initialPlain,
	); err != nil {
		t.Fatal(err)
	}
	completed, doneLogin, doneActor, err := usecase.CompleteUpdatePassword(
		t.Context(), ownerToken, flow3.GetFlowId(), nextPlain,
	)
	if err != nil || doneLogin == nil || doneActor != "user-1" || completed.GetContinueUrl() != "/account/" {
		t.Fatalf("CompleteUpdatePassword() = (%+v, %+v, %q, %v)", completed, doneLogin, doneActor, err)
	}
}

func TestAuthenticationRejectsInvalidCredentialsAndSessionResolution(t *testing.T) {
	hash, err := password.Hash("correct horse battery staple")
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name string
		user *userpb.User
		pass string
	}{
		{name: "wrong password", user: activeUser(), pass: "wrong password"},
		{name: "pending email", user: &userpb.User{UserId: "user-1", Email: new("person@example.com"), Status: userpb.UserStatus_USER_STATUS_PENDING_EMAIL_VERIFICATION}, pass: "correct horse battery staple"},
		{name: "disabled user", user: &userpb.User{UserId: "user-1", Email: new("person@example.com"), Status: userpb.UserStatus_USER_STATUS_DISABLED, EmailVerified: true}, pass: "correct horse battery staple"},
	} {
		t.Run(test.name, func(t *testing.T) {
			sessions := new(fakeSessions)
			users := &fakeAccountUsers{user: test.user}
			creds := &fakeCredentials{credential: &PasswordCredential{UserID: "user-1", PasswordHash: hash}}
			usecase, _ := newAuthenticationForTest(t, users, creds, sessions, nil)
			created, ownerTok, err := usecase.CreateAuthFlow(t.Context(), "", "", "")
			if err != nil {
				t.Fatal(err)
			}
			if _, _, _, err := usecase.SubmitPassword(
				t.Context(), ownerTok, created.GetFlowId(), "person@example.com", test.pass,
			); !errors.Is(err, ErrInvalidCredentials) {
				t.Fatalf("SubmitPassword() error = %v, want ErrInvalidCredentials", err)
			}
			if sessions.created != nil {
				t.Fatal("invalid login created a session")
			}
		})
	}

	u := activeUser()
	users := &fakeAccountUsers{user: u}
	sessions := &fakeSessions{created: &LoginSession{ID: "s-1", UserID: "user-1", AMR: []string{"pwd"}, AuthTime: time.Now()}}
	sessionUC := newSessionForTest(t, users, sessions)
	if _, resolved, err := sessionUC.Resolve(t.Context(), "s-1"); err != nil || !slices.Equal(resolved.AMR, []string{"pwd"}) {
		t.Fatalf("Resolve() = (%+v, %v)", resolved, err)
	}
	users.user.RequiredActions = []userpb.UserRequiredAction{userpb.UserRequiredAction_USER_REQUIRED_ACTION_UPDATE_PASSWORD}
	if _, _, err := sessionUC.Resolve(t.Context(), "s-1"); !errors.Is(err, ErrRequiredActionPending) {
		t.Fatalf("required action pending = %v, want ErrRequiredActionPending", err)
	}
	users.user.RequiredActions = nil
	if err := sessionUC.Logout(t.Context(), "s-1"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := sessionUC.Resolve(t.Context(), "s-1"); !errors.Is(err, ErrSessionRevoked) {
		t.Fatalf("revoked = %v, want ErrSessionRevoked", err)
	}
}
