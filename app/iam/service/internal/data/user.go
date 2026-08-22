package data

import (
	"context"
	"fmt"
	"time"

	userpb "github.com/Servora-Kit/plateau/api/gen/go/iam/user/v1"
	"github.com/Servora-Kit/plateau/app/iam/service/internal/biz"
	entmodel "github.com/Servora-Kit/plateau/app/iam/service/internal/data/ent"
	"github.com/Servora-Kit/plateau/app/iam/service/internal/data/ent/authenticator"
	"github.com/Servora-Kit/plateau/app/iam/service/internal/data/ent/loginidentifier"
	"github.com/Servora-Kit/plateau/app/iam/service/internal/data/ent/user"
	entcrud "github.com/Servora-Kit/servora/contrib/db/entgo/crud"
	corecrud "github.com/Servora-Kit/servora/core/crud"
	crudmapper "github.com/Servora-Kit/servora/core/crud/mapper"
	"google.golang.org/protobuf/types/known/fieldmaskpb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type userRepository struct {
	data       *Data
	listFields *entcrud.ListFields[*entmodel.User]
	mapper     *crudmapper.ResourceMapper[*userpb.User, entmodel.User]
	clear      *entcrud.ClearHelper[*entmodel.UserMutation]
}

func NewUserRepository(data *Data) (biz.UserRepo, error) {
	if data == nil {
		return nil, fmt.Errorf("user repository: data is nil")
	}
	listFields, err := entcrud.NewListFields[*entmodel.User](
		entcrud.Columns(user.ValidColumn),
		entcrud.Bind(userpb.UserFields.UserId, user.FieldID).Filter().Order(),
		entcrud.Bind(userpb.UserFields.Status, user.FieldStatus).Filter().Order(),
		entcrud.Bind(userpb.UserFields.CreateTime, user.FieldCreateTime).Filter().Order(),
		entcrud.Bind(userpb.UserFields.UpdateTime, user.FieldUpdateTime).Filter().Order(),
		entcrud.Bind(userpb.UserFields.ProfileName, user.FieldName).Filter().Order().Nullable(),
		entcrud.Bind(userpb.UserFields.ProfileGivenName, user.FieldGivenName).Filter().Order().Nullable(),
		entcrud.Bind(userpb.UserFields.ProfileFamilyName, user.FieldFamilyName).Filter().Order().Nullable(),
		entcrud.Bind(userpb.UserFields.ProfileNickname, user.FieldNickname).Filter().Order().Nullable(),
		entcrud.Bind(userpb.UserFields.ProfilePreferredUsername, user.FieldPreferredUsername).Filter().Order().Nullable(),
		entcrud.Bind(userpb.UserFields.ProfilePicture, user.FieldPicture).Filter().Order().Nullable(),
		entcrud.Bind(userpb.UserFields.ProfileLocale, user.FieldLocale).Filter().Order().Nullable(),
		entcrud.DefaultOrder(userpb.UserFields.CreateTime, corecrud.OrderDescending),
		entcrud.CursorKey[string](user.FieldID, corecrud.OrderAscending),
	)
	if err != nil {
		return nil, fmt.Errorf("build IAM User list fields: %w", err)
	}
	resourceMapper, err := crudmapper.NewResourceMapper[*userpb.User, entmodel.User](
		crudmapper.WithConverters(crudmapper.TypeConverter{
			SrcType: "", DstType: userpb.UserStatus(0),
			Fn: func(source any) (any, error) {
				status, ok := source.(string)
				if !ok {
					return nil, fmt.Errorf("user status source is %T", source)
				}
				switch status {
				case biz.UserStatusPendingVerification:
					return userpb.UserStatus_USER_STATUS_PENDING_EMAIL_VERIFICATION, nil
				case biz.UserStatusActive:
					return userpb.UserStatus_USER_STATUS_ACTIVE, nil
				case biz.UserStatusDisabled:
					return userpb.UserStatus_USER_STATUS_DISABLED, nil
				default:
					return userpb.UserStatus_USER_STATUS_UNSPECIFIED, nil
				}
			},
		}),
		crudmapper.WithFieldMapping("ID", userpb.UserFields.UserId),
		crudmapper.WithResourceName(func(value *entmodel.User) (string, error) {
			return userpb.NewUserName(value.ID).Format()
		}),
		crudmapper.WithPostToDTOHook(func(value *entmodel.User, resource *userpb.User) error {
			resource.Profile = &userpb.UserProfile{
				Name: value.Name, GivenName: value.GivenName, FamilyName: value.FamilyName,
				Nickname: value.Nickname, PreferredUsername: value.PreferredUsername,
				Picture: value.Picture, Locale: value.Locale,
			}
			return nil
		}),
	)
	if err != nil {
		return nil, fmt.Errorf("build IAM User resource mapper: %w", err)
	}
	clear, err := entcrud.NewClearHelper(
		entcrud.RenameClear[*entmodel.UserMutation](userpb.UserFields.ProfileName, user.FieldName),
		entcrud.RenameClear[*entmodel.UserMutation](userpb.UserFields.ProfileGivenName, user.FieldGivenName),
		entcrud.RenameClear[*entmodel.UserMutation](userpb.UserFields.ProfileFamilyName, user.FieldFamilyName),
		entcrud.RenameClear[*entmodel.UserMutation](userpb.UserFields.ProfileNickname, user.FieldNickname),
		entcrud.RenameClear[*entmodel.UserMutation](userpb.UserFields.ProfilePreferredUsername, user.FieldPreferredUsername),
		entcrud.RenameClear[*entmodel.UserMutation](userpb.UserFields.ProfilePicture, user.FieldPicture),
		entcrud.RenameClear[*entmodel.UserMutation](userpb.UserFields.ProfileLocale, user.FieldLocale),
	)
	if err != nil {
		return nil, fmt.Errorf("build IAM User Clear helper: %w", err)
	}
	return &userRepository{data: data, listFields: listFields, mapper: resourceMapper, clear: clear}, nil
}

