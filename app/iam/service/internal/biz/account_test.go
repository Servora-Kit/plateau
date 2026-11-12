package biz

import (
	"context"
	"errors"
	"testing"
	"time"

	userpb "github.com/Servora-Kit/plateau/api/gen/go/iam/user/v1"
	"github.com/Servora-Kit/plateau/security/password"
	corepb "github.com/Servora-Kit/servora/api/gen/go/servora/core/v1"
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
	sendErr    error
}

func (sender *fakeMailSender) SendVerification(_ context.Context, to, link string, _ int) error {
	if sender.sendErr != nil {
		return sender.sendErr
	}
	sender.calls++
	sender.to, sender.link = to, link
	return nil
}

func (sender *fakeMailSender) SendPasswordReset(_ context.Context, to, link string, _ int) error {
	if sender.sendErr != nil {
		return sender.sendErr
	}
	sender.resetCalls++
	sender.resetTo, sender.resetLink = to, link
	return nil
}

type fakeVerificationTokens struct {
	users     *fakeAccountUsers
	userID    string
	tokenHash string
	expires   time.Time
	consumed  bool
	createErr error
}

func (repo *fakeVerificationTokens) Create(_ context.Context, userID, _ string, tokenHash string, expires time.Time) error {
	if repo.createErr != nil {
		return repo.createErr
	}
	repo.userID, repo.tokenHash, repo.expires = userID, tokenHash, expires
	return nil
}

func (repo *fakeVerificationTokens) ConsumeAndActivate(_ context.Context, tokenHash string, now time.Time) (string, error) {
	if repo.consumed || tokenHash != repo.tokenHash {
		return "", ErrInvalidToken
	}
	repo.consumed = true
	if repo.users != nil && repo.users.user != nil {
		repo.users.activated = true
		repo.users.user.Status = userpb.UserStatus_USER_STATUS_ACTIVE
		repo.users.user.EmailVerified = true
		repo.users.user.EmailVerifiedTime = timestamppb.New(now)
	}
	return repo.userID, nil
}

type fakePasswordResetTokens struct {
	userID                 string
	tokenHash              string
	replaced               bool
	credential             PasswordCredential
	userEtag               string
	requiresPasswordChange bool
	consumeErr             error
}

func (repo *fakePasswordResetTokens) Create(_ context.Context, userID, tokenHash string, _ time.Time) error {
	repo.userID, repo.tokenHash = userID, tokenHash
	return nil
}

func (repo *fakePasswordResetTokens) Peek(_ context.Context, tokenHash string, _ time.Time) (*PasswordResetProof, error) {
	if repo.replaced || tokenHash != repo.tokenHash {
		return nil, ErrInvalidToken
	}
	cred := repo.credential
	if cred.UserID == "" {
		cred.UserID = repo.userID
	}
	return &PasswordResetProof{
		TokenID:                "token-1",
		TokenHash:              tokenHash,
		Credential:             cred,
		UserEtag:               repo.userEtag,
		RequiresPasswordChange: repo.requiresPasswordChange,
	}, nil
}

func (repo *fakePasswordResetTokens) ConsumeAndReplacePassword(_ context.Context, proof PasswordResetProof, _ string, _ time.Time) (string, error) {
	if repo.consumeErr != nil {
		return "", repo.consumeErr
	}
	if repo.replaced || proof.TokenHash != repo.tokenHash {
		return "", ErrMutationMiss
	}
	repo.replaced = true
	return repo.userID, nil
}

type fakeAccountUsers struct {
	user         *userpb.User
	anyEmailUser *userpb.User
	passwordHash string
	canonical    string
	actions      []userpb.UserRequiredAction
	created      int
	activated    bool
	softDeleted  *DeleteAccountProof
	expunged     bool
}

func (repo *fakeAccountUsers) Create(_ context.Context, user *userpb.User, passwordHash, canonicalEmail string) (*userpb.User, error) {
	repo.created++
	repo.passwordHash, repo.canonical = passwordHash, canonicalEmail
	repo.actions = append([]userpb.UserRequiredAction(nil), user.GetRequiredActions()...)
	if user.Etag == "" {
		user.Etag = "etag-1"
	}
	repo.user = user
	return user, nil
}

func (repo *fakeAccountUsers) FindByEmail(context.Context, string) (*userpb.User, error) {
	if repo.user == nil || repo.user.DeleteTime != nil {
		return nil, ErrNotFound
	}
	return repo.user, nil
}

func (repo *fakeAccountUsers) FindByEmailIncludingDeleted(context.Context, string) (*userpb.User, error) {
	if repo.anyEmailUser != nil {
		return repo.anyEmailUser, nil
	}
	if repo.user == nil {
		return nil, ErrNotFound
	}
	return repo.user, nil
}

