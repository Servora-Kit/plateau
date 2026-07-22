package data

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	examplev1 "github.com/Servora-Kit/servora-platform/api/gen/go/example/service/v1"
	"github.com/Servora-Kit/servora-platform/app/example/service/internal/biz"
	entmodel "github.com/Servora-Kit/servora-platform/app/example/service/internal/data/ent"
	entuser "github.com/Servora-Kit/servora-platform/app/example/service/internal/data/ent/user"
	entcrud "github.com/Servora-Kit/servora/contrib/db/entgo/crud"
	entgomixin "github.com/Servora-Kit/servora/contrib/db/entgo/mixin"
	corecrud "github.com/Servora-Kit/servora/core/crud"
	crudmapper "github.com/Servora-Kit/servora/core/crud/mapper"
	fieldmaskpb "google.golang.org/protobuf/types/known/fieldmaskpb"
)

type userRepo struct {
	data       *Data
	listFields *entcrud.ListFields[*entmodel.User]
	mapper     *crudmapper.ResourceMapper[*examplev1.User, entmodel.User]
	clear      *entcrud.ClearHelper[*entmodel.UserMutation]
	log        *slog.Logger
}

// NewUserRepo validates the User persistence bindings and implements biz.UserRepo.
func NewUserRepo(data *Data, logger *slog.Logger) (biz.UserRepo, error) {
	listFields, err := entcrud.NewListFields[*entmodel.User](
		entcrud.Columns(entuser.ValidColumn),
		entcrud.Bind(examplev1.UserFields.DisplayName, entuser.FieldDisplayName).Filter().Order(),
		entcrud.Bind(examplev1.UserFields.Email, entuser.FieldEmail).Filter(),
		entcrud.Bind(examplev1.UserFields.TenantPlan, entuser.FieldTenantPlan).Filter(),
		entcrud.Bind(examplev1.UserFields.Nickname, entuser.FieldNickname).Filter().Order().Nullable(),
		entcrud.Bind(examplev1.UserFields.CreateTime, entuser.FieldCreateTime).Filter().Order(),
		entcrud.Bind(examplev1.UserFields.UpdateTime, entuser.FieldUpdateTime).Filter().Order(),
		entcrud.Bind(examplev1.UserFields.DeleteTime, entuser.FieldDeleteTime).Filter().Order().Nullable(),
		entcrud.DefaultOrder(examplev1.UserFields.CreateTime, corecrud.OrderDescending),
		entcrud.CursorKey[int](entuser.FieldID, corecrud.OrderAscending),
	)
	if err != nil {
		return nil, fmt.Errorf("build User list fields: %w", err)
	}
	resourceMapper, err := crudmapper.NewResourceMapper[*examplev1.User, entmodel.User](
		crudmapper.WithResourceName(func(value *entmodel.User) (string, error) {
			return examplev1.NewUserName(value.TenantID, value.ResourceID).Format()
		}),
	)
	if err != nil {
		return nil, fmt.Errorf("build User resource mapper: %w", err)
	}
	clear, err := entcrud.NewClearHelper[*entmodel.UserMutation](
		entcrud.ClearToValue(examplev1.UserFields.DisplayName, func(mutation *entmodel.UserMutation) error {
			mutation.SetDisplayName("")
			return nil
		}),
		entcrud.RenameClear[*entmodel.UserMutation](examplev1.UserFields.TemporaryPassword, entuser.FieldPasswordHash),
	)
	if err != nil {
		return nil, fmt.Errorf("build User Clear helper: %w", err)
	}
	return &userRepo{
		data:       data,
		listFields: listFields,
		mapper:     resourceMapper,
		clear:      clear,
		log:        logger.With("scope", "example/data/user"),
	}, nil
}

func (repo *userRepo) CreateUser(
	ctx context.Context,
	name examplev1.UserName,
	resource *examplev1.User,
	passwordHash *string,
) (*examplev1.User, error) {
	builder := repo.data.Ent(ctx).User.Create().
		SetTenantID(name.Tenant).
		SetResourceID(name.User).
		SetEmail(resource.GetEmail()).
		SetEtag(resource.GetEtag())
	if resource.DisplayName != nil {
		builder.SetDisplayName(resource.GetDisplayName())
	}
	if resource.TenantPlan != nil {
		builder.SetTenantPlan(resource.GetTenantPlan())
	}
	if resource.Nickname != nil {
		builder.SetNickname(resource.GetNickname())
	}
	if passwordHash != nil {
		builder.SetPasswordHash(*passwordHash)
	}
	created, err := builder.Save(ctx)
	if err != nil {
		if entmodel.IsConstraintError(err) {
			return nil, fmt.Errorf("create User: %w", errors.Join(biz.ErrUserAlreadyExists, err))
		}
		return nil, repo.persistenceError(ctx, "create", err)
	}
	return repo.mapUser(created)
}

func (repo *userRepo) GetUser(
	ctx context.Context,
	name examplev1.UserName,
	includeDeleted bool,
) (*examplev1.User, error) {
	if includeDeleted {
		ctx = entgomixin.SkipSoftDelete(ctx)
	}
	entity, err := repo.data.Ent(ctx).User.Query().Where(
		entuser.TenantIDEQ(name.Tenant),
		entuser.ResourceIDEQ(name.User),
	).Only(ctx)
	if err != nil {
		if entmodel.IsNotFound(err) {
			return nil, fmt.Errorf("get User: %w", errors.Join(biz.ErrUserNotFound, err))
		}
		return nil, repo.persistenceError(ctx, "get", err)
	}
	return repo.mapUser(entity)
}

