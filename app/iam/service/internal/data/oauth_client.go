package data

import (
	"context"
	"fmt"
	"slices"
	"time"

	entsql "entgo.io/ent/dialect/sql"
	iamclientv1 "github.com/Servora-Kit/plateau/api/gen/go/plateau/iam/client/v1"
	"github.com/Servora-Kit/plateau/app/iam/service/internal/biz"
	entmodel "github.com/Servora-Kit/plateau/app/iam/service/internal/data/ent"
	"github.com/Servora-Kit/plateau/app/iam/service/internal/data/ent/authflow"
	"github.com/Servora-Kit/plateau/app/iam/service/internal/data/ent/oauthaccesstoken"
	"github.com/Servora-Kit/plateau/app/iam/service/internal/data/ent/oauthauthorizationcode"
	"github.com/Servora-Kit/plateau/app/iam/service/internal/data/ent/oauthclient"
	"github.com/Servora-Kit/plateau/app/iam/service/internal/data/ent/oauthclientsecret"
	"github.com/Servora-Kit/plateau/app/iam/service/internal/data/ent/oauthconsent"
	"github.com/Servora-Kit/plateau/app/iam/service/internal/data/ent/oauthtokensession"
	"github.com/Servora-Kit/plateau/app/iam/service/internal/data/ent/oidcauthorizationrequest"
	corecrud "github.com/Servora-Kit/plateau/crud"
	crudmapper "github.com/Servora-Kit/plateau/crud/mapper"
	entcrud "github.com/Servora-Kit/plateau/infra/entgo/crud"
	"google.golang.org/protobuf/types/known/fieldmaskpb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

var listOAuthClientsScope = []byte("iam.oauth_clients.v1:all")

type oauthClientRepository struct {
	ent        *entmodel.Client
	listFields *entcrud.ListFields[*entmodel.OAuthClient]
	mapper     *crudmapper.ResourceMapper[*iamclientv1.OAuthClient, entmodel.OAuthClient]
	clear      *entcrud.ClearHelper[*entmodel.OAuthClientMutation]
}

// NewOAuthClientRepository constructs the Ent-backed OAuthClientRepo adapter.
func NewOAuthClientRepository(data *Data) (biz.OAuthClientRepo, error) {
	if data == nil || data.ent == nil {
		return nil, fmt.Errorf("oauth client repository: data is nil")
	}
	listFields, err := entcrud.NewListFields[*entmodel.OAuthClient](
		entcrud.Columns(oauthclient.ValidColumn),
		entcrud.Bind(iamclientv1.OAuthClientFields.ClientId, oauthclient.FieldID).Filter().Order(),
		entcrud.Bind(iamclientv1.OAuthClientFields.Source, oauthclient.FieldSource).Filter().Order().
			WithQueryConverter(enumQueryConverter("source", biz.ClientSourceToStorage)).
			WithCursorConverter(enumCursorConverter("source", biz.ClientSourceFromStorage)),
		entcrud.Bind(iamclientv1.OAuthClientFields.Status, oauthclient.FieldStatus).Filter().Order().
			WithQueryConverter(enumQueryConverter("status", biz.ClientStatusToStorage)).
			WithCursorConverter(enumCursorConverter("status", biz.ClientStatusFromStorage)),
		entcrud.Bind(iamclientv1.OAuthClientFields.DisplayName, oauthclient.FieldDisplayName).Filter().Order(),
		entcrud.Bind(iamclientv1.OAuthClientFields.Description, oauthclient.FieldDescription).Filter().Order().Nullable(),
		entcrud.Bind(iamclientv1.OAuthClientFields.LogoUri, oauthclient.FieldLogoURI).Filter().Order().Nullable(),
		entcrud.Bind(iamclientv1.OAuthClientFields.ApplicationType, oauthclient.FieldApplicationType).Filter().Order().
			WithQueryConverter(enumQueryConverter("application_type", biz.ApplicationTypeToStorage)).
			WithCursorConverter(enumCursorConverter("application_type", biz.ApplicationTypeFromStorage)),
		entcrud.Bind(iamclientv1.OAuthClientFields.TokenEndpointAuthMethod, oauthclient.FieldTokenEndpointAuthMethod).Filter().Order().
			WithQueryConverter(enumQueryConverter("token_endpoint_auth_method", biz.AuthMethodToStorage)).
			WithCursorConverter(enumCursorConverter("token_endpoint_auth_method", biz.AuthMethodFromStorage)),
		entcrud.Bind(iamclientv1.OAuthClientFields.Trusted, oauthclient.FieldTrusted).Filter().Order(),
		entcrud.Bind(iamclientv1.OAuthClientFields.CreateTime, oauthclient.FieldCreateTime).Filter().Order(),
		entcrud.Bind(iamclientv1.OAuthClientFields.UpdateTime, oauthclient.FieldUpdateTime).Filter().Order(),
		entcrud.DefaultOrder(iamclientv1.OAuthClientFields.CreateTime, corecrud.OrderDescending),
		entcrud.CursorKey[string](oauthclient.FieldID, corecrud.OrderAscending),
	)
	if err != nil {
		return nil, fmt.Errorf("build IAM OAuthClient list fields: %w", err)
	}
	resourceMapper, err := newOAuthClientResourceMapper()
	if err != nil {
		return nil, err
	}
	clear, err := entcrud.NewClearHelper(
		entcrud.RenameClear[*entmodel.OAuthClientMutation](iamclientv1.OAuthClientFields.Description, oauthclient.FieldDescription),
		entcrud.RenameClear[*entmodel.OAuthClientMutation](iamclientv1.OAuthClientFields.LogoUri, oauthclient.FieldLogoURI),
		entcrud.ClearToValue[*entmodel.OAuthClientMutation](iamclientv1.OAuthClientFields.Trusted, func(mutation *entmodel.OAuthClientMutation) error {
			mutation.SetTrusted(false)
			return nil
		}),
	)
	if err != nil {
		return nil, fmt.Errorf("build IAM OAuthClient Clear helper: %w", err)
	}
	return &oauthClientRepository{
		ent:        data.ent,
		listFields: listFields,
		mapper:     resourceMapper,
		clear:      clear,
	}, nil
}

func enumQueryConverter[E ~int32](field string, toStorage func(E) (string, error)) func(corecrud.FilterValue) (any, error) {
	return func(value corecrud.FilterValue) (any, error) {
		num, ok := value.EnumNumber()
		if !ok {
			return nil, fmt.Errorf("%s filter requires enum value", field)
		}
		return toStorage(E(num))
	}
}

func enumCursorConverter[E ~int32](field string, fromStorage func(string) (E, error)) func(any) (any, error) {
	return func(raw any) (any, error) {
		stored, ok := raw.(string)
		if !ok {
			return nil, fmt.Errorf("%s cursor value is %T, want string", field, raw)
		}
		enumVal, err := fromStorage(stored)
		if err != nil {
			return nil, err
		}
		return int64(enumVal), nil
	}
}

func newOAuthClientResourceMapper() (*crudmapper.ResourceMapper[*iamclientv1.OAuthClient, entmodel.OAuthClient], error) {
	resourceMapper, err := crudmapper.NewResourceMapper[*iamclientv1.OAuthClient, entmodel.OAuthClient](
		crudmapper.WithConverters(
			crudmapper.TypeConverter{
				SrcType: "", DstType: iamclientv1.ClientSource(0),
				Fn: func(source any) (any, error) {
					val, ok := source.(string)
					if !ok {
						return nil, fmt.Errorf("client source is %T", source)
					}
					return biz.ClientSourceFromStorage(val)
				},
			},
			crudmapper.TypeConverter{
				SrcType: "", DstType: iamclientv1.ClientStatus(0),
				Fn: func(source any) (any, error) {
					val, ok := source.(string)
					if !ok {
						return nil, fmt.Errorf("client status is %T", source)
					}
					return biz.ClientStatusFromStorage(val)
				},
			},
			crudmapper.TypeConverter{
				SrcType: "", DstType: (*iamclientv1.ApplicationType)(nil),
				Fn: func(source any) (any, error) {
					val, ok := source.(string)
					if !ok {
						return nil, fmt.Errorf("application_type is %T", source)
					}
					enumVal, err := biz.ApplicationTypeFromStorage(val)
					if err != nil {
						return nil, err
					}
					return &enumVal, nil
				},
			},
			crudmapper.TypeConverter{
				SrcType: "", DstType: (*iamclientv1.TokenEndpointAuthMethod)(nil),
				Fn: func(source any) (any, error) {
					val, ok := source.(string)
					if !ok {
						return nil, fmt.Errorf("token_endpoint_auth_method is %T", source)
					}
					enumVal, err := biz.AuthMethodFromStorage(val)
					if err != nil {
						return nil, err
					}
					return &enumVal, nil
				},
			},
			crudmapper.TypeConverter{
				SrcType: false, DstType: (*bool)(nil),
				Fn: func(source any) (any, error) {
					val, ok := source.(bool)
					if !ok {
						return nil, fmt.Errorf("trusted is %T", source)
					}
					return &val, nil
				},
			},
		),
		crudmapper.WithFieldMapping("ID", iamclientv1.OAuthClientFields.ClientId),
		crudmapper.WithFieldMapping("LogoURI", iamclientv1.OAuthClientFields.LogoUri),
		crudmapper.WithResourceName(func(value *entmodel.OAuthClient) (string, error) {
			return iamclientv1.NewOAuthClientName(value.ID).Format()
		}),
	)
	if err != nil {
		return nil, fmt.Errorf("build IAM OAuthClient resource mapper: %w", err)
	}
	return resourceMapper, nil
}

func (repo *oauthClientRepository) Create(
	ctx context.Context,
	resource *iamclientv1.OAuthClient,
	initialSecret *iamclientv1.OAuthClientSecret,
	secretHash string,
	now time.Time,
) (*iamclientv1.OAuthClient, error) {
	now = now.UTC()
	appType, err := biz.ApplicationTypeToStorage(resource.GetApplicationType())
	if err != nil {
		return nil, err
	}
	authMethod, err := biz.AuthMethodToStorage(resource.GetTokenEndpointAuthMethod())
	if err != nil {
		return nil, err
	}
	etag, err := biz.NewID()
	if err != nil {
		return nil, err
	}

	var projected *iamclientv1.OAuthClient
	err = inTx(ctx, repo.ent, func(tx *entmodel.Tx) error {
		saved, err := tx.OAuthClient.Create().
			SetID(resource.GetClientId()).
			SetSource(biz.ClientSourceDynamic).
			SetStatus(biz.ClientStatusActive).
			SetDisplayName(resource.GetDisplayName()).
			SetNillableDescription(resource.Description).
			SetNillableLogoURI(resource.LogoUri).
			SetApplicationType(appType).
			SetTokenEndpointAuthMethod(authMethod).
			SetRedirectUris(slices.Concat([]string{}, resource.GetRedirectUris())).
			SetPostLogoutRedirectUris(slices.Concat([]string{}, resource.GetPostLogoutRedirectUris())).
			SetAllowedGrantTypes(slices.Concat([]string{}, resource.GetAllowedGrantTypes())).
			SetAllowedResponseTypes(slices.Concat([]string{}, resource.GetAllowedResponseTypes())).
			SetAllowedScopes(slices.Concat([]string{}, resource.GetAllowedScopes())).
			SetAudiences(slices.Concat([]string{}, resource.GetAudiences())).
			SetTrusted(resource.GetTrusted()).
			SetEtag(etag).
			SetCreateTime(now).
			SetUpdateTime(now).
			Save(ctx)
		if err != nil {
			return translateEntError(err)
		}
		if initialSecret != nil {
			secBuilder := tx.OAuthClientSecret.Create().
				SetID(initialSecret.GetSecretId()).
				SetClientID(saved.ID).
				SetName(initialSecret.GetName()).
				SetSecretPrefix(initialSecret.GetSecretPrefix()).
				SetSecretHash(secretHash).
				SetCreateTime(now)
			if initialSecret.GetExpiresTime() != nil {
				secBuilder.SetExpiresTime(initialSecret.GetExpiresTime().AsTime().UTC())
			}
			if _, err := secBuilder.Save(ctx); err != nil {
				return translateEntError(err)
			}
		}
		var projErr error
		projected, projErr = repo.projectOAuthClient(ctx, tx.Client(), saved)
		return projErr
	})
	if err != nil {
		return nil, err
	}
	return projected, nil
}

func (repo *oauthClientRepository) Get(ctx context.Context, clientID string) (*iamclientv1.OAuthClient, error) {
	entity, err := repo.ent.OAuthClient.Get(ctx, clientID)
	if err != nil {
		return nil, translateEntError(err)
	}
	return repo.projectOAuthClient(ctx, repo.ent, entity)
}

func (repo *oauthClientRepository) List(ctx context.Context, query corecrud.ListQuery) (corecrud.ListResult[*iamclientv1.OAuthClient], error) {
	result, err := entcrud.List(ctx, repo.ent.OAuthClient.Query(), query, repo.listFields, listOAuthClientsScope)
	if err != nil {
		return corecrud.ListResult[*iamclientv1.OAuthClient]{}, translateEntError(err)
	}
	resources, err := projectOAuthClientsWithClient(ctx, repo.ent, repo.mapper, result.Items())
	if err != nil {
		return corecrud.ListResult[*iamclientv1.OAuthClient]{}, err
	}
	var total *int64
	if value, ok := result.TotalSize(); ok {
		total = &value
	}
	return corecrud.NewListResult(query, resources, result.NextPageToken(), total)
}

func (repo *oauthClientRepository) Update(
	ctx context.Context,
	clientID string,
	resource *iamclientv1.OAuthClient,
	mask *fieldmaskpb.FieldMask,
	expectedEtag string,
	now time.Time,
) (*iamclientv1.OAuthClient, error) {
	now = now.UTC()
	etag, err := biz.NewID()
	if err != nil {
		return nil, err
	}

	var projected *iamclientv1.OAuthClient
	err = inTx(ctx, repo.ent, func(tx *entmodel.Tx) error {
		current, err := lockOAuthClient(ctx, tx, clientID)
		if err != nil {
			return err
		}
		if current.Source == biz.ClientSourceStatic {
			return biz.ErrStaticClientImmutable
		}
		if current.Etag != expectedEtag {
			return biz.ErrMutationMiss
		}

		builder := tx.OAuthClient.UpdateOneID(clientID).Where(oauthclient.EtagEQ(expectedEtag))
		if err := repo.clear.Apply(resource, mask, builder.Mutation()); err != nil {
			return fmt.Errorf("apply IAM OAuthClient Clear intents: %w", err)
		}

		effectiveAuthMethod := current.TokenEndpointAuthMethod
		for _, path := range mask.GetPaths() {
			switch path {
			case iamclientv1.OAuthClientFields.DisplayName.String():
				if resource.DisplayName != nil {
					builder.SetDisplayName(resource.GetDisplayName())
				}
			case iamclientv1.OAuthClientFields.Description.String():
				if resource.Description != nil {
					builder.SetDescription(resource.GetDescription())
				}
			case iamclientv1.OAuthClientFields.LogoUri.String():
				if resource.LogoUri != nil {
					builder.SetLogoURI(resource.GetLogoUri())
				}
			case iamclientv1.OAuthClientFields.TokenEndpointAuthMethod.String():
				if resource.TokenEndpointAuthMethod != nil {
					authMethod, convErr := biz.AuthMethodToStorage(resource.GetTokenEndpointAuthMethod())
					if convErr != nil {
						return convErr
					}
					builder.SetTokenEndpointAuthMethod(authMethod)
					effectiveAuthMethod = authMethod
				}
			case iamclientv1.OAuthClientFields.RedirectUris.String():
				builder.SetRedirectUris(slices.Concat([]string{}, resource.GetRedirectUris()))
			case iamclientv1.OAuthClientFields.PostLogoutRedirectUris.String():
				builder.SetPostLogoutRedirectUris(slices.Concat([]string{}, resource.GetPostLogoutRedirectUris()))
			case iamclientv1.OAuthClientFields.AllowedGrantTypes.String():
				builder.SetAllowedGrantTypes(slices.Concat([]string{}, resource.GetAllowedGrantTypes()))
			case iamclientv1.OAuthClientFields.AllowedResponseTypes.String():
				builder.SetAllowedResponseTypes(slices.Concat([]string{}, resource.GetAllowedResponseTypes()))
			case iamclientv1.OAuthClientFields.AllowedScopes.String():
				builder.SetAllowedScopes(slices.Concat([]string{}, resource.GetAllowedScopes()))
			case iamclientv1.OAuthClientFields.Audiences.String():
				builder.SetAudiences(slices.Concat([]string{}, resource.GetAudiences()))
			case iamclientv1.OAuthClientFields.Trusted.String():
				if resource.Trusted != nil {
					builder.SetTrusted(resource.GetTrusted())
				}
			}
		}

		if effectiveAuthMethod != biz.AuthMethodNone {
			activeSecrets, countErr := tx.OAuthClientSecret.Query().
				Where(
					oauthclientsecret.ClientIDEQ(clientID),
					oauthclientsecret.Or(
						oauthclientsecret.ExpiresTimeIsNil(),
						oauthclientsecret.ExpiresTimeGT(now),
					),
				).
				Count(ctx)
			if countErr != nil {
				return translateEntError(countErr)
			}
			if activeSecrets == 0 {
				return fmt.Errorf("%w: confidential client requires at least one active secret", biz.ErrInvalidClientConfig)
			}
		}

		updated, err := builder.SetEtag(etag).SetUpdateTime(now).Save(ctx)
		if err != nil {
			if entmodel.IsNotFound(err) {
				return biz.ErrMutationMiss
			}
			return translateEntError(err)
		}
		if effectiveAuthMethod == biz.AuthMethodNone && current.TokenEndpointAuthMethod != biz.AuthMethodNone {
			if _, err := tx.OAuthClientSecret.Delete().Where(oauthclientsecret.ClientIDEQ(clientID)).Exec(ctx); err != nil {
				return translateEntError(err)
			}
		}
		var projErr error
		projected, projErr = repo.projectOAuthClient(ctx, tx.Client(), updated)
		return projErr
	})
	if err != nil {
		return nil, err
	}
	return projected, nil
}

func (repo *oauthClientRepository) UpdateStatus(
	ctx context.Context,
	clientID, expectedEtag string,
	status iamclientv1.ClientStatus,
	now time.Time,
) (*iamclientv1.OAuthClient, error) {
	targetStatus, err := biz.ClientStatusToStorage(status)
	if err != nil {
		return nil, err
	}
	now = now.UTC()
	etag, err := biz.NewID()
	if err != nil {
		return nil, err
	}

	var projected *iamclientv1.OAuthClient
	err = inTx(ctx, repo.ent, func(tx *entmodel.Tx) error {
		current, err := lockOAuthClient(ctx, tx, clientID)
		if err != nil {
			return err
		}
		if current.Source == biz.ClientSourceStatic {
			return biz.ErrStaticClientImmutable
		}
		if targetStatus == biz.ClientStatusActive && current.Status != biz.ClientStatusDisabled {
			return biz.ErrClientInvalidState
		}
		if current.Etag != expectedEtag {
			return biz.ErrMutationMiss
		}
		updated, err := tx.OAuthClient.UpdateOneID(clientID).
			Where(oauthclient.EtagEQ(expectedEtag)).
			SetStatus(targetStatus).
			SetEtag(etag).
			SetUpdateTime(now).
			Save(ctx)
		if err != nil {
			if entmodel.IsNotFound(err) {
				return biz.ErrMutationMiss
			}
			return translateEntError(err)
		}
		if targetStatus == biz.ClientStatusDisabled {
			if err := cascadeRevokeClientTx(ctx, tx, clientID, false, now); err != nil {
				return err
			}
		}
		var projErr error
		projected, projErr = repo.projectOAuthClient(ctx, tx.Client(), updated)
		return projErr
	})
	if err != nil {
		return nil, err
	}
	return projected, nil
}

func (repo *oauthClientRepository) Delete(ctx context.Context, clientID, expectedEtag string, now time.Time) error {
	now = now.UTC()
	return inTx(ctx, repo.ent, func(tx *entmodel.Tx) error {
		current, err := lockOAuthClient(ctx, tx, clientID)
		if err != nil {
			return err
		}
		if current.Source == biz.ClientSourceStatic {
			return biz.ErrStaticClientImmutable
		}
		if current.Etag != expectedEtag {
			return biz.ErrMutationMiss
		}
		return cascadeRevokeClientTx(ctx, tx, clientID, true, now)
	})
}

func (repo *oauthClientRepository) CreateSecret(
	ctx context.Context,
	clientID string,
	secret *iamclientv1.OAuthClientSecret,
	secretHash string,
	now time.Time,
) (*iamclientv1.OAuthClientSecret, error) {
	now = now.UTC()
	var created *iamclientv1.OAuthClientSecret
	err := inTx(ctx, repo.ent, func(tx *entmodel.Tx) error {
		current, err := lockOAuthClient(ctx, tx, clientID)
		if err != nil {
			return err
		}
		if current.Source == biz.ClientSourceStatic {
			return biz.ErrStaticClientImmutable
		}
		if current.TokenEndpointAuthMethod == biz.AuthMethodNone {
			return fmt.Errorf("%w: public client cannot hold secrets", biz.ErrInvalidClientConfig)
		}
		count, err := tx.OAuthClientSecret.Query().Where(oauthclientsecret.ClientIDEQ(clientID)).Count(ctx)
		if err != nil {
			return translateEntError(err)
		}
		if count >= biz.MaxClientSecrets {
			return biz.ErrSecretLimitExceeded
		}
		builder := tx.OAuthClientSecret.Create().
			SetID(secret.GetSecretId()).
			SetClientID(clientID).
			SetName(secret.GetName()).
			SetSecretPrefix(secret.GetSecretPrefix()).
			SetSecretHash(secretHash).
			SetCreateTime(now)
		if secret.GetExpiresTime() != nil {
			builder.SetExpiresTime(secret.GetExpiresTime().AsTime().UTC())
		}
		saved, err := builder.Save(ctx)
		if err != nil {
			return translateEntError(err)
		}
		created = projectSecretMetadata(saved)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return created, nil
}

func (repo *oauthClientRepository) DeleteSecret(
	ctx context.Context,
	clientID, secretID string,
	now time.Time,
) error {
	now = now.UTC()
	return inTx(ctx, repo.ent, func(tx *entmodel.Tx) error {
		current, err := lockOAuthClient(ctx, tx, clientID)
		if err != nil {
			return err
		}
		if current.Source == biz.ClientSourceStatic {
			return biz.ErrStaticClientImmutable
		}
		if current.TokenEndpointAuthMethod == biz.AuthMethodNone {
			return fmt.Errorf("%w: public client has no secrets", biz.ErrInvalidClientConfig)
		}
		secrets, err := tx.OAuthClientSecret.Query().
			Where(oauthclientsecret.ClientIDEQ(clientID)).
			All(ctx)
		if err != nil {
			return translateEntError(err)
		}
		var (
			foundTarget     bool
			targetActive    bool
			remainingActive int
		)
		for _, item := range secrets {
			active := item.ExpiresTime == nil || item.ExpiresTime.After(now)
			if item.ID == secretID {
				foundTarget = true
				targetActive = active
				continue
			}
			if active {
				remainingActive++
			}
		}
		if !foundTarget {
			return biz.ErrNotFound
		}
		if (targetActive && remainingActive < 1) || len(secrets) <= 1 {
			return biz.ErrLastActiveSecret
		}
		return translateEntError(tx.OAuthClientSecret.DeleteOneID(secretID).Exec(ctx))
	})
}

func lockOAuthClient(ctx context.Context, tx *entmodel.Tx, clientID string) (*entmodel.OAuthClient, error) {
	entity, err := tx.OAuthClient.Query().Where(oauthclient.IDEQ(clientID)).ForUpdate().Only(ctx)
	return entity, translateEntError(err)
}

func cascadeRevokeClientTx(ctx context.Context, tx *entmodel.Tx, clientID string, deleteClient bool, now time.Time) error {
	pendingReqIDs, err := tx.OIDCAuthorizationRequest.Query().
		Where(
			oidcauthorizationrequest.ClientIDEQ(clientID),
			oidcauthorizationrequest.DoneEQ(false),
		).
		IDs(ctx)
	if err != nil {
		return translateEntError(err)
	}
	if len(pendingReqIDs) > 0 {
		if _, err := tx.AuthFlow.Update().
			Where(
				authflow.OidcRequestIDIn(pendingReqIDs...),
				authflow.StatusEQ(authflow.StatusActive),
			).
			SetStatus(authflow.StatusInvalidated).
			SetUpdateTime(now).
			Save(ctx); err != nil {
			return translateEntError(err)
		}
		if _, err := tx.OIDCAuthorizationRequest.Delete().
			Where(oidcauthorizationrequest.IDIn(pendingReqIDs...)).
			Exec(ctx); err != nil {
			return translateEntError(err)
		}
	}

	if _, err := tx.OAuthAuthorizationCode.Update().
		Where(
			oauthauthorizationcode.ClientIDEQ(clientID),
			oauthauthorizationcode.ConsumedTimeIsNil(),
		).
		SetConsumedTime(now).
		Save(ctx); err != nil {
		return translateEntError(err)
	}

	if err := revokeOAuthSessions(ctx, tx, now, oauthtokensession.ClientIDEQ(clientID)); err != nil {
		return err
	}

	if _, err := tx.OAuthAccessToken.Update().
		Where(
			oauthaccesstoken.ClientIDEQ(clientID),
			oauthaccesstoken.RevokedTimeIsNil(),
		).
		SetRevokedTime(now).
		Save(ctx); err != nil {
		return translateEntError(err)
	}

	if deleteClient {
		if _, err := tx.OAuthClientSecret.Delete().Where(oauthclientsecret.ClientIDEQ(clientID)).Exec(ctx); err != nil {
			return translateEntError(err)
		}
		if _, err := tx.OAuthConsent.Delete().Where(oauthconsent.ClientIDEQ(clientID)).Exec(ctx); err != nil {
			return translateEntError(err)
		}
		if err := tx.OAuthClient.DeleteOneID(clientID).Exec(ctx); err != nil {
			return translateEntError(err)
		}
	}
	return nil
}

func (repo *oauthClientRepository) projectOAuthClient(
	ctx context.Context,
	client *entmodel.Client,
	entity *entmodel.OAuthClient,
) (*iamclientv1.OAuthClient, error) {
	resources, err := projectOAuthClientsWithClient(ctx, client, repo.mapper, []*entmodel.OAuthClient{entity})
	if err != nil {
		return nil, err
	}
	return resources[0], nil
}

func projectOAuthClientsWithClient(
	ctx context.Context,
	client *entmodel.Client,
	mapper *crudmapper.ResourceMapper[*iamclientv1.OAuthClient, entmodel.OAuthClient],
	entities []*entmodel.OAuthClient,
) ([]*iamclientv1.OAuthClient, error) {
	if len(entities) == 0 {
		return nil, nil
	}
	clientIDs := make([]string, len(entities))
	for index, entity := range entities {
		clientIDs[index] = entity.ID
	}
	secretRows, err := client.OAuthClientSecret.Query().
		Where(oauthclientsecret.ClientIDIn(clientIDs...)).
		Order(oauthclientsecret.ByCreateTime(entsql.OrderAsc()), oauthclientsecret.ByID(entsql.OrderAsc())).
		All(ctx)
	if err != nil {
		return nil, translateEntError(err)
	}
	secretsByClientID := make(map[string][]*iamclientv1.OAuthClientSecret, len(entities))
	for _, sec := range secretRows {
		secretsByClientID[sec.ClientID] = append(secretsByClientID[sec.ClientID], projectSecretMetadata(sec))
	}
	resources := make([]*iamclientv1.OAuthClient, len(entities))
	for index, entity := range entities {
		resource, err := mapper.TryToDTO(entity)
		if err != nil {
			return nil, fmt.Errorf("map IAM OAuthClient %q: %w", entity.ID, err)
		}
		resource.Secrets = secretsByClientID[entity.ID]
		resources[index] = resource
	}
	return resources, nil
}

func projectSecretMetadata(entity *entmodel.OAuthClientSecret) *iamclientv1.OAuthClientSecret {
	out := &iamclientv1.OAuthClientSecret{
		SecretId:     entity.ID,
		Name:         entity.Name,
		SecretPrefix: entity.SecretPrefix,
		CreateTime:   timestamppb.New(entity.CreateTime.UTC()),
	}
	if entity.ExpiresTime != nil {
		out.ExpiresTime = timestamppb.New(entity.ExpiresTime.UTC())
	}
	if entity.LastUsedTime != nil {
		out.LastUsedTime = timestamppb.New(entity.LastUsedTime.UTC())
	}
	return out
}