func (repo *fakeAccountUsers) Get(_ context.Context, _ string) (*userpb.User, error) {
	if repo.user == nil || repo.user.DeleteTime != nil {
		return nil, ErrNotFound
	}
	return repo.user, nil
}

func (repo *fakeAccountUsers) UpdateProfile(_ context.Context, _ string, _ string, profile *userpb.UserProfile) (*userpb.User, error) {
	repo.user.Profile = profile
	return repo.user, nil
}

func (repo *fakeAccountUsers) UpdateStatus(_ context.Context, _ string, _ string, status userpb.UserStatus, _ time.Time) (*userpb.User, error) {
	repo.user.Status = status
	return repo.user, nil
}

func (repo *fakeAccountUsers) SoftDelete(_ context.Context, _ string, proof *DeleteAccountProof, now time.Time) (*userpb.User, error) {
	if repo.user == nil || repo.user.DeleteTime != nil {
		return nil, ErrNotFound
	}
	repo.softDeleted = proof
	repo.user.DeleteTime = timestamppb.New(now)
	return repo.user, nil
}

func (repo *fakeAccountUsers) Undelete(_ context.Context, _ string, _ time.Time) (*userpb.User, error) {
	if repo.user == nil {
		return nil, ErrNotFound
	}
	if repo.user.DeleteTime == nil {
		return nil, ErrAlreadyExists
	}
	repo.user.DeleteTime = nil
	return repo.user, nil
}

func (repo *fakeAccountUsers) Expunge(_ context.Context, _ string) error {
	if repo.user == nil {
		return ErrNotFound
	}
	if repo.user.DeleteTime == nil {
		return ErrUserInvalidState
	}
	repo.expunged = true
	repo.user = nil
	return nil
}

func (repo *fakeAccountUsers) GetUser(context.Context, userpb.UserName) (*userpb.User, error) {
	if repo.user == nil {
		return nil, ErrNotFound
	}
	return repo.user, nil
}

func (repo *fakeAccountUsers) ListUsers(_ context.Context, query corecrud.ListQuery, showDeleted bool) (corecrud.ListResult[*userpb.User], error) {
	if repo.user == nil || (!showDeleted && repo.user.DeleteTime != nil) {
		return corecrud.NewListResult[*userpb.User](query, nil, "", nil)
	}
	return corecrud.NewListResult[*userpb.User](query, []*userpb.User{repo.user}, "", nil)
}

func (repo *fakeAccountUsers) UpdateUser(context.Context, userpb.UserName, *userpb.User, *fieldmaskpb.FieldMask, string) (*userpb.User, error) {
	return repo.user, nil
}

func timestamppbFromUnix(sec int64) *timestamppb.Timestamp {
	return timestamppb.New(time.Unix(sec, 0).UTC())
}

func accountTestRuntime() *corepb.App {
	return &corepb.App{ExternalUrl: "https://iam.example"}
}

func newAccountForTest(t *testing.T, users *fakeAccountUsers, tokens *fakeVerificationTokens, resetTokens *fakePasswordResetTokens, mailer *fakeMailSender) *AccountUsecase {
	t.Helper()
	usecase, err := NewAccountUsecase(users, new(fakeCredentials), tokens, resetTokens, &fakeCAP{valid: true}, mailer, accountTestRuntime())
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
	usecase, err := NewAccountUsecase(users, new(fakeCredentials), tokens, new(fakePasswordResetTokens), captcha, mailer, accountTestRuntime())
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
	usecase, err := NewAccountUsecase(users, new(fakeCredentials), new(fakeVerificationTokens), new(fakePasswordResetTokens), captcha, new(fakeMailSender), accountTestRuntime())
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
		UserId: "user-1", Email: new("person@example.com"),
		Status: userpb.UserStatus_USER_STATUS_PENDING_EMAIL_VERIFICATION,
	}}
	tokens := &fakeVerificationTokens{users: users, userID: "user-1", tokenHash: HashOpaqueSecret("verification-token")}
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
		UserId: "user-1", Email: new("person@example.com"),
		Status: userpb.UserStatus_USER_STATUS_ACTIVE, EmailVerified: true,
	}}
	resetTokens := new(fakePasswordResetTokens)
	usecase, err := NewAccountUsecase(users, new(fakeCredentials), new(fakeVerificationTokens), resetTokens, &fakeCAP{valid: true}, mail, accountTestRuntime())
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
	if !resetTokens.replaced {
		t.Fatal("atomic reset operation was not invoked")
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
	usecase, err := NewAccountUsecase(users, credentials, new(fakeVerificationTokens), new(fakePasswordResetTokens), &fakeCAP{valid: true}, new(fakeMailSender), accountTestRuntime())
	if err != nil {
		t.Fatalf("NewAccountUsecase() error = %v", err)
	}
	if err := usecase.ChangePassword(t.Context(), "user-1", "session-1", oldPassword, "new correct horse battery staple"); err != nil {
		t.Fatalf("ChangePassword() error = %v", err)
	}
	match, _, err := password.Compare("new correct horse battery staple", credentials.credential.PasswordHash)
	if err != nil || !match || !credentials.replaced || credentials.keepLoginID != "session-1" {
		t.Fatalf("password operation: %v", err)
	}
}