func (repo *userRepo) ListUsers(
	ctx context.Context,
	scope biz.UserScope,
	includeDeleted bool,
	query corecrud.ListQuery,
) (corecrud.ListResult[*examplev1.User], error) {
	if includeDeleted {
		ctx = entgomixin.SkipSoftDelete(ctx)
	}
	result, err := entcrud.List(
		ctx,
		repo.data.Ent(ctx).User.Query().Where(entuser.TenantIDEQ(scope.TenantID())),
		query,
		repo.listFields,
		scope.Fingerprint(includeDeleted),
	)
	if err != nil {
		return corecrud.ListResult[*examplev1.User]{}, fmt.Errorf("list Users: %w", err)
	}
	resources, err := repo.mapper.TryToDTOs(result.Items())
	if err != nil {
		return corecrud.ListResult[*examplev1.User]{}, fmt.Errorf("map User list: %w", err)
	}
	var totalSize *int64
	if value, present := result.TotalSize(); present {
		totalSize = &value
	}
	mapped, err := corecrud.NewListResult(query, resources, result.NextPageToken(), totalSize)
	if err != nil {
		return corecrud.ListResult[*examplev1.User]{}, err
	}
	return mapped, nil
}

func (repo *userRepo) UpdateUser(
	ctx context.Context,
	name examplev1.UserName,
	resource *examplev1.User,
	mask *fieldmaskpb.FieldMask,
	passwordHash *string,
	expectedEtag string,
) (*examplev1.User, error) {
	builder := repo.data.Ent(ctx).User.Update().Where(
		entuser.TenantIDEQ(name.Tenant),
		entuser.ResourceIDEQ(name.User),
		entuser.DeleteTimeIsNil(),
		entuser.EtagEQ(expectedEtag),
	)
	if err := repo.clear.Apply(resource, mask, builder.Mutation()); err != nil {
		return nil, fmt.Errorf("apply User Clear intents: %w", err)
	}
	for _, path := range mask.GetPaths() {
		switch path {
		case examplev1.UserFields.DisplayName.String():
			if resource.DisplayName != nil {
				builder.SetDisplayName(resource.GetDisplayName())
			}
		case examplev1.UserFields.Email.String():
			if resource.Email != nil {
				builder.SetEmail(resource.GetEmail())
			}
		case examplev1.UserFields.Nickname.String():
			if resource.Nickname != nil {
				builder.SetNickname(resource.GetNickname())
			}
		case examplev1.UserFields.TemporaryPassword.String():
			if passwordHash != nil {
				builder.SetPasswordHash(*passwordHash)
			}
		}
	}
	builder.SetEtag(resource.GetEtag())
	updated, err := builder.Save(ctx)
	if err != nil {
		return nil, repo.persistenceError(ctx, "update", err)
	}
	if updated != 1 {
		return nil, repo.mutationMissError(ctx, "update", updated, name)
	}
	return repo.GetUser(ctx, name, false)
}

func (repo *userRepo) DeleteUser(ctx context.Context, name examplev1.UserName, expectedEtag string) (*examplev1.User, error) {
	deleted, err := repo.data.Ent(ctx).User.Delete().Where(
		entuser.TenantIDEQ(name.Tenant),
		entuser.ResourceIDEQ(name.User),
		entuser.DeleteTimeIsNil(),
		entuser.EtagEQ(expectedEtag),
	).Exec(ctx)
	if err != nil {
		return nil, repo.persistenceError(ctx, "delete", err)
	}
	if deleted != 1 {
		return nil, repo.mutationMissError(ctx, "delete", deleted, name)
	}
	return repo.GetUser(ctx, name, true)
}

func (repo *userRepo) UndeleteUser(ctx context.Context, name examplev1.UserName, newEtag string) (*examplev1.User, error) {
	ctx = entgomixin.SkipSoftDelete(ctx)
	restored, err := repo.data.Ent(ctx).User.Update().Where(
		entuser.TenantIDEQ(name.Tenant),
		entuser.ResourceIDEQ(name.User),
		entuser.DeleteTimeNotNil(),
	).ClearDeleteTime().ClearPurgeTime().ClearDeletedBy().SetEtag(newEtag).Save(ctx)
	if err != nil {
		return nil, repo.persistenceError(ctx, "undelete", err)
	}
	if restored != 1 {
		return nil, fmt.Errorf("undelete User affected %d rows: %w", restored, biz.ErrUserNotFound)
	}
	return repo.GetUser(ctx, name, true)
}

func (repo *userRepo) mutationMissError(
	ctx context.Context,
	operation string,
	affected int,
	name examplev1.UserName,
) error {
	current, err := repo.GetUser(ctx, name, true)
	if err != nil {
		if errors.Is(err, biz.ErrUserNotFound) {
			return fmt.Errorf("%s User affected %d rows: %w", operation, affected, err)
		}
		return err
	}
	if current.GetDeleteTime() != nil {
		return fmt.Errorf("%s User affected %d rows: %w", operation, affected, biz.ErrUserNotFound)
	}
	return fmt.Errorf("%s User affected %d rows: %w", operation, affected, biz.ErrUserEtagMismatch)
}

func (repo *userRepo) persistenceError(ctx context.Context, operation string, cause error) error {
	repo.log.ErrorContext(ctx, "User persistence failed", "operation", operation, "err", cause)
	return fmt.Errorf("%s User: %w", operation, cause)
}

func (repo *userRepo) mapUser(entity *entmodel.User) (*examplev1.User, error) {
	resource, err := repo.mapper.TryToDTO(entity)
	if err != nil {
		return nil, fmt.Errorf("map User: %w", err)
	}
	return resource, nil
}
