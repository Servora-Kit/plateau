package biz

import (
	"context"
	"fmt"
	"log/slog"

	adminuserpb "github.com/Servora-Kit/plateau/api/gen/go/admin/user/v1"
	userpb "github.com/Servora-Kit/plateau/api/gen/go/iam/user/v1"
	"github.com/Servora-Kit/plateau/security"
	"github.com/Servora-Kit/plateau/security/authz/openfga"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/fieldmaskpb"
)

var allowedProfileMaskPaths = map[string]struct{}{
	"profile.name":               {},
	"profile.given_name":         {},
	"profile.family_name":        {},
	"profile.nickname":           {},
	"profile.preferred_username": {},
	"profile.picture":            {},
	"profile.locale":             {},
}

// IAMUserRepo abstracts downstream IAM UserService gRPC operations.
type IAMUserRepo interface {
	ListUsers(ctx context.Context, req *userpb.ListUsersRequest) (*userpb.ListUsersResponse, error)
	GetUser(ctx context.Context, req *userpb.GetUserRequest) (*userpb.User, error)
	CreateUser(ctx context.Context, req *userpb.CreateUserRequest) (*userpb.User, error)
	SendUserVerificationEmail(ctx context.Context, req *userpb.SendUserVerificationEmailRequest) error
	UpdateUser(ctx context.Context, req *userpb.UpdateUserRequest) (*userpb.User, error)
	SetUserPassword(ctx context.Context, req *userpb.SetUserPasswordRequest) error
	RevokeUserSessions(ctx context.Context, req *userpb.RevokeUserSessionsRequest) error
	DisableUser(ctx context.Context, req *userpb.DisableUserRequest) (*userpb.User, error)
	EnableUser(ctx context.Context, req *userpb.EnableUserRequest) (*userpb.User, error)
	DeleteUser(ctx context.Context, req *userpb.DeleteUserRequest) (*userpb.User, error)
	UndeleteUser(ctx context.Context, req *userpb.UndeleteUserRequest) (*userpb.User, error)
	ExpungeUser(ctx context.Context, req *userpb.ExpungeUserRequest) error
}

// UserUsecase orchestrates IAM user management with Admin self-protection and fellow-admin guards.
type UserUsecase struct {
	repo       IAMUserRepo
	authorizer *openfga.Authorizer
	log        *slog.Logger
}

// NewUserUsecase constructs UserUsecase with IAM gRPC repository and OpenFGA authorizer.
func NewUserUsecase(repo IAMUserRepo, authorizer *openfga.Authorizer, l *slog.Logger) (*UserUsecase, error) {
	if repo == nil || authorizer == nil {
		return nil, fmt.Errorf("admin user usecase: dependency is nil")
	}
	if l == nil {
		l = slog.Default()
	}
	return &UserUsecase{
		repo:       repo,
		authorizer: authorizer,
		log:        l.With("scope", "admin/biz/user"),
	}, nil
}

func (uc *UserUsecase) ListUsers(ctx context.Context, req *adminuserpb.ListUsersRequest) (*adminuserpb.ListUsersResponse, error) {
	resp, err := uc.repo.ListUsers(ctx, &userpb.ListUsersRequest{
		PageSize:     req.GetPageSize(),
		PageToken:    req.GetPageToken(),
		Filter:       req.GetFilter(),
		OrderBy:      req.GetOrderBy(),
		ShowDeleted:  req.GetShowDeleted(),
		IncludeTotal: req.GetIncludeTotal(),
	})
	if err != nil {
		return nil, err
	}
	return &adminuserpb.ListUsersResponse{
		Users:         resp.GetUsers(),
		NextPageToken: resp.GetNextPageToken(),
		TotalSize:     resp.TotalSize,
	}, nil
}

func (uc *UserUsecase) GetUser(ctx context.Context, name userpb.UserName) (*userpb.User, error) {
	return uc.repo.GetUser(ctx, &userpb.GetUserRequest{Name: "users/" + name.User})
}

func (uc *UserUsecase) CreateUser(ctx context.Context, req *adminuserpb.CreateUserRequest) (*userpb.User, bool, error) {
	created, err := uc.repo.CreateUser(ctx, &userpb.CreateUserRequest{
		UserId:                req.GetUserId(),
		User:                  req.GetUser(),
		Password:              req.GetPassword(),
		RequirePasswordChange: req.RequirePasswordChange,
	})
	if err != nil {
		return nil, false, err
	}

	if err := uc.repo.SendUserVerificationEmail(ctx, &userpb.SendUserVerificationEmailRequest{
		Name: created.GetName(),
	}); err != nil {
		reason := adminuserpb.AdminUserErrorReason_ADMIN_USER_ERROR_REASON_DOWNSTREAM_UNAVAILABLE.String()
		if userpb.IsUserErrorReasonVerificationEmailFailed(err) {
			reason = userpb.UserErrorReason_USER_ERROR_REASON_VERIFICATION_EMAIL_FAILED.String()
		}
		uc.log.WarnContext(ctx, "verification email delivery failed after user creation", "user_name", created.GetName(), "reason", reason)
		return created, false, nil
	}

	return created, true, nil
}

