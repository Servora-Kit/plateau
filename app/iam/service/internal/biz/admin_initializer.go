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

// InitialAdminCreator creates the initial active, email-verified IAM user.
type InitialAdminCreator interface {
	CreateInitialAdmin(context.Context, string, string, string, string, time.Time) error
}

// AdminRelationWriter idempotently grants Plateau administrator membership.
type AdminRelationWriter interface {
	EnsurePlatformAdmin(context.Context, string) error
}

// AdminInitializer creates and authorizes the single configured initial administrator.
type AdminInitializer struct {
	email     string
	users     UserRepo
	creator   InitialAdminCreator
	relations AdminRelationWriter
	log       *slog.Logger
	now       func() time.Time
}

// NewAdminInitializer validates immutable initialization dependencies.
func NewAdminInitializer(config *iamconfpb.IAM, users UserRepo, creator InitialAdminCreator, relations AdminRelationWriter, logger *slog.Logger) (*AdminInitializer, error) {
	if config == nil || users == nil || creator == nil || relations == nil {
		return nil, fmt.Errorf("admin initializer: dependency is nil")
	}
	email := config.GetBootstrapAdminEmail()
	if email == "" {
		return nil, fmt.Errorf("admin initializer: email is empty")
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &AdminInitializer{
		email: email, users: users, creator: creator, relations: relations,
		log: logger.With("scope", "iam/startup"), now: time.Now,
	}, nil
}

// Initialize creates credentials once, emits the initial password once at startup, and ensures the admin tuple on every start.
func (initializer *AdminInitializer) Initialize(ctx context.Context) error {
	displayEmail, canonicalEmail, err := NormalizeEmail(initializer.email)
	if err != nil {
		return fmt.Errorf("normalize bootstrap admin email: %w", err)
	}

	admin, err := initializer.users.FindByEmail(ctx, canonicalEmail)
	switch {
	case err == nil:
		if admin.GetStatus() != userpb.UserStatus_USER_STATUS_ACTIVE || !admin.GetEmailVerified() {
			return fmt.Errorf("bootstrap admin %q exists but is not active and email-verified", displayEmail)
		}
	case errors.Is(err, ErrNotFound):
		admin, err = initializer.create(ctx, displayEmail, canonicalEmail)
		if err != nil {
			if !errors.Is(err, ErrAlreadyExists) {
				return err
			}
			admin, err = initializer.users.FindByEmail(ctx, canonicalEmail)
			if err != nil {
				return fmt.Errorf("load concurrently created bootstrap admin: %w", err)
			}
		}
	default:
		return fmt.Errorf("find bootstrap admin: %w", err)
	}

	if err := initializer.relations.EnsurePlatformAdmin(ctx, admin.GetUserId()); err != nil {
		return fmt.Errorf("ensure bootstrap admin relation: %w", err)
	}
	return nil
}
func (initializer *AdminInitializer) create(ctx context.Context, displayEmail, canonicalEmail string) (*userpb.User, error) {
	userID, err := NewUserID()
	if err != nil {
		return nil, err
	}
	plaintext, _, err := NewOpaqueSecret()
	if err != nil {
		return nil, fmt.Errorf("generate bootstrap admin password: %w", err)
	}
	passwordHash, err := password.Hash(plaintext)
	if err != nil {
		return nil, fmt.Errorf("hash bootstrap admin password: %w", err)
	}
	if err := initializer.creator.CreateInitialAdmin(ctx, userID, displayEmail, canonicalEmail, passwordHash, initializer.now()); err != nil {
		return nil, fmt.Errorf("create bootstrap admin: %w", err)
	}

	// This is intentionally a startup-only credential handoff. Request logging remains redacted.
	initializer.log.WarnContext(ctx, "bootstrap admin created; initial password is emitted once", "email", displayEmail, "initial_password", plaintext)
	return &userpb.User{
		Name: "users/" + userID, UserId: userID, Email: &displayEmail,
		Status: userpb.UserStatus_USER_STATUS_ACTIVE, EmailVerified: true,
	}, nil
}
