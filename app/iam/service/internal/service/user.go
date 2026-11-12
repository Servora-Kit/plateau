package service

import (
	"context"
	"errors"
	"fmt"
	"strings"

	userpb "github.com/Servora-Kit/plateau/api/gen/go/iam/user/v1"
	"github.com/Servora-Kit/plateau/app/iam/service/internal/biz"
	crudpb "github.com/Servora-Kit/servora/api/gen/go/servora/crud/v1"
	corecrud "github.com/Servora-Kit/servora/core/crud"
	kerrors "github.com/go-kratos/kratos/v3/errors"
	"google.golang.org/protobuf/types/known/emptypb"
)

// UserService exposes administrator-only global user management.
type UserService struct {
	userpb.UnimplementedUserServiceServer
	usecase      *biz.UserUsecase
	plan         *corecrud.ResourcePlan[*userpb.User]
	listPreparer *corecrud.ListPreparer
}

func NewUserService(usecase *biz.UserUsecase) (*UserService, error) {
	if usecase == nil {
		return nil, fmt.Errorf("user service: dependency is missing")
	}
	listPreparer, err := corecrud.NewListPreparer()
	if err != nil {
		return nil, fmt.Errorf("create User ListPreparer: %w", err)
	}
	return &UserService{
		usecase:      usecase,
		plan:         corecrud.MustBuildResourcePlan[*userpb.User](userpb.UserCRUDDescriptor()),
		listPreparer: listPreparer,
	}, nil
}

func (s *UserService) CreateUser(ctx context.Context, request *userpb.CreateUserRequest) (*userpb.User, error) {
	if request.GetUser() == nil {
		return nil, kerrors.New(400, "INVALID_ARGUMENT", "user is required")
	}
	userID := strings.TrimSpace(request.GetUserId())
	if userID == "" {
		var err error
		userID, err = biz.NewUserID()
		if err != nil {
			return nil, kerrors.InternalServer("IAM user id generation failed", "user id generation failed").WithCause(err)
		}
	}
	name := userpb.NewUserName(userID)
	if err := name.Validate(); err != nil {
		return nil, crudpb.ErrorCrudErrorReasonInvalidResourceName("user_id: %v", err)
	}
	prepared, err := s.plan.PrepareCreate(request.GetUser())
	if err != nil {
		return nil, err
	}
	requirePasswordChange := request.RequirePasswordChange == nil || request.GetRequirePasswordChange()
	resource, err := s.usecase.CreateUser(ctx, name, prepared, request.GetPassword(), requirePasswordChange)
	if err != nil {
		return nil, userError(err)
	}
	return s.plan.ToResponse(resource)
}

func (s *UserService) GetUser(ctx context.Context, request *userpb.GetUserRequest) (*userpb.User, error) {
	name, err := s.parseUserName(request.GetName())
	if err != nil {
		return nil, err
	}
	resource, err := s.usecase.GetUser(ctx, name)
	if err != nil {
		return nil, userError(err)
	}
	return s.plan.ToResponse(resource)
}

func (s *UserService) ListUsers(ctx context.Context, request *userpb.ListUsersRequest) (*userpb.ListUsersResponse, error) {
	if request == nil {
		request = new(userpb.ListUsersRequest)
	}
	query, err := s.listPreparer.PrepareList(s.plan, corecrud.ListInput{
		Collection: "users", PageSize: request.GetPageSize(), PageToken: request.GetPageToken(),
		Filter: request.GetFilter(), OrderBy: request.GetOrderBy(),
	})
	if err != nil {
		return nil, err
	}
	result, err := s.usecase.ListUsers(ctx, query, request.GetShowDeleted())
	if err != nil {
		return nil, userError(err)
	}
	resources, err := s.plan.ToResponses(result.Items())
	if err != nil {
		return nil, err
	}
	return &userpb.ListUsersResponse{Users: resources, NextPageToken: result.NextPageToken()}, nil
}

func (s *UserService) UpdateUser(ctx context.Context, request *userpb.UpdateUserRequest) (*userpb.User, error) {
	if request.GetUser() == nil {
		return nil, kerrors.New(400, "INVALID_ARGUMENT", "user is required")
	}
	name, err := s.parseUserName(request.GetUser().GetName())
	if err != nil {
		return nil, err
	}
	prepared, err := s.plan.PrepareUpdate(request.GetUser(), request.GetUpdateMask(), corecrud.UpdateOptions{Etag: request.GetUser().GetEtag()})
	if err != nil {
		return nil, err
	}
	resource, err := s.usecase.UpdateUser(ctx, name, prepared)
	if err != nil {
		return nil, userError(err)
	}
	return s.plan.ToResponse(resource)
}

func (s *UserService) DisableUser(ctx context.Context, request *userpb.DisableUserRequest) (*userpb.DisableUserResponse, error) {
	name, err := s.parseUserName(request.GetName())
	if err != nil {
		return nil, err
	}
	resource, err := s.usecase.DisableUser(ctx, name, request.GetEtag())
	if err != nil {
		return nil, userError(err)
	}
	resource, err = s.plan.ToResponse(resource)
	if err != nil {
		return nil, err
	}
	return &userpb.DisableUserResponse{User: resource}, nil
}