func TestUserDisableRevokesLoginAndOAuthSessions(t *testing.T) {
	users := &fakeAccountUsers{user: &userpb.User{
		Name: "users/user-1", UserId: "user-1", Email: new("person@example.com"),
		Status: userpb.UserStatus_USER_STATUS_ACTIVE, EmailVerified: true, Etag: "etag-1",
	}}
	account := newAccountForTest(t, users, new(fakeVerificationTokens), new(fakePasswordResetTokens), new(fakeMailSender))
	usecase, err := NewUserUsecase(account, users, new(fakeCredentials), new(fakeSessions))
	if err != nil {
		t.Fatalf("NewUserUsecase() error = %v", err)
	}

	disabled, err := usecase.DisableUser(t.Context(), userpb.NewUserName("user-1"), "etag-1")
	if err != nil {
		t.Fatalf("DisableUser() error = %v", err)
	}
	if disabled.GetStatus() != userpb.UserStatus_USER_STATUS_DISABLED {
		t.Fatal("atomic status operation was not invoked")
	}

	// Repeated disable still delegates to the atomic repository operation.
	if _, err := usecase.DisableUser(t.Context(), userpb.NewUserName("user-1"), ""); err != nil {
		t.Fatalf("repeated DisableUser() error = %v", err)
	}
}

func TestAccountChangeInitialPasswordAndDeleteAccount(t *testing.T) {
	const initialPlain = "correct horse battery staple"
	const nextPlain = "updated horse battery staple"
	initialHash, err := password.Hash(initialPlain)
	if err != nil {
		t.Fatalf("hash initial password: %v", err)
	}
	u := &userpb.User{
		UserId:          "user-1",
		Email:           new("person@example.com"),
		Status:          userpb.UserStatus_USER_STATUS_ACTIVE,
		EmailVerified:   true,
		Etag:            "etag-1",
		RequiredActions: []userpb.UserRequiredAction{userpb.UserRequiredAction_USER_REQUIRED_ACTION_UPDATE_PASSWORD},
	}
	users := &fakeAccountUsers{user: u}
	credentials := &fakeCredentials{credential: &PasswordCredential{
		UserID:                  "user-1",
		AuthenticatorID:         "auth-1",
		PasswordAuthenticatorID: "pa-1",
		PasswordHash:            initialHash,
	}}
	usecase, err := NewAccountUsecase(users, credentials, new(fakeVerificationTokens), new(fakePasswordResetTokens), &fakeCAP{valid: true}, new(fakeMailSender), accountTestRuntime())
	if err != nil {
		t.Fatal(err)
	}

	if _, err := usecase.ChangeInitialPassword(t.Context(), "person@example.com", "wrong-password", nextPlain); !errors.Is(err, ErrUnauthenticated) {
		t.Fatalf("wrong current password error = %v, want ErrUnauthenticated", err)
	}
	if _, err := usecase.ChangeInitialPassword(t.Context(), "person@example.com", initialPlain, initialPlain); !errors.Is(err, ErrInvalidPassword) {
		t.Fatalf("unchanged password error = %v, want ErrInvalidPassword", err)
	}
	session, err := usecase.ChangeInitialPassword(t.Context(), " Person@Example.com ", initialPlain, nextPlain)
	if err != nil || session == nil || !credentials.completedInitial {
		t.Fatalf("ChangeInitialPassword() session=%#v err=%v", session, err)
	}

	u.RequiredActions = nil
	if _, err := usecase.ChangeInitialPassword(t.Context(), "person@example.com", nextPlain, "another horse battery staple"); !errors.Is(err, ErrAccountInvalidState) {
		t.Fatalf("missing UPDATE_PASSWORD error = %v, want ErrAccountInvalidState", err)
	}

	if err := usecase.DeleteAccount(t.Context(), "user-1", "session-1", "wrong-password"); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("DeleteAccount wrong password error = %v, want ErrInvalidCredentials", err)
	}
	if err := usecase.DeleteAccount(t.Context(), "user-1", "session-1", nextPlain); err != nil {
		t.Fatalf("DeleteAccount() error = %v", err)
	}
	if users.softDeleted == nil || users.softDeleted.LoginID != "session-1" || users.user.DeleteTime == nil {
		t.Fatalf("DeleteAccount() did not pass proof to SoftDelete: %#v", users.softDeleted)
	}
}

