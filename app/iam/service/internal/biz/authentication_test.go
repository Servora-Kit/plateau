package biz

import (
	"context"
	"errors"
	"testing"
	"time"

	userpb "github.com/Servora-Kit/plateau/api/gen/go/iam/user/v1"
	"github.com/Servora-Kit/plateau/security/password"
)

type fakeCredentials struct {
	credential        *PasswordCredential
	replaced          bool
	keepLoginID       string
	adminSetCalls     int
	completedInitial  bool
	completeSessionID string
	completeErr       error
}

func (repo *fakeCredentials) FindActivePassword(context.Context, string) (*PasswordCredential, error) {
	if repo.credential == nil {
		return nil, ErrNotFound
	}
	copyCred := *repo.credential
	return &copyCred, nil
}

func (repo *fakeCredentials) ReplacePassword(_ context.Context, _ string, _ string, _ string, passwordHash, keepID string, _ time.Time) error {
	repo.credential.PasswordHash = passwordHash
	repo.replaced = true
	repo.keepLoginID = keepID
	return nil
}

func (repo *fakeCredentials) SetPasswordAndRevokeSessions(_ context.Context, userID, passwordHash string, _ time.Time) error {
	repo.adminSetCalls++
	if repo.credential == nil {
		repo.credential = &PasswordCredential{UserID: userID}
	}
	repo.credential.PasswordHash = passwordHash
	return nil
}

func (repo *fakeCredentials) CompleteInitialPassword(_ context.Context, expected PasswordCredential, _ string, newPasswordHash string, now time.Time) (*LoginSession, error) {
	if repo.completeErr != nil {
		return nil, repo.completeErr
	}
	repo.completedInitial = true
	if repo.credential != nil {
		repo.credential.PasswordHash = newPasswordHash
	}
	id := repo.completeSessionID
	if id == "" {
		id = "initial-session-1"
	}
	return &LoginSession{ID: id, UserID: expected.UserID, AuthTime: now}, nil
}

type fakeSessions struct {
	user       *userpb.User
	created    *LoginSession
	revoked    bool
	revokedAll int
	createErr  error
}

func (repo *fakeSessions) Create(_ context.Context, expected PasswordCredential, expectedEtag string, now time.Time) (*userpb.User, *LoginSession, error) {
	if repo.createErr != nil {
		return nil, nil, repo.createErr
	}
	u := repo.user
	if u == nil {
		u = &userpb.User{
			UserId:        expected.UserID,
			Status:        userpb.UserStatus_USER_STATUS_ACTIVE,
			EmailVerified: true,
			Etag:          expectedEtag,
		}
	}
	if len(u.GetRequiredActions()) > 0 {
		return u, nil, nil
	}
	repo.created = &LoginSession{ID: "session-1", UserID: expected.UserID, AuthTime: now}
	return u, repo.created, nil
}
func (repo *fakeSessions) Find(_ context.Context, id string) (*LoginSession, error) {
	if repo.created == nil || repo.created.ID != id {
		return nil, ErrNotFound
	}
	value := *repo.created
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

func newAuthenticationForTest(t *testing.T, users *fakeAccountUsers, credentials *fakeCredentials, sessions *fakeSessions) (*AuthenticationUsecase, *SessionUsecase) {
	t.Helper()
	sessionUsecase := newSessionForTest(t, users, sessions)
	usecase, err := NewAuthenticationUsecase(users, credentials, sessionUsecase)
	if err != nil {
		t.Fatalf("NewAuthenticationUsecase() error = %v", err)
	}
	return usecase, sessionUsecase
}

func TestAuthenticationLoginCreatesIndependentSession(t *testing.T) {
	plaintext := "correct horse battery staple"
	hash, err := password.Hash(plaintext)
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	u := activeUser()
	sessions := &fakeSessions{user: u}
	usecase, _ := newAuthenticationForTest(t,
		&fakeAccountUsers{user: u},
		&fakeCredentials{credential: &PasswordCredential{UserID: "user-1", AuthenticatorID: "auth-1", PasswordAuthenticatorID: "pa-1", PasswordHash: hash}},
		sessions,
	)
	_, loginSession, err := usecase.Login(t.Context(), " PERSON@EXAMPLE.COM ", plaintext)
	if err != nil {
		t.Fatalf("Login() error = %v", err)
	}
	if loginSession == nil || sessions.created == nil || loginSession.UserID != "user-1" {
		t.Fatalf("Login() session = %#v, want opaque session", loginSession)
	}
	if loginSession.AuthTime.IsZero() {
		t.Fatal("missing authentication time")
	}
}

func TestAuthenticationRejectsInvalidCredentialsWithoutSession(t *testing.T) {
	hash, err := password.Hash("correct horse battery staple")
	if err != nil {
		t.Fatalf("hash password: %v", err)
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
			usecase, _ := newAuthenticationForTest(t, &fakeAccountUsers{user: test.user}, &fakeCredentials{credential: &PasswordCredential{UserID: "user-1", PasswordHash: hash}}, sessions)
			if _, _, err := usecase.Login(t.Context(), "person@example.com", test.pass); !errors.Is(err, ErrInvalidCredentials) {
				t.Fatalf("Login() error = %v, want generic invalid credentials", err)
			}
			if sessions.created != nil {
				t.Fatal("invalid login created a session")
			}
		})
	}
}