func (s *UserService) EnableUser(ctx context.Context, request *userpb.EnableUserRequest) (*userpb.EnableUserResponse, error) {
	name, err := s.parseUserName(request.GetName())
	if err != nil {
		return nil, err
	}
	resource, err := s.usecase.EnableUser(ctx, name, request.GetEtag())
	if err != nil {
		return nil, userError(err)
	}
	resource, err = s.plan.ToResponse(resource)
	if err != nil {
		return nil, err
	}
	return &userpb.EnableUserResponse{User: resource}, nil
}

func (s *UserService) SendUserVerificationEmail(ctx context.Context, request *userpb.SendUserVerificationEmailRequest) (*userpb.SendUserVerificationEmailResponse, error) {
	name, err := s.parseUserName(request.GetName())
	if err != nil {
		return nil, err
	}
	if err := s.usecase.SendUserVerificationEmail(ctx, name); err != nil {
		return nil, userError(err)
	}
	return &userpb.SendUserVerificationEmailResponse{}, nil
}

func (s *UserService) SetUserPassword(ctx context.Context, request *userpb.SetUserPasswordRequest) (*userpb.SetUserPasswordResponse, error) {
	name, err := s.parseUserName(request.GetName())
	if err != nil {
		return nil, err
	}
	if err := s.usecase.SetUserPassword(ctx, name, request.GetNewPassword()); err != nil {
		return nil, userError(err)
	}
	return &userpb.SetUserPasswordResponse{}, nil
}

func (s *UserService) RevokeUserSessions(ctx context.Context, request *userpb.RevokeUserSessionsRequest) (*userpb.RevokeUserSessionsResponse, error) {
	name, err := s.parseUserName(request.GetName())
	if err != nil {
		return nil, err
	}
	if err := s.usecase.RevokeUserSessions(ctx, name); err != nil {
		return nil, userError(err)
	}
	return &userpb.RevokeUserSessionsResponse{}, nil
}

func (s *UserService) DeleteUser(ctx context.Context, request *userpb.DeleteUserRequest) (*userpb.User, error) {
	name, err := s.parseUserName(request.GetName())
	if err != nil {
		return nil, err
	}
	resource, err := s.usecase.DeleteUser(ctx, name)
	if err != nil {
		return nil, userError(err)
	}
	return s.plan.ToResponse(resource)
}

func (s *UserService) UndeleteUser(ctx context.Context, request *userpb.UndeleteUserRequest) (*userpb.User, error) {
	name, err := s.parseUserName(request.GetName())
	if err != nil {
		return nil, err
	}
	resource, err := s.usecase.UndeleteUser(ctx, name)
	if err != nil {
		return nil, userError(err)
	}
	return s.plan.ToResponse(resource)
}

func (s *UserService) ExpungeUser(ctx context.Context, request *userpb.ExpungeUserRequest) (*emptypb.Empty, error) {
	name, err := s.parseUserName(request.GetName())
	if err != nil {
		return nil, err
	}
	if err := s.usecase.ExpungeUser(ctx, name); err != nil {
		return nil, userError(err)
	}
	return &emptypb.Empty{}, nil
}

func (s *UserService) parseUserName(value string) (userpb.UserName, error) {
	parsed, err := s.plan.ParseName(value)
	if err != nil {
		return userpb.UserName{}, err
	}
	userID, ok := parsed.Variable("user")
	if !ok {
		return userpb.UserName{}, crudpb.ErrorCrudErrorReasonInvalidResourceName("name: user variable is missing")
	}
	return userpb.NewUserName(userID), nil
}

func userError(err error) error {
	switch {
	case errors.Is(err, biz.ErrNotFound):
		return userpb.ErrorUserErrorReasonNotFound("user not found").WithCause(err)
	case errors.Is(err, biz.ErrEmailAlreadyExists):
		return userpb.ErrorUserErrorReasonAlreadyExists("email is already registered").WithCause(err)
	case errors.Is(err, biz.ErrAlreadyExists):
		return userpb.ErrorUserErrorReasonAlreadyExists("user is not deleted").WithCause(err)
	case errors.Is(err, biz.ErrUserInvalidState):
		return userpb.ErrorUserErrorReasonInvalidState("user lifecycle state is invalid").WithCause(err)
	case errors.Is(err, biz.ErrUserEtagMismatch):
		return userpb.ErrorUserErrorReasonEtagMismatch("user etag does not match").WithCause(err)
	case errors.Is(err, biz.ErrVerificationEmailFailed):
		return userpb.ErrorUserErrorReasonVerificationEmailFailed("verification email delivery failed").WithCause(err)
	case errors.Is(err, biz.ErrInvalidPassword):
		return kerrors.New(400, "INVALID_ARGUMENT", "password does not meet policy").WithCause(err)
	}
	if _, ok := errors.AsType[*kerrors.Error](err); ok {
		return err
	}
	return kerrors.InternalServer("IAM user operation failed", "user operation failed").WithCause(err)
}