func (repo *userRepository) Create(ctx context.Context, resource *userpb.User, passwordHash, canonical string) (*userpb.User, error) {
	if resource == nil || resource.GetUserId() == "" || resource.GetEmail() == "" || passwordHash == "" || canonical == "" {
		return nil, fmt.Errorf("create user: required identity fields are missing")
	}
	now := time.Now()
	if resource.GetCreateTime() != nil {
		now = resource.GetCreateTime().AsTime()
	}
	if err := createUserAggregate(ctx, repo.data, resource, passwordHash, canonical, biz.UserStatusPendingVerification, nil, now); err != nil {
		return nil, err
	}
	return repo.Get(ctx, resource.GetUserId())
}

func createUserAggregate(ctx context.Context, data *Data, resource *userpb.User, passwordHash, canonical, status string, verifiedTime *time.Time, now time.Time) error {
	identifierID, err := biz.NewUserID()
	if err != nil {
		return err
	}
	authenticatorID, err := biz.NewUserID()
	if err != nil {
		return err
	}
	passwordAuthenticatorID, err := biz.NewUserID()
	if err != nil {
		return err
	}
	etag, err := biz.NewUserID()
	if err != nil {
		return err
	}
	return data.InTx(ctx, func(tx *entmodel.Tx) error {
		builder := tx.User.Create().SetID(resource.GetUserId()).SetStatus(status).SetEtag(etag)
		if profile := resource.GetProfile(); profile != nil {
			builder.SetNillableName(profile.Name).SetNillableGivenName(profile.GivenName).SetNillableFamilyName(profile.FamilyName).SetNillableNickname(profile.Nickname).SetNillablePreferredUsername(profile.PreferredUsername).SetNillablePicture(profile.Picture).SetNillableLocale(profile.Locale)
		}
		entity, err := builder.SetCreateTime(now).SetUpdateTime(now).Save(ctx)
		if err != nil {
			return translateEntError(err)
		}
		identifier := tx.LoginIdentifier.Create().SetID(identifierID).SetUserID(entity.ID).SetType(biz.LoginIdentifierEmail).SetCanonicalValue(canonical).SetDisplayValue(resource.GetEmail())
		if verifiedTime != nil {
			identifier.SetVerifiedTime(*verifiedTime)
		}
		if _, err := identifier.Save(ctx); err != nil {
			return translateEntError(err)
		}
		if _, err := tx.Authenticator.Create().SetID(authenticatorID).SetUserID(entity.ID).SetType(biz.AuthenticatorPassword).SetState(biz.AuthenticatorActive).Save(ctx); err != nil {
			return translateEntError(err)
		}
		if _, err := tx.PasswordAuthenticator.Create().SetID(passwordAuthenticatorID).SetAuthenticatorID(authenticatorID).SetPasswordHash(passwordHash).Save(ctx); err != nil {
			return translateEntError(err)
		}
		return nil
	})
}

