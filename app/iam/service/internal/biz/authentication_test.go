package biz

import (
	"context"
	"errors"
	"testing"
	"time"

	sessionpb "github.com/Servora-Kit/plateau/api/gen/go/iam/session/v1"
	userpb "github.com/Servora-Kit/plateau/api/gen/go/iam/user/v1"
	"github.com/Servora-Kit/plateau/security/password"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type fakeCredentials struct {
	credential *PasswordCredential
	replaced   bool
}

func (repo *fakeCredentials) FindActivePassword(context.Context, string) (*PasswordCredential, error) {
	if repo.credential == nil {
		return nil, ErrNotFound
	}
	return repo.credential, nil
}

func (repo *fakeCredentials) ReplacePassword(_ context.Context, _ string, _ string, passwordHash string, _ time.Time) error {
	repo.credential.PasswordHash = passwordHash
	repo.replaced = true
	return nil
}

type fakeSessions struct {
	created       *sessionpb.Session
	secretHash    string
	userID        string
	revoked       bool
	touched       bool
	revokedOthers bool
	revokedAll    bool
}

func (repo *fakeSessions) Create(_ context.Context, userID, secretHash string, now time.Time) (*sessionpb.Session, error) {
	repo.secretHash, repo.userID = secretHash, userID
	repo.created = &sessionpb.Session{
		Name: "sessions/session-1", SessionId: "session-1",
		CreateTime: timestamppb.New(now), LastSeenTime: timestamppb.New(now),
		IdleExpiresTime:     timestamppb.New(now.Add(LoginSessionIdleTTL)),
		AbsoluteExpiresTime: timestamppb.New(now.Add(LoginSessionAbsoluteTTL)),
	}
	return repo.created, nil
}

func (repo *fakeSessions) FindBySecretHash(_ context.Context, secretHash string) (*sessionpb.Session, string, bool, error) {
	if repo.created == nil || secretHash != repo.secretHash {
		return nil, "", false, ErrNotFound
	}
	return repo.created, repo.userID, repo.revoked, nil
}

func (repo *fakeSessions) Touch(_ context.Context, _ string, now time.Time) (*sessionpb.Session, error) {
	repo.touched = true
	repo.created.LastSeenTime = timestamppb.New(now)
	repo.created.IdleExpiresTime = timestamppb.New(now.Add(LoginSessionIdleTTL))
	return repo.created, nil
}

func (repo *fakeSessions) Revoke(context.Context, string, time.Time) error {
	repo.revoked = true
	return nil
}

func (repo *fakeSessions) RevokeOthersForUser(context.Context, string, string, time.Time) error {
	repo.revokedOthers = true
	return nil
}

func (repo *fakeSessions) RevokeAllForUser(context.Context, string, time.Time) error {
	repo.revokedAll = true
	return nil
}

type fakeTokenSessions struct {
	revokedForSession bool
	revokedForUser    bool
}

func (repo *fakeTokenSessions) RevokeForLoginSession(context.Context, string, time.Time) error {
	repo.revokedForSession = true
	return nil
}

func (repo *fakeTokenSessions) RevokeAllForUser(context.Context, string, time.Time) error {
	repo.revokedForUser = true
	return nil
}

func activeUser() *userpb.User {
	return &userpb.User{
		UserId: "user-1", Email: stringPtr("person@example.com"),
		Status: userpb.UserStatus_USER_STATUS_ACTIVE, EmailVerified: true,
	}
}

func newSessionForTest(t *testing.T, users *fakeAccountUsers, sessions *fakeSessions, tokens *fakeTokenSessions) *SessionUsecase {
	t.Helper()
	usecase, err := NewSessionUsecase(users, sessions, tokens)
	if err != nil {
		t.Fatalf("NewSessionUsecase() error = %v", err)
	}
	return usecase
}

func newAuthenticationForTest(t *testing.T, users *fakeAccountUsers, credentials *fakeCredentials, sessions *fakeSessions, tokens *fakeTokenSessions) (*AuthenticationUsecase, *SessionUsecase) {
	t.Helper()
	sessionUsecase := newSessionForTest(t, users, sessions, tokens)
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
		sessions, new(fakeTokenSessions),
	)
	_, loginSession, secret, err := usecase.Login(t.Context(), " PERSON@EXAMPLE.COM ", plaintext)
	if err != nil {
		t.Fatalf("Login() error = %v", err)
	}
	if secret == "" || loginSession == nil || sessions.created == nil {
		t.Fatalf("Login() session = %#v, want opaque session", loginSession)
	}
	if secret == plaintext || sessions.secretHash == secret {
		t.Fatal("Login() exposed or persisted plaintext session secret")
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
			usecase, _ := newAuthenticationForTest(t, &fakeAccountUsers{user: test.user}, &fakeCredentials{credential: &PasswordCredential{UserID: "user-1", PasswordHash: hash}}, sessions, new(fakeTokenSessions))
			if _, _, _, err := usecase.Login(t.Context(), "person@example.com", test.pass); !errors.Is(err, ErrInvalidCredentials) {
				t.Fatalf("Login() error = %v, want generic invalid credentials", err)
			}
			if sessions.created != nil {
				t.Fatal("invalid login created a session")
			}
		})
	}
}

func TestSessionResolveTouchesAndLogoutRevokes(t *testing.T) {
	sessions, tokenSessions := new(fakeSessions), new(fakeTokenSessions)
	usecase := newSessionForTest(t, &fakeAccountUsers{user: activeUser()}, sessions, tokenSessions)
	loginSession, secret, err := usecase.Create(t.Context(), "user-1")
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if _, _, err := usecase.Resolve(t.Context(), secret); err != nil || !sessions.touched {
		t.Fatalf("Resolve() error = %v touched=%t", err, sessions.touched)
	}
	if err := usecase.Logout(t.Context(), loginSession.GetSessionId()); err != nil {
		t.Fatalf("Logout() error = %v", err)
	}
	if !sessions.revoked || !tokenSessions.revokedForSession {
		t.Fatalf("Logout() revoked session=%t token sessions=%t", sessions.revoked, tokenSessions.revokedForSession)
	}
}

func TestSessionResolveRejectsRevokedSessionWithoutTouch(t *testing.T) {
	sessions := new(fakeSessions)
	usecase := newSessionForTest(t, &fakeAccountUsers{user: activeUser()}, sessions, new(fakeTokenSessions))
	_, secret, err := usecase.Create(t.Context(), "user-1")
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	sessions.revoked = true
	if _, _, err := usecase.Resolve(t.Context(), secret); !errors.Is(err, ErrSessionRevoked) {
		t.Fatalf("Resolve() error = %v, want revoked", err)
	}
	if sessions.touched {
		t.Fatal("revoked Session was touched")
	}
}