func TestAuthenticationLoginSkipsSessionOnRequiredActionsAndMapsRaces(t *testing.T) {
	plaintext := "correct horse battery staple"
	hash, err := password.Hash(plaintext)
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	pendingActionUser := activeUser()
	pendingActionUser.RequiredActions = []userpb.UserRequiredAction{userpb.UserRequiredAction_USER_REQUIRED_ACTION_UPDATE_PASSWORD}
	sessions := &fakeSessions{user: pendingActionUser}
	usecase, _ := newAuthenticationForTest(t,
		&fakeAccountUsers{user: pendingActionUser},
		&fakeCredentials{credential: &PasswordCredential{UserID: "user-1", AuthenticatorID: "auth-1", PasswordAuthenticatorID: "pa-1", PasswordHash: hash}},
		sessions,
	)
	gotUser, loginSession, err := usecase.Login(t.Context(), "person@example.com", plaintext)
	if err != nil {
		t.Fatalf("Login() error = %v", err)
	}
	if loginSession != nil || sessions.created != nil {
		t.Fatalf("Login() created session %#v for user with required actions", loginSession)
	}
	if len(gotUser.GetRequiredActions()) != 1 {
		t.Fatalf("Login() user required_actions = %v, want UPDATE_PASSWORD", gotUser.GetRequiredActions())
	}

	for _, raceErr := range []error{ErrUserEtagMismatch, ErrMutationMiss, ErrNotFound, ErrUserDisabled} {
		raceSessions := &fakeSessions{createErr: raceErr}
		raceUC, _ := newAuthenticationForTest(t,
			&fakeAccountUsers{user: activeUser()},
			&fakeCredentials{credential: &PasswordCredential{UserID: "user-1", AuthenticatorID: "auth-1", PasswordAuthenticatorID: "pa-1", PasswordHash: hash}},
			raceSessions,
		)
		if _, _, err := raceUC.Login(t.Context(), "person@example.com", plaintext); !errors.Is(err, ErrInvalidCredentials) {
			t.Fatalf("race error %v mapped to %v, want ErrInvalidCredentials", raceErr, err)
		}
	}
}

func TestSessionResolutionUsesCurrentFactsAndRejectsRequiredActions(t *testing.T) {
	u := activeUser()
	users := &fakeAccountUsers{user: u}
	sessions := &fakeSessions{user: u}
	usecase := newSessionForTest(t, users, sessions)
	_, login, err := usecase.Create(t.Context(), PasswordCredential{
		UserID: "user-1", AuthenticatorID: "auth-1", PasswordAuthenticatorID: "pa-1", PasswordHash: "hash-1",
	}, u.GetEtag())
	if err != nil || login == nil {
		t.Fatalf("Create() login=%#v error=%v", login, err)
	}
	_, resolved, err := usecase.Resolve(t.Context(), login.ID)
	if err != nil || !resolved.AuthTime.Equal(login.AuthTime) {
		t.Fatalf("resolve: %v", err)
	}
	users.user.RequiredActions = []userpb.UserRequiredAction{userpb.UserRequiredAction_USER_REQUIRED_ACTION_UPDATE_PASSWORD}
	if _, _, err := usecase.Resolve(t.Context(), login.ID); !errors.Is(err, ErrRequiredActionPending) {
		t.Fatalf("required action pending: %v, want ErrRequiredActionPending", err)
	}
	users.user.RequiredActions = nil
	users.user.Status = userpb.UserStatus_USER_STATUS_DISABLED
	if _, _, err := usecase.Resolve(t.Context(), login.ID); !errors.Is(err, ErrUserDisabled) {
		t.Fatalf("disabled: %v", err)
	}
	users.user.Status = userpb.UserStatus_USER_STATUS_ACTIVE
	if err := usecase.Logout(t.Context(), login.ID); err != nil {
		t.Fatal(err)
	}
	if _, _, err := usecase.Resolve(t.Context(), login.ID); !errors.Is(err, ErrSessionRevoked) {
		t.Fatalf("revoked: %v", err)
	}
}
