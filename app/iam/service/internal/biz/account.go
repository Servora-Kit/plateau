package biz

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	userpb "github.com/Servora-Kit/plateau/api/gen/go/iam/user/v1"
	"github.com/Servora-Kit/plateau/security/password"
	"github.com/Servora-Kit/servora/core/bootstrap"
)

const (
	EmailVerificationTTL = 24 * time.Hour
	PasswordResetTTL     = time.Hour
)

var (
	ErrInvalidToken        = errors.New("invalid account token")
	ErrTokenExpired        = errors.New("account token expired")
	ErrEmailAlreadyExists  = errors.New("email already registered")
	ErrProfileEtagMismatch = errors.New("profile etag mismatch")
)

type VerificationTokenRepo interface {
	Create(context.Context, string, string, string, time.Time) error
	Consume(context.Context, string, time.Time) (string, error)
}

type PasswordResetTokenRepo interface {
	Create(context.Context, string, string, time.Time) error
	ConsumeAndReplacePassword(context.Context, string, string, time.Time) (string, error)
}

type CAPVerifier interface {
	ValidateToken(context.Context, string) (bool, error)
}

type MailSender interface {
	SendVerification(context.Context, string, string, int) error
	SendPasswordReset(context.Context, string, string, int) error
}

// AccountUsecase owns public account lifecycle and synchronous verification-mail delivery.
type AccountUsecase struct {
	users              UserRepo
	credentials        CredentialRepo
	verificationTokens VerificationTokenRepo
	resetTokens        PasswordResetTokenRepo
	cap                CAPVerifier
	mailer             MailSender
	sessions           *SessionUsecase
	verificationOrigin string
	now                func() time.Time
}

// NewAccountUsecase wires public account lifecycle, credential changes, recovery and mail delivery.
func NewAccountUsecase(users UserRepo, credentials CredentialRepo, verificationTokens VerificationTokenRepo, resetTokens PasswordResetTokenRepo, captcha CAPVerifier, mailer MailSender, sessions *SessionUsecase, runtime *bootstrap.Runtime) (*AccountUsecase, error) {
	if users == nil || credentials == nil || verificationTokens == nil || resetTokens == nil || captcha == nil || mailer == nil || sessions == nil || runtime == nil || runtime.Bootstrap == nil || runtime.Bootstrap.App == nil {
		return nil, fmt.Errorf("account: dependency is nil")
	}
	verificationOrigin := strings.TrimRight(strings.TrimSpace(runtime.Bootstrap.App.GetExternalUrl()), "/")
	if verificationOrigin == "" {
		return nil, fmt.Errorf("account: verification origin is empty")
	}
	return &AccountUsecase{
		users: users, credentials: credentials, verificationTokens: verificationTokens, resetTokens: resetTokens,
		cap: captcha, mailer: mailer, sessions: sessions,
		verificationOrigin: verificationOrigin, now: time.Now,
	}, nil
}

// Register consumes CAP before creating a pending User and verification token.
func (uc *AccountUsecase) Register(ctx context.Context, email, plaintextPassword, capToken string, profile *userpb.UserProfile) (*userpb.User, error) {
	valid, err := uc.cap.ValidateToken(ctx, capToken)
	if err != nil {
		return nil, fmt.Errorf("validate registration CAP: %w", err)
	}
	if !valid {
		return nil, ErrInvalidToken
	}
	return uc.CreatePendingUser(ctx, "", email, plaintextPassword, profile)
}

