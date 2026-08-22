package biz

import (
	"context"
	"errors"
	"testing"
	"time"

	userpb "github.com/Servora-Kit/plateau/api/gen/go/iam/user/v1"
	"github.com/Servora-Kit/plateau/security/password"
	corepb "github.com/Servora-Kit/servora/api/gen/go/servora/core/v1"
	"github.com/Servora-Kit/servora/core/bootstrap"
	corecrud "github.com/Servora-Kit/servora/core/crud"
	"google.golang.org/protobuf/types/known/fieldmaskpb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type fakeCAP struct {
	valid bool
	calls int
}

func (captcha *fakeCAP) ValidateToken(context.Context, string) (bool, error) {
	captcha.calls++
	return captcha.valid, nil
}

type fakeMailSender struct {
	calls      int
	to         string
	link       string
	resetCalls int
	resetTo    string
	resetLink  string
}

func (sender *fakeMailSender) SendVerification(_ context.Context, to, link string, _ int) error {
	sender.calls++
	sender.to, sender.link = to, link
	return nil
}

func (sender *fakeMailSender) SendPasswordReset(_ context.Context, to, link string, _ int) error {
	sender.resetCalls++
	sender.resetTo, sender.resetLink = to, link
	return nil
}

type fakeVerificationTokens struct {
	userID    string
	tokenHash string
	expires   time.Time
	consumed  bool
}

func (repo *fakeVerificationTokens) Create(_ context.Context, userID, _ string, tokenHash string, expires time.Time) error {
	repo.userID, repo.tokenHash, repo.expires = userID, tokenHash, expires
	return nil
}

func (repo *fakeVerificationTokens) Consume(_ context.Context, tokenHash string, _ time.Time) (string, error) {
	if repo.consumed || tokenHash != repo.tokenHash {
		return "", ErrMutationMiss
	}
	repo.consumed = true
	return repo.userID, nil
}

type fakePasswordResetTokens struct {
	userID    string
	tokenHash string
	replaced  bool
}

func (repo *fakePasswordResetTokens) Create(_ context.Context, userID, tokenHash string, _ time.Time) error {
	repo.userID, repo.tokenHash = userID, tokenHash
	return nil
}

func (repo *fakePasswordResetTokens) ConsumeAndReplacePassword(_ context.Context, tokenHash, _ string, _ time.Time) (string, error) {
	if repo.replaced || tokenHash != repo.tokenHash {
		return "", ErrMutationMiss
	}
	repo.replaced = true
	return repo.userID, nil
}

type fakeAccountUsers struct {
	user         *userpb.User
	passwordHash string
	canonical    string
	created      int
	activated    bool
}

func (repo *fakeAccountUsers) Create(_ context.Context, user *userpb.User, passwordHash, canonicalEmail string) (*userpb.User, error) {
	repo.created++
	repo.passwordHash, repo.canonical = passwordHash, canonicalEmail
	repo.user = user
	return user, nil
}

func (repo *fakeAccountUsers) FindByEmail(context.Context, string) (*userpb.User, error) {
	if repo.user == nil {
		return nil, ErrNotFound
	}
	return repo.user, nil
}

func (repo *fakeAccountUsers) Get(_ context.Context, _ string) (*userpb.User, error) {
	if repo.user == nil {
		return nil, ErrNotFound
	}
	return repo.user, nil
}

func (repo *fakeAccountUsers) ActivateEmail(_ context.Context, _ string, now time.Time) error {
	repo.activated = true
	repo.user.Status = userpb.UserStatus_USER_STATUS_ACTIVE
	repo.user.EmailVerified = true
	repo.user.EmailVerifiedTime = timestamppb.New(now)
	return nil
}

func (repo *fakeAccountUsers) UpdateProfile(_ context.Context, _ string, _ string, profile *userpb.UserProfile) (*userpb.User, error) {
	repo.user.Profile = profile
	return repo.user, nil
}

func (repo *fakeAccountUsers) UpdateStatus(_ context.Context, _ string, _ string, status userpb.UserStatus, _ time.Time) (*userpb.User, error) {
	repo.user.Status = status
	return repo.user, nil
}

func (repo *fakeAccountUsers) GetUser(context.Context, userpb.UserName) (*userpb.User, error) {
	if repo.user == nil {
		return nil, ErrNotFound
	}
	return repo.user, nil
}

func (repo *fakeAccountUsers) ListUsers(ctx context.Context, query corecrud.ListQuery) (corecrud.ListResult[*userpb.User], error) {
	if repo.user == nil {
		return corecrud.NewListResult[*userpb.User](query, nil, "", nil)
	}
	return corecrud.NewListResult[*userpb.User](query, []*userpb.User{repo.user}, "", nil)
}

func (repo *fakeAccountUsers) UpdateUser(context.Context, userpb.UserName, *userpb.User, *fieldmaskpb.FieldMask, string) (*userpb.User, error) {
	return repo.user, nil
}

