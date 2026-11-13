package service

import (
	"context"
	"errors"
	"fmt"
	"strings"

	adminuserpb "github.com/Servora-Kit/plateau/api/gen/go/admin/user/v1"
	userpb "github.com/Servora-Kit/plateau/api/gen/go/iam/user/v1"
	securityerrorspb "github.com/Servora-Kit/plateau/api/gen/go/plateau/security/errors/v1"
	"github.com/Servora-Kit/plateau/app/admin/service/internal/biz"
	kerrors "github.com/go-kratos/kratos/v3/errors"
	"google.golang.org/protobuf/types/known/emptypb"
)

// UserService implements admin.user.v1.UserServiceServer.
type UserService struct {
	adminuserpb.UnimplementedUserServiceServer
	uc *biz.UserUsecase
}

// NewUserService constructs UserService with resource name parsing and domain error mapping.
func NewUserService(uc *biz.UserUsecase) (*UserService, error) {
	if uc == nil {
		return nil, fmt.Errorf("admin user service: usecase is nil")
	}
	return &UserService{uc: uc}, nil
}

func (s *UserService) ListUsers(ctx context.Context, req *adminuserpb.ListUsersRequest) (*adminuserpb.ListUsersResponse, error) {
	resp, err := s.uc.ListUsers(ctx, req)
	if err != nil {
		return nil, userError(err)
	}
	return resp, nil
}

func (s *UserService) GetUser(ctx context.Context, req *adminuserpb.GetUserRequest) (*userpb.User, error) {
	name, err := userpb.ParseUserName(req.GetName())
	if err != nil {
		return nil, err
	}
	user, err := s.uc.GetUser(ctx, name)
	if err != nil {
		return nil, userError(err)
	}
	return user, nil
}

func (s *UserService) CreateUser(ctx context.Context, req *adminuserpb.CreateUserRequest) (*adminuserpb.CreateUserResponse, error) {
	if req.GetUser() == nil || strings.TrimSpace(req.GetUser().GetEmail()) == "" {
		return nil, kerrors.BadRequest("INVALID_ARGUMENT", "user.email is required")
	}
	user, sent, err := s.uc.CreateUser(ctx, req)
	if err != nil {
		return nil, userError(err)
	}
	return &adminuserpb.CreateUserResponse{
		User:                  user,
		VerificationEmailSent: sent,
	}, nil
}

func (s *UserService) SendUserVerificationEmail(ctx context.Context, req *adminuserpb.SendUserVerificationEmailRequest) (*adminuserpb.SendUserVerificationEmailResponse, error) {
	name, err := userpb.ParseUserName(req.GetName())
	if err != nil {
		return nil, err
	}
	if err := s.uc.SendUserVerificationEmail(ctx, name); err != nil {
		return nil, userError(err)
	}
	return &adminuserpb.SendUserVerificationEmailResponse{}, nil
}

func (s *UserService) UpdateUser(ctx context.Context, req *adminuserpb.UpdateUserRequest) (*userpb.User, error) {
	if req.GetUser() == nil {
		return nil, kerrors.BadRequest("INVALID_ARGUMENT", "user is required")
	}
	name, err := userpb.ParseUserName(req.GetUser().GetName())
	if err != nil {
		return nil, err
	}
	user, err := s.uc.UpdateUser(ctx, name, req.GetUser(), req.GetUpdateMask())
	if err != nil {
		return nil, userError(err)
	}
	return user, nil
}

func (s *UserService) SetUserPassword(ctx context.Context, req *adminuserpb.SetUserPasswordRequest) (*adminuserpb.SetUserPasswordResponse, error) {
	name, err := userpb.ParseUserName(req.GetName())
	if err != nil {
		return nil, err
	}
	if err := s.uc.SetUserPassword(ctx, name, req.GetNewPassword()); err != nil {
		return nil, userError(err)
	}
	return &adminuserpb.SetUserPasswordResponse{}, nil
}

func (s *UserService) RevokeUserSessions(ctx context.Context, req *adminuserpb.RevokeUserSessionsRequest) (*adminuserpb.RevokeUserSessionsResponse, error) {
	name, err := userpb.ParseUserName(req.GetName())
	if err != nil {
		return nil, err
	}
	if err := s.uc.RevokeUserSessions(ctx, name); err != nil {
		return nil, userError(err)
	}
	return &adminuserpb.RevokeUserSessionsResponse{}, nil
}

