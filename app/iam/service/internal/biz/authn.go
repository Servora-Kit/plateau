package biz

import (
	"context"
	"errors"
	"fmt"
	"time"

	sessionpb "github.com/Servora-Kit/plateau/api/gen/go/iam/session/v1"
	userpb "github.com/Servora-Kit/plateau/api/gen/go/iam/user/v1"
	"github.com/Servora-Kit/plateau/security/password"
)

var ErrInvalidCredentials = errors.New("invalid credentials")

// PasswordCredential contains server-only authenticator fields absent from public Proto.
type PasswordCredential struct {
	UserID          string
	AuthenticatorID string
	PasswordHash    string
}

type CredentialRepo interface {
	FindActivePassword(context.Context, string) (*PasswordCredential, error)
	ReplacePassword(context.Context, string, string, string, time.Time) error
}

type AuthenticationUsecase struct {
	users       UserRepo
	credentials CredentialRepo
	sessions    *SessionUsecase
}

// NewAuthenticationUsecase wires credential verification to successful Session creation.
func NewAuthenticationUsecase(users UserRepo, credentials CredentialRepo, sessions *SessionUsecase) (*AuthenticationUsecase, error) {
	if users == nil || credentials == nil || sessions == nil {
		return nil, fmt.Errorf("authentication: dependency is nil")
	}
	return &AuthenticationUsecase{users: users, credentials: credentials, sessions: sessions}, nil
}

// Login verifies email/password and creates one independent opaque session.
func (uc *AuthenticationUsecase) Login(ctx context.Context, email, plaintextPassword string) (*userpb.User, *sessionpb.Session, string, error) {
	canonical, _, err := NormalizeEmail(email)
	if err != nil {
		return nil, nil, "", ErrInvalidCredentials
	}
	user, err := uc.users.FindByEmail(ctx, canonical)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil, nil, "", ErrInvalidCredentials
		}
		return nil, nil, "", fmt.Errorf("find login user: %w", err)
	}
	if user.GetStatus() != userpb.UserStatus_USER_STATUS_ACTIVE || !user.GetEmailVerified() {
		return nil, nil, "", ErrInvalidCredentials
	}
	credential, err := uc.credentials.FindActivePassword(ctx, user.GetUserId())
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil, nil, "", ErrInvalidCredentials
		}
		return nil, nil, "", fmt.Errorf("find password credential: %w", err)
	}
	match, _, err := password.Compare(plaintextPassword, credential.PasswordHash)
	if err != nil || !match {
		return nil, nil, "", ErrInvalidCredentials
	}
	session, secret, err := uc.sessions.Create(ctx, user.GetUserId())
	if err != nil {
		return nil, nil, "", fmt.Errorf("create login session: %w", err)
	}
	return user, session, secret, nil
}