func (uc *UserUsecase) SendUserVerificationEmail(ctx context.Context, name userpb.UserName) error {
	return uc.repo.SendUserVerificationEmail(ctx, &userpb.SendUserVerificationEmailRequest{Name: "users/" + name.User})
}

func (uc *UserUsecase) UpdateUser(ctx context.Context, name userpb.UserName, user *userpb.User, mask *fieldmaskpb.FieldMask) (*userpb.User, error) {
	paths := mask.GetPaths()
	if len(paths) == 0 {
		return nil, ErrInvalidUpdateMask
	}
	normalized := make([]string, 0, len(paths))
	seen := make(map[string]struct{}, len(paths))
	for _, path := range paths {
		if _, ok := allowedProfileMaskPaths[path]; !ok {
			return nil, ErrInvalidUpdateMask
		}
		if _, dup := seen[path]; !dup {
			seen[path] = struct{}{}
			normalized = append(normalized, path)
		}
	}

	sanitized := proto.Clone(user).(*userpb.User)
	sanitized.Name = "users/" + name.User
	sanitized.Email = nil

	return uc.repo.UpdateUser(ctx, &userpb.UpdateUserRequest{
		User:       sanitized,
		UpdateMask: &fieldmaskpb.FieldMask{Paths: normalized},
	})
}

func (uc *UserUsecase) SetUserPassword(ctx context.Context, name userpb.UserName, newPassword string) error {
	return uc.repo.SetUserPassword(ctx, &userpb.SetUserPasswordRequest{
		Name:        "users/" + name.User,
		NewPassword: newPassword,
	})
}

func (uc *UserUsecase) RevokeUserSessions(ctx context.Context, name userpb.UserName) error {
	return uc.repo.RevokeUserSessions(ctx, &userpb.RevokeUserSessionsRequest{Name: "users/" + name.User})
}

func (uc *UserUsecase) DisableUser(ctx context.Context, name userpb.UserName, reason, etag string) (*userpb.User, error) {
	if err := uc.ensureTargetNotProtected(ctx, name.User); err != nil {
		return nil, err
	}
	return uc.repo.DisableUser(ctx, &userpb.DisableUserRequest{
		Name:   "users/" + name.User,
		Reason: reason,
		Etag:   etag,
	})
}

func (uc *UserUsecase) EnableUser(ctx context.Context, name userpb.UserName, etag string) (*userpb.User, error) {
	return uc.repo.EnableUser(ctx, &userpb.EnableUserRequest{
		Name: "users/" + name.User,
		Etag: etag,
	})
}

func (uc *UserUsecase) DeleteUser(ctx context.Context, name userpb.UserName) (*userpb.User, error) {
	if err := uc.ensureTargetNotProtected(ctx, name.User); err != nil {
		return nil, err
	}
	return uc.repo.DeleteUser(ctx, &userpb.DeleteUserRequest{Name: "users/" + name.User})
}

func (uc *UserUsecase) UndeleteUser(ctx context.Context, name userpb.UserName) (*userpb.User, error) {
	return uc.repo.UndeleteUser(ctx, &userpb.UndeleteUserRequest{Name: "users/" + name.User})
}

func (uc *UserUsecase) ExpungeUser(ctx context.Context, name userpb.UserName) error {
	if err := uc.ensureTargetNotProtected(ctx, name.User); err != nil {
		return err
	}
	return uc.repo.ExpungeUser(ctx, &userpb.ExpungeUserRequest{Name: "users/" + name.User})
}

func (uc *UserUsecase) ensureTargetNotProtected(ctx context.Context, targetUserID string) error {
	actor, ok := security.ActorFrom(ctx)
	if !ok || !actor.Valid() || actor.Type != security.ActorTypeHuman {
		return ErrUnauthenticated
	}
	if actor.ID == targetUserID {
		return ErrSelfActionForbidden
	}
	isAdmin, err := uc.authorizer.Check(
		ctx,
		security.Actor{Type: security.ActorTypeHuman, ID: targetUserID},
		RelationAdmin,
		AdminResourceType,
		AdminResourceID,
	)
	if err != nil {
		return err
	}
	if isAdmin {
		return ErrTargetIsAdmin
	}
	return nil
}
