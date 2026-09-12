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
	credential  *PasswordCredential
	replaced    bool
	keepLoginID string
}

func (repo *fakeCredentials) FindActivePassword(context.Context, string) (*PasswordCredential, error) {
	if repo.credential == nil {
		return nil, ErrNotFound
	}
	return repo.credential, nil
}

func (repo *fakeCredentials) ReplacePassword(_ context.Context, _ string, _ string, _ string, passwordHash, keepID string, _ time.Time) error {
	repo.credential.PasswordHash = passwordHash
	repo.replaced = true
	repo.keepLoginID = keepID
	return nil
}

type fakeSessions struct {
	created *LoginSession
	revoked bool
}

func (repo *fakeSessions) Create(_ context.Context, userID string, now time.Time) (*LoginSession, error) {
	repo.created = &LoginSession{ID: "session-1", UserID: userID, AuthTime: now}
	return repo.created, nil
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

func activeUser() *userpb.User {
	return &userpb.User{
		UserId: "user-1", Email: stringPtr("person@example.com"),
		Status: userpb.UserStatus_USER_STATUS_ACTIVE, EmailVerified: true,
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
	sessions := new(fakeSessions)
	usecase, _ := newAuthenticationForTest(t,
		&fakeAccountUsers{user: activeUser()},
		&fakeCredentials{credential: &PasswordCredential{UserID: "user-1", PasswordHash: hash}},
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
		{name: "pending email", user: &userpb.User{UserId: "user-1", Email: stringPtr("person@example.com"), Status: userpb.UserStatus_USER_STATUS_PENDING_EMAIL_VERIFICATION}, pass: "correct horse battery staple"},
		{name: "disabled user", user: &userpb.User{UserId: "user-1", Email: stringPtr("person@example.com"), Status: userpb.UserStatus_USER_STATUS_DISABLED, EmailVerified: true}, pass: "correct horse battery staple"},
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

func TestSessionResolutionUsesCurrentFacts(t *testing.T) {
	users := &fakeAccountUsers{user: activeUser()}
	sessions := new(fakeSessions)
	usecase := newSessionForTest(t, users, sessions)
	login, err := usecase.Create(t.Context(), "user-1")
	if err != nil {
		t.Fatal(err)
	}
	_, resolved, err := usecase.Resolve(t.Context(), login.ID)
	if err != nil || !resolved.AuthTime.Equal(login.AuthTime) {
		t.Fatalf("resolve: %v", err)
	}
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