// CreatePendingUser creates the aggregate, persists its verification token, then sends mail.
func (uc *AccountUsecase) CreatePendingUser(ctx context.Context, userID, email, plaintextPassword string, profile *userpb.UserProfile) (*userpb.User, error) {
	canonical, display, err := NormalizeEmail(email)
	if err != nil {
		return nil, err
	}
	if _, err := uc.users.FindByEmail(ctx, canonical); err == nil {
		return nil, ErrEmailAlreadyExists
	} else if !errors.Is(err, ErrNotFound) {
		return nil, fmt.Errorf("check registered email: %w", err)
	}
	if userID == "" {
		userID, err = NewUserID()
		if err != nil {
			return nil, err
		}
	}
	emailValue := display
	resource := &userpb.User{
		Name: "users/" + userID, UserId: userID, Email: &emailValue,
		Profile: profile, Status: userpb.UserStatus_USER_STATUS_PENDING_EMAIL_VERIFICATION,
	}
	hash, err := password.Hash(plaintextPassword)
	if err != nil {
		return nil, fmt.Errorf("hash account password: %w", err)
	}
	user, err := uc.users.Create(ctx, resource, hash, canonical)
	if err != nil {
		if errors.Is(err, ErrAlreadyExists) {
			return nil, ErrEmailAlreadyExists
		}
		return nil, fmt.Errorf("create pending user: %w", err)
	}
	token, _, err := NewOpaqueSecret()
	if err != nil {
		return nil, err
	}
	now := uc.now()
	if err := uc.verificationTokens.Create(ctx, user.GetUserId(), "", HashOpaqueSecret(token), now.Add(EmailVerificationTTL)); err != nil {
		return nil, fmt.Errorf("create verification token: %w", err)
	}
	if err := uc.mailer.SendVerification(ctx, user.GetEmail(), uc.verificationLink(token), int(EmailVerificationTTL/time.Hour)); err != nil {
		return nil, fmt.Errorf("send verification email: %w", err)
	}
	return user, nil
}

// VerifyEmail consumes one token and activates the pending User.
func (uc *AccountUsecase) VerifyEmail(ctx context.Context, token string) (*userpb.User, error) {
	if token == "" {
		return nil, ErrInvalidToken
	}
	userID, err := uc.verificationTokens.Consume(ctx, HashOpaqueSecret(token), uc.now())
	if err != nil {
		return nil, accountTokenError(err)
	}
	if err := uc.users.ActivateEmail(ctx, userID, uc.now()); err != nil {
		return nil, err
	}
	return uc.users.Get(ctx, userID)
}

// ResendVerification consumes CAP and sends mail only for pending users.
func (uc *AccountUsecase) ResendVerification(ctx context.Context, email, capToken string) error {
	valid, err := uc.cap.ValidateToken(ctx, capToken)
	if err != nil {
		return fmt.Errorf("validate resend CAP: %w", err)
	}
	if !valid {
		return ErrInvalidToken
	}
	canonical, _, err := NormalizeEmail(email)
	if err != nil {
		return nil
	}
	user, err := uc.users.FindByEmail(ctx, canonical)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil
		}
		return fmt.Errorf("find resend user: %w", err)
	}
	if user.GetStatus() != userpb.UserStatus_USER_STATUS_PENDING_EMAIL_VERIFICATION || user.GetEmailVerified() {
		return nil
	}
	token, _, err := NewOpaqueSecret()
	if err != nil {
		return err
	}
	if err := uc.verificationTokens.Create(ctx, user.GetUserId(), "", HashOpaqueSecret(token), uc.now().Add(EmailVerificationTTL)); err != nil {
		return fmt.Errorf("create resend token: %w", err)
	}
	if err := uc.mailer.SendVerification(ctx, user.GetEmail(), uc.verificationLink(token), int(EmailVerificationTTL/time.Hour)); err != nil {
		return fmt.Errorf("send verification email: %w", err)
	}
	return nil
}

// RequestPasswordReset consumes CAP and sends a generic reset response for unknown or ineligible users.
func (uc *AccountUsecase) RequestPasswordReset(ctx context.Context, email, capToken string) error {
	valid, err := uc.cap.ValidateToken(ctx, capToken)
	if err != nil {
		return fmt.Errorf("validate password reset CAP: %w", err)
	}
	if !valid {
		return ErrInvalidToken
	}
	canonical, _, err := NormalizeEmail(email)
	if err != nil {
		return nil
	}
	user, err := uc.users.FindByEmail(ctx, canonical)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil
		}
		return fmt.Errorf("find password reset user: %w", err)
	}
	if user.GetStatus() != userpb.UserStatus_USER_STATUS_ACTIVE || !user.GetEmailVerified() {
		return nil
	}
	token, _, err := NewOpaqueSecret()
	if err != nil {
		return err
	}
	if err := uc.resetTokens.Create(ctx, user.GetUserId(), HashOpaqueSecret(token), uc.now().Add(PasswordResetTTL)); err != nil {
		return fmt.Errorf("create password reset token: %w", err)
	}
	if err := uc.mailer.SendPasswordReset(ctx, user.GetEmail(), uc.verificationOrigin+"/reset-password#"+token, int(PasswordResetTTL/time.Hour)); err != nil {
		return fmt.Errorf("send password reset email: %w", err)
	}
	return nil
}