func (s *UserService) DisableUser(ctx context.Context, req *adminuserpb.DisableUserRequest) (*userpb.User, error) {
	name, err := userpb.ParseUserName(req.GetName())
	if err != nil {
		return nil, err
	}
	user, err := s.uc.DisableUser(ctx, name, req.GetReason(), req.GetEtag())
	if err != nil {
		return nil, userError(err)
	}
	return user, nil
}

func (s *UserService) EnableUser(ctx context.Context, req *adminuserpb.EnableUserRequest) (*userpb.User, error) {
	name, err := userpb.ParseUserName(req.GetName())
	if err != nil {
		return nil, err
	}
	user, err := s.uc.EnableUser(ctx, name, req.GetEtag())
	if err != nil {
		return nil, userError(err)
	}
	return user, nil
}

func (s *UserService) DeleteUser(ctx context.Context, req *adminuserpb.DeleteUserRequest) (*userpb.User, error) {
	name, err := userpb.ParseUserName(req.GetName())
	if err != nil {
		return nil, err
	}
	user, err := s.uc.DeleteUser(ctx, name)
	if err != nil {
		return nil, userError(err)
	}
	return user, nil
}

func (s *UserService) UndeleteUser(ctx context.Context, req *adminuserpb.UndeleteUserRequest) (*userpb.User, error) {
	name, err := userpb.ParseUserName(req.GetName())
	if err != nil {
		return nil, err
	}
	user, err := s.uc.UndeleteUser(ctx, name)
	if err != nil {
		return nil, userError(err)
	}
	return user, nil
}

func (s *UserService) ExpungeUser(ctx context.Context, req *adminuserpb.ExpungeUserRequest) (*emptypb.Empty, error) {
	name, err := userpb.ParseUserName(req.GetName())
	if err != nil {
		return nil, err
	}
	if err := s.uc.ExpungeUser(ctx, name); err != nil {
		return nil, userError(err)
	}
	return &emptypb.Empty{}, nil
}

func userError(err error) error {
	switch {
	case errors.Is(err, context.Canceled):
		return kerrors.ClientClosed("CLIENT_CLOSED", "client closed request").WithCause(err)
	case errors.Is(err, context.DeadlineExceeded):
		return kerrors.GatewayTimeout("GATEWAY_TIMEOUT", "request deadline exceeded").WithCause(err)
	case errors.Is(err, biz.ErrUnauthenticated):
		return securityerrorspb.ErrorSecurityErrorReasonUnauthenticated("authentication required").WithCause(err)
	case errors.Is(err, biz.ErrSelfActionForbidden):
		return adminuserpb.ErrorAdminUserErrorReasonSelfActionForbidden("cannot disable or delete your own account").WithCause(err)
	case errors.Is(err, biz.ErrTargetIsAdmin):
		return adminuserpb.ErrorAdminUserErrorReasonTargetIsAdmin("cannot disable or delete an administrator account").WithCause(err)
	case errors.Is(err, biz.ErrInvalidUpdateMask):
		return kerrors.BadRequest("INVALID_ARGUMENT", "update_mask must be non-empty and contain only editable profile fields").WithCause(err)
	case errors.Is(err, biz.ErrUserNotFound):
		return adminuserpb.ErrorAdminUserErrorReasonUserNotFound("user not found").WithCause(err)
	case errors.Is(err, biz.ErrUserAlreadyExists):
		return adminuserpb.ErrorAdminUserErrorReasonUserAlreadyExists("user already exists or email is already registered").WithCause(err)
	case errors.Is(err, biz.ErrUserInvalidState):
		return adminuserpb.ErrorAdminUserErrorReasonInvalidState("user lifecycle state does not allow this operation").WithCause(err)
	case errors.Is(err, biz.ErrUserEtagMismatch):
		return adminuserpb.ErrorAdminUserErrorReasonEtagMismatch("user etag does not match").WithCause(err)
	case errors.Is(err, biz.ErrVerificationEmailFailed):
		return adminuserpb.ErrorAdminUserErrorReasonVerificationEmailFailed("verification email delivery failed").WithCause(err)
	case errors.Is(err, biz.ErrDownstreamUnavailable):
		return adminuserpb.ErrorAdminUserErrorReasonDownstreamUnavailable("downstream identity service is temporarily unavailable").WithCause(err)
	}
	if _, ok := errors.AsType[*kerrors.Error](err); ok {
		return err
	}
	return adminuserpb.ErrorAdminUserErrorReasonDownstreamUnavailable("downstream identity service is temporarily unavailable").WithCause(err)
}
