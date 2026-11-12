package biz

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	userpb "github.com/Servora-Kit/plateau/api/gen/go/iam/user/v1"
	"github.com/Servora-Kit/plateau/security/password"
	corepb "github.com/Servora-Kit/servora/api/gen/go/servora/core/v1"
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

// PasswordResetProof captures the pre-commit snapshot verified before hashing a replacement password.
type PasswordResetProof struct {
	TokenID                string
	TokenHash              string
	UserEtag               string
	Credential             PasswordCredential
	RequiresPasswordChange bool
}

type VerificationTokenRepo interface {
	Create(context.Context, string, string, string, time.Time) error
	ConsumeAndActivate(context.Context, string, time.Time) (string, error)
}

type PasswordResetTokenRepo interface {
	Create(context.Context, string, string, time.Time) error
	Peek(context.Context, string, time.Time) (*PasswordResetProof, error)
	ConsumeAndReplacePassword(context.Context, PasswordResetProof, string, time.Time) (string, error)
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
	verificationOrigin string
	now                func() time.Time
}

// NewAccountUsecase wires public account lifecycle, credential changes, recovery and mail delivery.
func NewAccountUsecase(users UserRepo, credentials CredentialRepo, verificationTokens VerificationTokenRepo, resetTokens PasswordResetTokenRepo, captcha CAPVerifier, mailer MailSender, app *corepb.App) (*AccountUsecase, error) {
	if users == nil || credentials == nil || verificationTokens == nil || resetTokens == nil || captcha == nil || mailer == nil {
		return nil, fmt.Errorf("account: dependency is nil")
	}
	verificationOrigin := strings.TrimRight(strings.TrimSpace(app.GetExternalUrl()), "/")
	if verificationOrigin == "" {
		return nil, fmt.Errorf("account: verification origin is empty")
	}
	return &AccountUsecase{
		users: users, credentials: credentials, verificationTokens: verificationTokens, resetTokens: resetTokens,
		cap: captcha, mailer: mailer,
		verificationOrigin: verificationOrigin, now: time.Now,
	}, nil
}

// Register consumes CAP before creating a pending User and sending a verification email.
func (uc *AccountUsecase) Register(ctx context.Context, email, plaintextPassword, capToken string, profile *userpb.UserProfile) (*userpb.User, error) {
	valid, err := uc.cap.ValidateToken(ctx, capToken)
	if err != nil {
		return nil, fmt.Errorf("validate registration CAP: %w", err)
	}
	if !valid {
		return nil, ErrInvalidToken
	}
	user, err := uc.CreatePendingUser(ctx, "", email, plaintextPassword, profile, false)
	if err != nil {
		return nil, err
	}
	if err := uc.sendVerificationEmail(ctx, user); err != nil {
		return nil, err
	}
	return user, nil
}