// ConfirmPasswordReset atomically consumes a reset token, replaces its password and revokes sessions.
func (uc *AccountUsecase) ConfirmPasswordReset(ctx context.Context, token, newPassword string) error {
	if token == "" {
		return ErrInvalidToken
	}
	hash, err := password.Hash(newPassword)
	if err != nil {
		return ErrInvalidPassword
	}
	userID, err := uc.resetTokens.ConsumeAndReplacePassword(ctx, HashOpaqueSecret(token), hash, uc.now())
	if err != nil {
		return accountTokenError(err)
	}
	if err := uc.sessions.RevokeAllForUser(ctx, userID); err != nil {
		return fmt.Errorf("revoke sessions after password reset: %w", err)
	}
	return nil
}

// ChangePassword replaces the current password, preserves the current Login Session and revokes all OAuth Token Sessions.
func (uc *AccountUsecase) ChangePassword(ctx context.Context, userID, currentSessionID, currentPassword, newPassword string) error {
	if userID == "" || currentSessionID == "" {
		return ErrUnauthenticated
	}
	credential, err := uc.credentials.FindActivePassword(ctx, userID)
	if err != nil {
		return fmt.Errorf("find current password: %w", err)
	}
	match, _, err := password.Compare(currentPassword, credential.PasswordHash)
	if err != nil || !match {
		return ErrInvalidCredentials
	}
	hash, err := password.Hash(newPassword)
	if err != nil {
		return fmt.Errorf("hash new password: %w", err)
	}
	if err := uc.credentials.ReplacePassword(ctx, userID, credential.AuthenticatorID, hash, uc.now()); err != nil {
		return fmt.Errorf("replace password: %w", err)
	}
	if err := uc.sessions.RevokeOthersForUser(ctx, userID, currentSessionID); err != nil {
		return fmt.Errorf("revoke sessions after password change: %w", err)
	}
	return nil
}

// GetProfile loads one authenticated user's protobuf resource.
func (uc *AccountUsecase) GetProfile(ctx context.Context, userID string) (*userpb.User, error) {
	if strings.TrimSpace(userID) == "" {
		return nil, ErrUnauthenticated
	}
	return uc.users.Get(ctx, userID)
}

// UpdateCurrentProfile uses the current protobuf resource ETag.
func (uc *AccountUsecase) UpdateCurrentProfile(ctx context.Context, userID string, profile *userpb.UserProfile) (*userpb.User, error) {
	current, err := uc.GetProfile(ctx, userID)
	if err != nil {
		return nil, err
	}
	return uc.UpdateProfile(ctx, userID, current.GetEtag(), profile)
}

// UpdateProfile persists OIDC standard profile fields with optimistic concurrency.
func (uc *AccountUsecase) UpdateProfile(ctx context.Context, userID, etag string, profile *userpb.UserProfile) (*userpb.User, error) {
	if userID == "" || etag == "" {
		return nil, ErrUnauthenticated
	}
	updated, err := uc.users.UpdateProfile(ctx, userID, etag, profile)
	if errors.Is(err, ErrMutationMiss) {
		return nil, ErrProfileEtagMismatch
	}
	return updated, err
}

func accountTokenError(err error) error {
	switch {
	case errors.Is(err, ErrExpired):
		return ErrTokenExpired
	case errors.Is(err, ErrNotFound), errors.Is(err, ErrMutationMiss):
		return ErrInvalidToken
	default:
		return fmt.Errorf("consume account token: %w", err)
	}
}

func (uc *AccountUsecase) verificationLink(token string) string {
	return uc.verificationOrigin + "/verify-email#" + token
}
