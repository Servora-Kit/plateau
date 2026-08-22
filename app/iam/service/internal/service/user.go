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
	if request == nil || request.GetUser() == nil {
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
	resource, err := s.usecase.CreateUser(ctx, name, prepared, request.GetPassword())
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
	result, err := s.usecase.ListUsers(ctx, query)
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
	if request == nil || request.GetUser() == nil {
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
	name, err := s.parseUserName(requestName(request))
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
	name, err := s.parseUserName(enableRequestName(request))
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

func requestName(request *userpb.DisableUserRequest) string {
	if request == nil {
		return ""
	}
	return request.GetName()
}

func enableRequestName(request *userpb.EnableUserRequest) string {
	if request == nil {
		return ""
	}
	return request.GetName()
}

func userError(err error) error {
	switch {
	case errors.Is(err, biz.ErrNotFound):
		return userpb.ErrorUserErrorReasonNotFound("user not found").WithCause(err)
	case errors.Is(err, biz.ErrEmailAlreadyExists):
		return userpb.ErrorUserErrorReasonAlreadyExists("email is already registered").WithCause(err)
	case errors.Is(err, biz.ErrUserInvalidState):
		return userpb.ErrorUserErrorReasonInvalidState("user lifecycle state is invalid").WithCause(err)
	case errors.Is(err, biz.ErrUserEtagMismatch):
		return userpb.ErrorUserErrorReasonEtagMismatch("user etag does not match").WithCause(err)
	case errors.Is(err, biz.ErrInvalidPassword):
		return kerrors.New(400, "INVALID_ARGUMENT", "password does not meet policy").WithCause(err)
	default:
		return kerrors.InternalServer("IAM user operation failed", "user operation failed").WithCause(err)
	}
}