// CreatePendingUser creates the pending User aggregate without generating tokens or sending mail.
func (uc *AccountUsecase) CreatePendingUser(ctx context.Context, userID, email, plaintextPassword string, profile *userpb.UserProfile, requirePasswordChange bool) (*userpb.User, error) {
	canonical, display, err := NormalizeEmail(email)
	if err != nil {
		return nil, err
	}
	if err := ValidatePassword(plaintextPassword); err != nil {
		return nil, err
	}
	if _, err := uc.users.FindByEmailIncludingDeleted(ctx, canonical); err == nil {
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
	var actions []userpb.UserRequiredAction
	if requirePasswordChange {
		actions = []userpb.UserRequiredAction{userpb.UserRequiredAction_USER_REQUIRED_ACTION_UPDATE_PASSWORD}
	}
	emailValue := display
	resource := &userpb.User{
		Name:            "users/" + userID,
		UserId:          userID,
		Email:           &emailValue,
		Profile:         profile,
		Status:          userpb.UserStatus_USER_STATUS_PENDING_EMAIL_VERIFICATION,
		RequiredActions: actions,
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
	return user, nil
}

// SendUserVerificationEmail sends a verification email for an administrator-created or pending user.
func (uc *AccountUsecase) SendUserVerificationEmail(ctx context.Context, userID string) error {
	if strings.TrimSpace(userID) == "" {
		return ErrNotFound
	}
	user, err := uc.users.Get(ctx, userID)
	if err != nil {
		return err
	}
	if user.GetStatus() != userpb.UserStatus_USER_STATUS_PENDING_EMAIL_VERIFICATION || user.GetEmailVerified() {
		return ErrUserInvalidState
	}
	if err := uc.sendVerificationEmail(ctx, user); err != nil {
		switch {
		case errors.Is(err, ErrNotFound):
			return ErrNotFound
		case errors.Is(err, ErrUserInvalidState):
			return ErrUserInvalidState
		default:
			return fmt.Errorf("%w: %w", ErrVerificationEmailFailed, err)
		}
	}
	return nil
}

func (uc *AccountUsecase) sendVerificationEmail(ctx context.Context, user *userpb.User) error {
	token, _, err := NewOpaqueSecret()
	if err != nil {
		return err
	}
	now := uc.now()
	if err := uc.verificationTokens.Create(ctx, user.GetUserId(), "", HashOpaqueSecret(token), now.Add(EmailVerificationTTL)); err != nil {
		return fmt.Errorf("create verification token: %w", err)
	}
	if err := uc.mailer.SendVerification(ctx, user.GetEmail(), uc.verificationLink(token), int(EmailVerificationTTL/time.Hour)); err != nil {
		return fmt.Errorf("send verification email: %w", err)
	}
	return nil
}

// VerifyEmail atomically consumes one token and activates the pending User.
func (uc *AccountUsecase) VerifyEmail(ctx context.Context, token string) (*userpb.User, error) {
	if token == "" {
		return nil, ErrInvalidToken
	}
	userID, err := uc.verificationTokens.ConsumeAndActivate(ctx, HashOpaqueSecret(token), uc.now())
	if err != nil {
		return nil, accountTokenError(err)
	}
	user, err := uc.users.Get(ctx, userID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil, ErrInvalidToken
		}
		return nil, err
	}
	return user, nil
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
	if err := uc.sendVerificationEmail(ctx, user); err != nil {
		if errors.Is(err, ErrNotFound) || errors.Is(err, ErrUserInvalidState) {
			return nil
		}
		return err
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
		if errors.Is(err, ErrNotFound) || errors.Is(err, ErrUserInvalidState) || errors.Is(err, ErrUserNotActive) {
			return nil
		}
		return fmt.Errorf("create password reset token: %w", err)
	}
	if err := uc.mailer.SendPasswordReset(ctx, user.GetEmail(), uc.verificationOrigin+"/reset-password#"+token, int(PasswordResetTTL/time.Hour)); err != nil {
		return fmt.Errorf("send password reset email: %w", err)
	}
	return nil
}

// ConfirmPasswordReset peeks a reset token snapshot, enforces initial-password change if required, and atomically commits.
func (uc *AccountUsecase) ConfirmPasswordReset(ctx context.Context, token, newPassword string) error {
	if token == "" {
		return ErrInvalidToken
	}
	if err := ValidatePassword(newPassword); err != nil {
		return err
	}
	now := uc.now()
	proof, err := uc.resetTokens.Peek(ctx, HashOpaqueSecret(token), now)
	if err != nil {
		return accountTokenError(err)
	}
	if proof.RequiresPasswordChange {
		same, _, err := password.Compare(newPassword, proof.Credential.PasswordHash)
		if err != nil {
			return fmt.Errorf("compare reset password: %w", err)
		}
		if same {
			return ErrInvalidPassword
		}
	}
	hash, err := password.Hash(newPassword)
	if err != nil {
		return ErrInvalidPassword
	}
	if _, err := uc.resetTokens.ConsumeAndReplacePassword(ctx, *proof, hash, uc.now()); err != nil {
		return accountTokenError(err)
	}
	return nil
}

// ChangePassword replaces the current password, preserves the current Login Session and revokes all OAuth Token Sessions.
func (uc *AccountUsecase) ChangePassword(ctx context.Context, userID, currentSessionID, currentPassword, newPassword string) error {
	if userID == "" || currentSessionID == "" {
		return ErrUnauthenticated
	}
	if err := ValidatePassword(newPassword); err != nil {
		return err
	}
	credential, err := uc.credentials.FindActivePassword(ctx, userID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return ErrUnauthenticated
		}
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
	if err := uc.credentials.ReplacePassword(ctx, userID, credential.AuthenticatorID, credential.PasswordHash, hash, currentSessionID, uc.now()); err != nil {
		return fmt.Errorf("replace password: %w", err)
	}
	return nil
}

// ChangeInitialPassword verifies the initial password without a prior session, clears UPDATE_PASSWORD, and creates a normal login session.
func (uc *AccountUsecase) ChangeInitialPassword(ctx context.Context, email, currentPassword, newPassword string) (*LoginSession, error) {
	canonical, _, err := NormalizeEmail(email)
	if err != nil || currentPassword == "" {
		return nil, ErrUnauthenticated
	}
	user, err := uc.users.FindByEmail(ctx, canonical)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil, ErrUnauthenticated
		}
		return nil, fmt.Errorf("find initial-password user: %w", err)
	}
	if user.GetStatus() != userpb.UserStatus_USER_STATUS_ACTIVE || !user.GetEmailVerified() {
		return nil, ErrUnauthenticated
	}
	credential, err := uc.credentials.FindActivePassword(ctx, user.GetUserId())
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil, ErrUnauthenticated
		}
		return nil, fmt.Errorf("find initial-password credential: %w", err)
	}
	match, _, err := password.Compare(currentPassword, credential.PasswordHash)
	if err != nil || !match {
		return nil, ErrUnauthenticated
	}
	if !slices.Contains(user.GetRequiredActions(), userpb.UserRequiredAction_USER_REQUIRED_ACTION_UPDATE_PASSWORD) || len(user.GetRequiredActions()) != 1 {
		return nil, ErrAccountInvalidState
	}
	if err := ValidatePassword(newPassword); err != nil {
		return nil, err
	}
	if newPassword == currentPassword {
		return nil, ErrInvalidPassword
	}
	same, _, err := password.Compare(newPassword, credential.PasswordHash)
	if err != nil {
		return nil, fmt.Errorf("compare new initial password: %w", err)
	}
	if same {
		return nil, ErrInvalidPassword
	}
	expected := *credential
	newHash, err := password.Hash(newPassword)
	if err != nil {
		return nil, fmt.Errorf("hash new initial password: %w", err)
	}
	login, err := uc.credentials.CompleteInitialPassword(ctx, expected, user.GetEtag(), newHash, uc.now())
	if err != nil {
		switch {
		case errors.Is(err, ErrNotFound), errors.Is(err, ErrUserDisabled), errors.Is(err, ErrUserNotActive), errors.Is(err, ErrInvalidCredentials):
			return nil, ErrUnauthenticated
		case errors.Is(err, ErrAccountInvalidState), errors.Is(err, ErrUserEtagMismatch), errors.Is(err, ErrMutationMiss):
			return nil, ErrAccountInvalidState
		default:
			return nil, err
		}
	}
	return login, nil
}

