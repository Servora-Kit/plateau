package service

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
	accountpb "github.com/Servora-Kit/plateau/api/gen/go/iam/account/v1"
	authnpb "github.com/Servora-Kit/plateau/api/gen/go/iam/authn/v1"
	userpb "github.com/Servora-Kit/plateau/api/gen/go/iam/user/v1"
	securityerrorspb "github.com/Servora-Kit/plateau/api/gen/go/plateau/security/errors/v1"
	iamauthn "github.com/Servora-Kit/plateau/app/iam/service/internal/authn"
	"github.com/Servora-Kit/plateau/app/iam/service/internal/biz"
	authnsecurity "github.com/Servora-Kit/plateau/security/authn"
	sessionauthn "github.com/Servora-Kit/plateau/security/authn/session"
	"github.com/Servora-Kit/plateau/security/password"
	sessions "github.com/Servora-Kit/plateau/security/session"
	corepb "github.com/Servora-Kit/servora/api/gen/go/servora/core/v1"
	crudpb "github.com/Servora-Kit/servora/api/gen/go/servora/crud/v1"
	corecrud "github.com/Servora-Kit/servora/core/crud"
	"github.com/alexedwards/scs/v2"
	"github.com/alexedwards/scs/v2/memstore"
	"github.com/go-kratos/kratos/v3/transport"
	"google.golang.org/protobuf/types/known/fieldmaskpb"
	"google.golang.org/protobuf/types/known/timestamppb"
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
func (r *stubCredentialRepo) CompleteInitialPassword(_ context.Context, expected biz.PasswordCredential, _, newHash string, now time.Time) (*biz.LoginSession, error) {
	if r.cred != nil {
		r.cred.PasswordHash = newHash
	}
	return &biz.LoginSession{ID: "new-login-id", UserID: expected.UserID, AuthTime: now}, nil
}

type stubSessionRepo struct {
	users   *stubUserRepo
	session *biz.LoginSession
}

func (r *stubSessionRepo) Create(_ context.Context, expected biz.PasswordCredential, _ string, now time.Time) (*userpb.User, *biz.LoginSession, error) {
	u := r.users.user
	if len(u.GetRequiredActions()) > 0 {
		return u, nil, nil
	}
	r.session = &biz.LoginSession{ID: "login-1", UserID: expected.UserID, AuthTime: now}
	return u, r.session, nil
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

func (m stubMailer) SendVerification(context.Context, string, string, int) error { return m.err }
func (m stubMailer) SendPasswordReset(context.Context, string, string, int) error { return m.err }

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
	creds := &stubCredentialRepo{cred: &biz.PasswordCredential{
		UserID: userID, AuthenticatorID: "auth-1", PasswordAuthenticatorID: "pa-1", PasswordHash: hash,
	}}
	sessRepo := &stubSessionRepo{users: users}
	sessUC, err := biz.NewSessionUsecase(users, sessRepo)
	if err != nil {
		t.Fatal(err)
	}
	authnUC, err := biz.NewAuthenticationUsecase(users, creds, sessUC)
	if err != nil {
		t.Fatal(err)
	}
	app := &corepb.App{ExternalUrl: "https://iam.example"}
	accUC, err := biz.NewAccountUsecase(users, creds, stubVerifyRepo{}, stubResetRepo{}, stubCAP{}, stubMailer{}, app)
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

	var ctx context.Context
	sessions.LoadAndSave(manager)(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		ctx = r.Context()
	})).ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/v1/iam/authn/login", nil))
	manager.Put(ctx, iamauthn.LoginReferenceKey, "stale-login-id")
	loginResp, err := authnSvc.Login(ctx, &authnpb.LoginRequest{Email: email, Password: initPass})
	if err != nil {
		t.Fatalf("Login(required_actions) error = %v", err)
	}
	if got := manager.GetString(ctx, iamauthn.LoginReferenceKey); got != "" {
		t.Fatalf("Login(required_actions) retained LoginReferenceKey = %q, want empty", got)
	}
	if len(loginResp.GetUser().GetRequiredActions()) != 1 {
		t.Fatalf("Login response required_actions = %v", loginResp.GetUser().GetRequiredActions())
	}

	// ChangeInitialPassword establishes a normal login reference in SCS.
	if _, err := accSvc.ChangeInitialPassword(ctx, &accountpb.ChangeInitialPasswordRequest{
		Email:           email,
		CurrentPassword: "wrong-password",
		NewPassword:     nextPass,
	}); !accountpb.IsAccountErrorReasonUnauthenticated(err) {
		t.Fatalf("ChangeInitialPassword(wrong password) = %v, want UNAUTHENTICATED", err)
	}
	if _, err := accSvc.ChangeInitialPassword(ctx, &accountpb.ChangeInitialPasswordRequest{
		Email:           email,
		CurrentPassword: initPass,
		NewPassword:     nextPass,
	}); err != nil {
		t.Fatalf("ChangeInitialPassword() error = %v", err)
	}
	users.user.RequiredActions = nil
	sessRepo.session = &biz.LoginSession{ID: "new-login-id", UserID: userID, AuthTime: time.Now()}
	if got := manager.GetString(ctx, iamauthn.LoginReferenceKey); got != "new-login-id" {
		t.Fatalf("ChangeInitialPassword LoginReferenceKey = %q, want new-login-id", got)
	}

	// Calling ChangeInitialPassword again when UPDATE_PASSWORD is already cleared -> 409 INVALID_STATE.
	if _, err := accSvc.ChangeInitialPassword(ctx, &accountpb.ChangeInitialPasswordRequest{
		Email:           email,
		CurrentPassword: nextPass,
		NewPassword:     "third horse battery staple",
	}); !accountpb.IsAccountErrorReasonInvalidState(err) {
		t.Fatalf("ChangeInitialPassword(no action) = %v, want INVALID_STATE", err)
	}

	// Authenticated DeleteAccount verifies password, soft-deletes, and destroys SCS session.
	sessionAuthn, err := iamauthn.NewSessionAuthenticator(sessUC, manager)
	if err != nil {
		t.Fatal(err)
	}
	authnMW := sessionauthn.Server(sessionAuthn, authnsecurity.WithRulesFuncs(accountpb.AuthnRules))
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
	sessRepo := &stubSessionRepo{users: users}
	app := &corepb.App{ExternalUrl: "https://iam.example"}
	mailer := &stubMailer{err: errors.New("smtp unavailable")}
	accUC, err := biz.NewAccountUsecase(users, creds, stubVerifyRepo{}, stubResetRepo{}, stubCAP{}, mailer, app)
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