func accountTestRuntime() *bootstrap.Runtime {
	return &bootstrap.Runtime{Bootstrap: &corepb.Bootstrap{App: &corepb.App{ExternalUrl: "https://iam.example"}}}
}

func newAccountForTest(t *testing.T, users *fakeAccountUsers, tokens *fakeVerificationTokens, resetTokens *fakePasswordResetTokens, mailer *fakeMailSender) *AccountUsecase {
	t.Helper()
	sessions := newSessionForTest(t, users, new(fakeSessions), new(fakeTokenSessions))
	usecase, err := NewAccountUsecase(users, new(fakeCredentials), tokens, resetTokens, &fakeCAP{valid: true}, mailer, sessions, accountTestRuntime())
	if err != nil {
		t.Fatalf("NewAccountUsecase() error = %v", err)
	}
	return usecase
}

func TestAccountRegisterConsumesCAPBeforeCreatingPendingUser(t *testing.T) {
	captcha := &fakeCAP{valid: true}
	users := new(fakeAccountUsers)
	tokens := new(fakeVerificationTokens)
	mailer := new(fakeMailSender)
	sessions := newSessionForTest(t, users, new(fakeSessions), new(fakeTokenSessions))
	usecase, err := NewAccountUsecase(users, new(fakeCredentials), tokens, new(fakePasswordResetTokens), captcha, mailer, sessions, accountTestRuntime())
	if err != nil {
		t.Fatalf("NewAccountUsecase() error = %v", err)
	}

	user, err := usecase.Register(t.Context(), " Person@Example.com ", "correct horse battery staple", "cap-token", &userpb.UserProfile{})
	if err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	if captcha.calls != 1 || users.created != 1 || user.GetStatus() != userpb.UserStatus_USER_STATUS_PENDING_EMAIL_VERIFICATION || mailer.calls != 1 {
		t.Fatalf("Register() calls=%d created=%d user=%#v mail=%d", captcha.calls, users.created, user, mailer.calls)
	}
	match, _, err := password.Compare("correct horse battery staple", users.passwordHash)
	if err != nil || !match {
		t.Fatalf("stored password hash match=%t error=%v", match, err)
	}
	if tokens.tokenHash == "" || tokens.tokenHash == "cap-token" || mailer.link == "" {
		t.Fatalf("verification token handling invalid: hash=%q link=%q", tokens.tokenHash, mailer.link)
	}
}

func TestAccountRejectsInvalidCAPWithoutUserSideEffects(t *testing.T) {
	captcha := &fakeCAP{}
	users := new(fakeAccountUsers)
	sessions := newSessionForTest(t, users, new(fakeSessions), new(fakeTokenSessions))
	usecase, err := NewAccountUsecase(users, new(fakeCredentials), new(fakeVerificationTokens), new(fakePasswordResetTokens), captcha, new(fakeMailSender), sessions, accountTestRuntime())
	if err != nil {
		t.Fatalf("NewAccountUsecase() error = %v", err)
	}
	if _, err := usecase.Register(t.Context(), "person@example.com", "correct horse battery staple", "invalid", &userpb.UserProfile{}); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("Register() error = %v, want invalid CAP token", err)
	}
	if users.created != 0 {
		t.Fatal("invalid CAP created a User")
	}
}

func TestAccountVerifyEmailConsumesTokenAndActivatesUser(t *testing.T) {
	users := &fakeAccountUsers{user: &userpb.User{
		UserId: "user-1", Email: stringPtr("person@example.com"),
		Status: userpb.UserStatus_USER_STATUS_PENDING_EMAIL_VERIFICATION,
	}}
	tokens := &fakeVerificationTokens{userID: "user-1", tokenHash: HashOpaqueSecret("verification-token")}
	usecase := newAccountForTest(t, users, tokens, new(fakePasswordResetTokens), new(fakeMailSender))
	verified, err := usecase.VerifyEmail(t.Context(), "verification-token")
	if err != nil {
		t.Fatalf("VerifyEmail() error = %v", err)
	}
	if !tokens.consumed || !users.activated || verified.GetStatus() != userpb.UserStatus_USER_STATUS_ACTIVE || !verified.GetEmailVerified() {
		t.Fatalf("VerifyEmail() tokens=%t activated=%t user=%#v", tokens.consumed, users.activated, verified)
	}
}

