package biz

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	userpb "github.com/Servora-Kit/plateau/api/gen/go/iam/user/v1"
	corecrud "github.com/Servora-Kit/servora/core/crud"
	"google.golang.org/protobuf/types/known/fieldmaskpb"
)

var (
	ErrSessionRevocation = errors.New("IAM session revocation failed")
	ErrUserInvalidState  = errors.New("user lifecycle state is invalid")
	ErrUserEtagMismatch  = errors.New("user etag does not match")
)

// UserRepo is the user persistence port shared by account, authentication and admin CRUD.
type UserRepo interface {
	Create(context.Context, *userpb.User, string, string) (*userpb.User, error)
	FindByEmail(context.Context, string) (*userpb.User, error)
	Get(context.Context, string) (*userpb.User, error)
	ActivateEmail(context.Context, string, time.Time) error
	UpdateProfile(context.Context, string, string, *userpb.UserProfile) (*userpb.User, error)
	UpdateStatus(context.Context, string, string, userpb.UserStatus, time.Time) (*userpb.User, error)
	GetUser(context.Context, userpb.UserName) (*userpb.User, error)
	ListUsers(context.Context, corecrud.ListQuery) (corecrud.ListResult[*userpb.User], error)
	UpdateUser(context.Context, userpb.UserName, *userpb.User, *fieldmaskpb.FieldMask, string) (*userpb.User, error)
}

// UserUsecase owns administrator CRUD semantics over the repository port.
type UserUsecase struct {
	account  *AccountUsecase
	sessions *SessionUsecase
	users    UserRepo
	log      *slog.Logger
}

// NewUserUsecase wires administrator user management and lifecycle side effects.
func NewUserUsecase(account *AccountUsecase, users UserRepo, sessions *SessionUsecase, logger *slog.Logger) (*UserUsecase, error) {
	if account == nil || users == nil || sessions == nil {
		return nil, fmt.Errorf("user: usecase dependency is nil")
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &UserUsecase{
		account: account, users: users, sessions: sessions,
		log: logger.With("scope", "iam/biz/user"),
	}, nil
}

// CreateUser delegates identity creation to AccountUsecase and keeps password handling out of CRUD.
func (uc *UserUsecase) CreateUser(ctx context.Context, name userpb.UserName, prepared corecrud.PreparedCreate[*userpb.User], plaintextPassword string) (*userpb.User, error) {
	resource := prepared.Resource()
	return uc.account.CreatePendingUser(ctx, name.User, resource.GetEmail(), plaintextPassword, resource.GetProfile())
}

func (uc *UserUsecase) GetUser(ctx context.Context, name userpb.UserName) (*userpb.User, error) {
	return uc.users.GetUser(ctx, name)
}

func (uc *UserUsecase) ListUsers(ctx context.Context, query corecrud.ListQuery) (corecrud.ListResult[*userpb.User], error) {
	return uc.users.ListUsers(ctx, query)
}

func (uc *UserUsecase) UpdateUser(ctx context.Context, name userpb.UserName, prepared corecrud.PreparedUpdate[*userpb.User]) (*userpb.User, error) {
	if prepared.Options().Etag == "" {
		return nil, ErrUserEtagMismatch
	}
	current, err := uc.users.GetUser(ctx, name)
	if err != nil {
		return nil, err
	}
	if err := prepared.ValidateImmutable(current); err != nil {
		return nil, err
	}
	updated, err := uc.users.UpdateUser(ctx, name, prepared.Resource(), prepared.WriteMask(), prepared.Options().Etag)
	if errors.Is(err, ErrMutationMiss) {
		return nil, ErrUserEtagMismatch
	}
	return updated, err
}

func (uc *UserUsecase) DisableUser(ctx context.Context, name userpb.UserName, expectedEtag string) (*userpb.User, error) {
	return uc.updateStatus(ctx, name, expectedEtag, UserStatusDisabled)
}

func (uc *UserUsecase) EnableUser(ctx context.Context, name userpb.UserName, expectedEtag string) (*userpb.User, error) {
	return uc.updateStatus(ctx, name, expectedEtag, UserStatusActive)
}

func (uc *UserUsecase) updateStatus(ctx context.Context, name userpb.UserName, expectedEtag, status string) (*userpb.User, error) {
	current, err := uc.users.Get(ctx, name.User)
	if err != nil {
		return nil, err
	}
	if status == UserStatusActive && current.GetStatus() != userpb.UserStatus_USER_STATUS_DISABLED {
		return nil, ErrUserInvalidState
	}
	if expectedEtag != "" && expectedEtag != current.GetEtag() {
		return nil, ErrUserEtagMismatch
	}

	now := time.Now()
	updated := current
	if status != UserStatusDisabled || current.GetStatus() != userpb.UserStatus_USER_STATUS_DISABLED {
		updated, err = uc.users.UpdateStatus(ctx, name.User, current.GetEtag(), userStatus(status), now)
		if errors.Is(err, ErrMutationMiss) {
			return nil, ErrUserEtagMismatch
		}
		if err != nil {
			return nil, err
		}
	}
	if status != UserStatusDisabled {
		return updated, nil
	}

	revokeErr := uc.sessions.RevokeAllForUser(ctx, name.User)
	if revokeErr != nil {
		uc.log.ErrorContext(ctx, "revoke disabled user sessions failed", "user_id", name.User, "err", revokeErr)
		return nil, errors.Join(ErrSessionRevocation, revokeErr)
	}
	return updated, nil
}

func userStatus(status string) userpb.UserStatus {
	switch status {
	case UserStatusPendingVerification:
		return userpb.UserStatus_USER_STATUS_PENDING_EMAIL_VERIFICATION
	case UserStatusActive:
		return userpb.UserStatus_USER_STATUS_ACTIVE
	case UserStatusDisabled:
		return userpb.UserStatus_USER_STATUS_DISABLED
	default:
		return userpb.UserStatus_USER_STATUS_UNSPECIFIED
	}
}