func (repo *userRepository) FindByEmail(ctx context.Context, canonical string) (*userpb.User, error) {
	identifier, err := repo.data.ent.LoginIdentifier.Query().Where(loginidentifier.TypeEQ(biz.LoginIdentifierEmail), loginidentifier.CanonicalValueEQ(canonical)).Only(ctx)
	if err != nil {
		return nil, translateEntError(err)
	}
	return repo.Get(ctx, identifier.UserID)
}

func (repo *userRepository) Get(ctx context.Context, id string) (*userpb.User, error) {
	entity, err := repo.data.ent.User.Get(ctx, id)
	if err != nil {
		return nil, translateEntError(err)
	}
	resources, err := repo.toResources(ctx, []*entmodel.User{entity})
	if err != nil {
		return nil, err
	}
	return resources[0], nil
}

func (repo *userRepository) GetUser(ctx context.Context, name userpb.UserName) (*userpb.User, error) {
	return repo.Get(ctx, name.User)
}

func (repo *userRepository) ListUsers(ctx context.Context, query corecrud.ListQuery) (corecrud.ListResult[*userpb.User], error) {
	result, err := entcrud.List(ctx, repo.data.ent.User.Query(), query, repo.listFields, nil)
	if err != nil {
		return corecrud.ListResult[*userpb.User]{}, translateEntError(err)
	}
	resources, err := repo.toResources(ctx, result.Items())
	if err != nil {
		return corecrud.ListResult[*userpb.User]{}, err
	}
	var total *int64
	if value, ok := result.TotalSize(); ok {
		total = &value
	}
	return corecrud.NewListResult(query, resources, result.NextPageToken(), total)
}

func (repo *userRepository) UpdateUser(ctx context.Context, name userpb.UserName, resource *userpb.User, mask *fieldmaskpb.FieldMask, expectedEtag string) (*userpb.User, error) {
	if resource == nil || mask == nil || expectedEtag == "" {
		return nil, fmt.Errorf("update User resource, mask, and expected etag are required")
	}
	return repo.updateProfileFields(ctx, name.User, expectedEtag, resource, mask)
}

func (repo *userRepository) UpdateStatus(ctx context.Context, userID, expectedEtag string, status userpb.UserStatus, now time.Time) (*userpb.User, error) {
	if userID == "" || expectedEtag == "" {
		return nil, fmt.Errorf("update User status requires user ID and expected etag")
	}
	storageStatus := biz.UserStatusActive
	if status == userpb.UserStatus_USER_STATUS_DISABLED {
		storageStatus = biz.UserStatusDisabled
	}
	etag, err := biz.NewUserID()
	if err != nil {
		return nil, err
	}
	if now.IsZero() {
		now = time.Now()
	}
	err = repo.data.InTx(ctx, func(tx *entmodel.Tx) error {
		if _, err := tx.User.UpdateOneID(userID).Where(user.EtagEQ(expectedEtag)).SetStatus(storageStatus).SetEtag(etag).SetUpdateTime(now).Save(ctx); err != nil {
			if entmodel.IsNotFound(err) {
				return biz.ErrMutationMiss
			}
			return translateEntError(err)
		}
		mutation := tx.Authenticator.Update().Where(authenticator.UserIDEQ(userID))
		if status == userpb.UserStatus_USER_STATUS_DISABLED {
			mutation.SetState(biz.AuthenticatorDisabled).SetRevokedTime(now)
		} else {
			mutation.SetState(biz.AuthenticatorActive).ClearRevokedTime()
		}
		_, err := mutation.Save(ctx)
		return translateEntError(err)
	})
	if err != nil {
		return nil, err
	}
	return repo.Get(ctx, userID)
}

func (repo *userRepository) UpdateProfile(ctx context.Context, userID, expectedEtag string, profile *userpb.UserProfile) (*userpb.User, error) {
	if userID == "" || expectedEtag == "" {
		return nil, fmt.Errorf("update User profile requires user ID and expected etag")
	}
	return repo.updateProfileFields(ctx, userID, expectedEtag, &userpb.User{Profile: profile}, userProfileWriteMask())
}

