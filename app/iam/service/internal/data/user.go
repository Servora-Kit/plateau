package data

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	entsql "entgo.io/ent/dialect/sql"
	userpb "github.com/Servora-Kit/plateau/api/gen/go/iam/user/v1"
	"github.com/Servora-Kit/plateau/app/iam/service/internal/biz"
	entmodel "github.com/Servora-Kit/plateau/app/iam/service/internal/data/ent"
	"github.com/Servora-Kit/plateau/app/iam/service/internal/data/ent/authenticator"
	"github.com/Servora-Kit/plateau/app/iam/service/internal/data/ent/emailverificationtoken"
	"github.com/Servora-Kit/plateau/app/iam/service/internal/data/ent/iamloginsession"
	"github.com/Servora-Kit/plateau/app/iam/service/internal/data/ent/loginidentifier"
	"github.com/Servora-Kit/plateau/app/iam/service/internal/data/ent/oauthaccesstoken"
	"github.com/Servora-Kit/plateau/app/iam/service/internal/data/ent/oauthauthorizationcode"
	"github.com/Servora-Kit/plateau/app/iam/service/internal/data/ent/oauthrefreshtoken"
	"github.com/Servora-Kit/plateau/app/iam/service/internal/data/ent/oauthtokensession"
	"github.com/Servora-Kit/plateau/app/iam/service/internal/data/ent/oidcauthorizationrequest"
	"github.com/Servora-Kit/plateau/app/iam/service/internal/data/ent/passwordauthenticator"
	"github.com/Servora-Kit/plateau/app/iam/service/internal/data/ent/passwordresettoken"
	"github.com/Servora-Kit/plateau/app/iam/service/internal/data/ent/predicate"
	"github.com/Servora-Kit/plateau/app/iam/service/internal/data/ent/user"
	entgomixin "github.com/Servora-Kit/plateau/infra/entgo/mixin"
	"github.com/Servora-Kit/plateau/security"
	entcrud "github.com/Servora-Kit/servora/contrib/db/entgo/crud"
	corecrud "github.com/Servora-Kit/servora/core/crud"
	crudmapper "github.com/Servora-Kit/servora/core/crud/mapper"
	"google.golang.org/protobuf/types/known/fieldmaskpb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

var (
	listUsersActiveScope = []byte("iam.users.v1:active")
	listUsersAllScope    = []byte("iam.users.v1:all")
)
type userRepository struct {
	ent        *entmodel.Client
	listFields *entcrud.ListFields[*entmodel.User]
	mapper     *crudmapper.ResourceMapper[*userpb.User, entmodel.User]
	clear      *entcrud.ClearHelper[*entmodel.UserMutation]
}