func TestConfirmPasswordResetRejectsUnchangedPasswordWhenUpdateRequired(t *testing.T) {
	const currentPlain = "correct horse battery staple"
	currentHash, err := password.Hash(currentPlain)
	if err != nil {
		t.Fatal(err)
	}
	resetTokens := &fakePasswordResetTokens{
		userID:    "user-1",
		tokenHash: HashOpaqueSecret("reset-token"),
		credential: PasswordCredential{
			UserID:                  "user-1",
			AuthenticatorID:         "auth-1",
			PasswordAuthenticatorID: "pa-1",
			PasswordHash:            currentHash,
		},
		userEtag:               "etag-1",
		requiresPasswordChange: true,
	}
	usecase, err := NewAccountUsecase(new(fakeAccountUsers), new(fakeCredentials), new(fakeVerificationTokens), resetTokens, &fakeCAP{valid: true}, new(fakeMailSender), accountTestRuntime())
	if err != nil {
		t.Fatal(err)
	}
	if err := usecase.ConfirmPasswordReset(t.Context(), "reset-token", currentPlain); !errors.Is(err, ErrInvalidPassword) {
		t.Fatalf("unchanged password error = %v, want ErrInvalidPassword", err)
	}
	if resetTokens.replaced {
		t.Fatal("unchanged password consumed reset token")
	}
	if err := usecase.ConfirmPasswordReset(t.Context(), "reset-token", "brand new horse battery staple"); err != nil {
		t.Fatalf("ConfirmPasswordReset() with distinct password error = %v", err)
	}
	if !resetTokens.replaced {
		t.Fatal("distinct password did not consume reset token")
	}
}

func TestAdminCreateUserAndSendVerificationEmailSeparation(t *testing.T) {
	users := new(fakeAccountUsers)
	tokens := new(fakeVerificationTokens)
	mailer := new(fakeMailSender)
	accountUC := newAccountForTest(t, users, tokens, new(fakePasswordResetTokens), mailer)
	creds := new(fakeCredentials)
	sessions := new(fakeSessions)
	userUC, err := NewUserUsecase(accountUC, users, creds, sessions)
	if err != nil {
		t.Fatal(err)
	}
	plan := corecrud.MustBuildResourcePlan[*userpb.User](userpb.UserCRUDDescriptor())
	email := "created@example.com"
	prepared, err := plan.PrepareCreate(&userpb.User{Email: &email})
	if err != nil {
		t.Fatal(err)
	}
	name := userpb.NewUserName("01912345-6789-7abc-8def-0123456789ab")
	created, err := userUC.CreateUser(t.Context(), name, prepared, "correct horse battery staple", true)
	if err != nil {
		t.Fatalf("CreateUser() error = %v", err)
	}
	if mailer.calls != 0 || tokens.tokenHash != "" {
		t.Fatalf("CreateUser() sent verification mail=%d or created token=%q", mailer.calls, tokens.tokenHash)
	}
	if len(created.GetRequiredActions()) != 1 || created.GetRequiredActions()[0] != userpb.UserRequiredAction_USER_REQUIRED_ACTION_UPDATE_PASSWORD {
		t.Fatalf("CreateUser() required_actions = %v, want [UPDATE_PASSWORD]", created.GetRequiredActions())
	}

	mailer.sendErr = errors.New("smtp down")
	if err := userUC.SendUserVerificationEmail(t.Context(), name); !errors.Is(err, ErrVerificationEmailFailed) {
		t.Fatalf("SendUserVerificationEmail() smtp failure = %v, want ErrVerificationEmailFailed", err)
	}
	mailer.sendErr = nil
	if err := userUC.SendUserVerificationEmail(t.Context(), name); err != nil {
		t.Fatalf("SendUserVerificationEmail() error = %v", err)
	}
	if mailer.calls != 1 {
		t.Fatalf("SendUserVerificationEmail() mail calls = %d, want 1", mailer.calls)
	}

	if err := userUC.SetUserPassword(t.Context(), name, "admin set horse battery staple"); err != nil {
		t.Fatalf("SetUserPassword() error = %v", err)
	}
	if creds.adminSetCalls != 1 {
		t.Fatalf("SetPasswordAndRevokeSessions calls = %d, want 1", creds.adminSetCalls)
	}
	if err := userUC.RevokeUserSessions(t.Context(), name); err != nil {
		t.Fatalf("RevokeUserSessions() error = %v", err)
	}
	if sessions.revokedAll != 1 {
		t.Fatalf("RevokeAll calls = %d, want 1", sessions.revokedAll)
	}
}