func (repo *userRepository) updateProfileFields(ctx context.Context, userID, expectedEtag string, resource *userpb.User, mask *fieldmaskpb.FieldMask) (*userpb.User, error) {
	etag, err := biz.NewUserID()
	if err != nil {
		return nil, err
	}
	builder := repo.data.ent.User.UpdateOneID(userID).Where(user.EtagEQ(expectedEtag))
	if err := repo.clear.Apply(resource, mask, builder.Mutation()); err != nil {
		return nil, fmt.Errorf("apply IAM User Clear intents: %w", err)
	}
	profile := resource.GetProfile()
	for _, path := range mask.GetPaths() {
		switch path {
		case userpb.UserFields.ProfileName.String():
			if profile != nil && profile.Name != nil {
				builder.SetName(profile.GetName())
			}
		case userpb.UserFields.ProfileGivenName.String():
			if profile != nil && profile.GivenName != nil {
				builder.SetGivenName(profile.GetGivenName())
			}
		case userpb.UserFields.ProfileFamilyName.String():
			if profile != nil && profile.FamilyName != nil {
				builder.SetFamilyName(profile.GetFamilyName())
			}
		case userpb.UserFields.ProfileNickname.String():
			if profile != nil && profile.Nickname != nil {
				builder.SetNickname(profile.GetNickname())
			}
		case userpb.UserFields.ProfilePreferredUsername.String():
			if profile != nil && profile.PreferredUsername != nil {
				builder.SetPreferredUsername(profile.GetPreferredUsername())
			}
		case userpb.UserFields.ProfilePicture.String():
			if profile != nil && profile.Picture != nil {
				builder.SetPicture(profile.GetPicture())
			}
		case userpb.UserFields.ProfileLocale.String():
			if profile != nil && profile.Locale != nil {
				builder.SetLocale(profile.GetLocale())
			}
		}
	}
	updated, err := builder.SetEtag(etag).Save(ctx)
	if err != nil {
		if entmodel.IsNotFound(err) {
			return nil, biz.ErrMutationMiss
		}
		return nil, translateEntError(err)
	}
	return repo.Get(ctx, updated.ID)
}

func userProfileWriteMask() *fieldmaskpb.FieldMask {
	return &fieldmaskpb.FieldMask{Paths: []string{
		userpb.UserFields.ProfileName.String(),
		userpb.UserFields.ProfileGivenName.String(),
		userpb.UserFields.ProfileFamilyName.String(),
		userpb.UserFields.ProfileNickname.String(),
		userpb.UserFields.ProfilePreferredUsername.String(),
		userpb.UserFields.ProfilePicture.String(),
		userpb.UserFields.ProfileLocale.String(),
	}}
}

func (repo *userRepository) ActivateEmail(ctx context.Context, userID string, now time.Time) error {
	if now.IsZero() {
		now = time.Now()
	}
	etag, err := biz.NewUserID()
	if err != nil {
		return err
	}
	identifier, err := repo.data.ent.LoginIdentifier.Query().Where(loginidentifier.UserIDEQ(userID), loginidentifier.TypeEQ(biz.LoginIdentifierEmail)).Only(ctx)
	if err != nil {
		return translateEntError(err)
	}
	return repo.data.InTx(ctx, func(tx *entmodel.Tx) error {
		if _, err := tx.User.UpdateOneID(userID).Where(user.StatusEQ(biz.UserStatusPendingVerification)).SetStatus(biz.UserStatusActive).SetEtag(etag).SetUpdateTime(now).Save(ctx); err != nil {
			return translateEntError(err)
		}
		_, err := tx.LoginIdentifier.UpdateOneID(identifier.ID).SetVerifiedTime(now).Save(ctx)
		return translateEntError(err)
	})
}

func (repo *userRepository) toResources(ctx context.Context, entities []*entmodel.User) ([]*userpb.User, error) {
	if len(entities) == 0 {
		return nil, nil
	}
	userIDs := make([]string, len(entities))
	for index, entity := range entities {
		userIDs[index] = entity.ID
	}
	identifiers, err := repo.data.ent.LoginIdentifier.Query().Where(
		loginidentifier.UserIDIn(userIDs...),
		loginidentifier.TypeEQ(biz.LoginIdentifierEmail),
	).All(ctx)
	if err != nil {
		return nil, translateEntError(err)
	}
	emailByUserID := make(map[string]*entmodel.LoginIdentifier, len(identifiers))
	for _, identifier := range identifiers {
		emailByUserID[identifier.UserID] = identifier
	}
	resources := make([]*userpb.User, len(entities))
	for index, entity := range entities {
		identifier := emailByUserID[entity.ID]
		if identifier == nil {
			return nil, fmt.Errorf("IAM User %q has no email Login Identifier", entity.ID)
		}
		resource := repo.mapper.ToDTO(entity)
		resource.Email = new(identifier.DisplayValue)
		resource.EmailVerified = identifier.VerifiedTime != nil
		if identifier.VerifiedTime != nil {
			resource.EmailVerifiedTime = timestamppb.New(*identifier.VerifiedTime)
		}
		resources[index] = resource
	}
	return resources, nil
}