func NewUserRepository(data *Data) (biz.UserRepo, error) {
	if data == nil || data.ent == nil {
		return nil, fmt.Errorf("user repository: data is nil")
	}
	listFields, err := entcrud.NewListFields[*entmodel.User](
		entcrud.Columns(user.ValidColumn),
		entcrud.Bind(userpb.UserFields.UserId, user.FieldID).Filter().Order(),
		entcrud.Custom(userpb.UserFields.Email, "email", emailFilterPredicate).Filter(),
		entcrud.Bind(userpb.UserFields.Status, user.FieldStatus).Filter().Order(),
		entcrud.Bind(userpb.UserFields.CreateTime, user.FieldCreateTime).Filter().Order(),
		entcrud.Bind(userpb.UserFields.UpdateTime, user.FieldUpdateTime).Filter().Order(),
		entcrud.Bind(userpb.UserFields.DeleteTime, user.FieldDeleteTime).Filter().Order().Nullable(),
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
	resourceMapper, err := newUserResourceMapper()
	if err != nil {
		return nil, err
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
	return &userRepository{ent: data.ent, listFields: listFields, mapper: resourceMapper, clear: clear}, nil
}

func emailFilterPredicate(operator corecrud.FilterOperator, value corecrud.FilterValue) (entcrud.SelectorPredicate, error) {
	match, ok := value.StringMatch()
	if !ok {
		return nil, fmt.Errorf("email filter requires a string value")
	}
	canonical := security.NormalizeEmail(match.Literal())
	if canonical == "" {
		return nil, fmt.Errorf("email filter literal is empty")
	}
	kind := match.Kind()
	if operator == corecrud.FilterOperatorHas {
		if kind != corecrud.StringMatchExact {
			return nil, fmt.Errorf("has operator does not accept wildcard patterns")
		}
		operator = corecrud.FilterOperatorEqual
		kind = corecrud.StringMatchContains
	}
	switch operator {
	case corecrud.FilterOperatorEqual, corecrud.FilterOperatorNotEqual:
		if operator == corecrud.FilterOperatorNotEqual && kind != corecrud.StringMatchExact {
			return nil, fmt.Errorf("!= does not support wildcard patterns")
		}
	default:
		return nil, fmt.Errorf("unsupported email filter operator %q", operator)
	}
	return func(selector *entsql.Selector) *entsql.Predicate {
		d := entsql.Dialect(selector.Dialect())
		t := d.Table(loginidentifier.Table)
		var valPred *entsql.Predicate
		switch kind {
		case corecrud.StringMatchExact:
			valPred = entsql.EQ(t.C(loginidentifier.FieldCanonicalValue), canonical)
		case corecrud.StringMatchPrefix:
			valPred = entsql.HasPrefix(t.C(loginidentifier.FieldCanonicalValue), canonical)
		case corecrud.StringMatchSuffix:
			valPred = entsql.HasSuffix(t.C(loginidentifier.FieldCanonicalValue), canonical)
		case corecrud.StringMatchContains:
			valPred = entsql.Contains(t.C(loginidentifier.FieldCanonicalValue), canonical)
		}
		sub := d.Select(t.C(loginidentifier.FieldID)).From(t).Where(entsql.And(
			entsql.ColumnsEQ(t.C(loginidentifier.FieldUserID), selector.C(user.FieldID)),
			entsql.EQ(t.C(loginidentifier.FieldType), biz.LoginIdentifierEmail),
			valPred,
		))
		if operator == corecrud.FilterOperatorNotEqual {
			return entsql.Not(entsql.Exists(sub))
		}
		return entsql.Exists(sub)
	}, nil
}

func (repo *userRepository) Create(ctx context.Context, resource *userpb.User, passwordHash, canonical string) (*userpb.User, error) {
	if resource == nil || resource.GetUserId() == "" || resource.GetEmail() == "" || passwordHash == "" || canonical == "" {
		return nil, fmt.Errorf("create user: required identity fields are missing")
	}
	now := time.Now()
	if resource.GetCreateTime() != nil {
		now = resource.GetCreateTime().AsTime()
	}
	if err := createUserAggregate(ctx, repo.ent, resource, passwordHash, canonical, biz.UserStatusPendingVerification, nil, now); err != nil {
		return nil, err
	}
	return repo.Get(ctx, resource.GetUserId())
}

func newUserResourceMapper() (*crudmapper.ResourceMapper[*userpb.User, entmodel.User], error) {
	resourceMapper, err := crudmapper.NewResourceMapper[*userpb.User, entmodel.User](
		crudmapper.WithConverters(
			crudmapper.TypeConverter{
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
			},
			crudmapper.TypeConverter{
				SrcType: []string(nil), DstType: []userpb.UserRequiredAction(nil),
				Fn: func(source any) (any, error) {
					values, ok := source.([]string)
					if !ok {
						return nil, fmt.Errorf("user required actions source is %T", source)
					}
					return requiredActionsFromStrings(values)
				},
			},
		),
		crudmapper.WithFieldMapping("ID", userpb.UserFields.UserId),
		crudmapper.WithFlatten(userpb.UserFields.Profile),
		crudmapper.WithResourceName(func(value *entmodel.User) (string, error) {
			return userpb.NewUserName(value.ID).Format()
		}),
	)
	if err != nil {
		return nil, fmt.Errorf("build IAM User resource mapper: %w", err)
	}
	return resourceMapper, nil
}

const storedRequiredActionUpdatePassword = "UPDATE_PASSWORD"

func requiredActionsToStrings(actions []userpb.UserRequiredAction) ([]string, error) {
	if len(actions) == 0 {
		return nil, nil
	}
	out := make([]string, 0, len(actions))
	for _, action := range actions {
		switch action {
		case userpb.UserRequiredAction_USER_REQUIRED_ACTION_UPDATE_PASSWORD:
			if !slices.Contains(out, storedRequiredActionUpdatePassword) {
				out = append(out, storedRequiredActionUpdatePassword)
			}
		default:
			return nil, fmt.Errorf("unsupported user required action %v", action)
		}
	}
	return out, nil
}

func requiredActionsFromStrings(values []string) ([]userpb.UserRequiredAction, error) {
	if len(values) == 0 {
		return nil, nil
	}
	out := make([]userpb.UserRequiredAction, 0, len(values))
	for _, value := range values {
		switch value {
		case storedRequiredActionUpdatePassword:
			if !slices.Contains(out, userpb.UserRequiredAction_USER_REQUIRED_ACTION_UPDATE_PASSWORD) {
				out = append(out, userpb.UserRequiredAction_USER_REQUIRED_ACTION_UPDATE_PASSWORD)
			}
		default:
			return nil, fmt.Errorf("unknown persisted user required action %q", value)
		}
	}
	return out, nil
}

func createUserAggregate(ctx context.Context, client *entmodel.Client, resource *userpb.User, passwordHash, canonical, status string, verifiedTime *time.Time, now time.Time) error {
	requiredActions, err := requiredActionsToStrings(resource.GetRequiredActions())
	if err != nil {
		return err
	}
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
	return inTx(ctx, client, func(tx *entmodel.Tx) error {
		builder := tx.User.Create().SetID(resource.GetUserId()).SetStatus(status).SetEtag(etag)
		if len(requiredActions) > 0 {
			builder.SetRequiredActions(requiredActions)
		}
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
	identifier, err := repo.ent.LoginIdentifier.Query().Where(loginidentifier.TypeEQ(biz.LoginIdentifierEmail), loginidentifier.CanonicalValueEQ(canonical)).Only(ctx)
	if err != nil {
		return nil, translateEntError(err)
	}
	return repo.Get(ctx, identifier.UserID)
}

func (repo *userRepository) FindByEmailIncludingDeleted(ctx context.Context, canonical string) (*userpb.User, error) {
	identifier, err := repo.ent.LoginIdentifier.Query().Where(loginidentifier.TypeEQ(biz.LoginIdentifierEmail), loginidentifier.CanonicalValueEQ(canonical)).Only(ctx)
	if err != nil {
		return nil, translateEntError(err)
	}
	skipCtx := entgomixin.SkipSoftDelete(ctx)
	entity, err := repo.ent.User.Get(skipCtx, identifier.UserID)
	if err != nil {
		return nil, translateEntError(err)
	}
	return repo.projectUser(skipCtx, repo.ent, entity)
}

func (repo *userRepository) Get(ctx context.Context, id string) (*userpb.User, error) {
	entity, err := repo.ent.User.Get(ctx, id)
	if err != nil {
		return nil, translateEntError(err)
	}
	return repo.projectUser(ctx, repo.ent, entity)
}

func (repo *userRepository) GetUser(ctx context.Context, name userpb.UserName) (*userpb.User, error) {
	skipCtx := entgomixin.SkipSoftDelete(ctx)
	entity, err := repo.ent.User.Get(skipCtx, name.User)
	if err != nil {
		return nil, translateEntError(err)
	}
	return repo.projectUser(skipCtx, repo.ent, entity)
}

func (repo *userRepository) ListUsers(ctx context.Context, query corecrud.ListQuery, showDeleted bool) (corecrud.ListResult[*userpb.User], error) {
	listCtx := ctx
	scope := listUsersActiveScope
	if showDeleted {
		listCtx = entgomixin.SkipSoftDelete(ctx)
		scope = listUsersAllScope
	}
	result, err := entcrud.List(listCtx, repo.ent.User.Query(), query, repo.listFields, scope)
	if err != nil {
		return corecrud.ListResult[*userpb.User]{}, translateEntError(err)
	}
	resources, err := projectUsersWithClient(listCtx, repo.ent, repo.mapper, result.Items())
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
	if status != userpb.UserStatus_USER_STATUS_DISABLED && status != userpb.UserStatus_USER_STATUS_ACTIVE {
		return nil, fmt.Errorf("unsupported target User status %v", status)
	}
	etag, err := biz.NewUserID()
	if err != nil {
		return nil, err
	}
	if now.IsZero() {
		now = time.Now()
	}
	var projected *userpb.User
	err = inTx(ctx, repo.ent, func(tx *entmodel.Tx) error {
		current, err := lockUser(ctx, tx, userID)
		if err != nil {
			return err
		}
		if status == userpb.UserStatus_USER_STATUS_ACTIVE && current.Status != biz.UserStatusDisabled {
			return biz.ErrUserInvalidState
		}
		if current.Etag != expectedEtag {
			return biz.ErrMutationMiss
		}
		storageStatus := biz.UserStatusDisabled
		if status == userpb.UserStatus_USER_STATUS_ACTIVE {
			identifier, err := tx.LoginIdentifier.Query().Where(
				loginidentifier.UserIDEQ(userID),
				loginidentifier.TypeEQ(biz.LoginIdentifierEmail),
			).Only(ctx)
			if err != nil {
				return translateEntError(err)
			}
			if identifier.VerifiedTime != nil {
				storageStatus = biz.UserStatusActive
			} else {
				storageStatus = biz.UserStatusPendingVerification
			}
		}
		updated, err := tx.User.UpdateOneID(userID).
			Where(user.DeleteTimeIsNil(), user.EtagEQ(expectedEtag)).
			SetStatus(storageStatus).
			SetEtag(etag).
			SetUpdateTime(now).
			Save(ctx)
		if err != nil {
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
		if _, err := mutation.Save(ctx); err != nil {
			return translateEntError(err)
		}
		if status == userpb.UserStatus_USER_STATUS_DISABLED {
			if err := revokeUserSessions(ctx, tx, userID, "", now); err != nil {
				return err
			}
		}
		var projErr error
		projected, projErr = repo.projectUser(ctx, tx.Client(), updated)
		return projErr
	})
	if err != nil {
		return nil, err
	}
	return projected, nil
}

func (repo *userRepository) UpdateProfile(ctx context.Context, userID, expectedEtag string, profile *userpb.UserProfile) (*userpb.User, error) {
	if userID == "" || expectedEtag == "" {
		return nil, fmt.Errorf("update User profile requires user ID and expected etag")
	}
	return repo.updateProfileFields(ctx, userID, expectedEtag, &userpb.User{Profile: profile}, &fieldmaskpb.FieldMask{Paths: []string{
		userpb.UserFields.ProfileName.String(),
		userpb.UserFields.ProfileGivenName.String(),
		userpb.UserFields.ProfileFamilyName.String(),
		userpb.UserFields.ProfileNickname.String(),
		userpb.UserFields.ProfilePreferredUsername.String(),
		userpb.UserFields.ProfilePicture.String(),
		userpb.UserFields.ProfileLocale.String(),
	}})
}

func (repo *userRepository) updateProfileFields(ctx context.Context, userID, expectedEtag string, resource *userpb.User, mask *fieldmaskpb.FieldMask) (*userpb.User, error) {
	etag, err := biz.NewUserID()
	if err != nil {
		return nil, err
	}
	now := time.Now()
	var projected *userpb.User
	err = inTx(ctx, repo.ent, func(tx *entmodel.Tx) error {
		if _, err := lockUser(ctx, tx, userID); err != nil {
			return err
		}
		builder := tx.User.UpdateOneID(userID).Where(user.DeleteTimeIsNil(), user.EtagEQ(expectedEtag))
		if err := repo.clear.Apply(resource, mask, builder.Mutation()); err != nil {
			return fmt.Errorf("apply IAM User Clear intents: %w", err)
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
		updated, err := builder.SetEtag(etag).SetUpdateTime(now).Save(ctx)
		if err != nil {
			if entmodel.IsNotFound(err) {
				return biz.ErrMutationMiss
			}
			return translateEntError(err)
		}
		var projErr error
		projected, projErr = repo.projectUser(ctx, tx.Client(), updated)
		return projErr
	})
	if err != nil {
		return nil, err
	}
	return projected, nil
}

func (repo *userRepository) SoftDelete(ctx context.Context, userID string, proof *biz.DeleteAccountProof, now time.Time) (*userpb.User, error) {
	if userID == "" {
		return nil, biz.ErrNotFound
	}
	if now.IsZero() {
		now = time.Now()
	}
	etag, err := biz.NewUserID()
	if err != nil {
		return nil, err
	}
	var projected *userpb.User
	err = inTx(ctx, repo.ent, func(tx *entmodel.Tx) error {
		current, err := lockUser(ctx, tx, userID)
		if err != nil {
			return err
		}
		if proof != nil {
			if _, err := tx.IAMLoginSession.Query().Where(
				iamloginsession.IDEQ(proof.LoginID),
				iamloginsession.UserIDEQ(userID),
				iamloginsession.RevokedTimeIsNil(),
			).Only(ctx); err != nil {
				if entmodel.IsNotFound(err) {
					return biz.ErrSessionRevoked
				}
				return translateEntError(err)
			}
			if current.Status != biz.UserStatusActive || len(current.RequiredActions) > 0 {
				return biz.ErrInvalidCredentials
			}
			if _, err := ensureVerifiedEmail(ctx, tx, userID); err != nil {
				if errors.Is(err, biz.ErrNotFound) || errors.Is(err, biz.ErrMutationMiss) {
					return biz.ErrInvalidCredentials
				}
				return err
			}
			_, passEntity, err := ensureActivePasswordAuthenticator(ctx, tx, userID)
			if err != nil {
				if errors.Is(err, biz.ErrNotFound) {
					return biz.ErrInvalidCredentials
				}
				return err
			}
			if passEntity.PasswordHash != proof.PasswordHash {
				return biz.ErrInvalidCredentials
			}
		}
		affected, err := tx.User.Update().
			Where(user.IDEQ(userID), user.DeleteTimeIsNil()).
			SetDeleteTime(now).
			ClearPurgeTime().
			SetEtag(etag).
			SetUpdateTime(now).
			Save(ctx)
		if err != nil {
			return translateEntError(err)
		}
		if affected != 1 {
			return biz.ErrNotFound
		}
		if err := revokeUserSessions(ctx, tx, userID, "", now); err != nil {
			return err
		}
		if _, err := tx.OAuthAccessToken.Update().Where(
			oauthaccesstoken.ActorTypeEQ(oauthaccesstoken.ActorTypeHuman),
			oauthaccesstoken.SubjectEQ(userID),
			oauthaccesstoken.RevokedTimeIsNil(),
		).SetRevokedTime(now).Save(ctx); err != nil {
			return translateEntError(err)
		}
		if _, err := tx.EmailVerificationToken.Update().Where(
			emailverificationtoken.UserIDEQ(userID),
			emailverificationtoken.ConsumedTimeIsNil(),
		).SetConsumedTime(now).Save(ctx); err != nil {
			return translateEntError(err)
		}
		if _, err := tx.PasswordResetToken.Update().Where(
			passwordresettoken.UserIDEQ(userID),
			passwordresettoken.ConsumedTimeIsNil(),
		).SetConsumedTime(now).Save(ctx); err != nil {
			return translateEntError(err)
		}
		loginIDs, err := tx.IAMLoginSession.Query().Where(iamloginsession.UserIDEQ(userID)).IDs(ctx)
		if err != nil {
			return translateEntError(err)
		}
		tokenSessionIDs, err := tx.OAuthTokenSession.Query().Where(oauthtokensession.UserIDEQ(userID)).IDs(ctx)
		if err != nil {
			return translateEntError(err)
		}
		if err := deleteIdentityBoundOIDCArtifacts(ctx, tx, userID, loginIDs, tokenSessionIDs); err != nil {
			return err
		}
		skipCtx := entgomixin.SkipSoftDelete(ctx)
		updated, err := tx.User.Get(skipCtx, userID)
		if err != nil {
			return translateEntError(err)
		}
		var projErr error
		projected, projErr = repo.projectUser(skipCtx, tx.Client(), updated)
		return projErr
	})
	if err != nil {
		return nil, err
	}
	return projected, nil
}

func (repo *userRepository) Undelete(ctx context.Context, userID string, now time.Time) (*userpb.User, error) {
	if userID == "" {
		return nil, biz.ErrNotFound
	}
	if now.IsZero() {
		now = time.Now()
	}
	etag, err := biz.NewUserID()
	if err != nil {
		return nil, err
	}
	var projected *userpb.User
	err = inTx(ctx, repo.ent, func(tx *entmodel.Tx) error {
		current, err := lockUserIncludingDeleted(ctx, tx, userID)
		if err != nil {
			return err
		}
		if current.DeleteTime == nil {
			return biz.ErrAlreadyExists
		}
		affected, err := tx.User.Update().
			Where(user.IDEQ(userID), user.DeleteTimeNotNil()).
			ClearDeleteTime().
			ClearPurgeTime().
			ClearDeletedBy().
			SetEtag(etag).
			SetUpdateTime(now).
			Save(ctx)
		if err != nil {
			return translateEntError(err)
		}
		if affected != 1 {
			return biz.ErrAlreadyExists
		}
		updated, err := tx.User.Get(ctx, userID)
		if err != nil {
			return translateEntError(err)
		}
		var projErr error
		projected, projErr = repo.projectUser(ctx, tx.Client(), updated)
		return projErr
	})
	if err != nil {
		return nil, err
	}
	return projected, nil
}

func (repo *userRepository) Expunge(ctx context.Context, userID string) error {
	if userID == "" {
		return biz.ErrNotFound
	}
	return inTx(ctx, repo.ent, func(tx *entmodel.Tx) error {
		current, err := lockUserIncludingDeleted(ctx, tx, userID)
		if err != nil {
			return err
		}
		if current.DeleteTime == nil {
			return biz.ErrUserInvalidState
		}
		loginIDs, err := tx.IAMLoginSession.Query().Where(iamloginsession.UserIDEQ(userID)).IDs(ctx)
		if err != nil {
			return translateEntError(err)
		}
		authenticatorIDs, err := tx.Authenticator.Query().Where(authenticator.UserIDEQ(userID)).IDs(ctx)
		if err != nil {
			return translateEntError(err)
		}
		tokenSessionIDs, err := tx.OAuthTokenSession.Query().Where(oauthtokensession.UserIDEQ(userID)).IDs(ctx)
		if err != nil {
			return translateEntError(err)
		}

		// 1. oauth_refresh_tokens
		if len(tokenSessionIDs) > 0 {
			if _, err := tx.OAuthRefreshToken.Delete().Where(oauthrefreshtoken.TokenSessionIDIn(tokenSessionIDs...)).Exec(ctx); err != nil {
				return translateEntError(err)
			}
		}
		// 2. oauth_access_tokens (human only; preserve service tokens even if subject == userID)
		accessOwnership := oauthaccesstoken.SubjectEQ(userID)
		if len(tokenSessionIDs) > 0 {
			accessOwnership = oauthaccesstoken.Or(
				oauthaccesstoken.SubjectEQ(userID),
				oauthaccesstoken.TokenSessionIDIn(tokenSessionIDs...),
			)
		}
		if _, err := tx.OAuthAccessToken.Delete().Where(
			oauthaccesstoken.ActorTypeEQ(oauthaccesstoken.ActorTypeHuman),
			accessOwnership,
		).Exec(ctx); err != nil {
			return translateEntError(err)
		}
		// 3. oauth_authorization_codes & 4. oidc_authorization_requests
		if err := deleteIdentityBoundOIDCArtifacts(ctx, tx, userID, loginIDs, tokenSessionIDs); err != nil {
			return err
		}
		// 5. oauth_token_sessions
		if len(tokenSessionIDs) > 0 {
			if _, err := tx.OAuthTokenSession.Delete().Where(oauthtokensession.IDIn(tokenSessionIDs...)).Exec(ctx); err != nil {
				return translateEntError(err)
			}
		}
		// 6. iam_login_sessions
		if _, err := tx.IAMLoginSession.Delete().Where(iamloginsession.UserIDEQ(userID)).Exec(ctx); err != nil {
			return translateEntError(err)
		}
		// 7. email_verification_tokens
		if _, err := tx.EmailVerificationToken.Delete().Where(emailverificationtoken.UserIDEQ(userID)).Exec(ctx); err != nil {
			return translateEntError(err)
		}
		// 8. password_reset_tokens
		if _, err := tx.PasswordResetToken.Delete().Where(passwordresettoken.UserIDEQ(userID)).Exec(ctx); err != nil {
			return translateEntError(err)
		}
		// 9. password_authenticators
		if len(authenticatorIDs) > 0 {
			if _, err := tx.PasswordAuthenticator.Delete().Where(passwordauthenticator.AuthenticatorIDIn(authenticatorIDs...)).Exec(ctx); err != nil {
				return translateEntError(err)
			}
		}
		// 10. authenticators
		if _, err := tx.Authenticator.Delete().Where(authenticator.UserIDEQ(userID)).Exec(ctx); err != nil {
			return translateEntError(err)
		}
		// 11. login_identifiers
		if _, err := tx.LoginIdentifier.Delete().Where(loginidentifier.UserIDEQ(userID)).Exec(ctx); err != nil {
			return translateEntError(err)
		}
		// 12. users (local SkipSoftDelete + DeleteTimeNotNil, verifying 1 row deleted)
		deleted, err := tx.User.Delete().
			Where(user.IDEQ(userID), user.DeleteTimeNotNil()).
			Exec(entgomixin.SkipSoftDelete(ctx))
		if err != nil {
			return translateEntError(err)
		}
		if deleted != 1 {
			return biz.ErrUserInvalidState
		}
		return nil
	})
}

func deleteIdentityBoundOIDCArtifacts(ctx context.Context, tx *entmodel.Tx, userID string, loginIDs, tokenSessionIDs []string) error {
	reqPred := oidcauthorizationrequest.SubjectEQ(userID)
	if len(loginIDs) > 0 {
		reqPred = oidcauthorizationrequest.Or(
			oidcauthorizationrequest.SubjectEQ(userID),
			oidcauthorizationrequest.IamLoginSessionIDIn(loginIDs...),
		)
	}
	requestIDs, err := tx.OIDCAuthorizationRequest.Query().Where(reqPred).IDs(ctx)
	if err != nil {
		return translateEntError(err)
	}

	codePreds := make([]predicate.OAuthAuthorizationCode, 0, 3)
	codePreds = append(codePreds, oauthauthorizationcode.SubjectEQ(userID))
	if len(requestIDs) > 0 {
		codePreds = append(codePreds, oauthauthorizationcode.AuthorizationRequestIDIn(requestIDs...))
	}
	if len(tokenSessionIDs) > 0 {
		codePreds = append(codePreds, oauthauthorizationcode.TokenSessionIDIn(tokenSessionIDs...))
	}
	codeWhere := codePreds[0]
	if len(codePreds) > 1 {
		codeWhere = oauthauthorizationcode.Or(codePreds...)
	}
	codes, err := tx.OAuthAuthorizationCode.Query().Where(codeWhere).All(ctx)
	if err != nil {
		return translateEntError(err)
	}
	codeIDs := make([]string, 0, len(codes))
	for _, code := range codes {
		codeIDs = append(codeIDs, code.ID)
		if code.AuthorizationRequestID != "" && !slices.Contains(requestIDs, code.AuthorizationRequestID) {
			requestIDs = append(requestIDs, code.AuthorizationRequestID)
		}
	}

	delCodePreds := make([]predicate.OAuthAuthorizationCode, 0, 2)
	if len(codeIDs) > 0 {
		delCodePreds = append(delCodePreds, oauthauthorizationcode.IDIn(codeIDs...))
	}
	if len(requestIDs) > 0 {
		delCodePreds = append(delCodePreds, oauthauthorizationcode.AuthorizationRequestIDIn(requestIDs...))
	}
	if len(delCodePreds) > 0 {
		delWhere := delCodePreds[0]
		if len(delCodePreds) > 1 {
			delWhere = oauthauthorizationcode.Or(delCodePreds...)
		}
		if _, err := tx.OAuthAuthorizationCode.Delete().Where(delWhere).Exec(ctx); err != nil {
			return translateEntError(err)
		}
	}
	if len(requestIDs) > 0 {
		if _, err := tx.OIDCAuthorizationRequest.Delete().Where(oidcauthorizationrequest.IDIn(requestIDs...)).Exec(ctx); err != nil {
			return translateEntError(err)
		}
	}
	return nil
}

func (repo *userRepository) projectUser(ctx context.Context, client *entmodel.Client, entity *entmodel.User) (*userpb.User, error) {
	resources, err := projectUsersWithClient(ctx, client, repo.mapper, []*entmodel.User{entity})
	if err != nil {
		return nil, err
	}
	return resources[0], nil
}

func projectUsersWithClient(ctx context.Context, client *entmodel.Client, mapper *crudmapper.ResourceMapper[*userpb.User, entmodel.User], entities []*entmodel.User) ([]*userpb.User, error) {
	if len(entities) == 0 {
		return nil, nil
	}
	userIDs := make([]string, len(entities))
	for index, entity := range entities {
		userIDs[index] = entity.ID
	}
	identifiers, err := client.LoginIdentifier.Query().Where(
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
		resource, err := mapper.TryToDTO(entity)
		if err != nil {
			return nil, fmt.Errorf("map IAM User %q: %w", entity.ID, err)
		}
		resource.Email = new(identifier.DisplayValue)
		resource.EmailVerified = identifier.VerifiedTime != nil
		if identifier.VerifiedTime != nil {
			resource.EmailVerifiedTime = timestamppb.New(*identifier.VerifiedTime)
		}
		resources[index] = resource
	}
	return resources, nil
}
