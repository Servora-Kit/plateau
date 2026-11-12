package biz

import (
	"context"
	"errors"
	"fmt"
	"time"

	userpb "github.com/Servora-Kit/plateau/api/gen/go/iam/user/v1"
	"github.com/Servora-Kit/plateau/security/password"
	corecrud "github.com/Servora-Kit/servora/core/crud"
	"google.golang.org/protobuf/types/known/fieldmaskpb"
)

var (
	ErrUserInvalidState = errors.New("user lifecycle state is invalid")
	ErrUserEtagMismatch = errors.New("user etag does not match")
)

// DeleteAccountProof binds self-service soft deletion to a verified credential and active login snapshot.
type DeleteAccountProof struct {
	LoginID      string
	PasswordHash string
}

// UserRepo is the user persistence port shared by account, authentication and admin CRUD.
type UserRepo interface {
	Create(context.Context, *userpb.User, string, string) (*userpb.User, error)
	FindByEmail(context.Context, string) (*userpb.User, error)
	FindByEmailIncludingDeleted(context.Context, string) (*userpb.User, error)
	Get(context.Context, string) (*userpb.User, error)
	UpdateProfile(context.Context, string, string, *userpb.UserProfile) (*userpb.User, error)
	UpdateStatus(context.Context, string, string, userpb.UserStatus, time.Time) (*userpb.User, error)
	GetUser(context.Context, userpb.UserName) (*userpb.User, error)
	ListUsers(context.Context, corecrud.ListQuery, bool) (corecrud.ListResult[*userpb.User], error)
	UpdateUser(context.Context, userpb.UserName, *userpb.User, *fieldmaskpb.FieldMask, string) (*userpb.User, error)
	SoftDelete(context.Context, string, *DeleteAccountProof, time.Time) (*userpb.User, error)
	Undelete(context.Context, string, time.Time) (*userpb.User, error)
	Expunge(context.Context, string) error
}

// UserUsecase owns administrator CRUD semantics over the repository port.
type UserUsecase struct {
	account     *AccountUsecase
	users       UserRepo
	credentials CredentialRepo
	sessions    SessionRepo
	now         func() time.Time
}

// NewUserUsecase wires administrator user management and lifecycle side effects.
func NewUserUsecase(account *AccountUsecase, users UserRepo, credentials CredentialRepo, sessions SessionRepo) (*UserUsecase, error) {
	if account == nil || users == nil || credentials == nil || sessions == nil {
		return nil, fmt.Errorf("user: usecase dependency is nil")
	}
	return &UserUsecase{
		account:     account,
		users:       users,
		credentials: credentials,
		sessions:    sessions,
		now:         time.Now,
	}, nil
}

// CreateUser delegates identity creation to AccountUsecase without sending verification mail.
func (uc *UserUsecase) CreateUser(ctx context.Context, name userpb.UserName, prepared corecrud.PreparedCreate[*userpb.User], plaintextPassword string, requirePasswordChange bool) (*userpb.User, error) {
	resource := prepared.Resource()
	return uc.account.CreatePendingUser(ctx, name.User, resource.GetEmail(), plaintextPassword, resource.GetProfile(), requirePasswordChange)
}

// SendUserVerificationEmail sends a verification email for an eligible pending user.
func (uc *UserUsecase) SendUserVerificationEmail(ctx context.Context, name userpb.UserName) error {
	return uc.account.SendUserVerificationEmail(ctx, name.User)
}

func (uc *UserUsecase) GetUser(ctx context.Context, name userpb.UserName) (*userpb.User, error) {
	return uc.users.GetUser(ctx, name)
}

func (uc *UserUsecase) ListUsers(ctx context.Context, query corecrud.ListQuery, showDeleted bool) (corecrud.ListResult[*userpb.User], error) {
	return uc.users.ListUsers(ctx, query, showDeleted)
}

func (uc *UserUsecase) UpdateUser(ctx context.Context, name userpb.UserName, prepared corecrud.PreparedUpdate[*userpb.User]) (*userpb.User, error) {
	if prepared.Options().Etag == "" {
		return nil, ErrUserEtagMismatch
	}
	current, err := uc.users.Get(ctx, name.User)
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
	return uc.updateStatus(ctx, name, expectedEtag, userpb.UserStatus_USER_STATUS_DISABLED)
}

func (uc *UserUsecase) EnableUser(ctx context.Context, name userpb.UserName, expectedEtag string) (*userpb.User, error) {
	return uc.updateStatus(ctx, name, expectedEtag, userpb.UserStatus_USER_STATUS_ACTIVE)
}

func (uc *UserUsecase) updateStatus(ctx context.Context, name userpb.UserName, expectedEtag string, status userpb.UserStatus) (*userpb.User, error) {
	current, err := uc.users.Get(ctx, name.User)
	if err != nil {
		return nil, err
	}
	if status == userpb.UserStatus_USER_STATUS_ACTIVE && current.GetStatus() != userpb.UserStatus_USER_STATUS_DISABLED {
		return nil, ErrUserInvalidState
	}
	if expectedEtag != "" && expectedEtag != current.GetEtag() {
		return nil, ErrUserEtagMismatch
	}

	updated, err := uc.users.UpdateStatus(ctx, name.User, current.GetEtag(), status, uc.now())
	if errors.Is(err, ErrMutationMiss) {
		return nil, ErrUserEtagMismatch
	}
	return updated, err
}

// SetUserPassword replaces the target user's password and revokes all user sessions without altering verification or required actions.
func (uc *UserUsecase) SetUserPassword(ctx context.Context, name userpb.UserName, newPassword string) error {
	if err := ValidatePassword(newPassword); err != nil {
		return err
	}
	if _, err := uc.users.Get(ctx, name.User); err != nil {
		return err
	}
	hash, err := password.Hash(newPassword)
	if err != nil {
		return fmt.Errorf("hash user password: %w", err)
	}
	return uc.credentials.SetPasswordAndRevokeSessions(ctx, name.User, hash, uc.now())
}

// RevokeUserSessions revokes all IAM login sessions and associated OAuth sessions for a non-deleted user.
func (uc *UserUsecase) RevokeUserSessions(ctx context.Context, name userpb.UserName) error {
	return uc.sessions.RevokeAll(ctx, name.User, uc.now())
}

// DeleteUser soft-deletes a non-deleted user by current stored state.
func (uc *UserUsecase) DeleteUser(ctx context.Context, name userpb.UserName) (*userpb.User, error) {
	return uc.users.SoftDelete(ctx, name.User, nil, uc.now())
}

// UndeleteUser restores a soft-deleted user without reviving revoked sessions or consumed tokens.
func (uc *UserUsecase) UndeleteUser(ctx context.Context, name userpb.UserName) (*userpb.User, error) {
	return uc.users.Undelete(ctx, name.User, uc.now())
}

// ExpungeUser permanently deletes a soft-deleted user and all owned IAM/OAuth records.
func (uc *UserUsecase) ExpungeUser(ctx context.Context, name userpb.UserName) error {
	return uc.users.Expunge(ctx, name.User)
}
