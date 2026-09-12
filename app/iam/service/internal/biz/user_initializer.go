package biz

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	iamconfpb "github.com/Servora-Kit/plateau/api/gen/go/iam/conf/v1"
	userpb "github.com/Servora-Kit/plateau/api/gen/go/iam/user/v1"
	"github.com/Servora-Kit/plateau/security/password"
)

// InitialUserCreator creates the initial active, email-verified IAM user.
type InitialUserCreator interface {
	CreateInitialUser(context.Context, string, string, string, string, time.Time) error
}

// UserInitializer creates the configured initial active user.
type UserInitializer struct {
	email   string
	users   UserRepo
	creator InitialUserCreator
	log     *slog.Logger
	now     func() time.Time
}

// NewUserInitializer validates immutable initialization dependencies.
func NewUserInitializer(config *iamconfpb.IAM, users UserRepo, creator InitialUserCreator, logger *slog.Logger) (*UserInitializer, error) {
	if config == nil || users == nil || creator == nil {
		return nil, fmt.Errorf("user initializer: dependency is nil")
	}
	email := config.GetBootstrapUserEmail()
	if email == "" {
		return nil, fmt.Errorf("user initializer: email is empty")
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &UserInitializer{
		email: email, users: users, creator: creator,
		log: logger.With("scope", "iam/startup"), now: time.Now,
	}, nil
}

// Initialize creates the initial user and emits its password only on first creation.
func (initializer *UserInitializer) Initialize(ctx context.Context) error {
	canonicalEmail, displayEmail, err := NormalizeEmail(initializer.email)
	if err != nil {
		return fmt.Errorf("normalize bootstrap user email: %w", err)
	}

	user, err := initializer.users.FindByEmail(ctx, canonicalEmail)
	switch {
	case err == nil:
	case errors.Is(err, ErrNotFound):
		user, err = initializer.create(ctx, displayEmail, canonicalEmail)
		if err != nil {
			if !errors.Is(err, ErrAlreadyExists) {
				return err
			}
			user, err = initializer.users.FindByEmail(ctx, canonicalEmail)
			if err != nil {
				return fmt.Errorf("load concurrently created bootstrap user: %w", err)
			}
		}
	default:
		return fmt.Errorf("find bootstrap user: %w", err)
	}
	if user.GetStatus() != userpb.UserStatus_USER_STATUS_ACTIVE || !user.GetEmailVerified() {
		return fmt.Errorf("bootstrap user %q exists but is not active and email-verified", displayEmail)
	}
	return nil
}
func (initializer *UserInitializer) create(ctx context.Context, displayEmail, canonicalEmail string) (*userpb.User, error) {
	userID, err := NewUserID()
	if err != nil {
		return nil, err
	}
	plaintext, _, err := NewOpaqueSecret()
	if err != nil {
		return nil, fmt.Errorf("generate bootstrap user password: %w", err)
	}
	passwordHash, err := password.Hash(plaintext)
	if err != nil {
		return nil, fmt.Errorf("hash bootstrap user password: %w", err)
	}
	if err := initializer.creator.CreateInitialUser(ctx, userID, displayEmail, canonicalEmail, passwordHash, initializer.now()); err != nil {
		return nil, fmt.Errorf("create bootstrap user: %w", err)
	}

	// This is intentionally a startup-only credential handoff. Request logging remains redacted.
	initializer.log.WarnContext(ctx, "bootstrap user created; initial password is emitted once", "email", displayEmail, "initial_password", plaintext)
	return &userpb.User{
		Name: "users/" + userID, UserId: userID, Email: &displayEmail,
		Status: userpb.UserStatus_USER_STATUS_ACTIVE, EmailVerified: true,
	}, nil
}
