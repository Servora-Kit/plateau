package biz

import (
	"context"
	"errors"
	"fmt"
	"time"

	sessionpb "github.com/Servora-Kit/plateau/api/gen/go/iam/session/v1"
	userpb "github.com/Servora-Kit/plateau/api/gen/go/iam/user/v1"
)

const (
	LoginSessionIdleTTL     = 7 * 24 * time.Hour
	LoginSessionAbsoluteTTL = 30 * 24 * time.Hour
)

var (
	ErrSessionRevoked = errors.New("session revoked")
	ErrUserDisabled   = errors.New("user disabled")
	ErrUserNotActive  = errors.New("user not active")
)

// SessionRepo persists independently revocable IAM Login Sessions.
type SessionRepo interface {
	Create(context.Context, string, string, time.Time) (*sessionpb.Session, error)
	FindBySecretHash(context.Context, string) (*sessionpb.Session, string, bool, error)
	Touch(context.Context, string, time.Time) (*sessionpb.Session, error)
	Revoke(context.Context, string, time.Time) error
	RevokeOthersForUser(context.Context, string, string, time.Time) error
	RevokeAllForUser(context.Context, string, time.Time) error
}

// TokenSessionRepo revokes OAuth token sessions associated with IAM sessions or users.
type TokenSessionRepo interface {
	RevokeForLoginSession(context.Context, string, time.Time) error
	RevokeAllForUser(context.Context, string, time.Time) error
}

// SessionUsecase owns IAM Login Session creation, resolution and revocation.
type SessionUsecase struct {
	users         UserRepo
	sessions      SessionRepo
	tokenSessions TokenSessionRepo
	now           func() time.Time
}

// NewSessionUsecase wires Login Session persistence and associated OAuth Token Session revocation.
func NewSessionUsecase(users UserRepo, sessions SessionRepo, tokenSessions TokenSessionRepo) (*SessionUsecase, error) {
	if users == nil || sessions == nil || tokenSessions == nil {
		return nil, fmt.Errorf("session: repository dependency is nil")
	}
	return &SessionUsecase{users: users, sessions: sessions, tokenSessions: tokenSessions, now: time.Now}, nil
}

// Create establishes one independent opaque Login Session after successful Authentication.
func (uc *SessionUsecase) Create(ctx context.Context, userID string) (*sessionpb.Session, string, error) {
	if userID == "" {
		return nil, "", ErrUnauthenticated
	}
	secret, _, err := NewOpaqueSecret()
	if err != nil {
		return nil, "", fmt.Errorf("create login session secret: %w", err)
	}
	loginSession, err := uc.sessions.Create(ctx, userID, HashOpaqueSecret(secret), uc.now())
	if err != nil {
		return nil, "", fmt.Errorf("persist login session: %w", err)
	}
	return loginSession, secret, nil
}

// Resolve validates a Cookie secret, checks its User and touches idle expiry only on success.
func (uc *SessionUsecase) Resolve(ctx context.Context, secret string) (*userpb.User, *sessionpb.Session, error) {
	if secret == "" {
		return nil, nil, ErrUnauthenticated
	}
	loginSession, userID, revoked, err := uc.sessions.FindBySecretHash(ctx, HashOpaqueSecret(secret))
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil, nil, ErrSessionRevoked
		}
		return nil, nil, fmt.Errorf("find login session: %w", err)
	}
	now := uc.now()
	active := loginSession != nil && loginSession.GetSessionId() != "" && loginSession.GetIdleExpiresTime() != nil && loginSession.GetAbsoluteExpiresTime() != nil && now.Before(loginSession.GetIdleExpiresTime().AsTime()) && now.Before(loginSession.GetAbsoluteExpiresTime().AsTime())
	if revoked || !active {
		return nil, nil, ErrSessionRevoked
	}
	user, err := uc.users.Get(ctx, userID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil, nil, ErrSessionRevoked
		}
		return nil, nil, fmt.Errorf("find session user: %w", err)
	}
	if user.GetStatus() == userpb.UserStatus_USER_STATUS_DISABLED {
		return nil, nil, ErrUserDisabled
	}
	if user.GetStatus() != userpb.UserStatus_USER_STATUS_ACTIVE || !user.GetEmailVerified() {
		return nil, nil, ErrUserNotActive
	}
	loginSession, err = uc.sessions.Touch(ctx, loginSession.GetSessionId(), now)
	if err != nil {
		if errors.Is(err, ErrMutationMiss) || errors.Is(err, ErrNotFound) {
			return nil, nil, ErrSessionRevoked
		}
		return nil, nil, fmt.Errorf("touch login session: %w", err)
	}
	return user, loginSession, nil
}

// Logout revokes the current Login Session and every OAuth Token Session created from it.
func (uc *SessionUsecase) Logout(ctx context.Context, sessionID string) error {
	if sessionID == "" {
		return ErrUnauthenticated
	}
	now := uc.now()
	if err := errors.Join(
		uc.sessions.Revoke(ctx, sessionID, now),
		uc.tokenSessions.RevokeForLoginSession(ctx, sessionID, now),
	); err != nil {
		return fmt.Errorf("logout current login session: %w", err)
	}
	return nil
}

// RevokeOthersForUser preserves one Login Session and revokes all OAuth Token Sessions for the User.
func (uc *SessionUsecase) RevokeOthersForUser(ctx context.Context, userID, keepSessionID string) error {
	if userID == "" || keepSessionID == "" {
		return ErrUnauthenticated
	}
	now := uc.now()
	if err := errors.Join(
		uc.sessions.RevokeOthersForUser(ctx, userID, keepSessionID, now),
		uc.tokenSessions.RevokeAllForUser(ctx, userID, now),
	); err != nil {
		return fmt.Errorf("revoke other user sessions: %w", err)
	}
	return nil
}

// RevokeAllForUser revokes every Login Session and OAuth Token Session for a User.
func (uc *SessionUsecase) RevokeAllForUser(ctx context.Context, userID string) error {
	if userID == "" {
		return ErrUnauthenticated
	}
	now := uc.now()
	if err := errors.Join(
		uc.sessions.RevokeAllForUser(ctx, userID, now),
		uc.tokenSessions.RevokeAllForUser(ctx, userID, now),
	); err != nil {
		return fmt.Errorf("revoke all user sessions: %w", err)
	}
	return nil
}