// DeleteAccount verifies the authenticated user's current password before soft-deleting the account.
func (uc *AccountUsecase) DeleteAccount(ctx context.Context, userID, loginID, currentPassword string) error {
	if strings.TrimSpace(userID) == "" || strings.TrimSpace(loginID) == "" {
		return ErrUnauthenticated
	}
	if currentPassword == "" {
		return ErrInvalidCredentials
	}
	user, err := uc.users.Get(ctx, userID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return ErrUnauthenticated
		}
		return fmt.Errorf("find account user: %w", err)
	}
	if user.GetStatus() != userpb.UserStatus_USER_STATUS_ACTIVE || !user.GetEmailVerified() || len(user.GetRequiredActions()) > 0 {
		return ErrUnauthenticated
	}
	credential, err := uc.credentials.FindActivePassword(ctx, userID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return ErrInvalidCredentials
		}
		return fmt.Errorf("find account password: %w", err)
	}
	match, _, err := password.Compare(currentPassword, credential.PasswordHash)
	if err != nil || !match {
		return ErrInvalidCredentials
	}
	_, err = uc.users.SoftDelete(ctx, userID, &DeleteAccountProof{
		LoginID:      loginID,
		PasswordHash: credential.PasswordHash,
	}, uc.now())
	return err
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

func (uc *AccountUsecase) verificationLink(token string) string {
	return uc.verificationOrigin + "/verify-email#" + token
}

func accountTokenError(err error) error {
	switch {
	case errors.Is(err, ErrExpired), errors.Is(err, ErrTokenExpired):
		return ErrTokenExpired
	case errors.Is(err, ErrInvalidToken), errors.Is(err, ErrNotFound), errors.Is(err, ErrMutationMiss):
		return ErrInvalidToken
	case errors.Is(err, ErrAccountInvalidState), errors.Is(err, ErrUserEtagMismatch):
		return ErrAccountInvalidState
	default:
		return fmt.Errorf("consume account token: %w", err)
	}
}
