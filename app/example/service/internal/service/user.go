package service

import (
	"context"
	"fmt"

	examplev1 "github.com/Servora-Kit/servora-platform/api/gen/go/example/service/v1"
	"github.com/Servora-Kit/servora-platform/app/example/service/internal/biz"
	crudpb "github.com/Servora-Kit/servora/api/gen/go/servora/crud/v1"
	corecrud "github.com/Servora-Kit/servora/core/crud"
)

// UserService implements the generated gRPC and HTTP User service contracts.
type UserService struct {
	examplev1.UnimplementedUserServiceServer
	usecase       *biz.UserUsecase
	plan          *corecrud.ResourcePlan[*examplev1.User]
	listPreparer  *corecrud.ListPreparer
	parentMatcher *corecrud.ResourceNameMatcher
}

// NewUserService validates immutable service-layer CRUD contracts once.
func NewUserService(usecase *biz.UserUsecase) (*UserService, error) {
	listPreparer, err := corecrud.NewListPreparer()
	if err != nil {
		return nil, fmt.Errorf("create User ListPreparer: %w", err)
	}
	parentMatcher, err := corecrud.NewResourceNameMatcher("tenants/{tenant}")
	if err != nil {
		return nil, fmt.Errorf("create User parent matcher: %w", err)
	}
	return &UserService{
		usecase:       usecase,
		plan:          corecrud.MustBuildResourcePlan[*examplev1.User](examplev1.UserCRUDDescriptor()),
		listPreparer:  listPreparer,
		parentMatcher: parentMatcher,
	}, nil
}

// GetUser returns active or tombstoned Users according to biz authorization semantics.
func (service *UserService) GetUser(
	ctx context.Context,
	request *examplev1.GetUserRequest,
) (*examplev1.User, error) {
	name, err := service.parseUserName(request.GetName())
	if err != nil {
		return nil, err
	}
	resource, err := service.usecase.GetUser(ctx, name)
	if err != nil {
		return nil, err
	}
	return service.plan.ToResponse(resource)
}

// ListUsers prepares the backend-neutral query and cleans every response resource.
func (service *UserService) ListUsers(
	ctx context.Context,
	request *examplev1.ListUsersRequest,
) (*examplev1.ListUsersResponse, error) {
	scope, err := service.userScope(request.GetParent())
	if err != nil {
		return nil, err
	}
	query, err := service.listPreparer.PrepareList(service.plan, corecrud.ListInput{
		Collection: request.GetParent(), PageSize: request.GetPageSize(), PageToken: request.GetPageToken(),
		Skip: request.GetSkip(), Filter: request.GetFilter(), OrderBy: request.GetOrderBy(),
		IncludeTotal: request.GetIncludeTotal(),
	})
	if err != nil {
		return nil, err
	}
	result, err := service.usecase.ListUsers(ctx, scope, query, corecrud.ListOptions{
		ShowDeleted: request.GetShowDeleted(),
	})
	if err != nil {
		return nil, err
	}
	resources, err := service.plan.ToResponses(result.Items())
	if err != nil {
		return nil, err
	}
	response := &examplev1.ListUsersResponse{
		Users: resources, NextPageToken: result.NextPageToken(),
	}
	if totalSize, present := result.TotalSize(); present {
		response.TotalSize = &totalSize
	}
	return response, nil
}

// CreateUser prepares client input and passes only scoped values into biz.
func (service *UserService) CreateUser(
	ctx context.Context,
	request *examplev1.CreateUserRequest,
) (*examplev1.User, error) {
	scope, err := service.userScope(request.GetParent())
	if err != nil {
		return nil, err
	}
	name := examplev1.NewUserName(scope.TenantID(), request.GetUserId())
	if err := name.Validate(); err != nil {
		return nil, crudpb.ErrorCrudErrorReasonInvalidResourceName("user_id: %v", err)
	}
	prepared, err := service.plan.PrepareCreate(request.GetUser())
	if err != nil {
		return nil, err
	}
	resource, err := service.usecase.CreateUser(ctx, name, prepared)
	if err != nil {
		return nil, err
	}
	return service.plan.ToResponse(resource)
}

// UpdateUser normalizes FieldMask and lifecycle behavior before biz.
func (service *UserService) UpdateUser(
	ctx context.Context,
	request *examplev1.UpdateUserRequest,
) (*examplev1.User, error) {
	resource := request.GetUser()
	name, err := service.parseUserName(resource.GetName())
	if err != nil {
		return nil, err
	}
	prepared, err := service.plan.PrepareUpdate(resource, request.GetUpdateMask(), corecrud.UpdateOptions{
		AllowMissing: request.GetAllowMissing(), Etag: resource.GetEtag(),
	})
	if err != nil {
		return nil, err
	}
	var missingCreate *corecrud.PreparedCreate[*examplev1.User]
	if request.GetAllowMissing() {
		fallback, prepareErr := service.plan.PrepareCreate(resource)
		if prepareErr != nil {
			return nil, prepareErr
		}
		missingCreate = &fallback
	}
	updated, err := service.usecase.UpdateUser(ctx, name, prepared, missingCreate)
	if err != nil {
		return nil, err
	}
	return service.plan.ToResponse(updated)
}

// DeleteUser applies the explicit AIP-164 business branch and returns its tombstone.
func (service *UserService) DeleteUser(
	ctx context.Context,
	request *examplev1.DeleteUserRequest,
) (*examplev1.User, error) {
	name, err := service.parseUserName(request.GetName())
	if err != nil {
		return nil, err
	}
	deleted, err := service.usecase.DeleteUser(ctx, name, corecrud.DeleteOptions{
		AllowMissing: request.GetAllowMissing(),
		Etag:         request.GetEtag(),
	})
	if err != nil {
		return nil, err
	}
	return service.plan.ToResponse(deleted)
}

// UndeleteUser explicitly restores one tombstone.
func (service *UserService) UndeleteUser(
	ctx context.Context,
	request *examplev1.UndeleteUserRequest,
) (*examplev1.User, error) {
	name, err := service.parseUserName(request.GetName())
	if err != nil {
		return nil, err
	}
	restored, err := service.usecase.UndeleteUser(ctx, name)
	if err != nil {
		return nil, err
	}
	return service.plan.ToResponse(restored)
}

func (service *UserService) userScope(parent string) (biz.UserScope, error) {
	name, err := service.parentMatcher.Parse(parent)
	if err != nil {
		return biz.UserScope{}, crudpb.ErrorCrudErrorReasonInvalidResourceName("parent: %v", err)
	}
	tenant, ok := name.Variable("tenant")
	if !ok {
		return biz.UserScope{}, crudpb.ErrorCrudErrorReasonInvalidResourceName("parent has no tenant")
	}
	scope, err := biz.NewUserScope(tenant)
	if err != nil {
		return biz.UserScope{}, crudpb.ErrorCrudErrorReasonInvalidResourceName("parent: %v", err)
	}
	return scope, nil
}

func (service *UserService) parseUserName(value string) (examplev1.UserName, error) {
	name, err := examplev1.ParseUserName(value)
	if err != nil {
		return examplev1.UserName{}, crudpb.ErrorCrudErrorReasonInvalidResourceName("name: %v", err)
	}
	if name.User == "-" {
		return examplev1.UserName{}, crudpb.ErrorCrudErrorReasonInvalidResourceName("name must identify one User")
	}
	return name, nil
}