func TestAccountPasswordResetUsesCAPAndRevokesSessions(t *testing.T) {
	mail := new(fakeMailSender)
	users := &fakeAccountUsers{user: &userpb.User{
		UserId: "user-1", Email: stringPtr("person@example.com"),
		Status: userpb.UserStatus_USER_STATUS_ACTIVE, EmailVerified: true,
	}}
	resetTokens := new(fakePasswordResetTokens)
	sessionRepo, tokenSessions := new(fakeSessions), new(fakeTokenSessions)
	sessions := newSessionForTest(t, users, sessionRepo, tokenSessions)
	usecase, err := NewAccountUsecase(users, new(fakeCredentials), new(fakeVerificationTokens), resetTokens, &fakeCAP{valid: true}, mail, sessions, accountTestRuntime())
	if err != nil {
		t.Fatalf("NewAccountUsecase() error = %v", err)
	}
	if err := usecase.RequestPasswordReset(t.Context(), "person@example.com", "cap-token"); err != nil {
		t.Fatalf("RequestPasswordReset() error = %v", err)
	}
	if mail.resetCalls != 1 || resetTokens.tokenHash == "" || mail.resetLink == "" {
		t.Fatalf("reset request mail=%d token=%q link=%q", mail.resetCalls, resetTokens.tokenHash, mail.resetLink)
	}
	if err := usecase.ConfirmPasswordReset(t.Context(), "reset-token", "new correct horse battery staple"); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("ConfirmPasswordReset() error = %v, want invalid token", err)
	}
	resetTokens.tokenHash = HashOpaqueSecret("reset-token")
	if err := usecase.ConfirmPasswordReset(t.Context(), "reset-token", "new correct horse battery staple"); err != nil {
		t.Fatalf("ConfirmPasswordReset() error = %v", err)
	}
	if !resetTokens.replaced || !sessionRepo.revokedAll || !tokenSessions.revokedForUser {
		t.Fatalf("reset side effects replaced=%t sessions=%t tokens=%t", resetTokens.replaced, sessionRepo.revokedAll, tokenSessions.revokedForUser)
	}
}

func TestAccountChangePasswordPreservesCurrentSessionAndRevokesOthers(t *testing.T) {
	const oldPassword = "correct horse battery staple"
	hash, err := password.Hash(oldPassword)
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	users := &fakeAccountUsers{user: activeUser()}
	credentials := &fakeCredentials{credential: &PasswordCredential{UserID: "user-1", AuthenticatorID: "auth-1", PasswordHash: hash}}
	sessionRepo, tokenSessions := new(fakeSessions), new(fakeTokenSessions)
	sessions := newSessionForTest(t, users, sessionRepo, tokenSessions)
	usecase, err := NewAccountUsecase(users, credentials, new(fakeVerificationTokens), new(fakePasswordResetTokens), &fakeCAP{valid: true}, new(fakeMailSender), sessions, accountTestRuntime())
	if err != nil {
		t.Fatalf("NewAccountUsecase() error = %v", err)
	}
	if err := usecase.ChangePassword(t.Context(), "user-1", "session-1", oldPassword, "new correct horse battery staple"); err != nil {
		t.Fatalf("ChangePassword() error = %v", err)
	}
	match, _, err := password.Compare("new correct horse battery staple", credentials.credential.PasswordHash)
	if err != nil || !match || !credentials.replaced || !sessionRepo.revokedOthers || sessionRepo.revoked || !tokenSessions.revokedForUser {
		t.Fatalf("password match=%t error=%v replaced=%t others=%t current=%t tokens=%t", match, err, credentials.replaced, sessionRepo.revokedOthers, sessionRepo.revoked, tokenSessions.revokedForUser)
	}
}

func TestUserDisableRevokesLoginAndOAuthSessions(t *testing.T) {
	users := &fakeAccountUsers{user: &userpb.User{
		Name: "users/user-1", UserId: "user-1", Email: stringPtr("person@example.com"),
		Status: userpb.UserStatus_USER_STATUS_ACTIVE, EmailVerified: true, Etag: "etag-1",
	}}
	sessionRepo, tokenSessions := new(fakeSessions), new(fakeTokenSessions)
	sessions := newSessionForTest(t, users, sessionRepo, tokenSessions)
	account := newAccountForTest(t, users, new(fakeVerificationTokens), new(fakePasswordResetTokens), new(fakeMailSender))
	usecase, err := NewUserUsecase(account, users, sessions, nil)
	if err != nil {
		t.Fatalf("NewUserUsecase() error = %v", err)
	}

	disabled, err := usecase.DisableUser(t.Context(), userpb.NewUserName("user-1"), "etag-1")
	if err != nil {
		t.Fatalf("DisableUser() error = %v", err)
	}
	if disabled.GetStatus() != userpb.UserStatus_USER_STATUS_DISABLED || !sessionRepo.revokedAll || !tokenSessions.revokedForUser {
		t.Fatalf("disabled=%s login sessions=%t OAuth sessions=%t", disabled.GetStatus(), sessionRepo.revokedAll, tokenSessions.revokedForUser)
	}

	// Repeating disable remains safe and retries revocation rather than restoring any session.
	if _, err := usecase.DisableUser(t.Context(), userpb.NewUserName("user-1"), ""); err != nil {
		t.Fatalf("repeated DisableUser() error = %v", err)
	}
}

func stringPtr(value string) *string { return &value }
