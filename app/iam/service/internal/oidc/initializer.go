package oidc

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	iamclientv1 "github.com/Servora-Kit/plateau/api/gen/go/plateau/iam/client/v1"
	oidcconfpb "github.com/Servora-Kit/plateau/api/gen/go/plateau/iam/oidc/conf/v1"
	"github.com/Servora-Kit/plateau/app/iam/service/internal/biz"
	"github.com/Servora-Kit/plateau/app/iam/service/internal/data/ent"
	"github.com/Servora-Kit/plateau/app/iam/service/internal/data/ent/oauthclient"
	"github.com/Servora-Kit/plateau/app/iam/service/internal/data/ent/oauthclientsecret"
	"github.com/Servora-Kit/plateau/app/iam/service/internal/data/ent/predicate"
	"github.com/go-jose/go-jose/v4"
)

// OIDCInitializer reconciles static OAuth clients and current signing-key metadata before serving traffic.
type OIDCInitializer struct {
	config  *oidcconfpb.OIDC
	storage *OIDCStorage
}

func NewOIDCInitializer(config *oidcconfpb.OIDC, storage *OIDCStorage) (*OIDCInitializer, error) {
	if config == nil || storage == nil {
		return nil, fmt.Errorf("OIDC bootstrap dependencies are nil")
	}
	if err := config.Apply(); err != nil {
		return nil, fmt.Errorf("OIDC initializer config: %w", err)
	}
	if _, _, err := biz.NormalizeOIDCIssuer(config.GetIssuer()); err != nil {
		return nil, err
	}
	return &OIDCInitializer{config: config, storage: storage}, nil
}

func (initializer *OIDCInitializer) Initialize(ctx context.Context) error {
	if err := initializer.reconcileSigningKey(ctx); err != nil {
		return err
	}
	now := initializer.storage.now().UTC()
	configuredIDs := make([]string, 0, len(initializer.config.GetClients()))
	seenIDs := make(map[string]struct{}, len(initializer.config.GetClients()))
	for _, configured := range initializer.config.GetClients() {
		normalized, rawSecret, err := parseAndValidateConfiguredClient(configured, seenIDs)
		if err != nil {
			return err
		}
		configuredIDs = append(configuredIDs, normalized.GetClientId())
		if err := initializer.reconcileClient(ctx, normalized, rawSecret, now); err != nil {
			return err
		}
	}

	removePreds := []predicate.OAuthClient{oauthclient.SourceEQ(biz.ClientSourceStatic)}
	if len(configuredIDs) > 0 {
		removePreds = append(removePreds, oauthclient.IDNotIn(configuredIDs...))
	}
	removedStaticIDs, err := initializer.storage.client.OAuthClient.Query().
		Where(removePreds...).
		IDs(ctx)
	if err != nil {
		return fmt.Errorf("query static OAuth clients to remove: %w", err)
	}
	for _, removedID := range removedStaticIDs {
		if err := initializer.storage.tokens.PurgeStaticClient(ctx, removedID, now); err != nil {
			return fmt.Errorf("delete OAuth client %q removed from static configuration: %w", removedID, err)
		}
	}
	return nil
}

func (initializer *OIDCInitializer) reconcileSigningKey(ctx context.Context) error {
	now := initializer.storage.now().UTC()
	existing, err := initializer.storage.client.OIDCSigningKey.Get(ctx, initializer.storage.signingPrivate.id)
	if ent.IsNotFound(err) {
		if _, err := initializer.storage.client.OIDCSigningKey.Create().
			SetID(initializer.storage.signingPrivate.id).
			SetPublicJwk(initializer.storage.signingPublicJWK).
			SetAlgorithm(string(jose.RS256)).
			SetNotBeforeTime(now.Add(-time.Minute)).
			SetExpiresTime(now.AddDate(10, 0, 0)).
			Save(ctx); err != nil {
			return fmt.Errorf("seed OIDC signing key metadata: %w", err)
		}
		return nil
	}
	if err != nil {
		return fmt.Errorf("query OIDC signing key metadata: %w", err)
	}
	if existing.PublicJwk != initializer.storage.signingPublicJWK || existing.Algorithm != string(jose.RS256) {
		return fmt.Errorf("OIDC signing key metadata conflicts with mounted private key")
	}
	if existing.RevokedTime != nil || !existing.ExpiresTime.After(now) || existing.NotBeforeTime.After(now) {
		return fmt.Errorf("OIDC signing key metadata is inactive")
	}
	return nil
}

func (initializer *OIDCInitializer) reconcileClient(
	ctx context.Context,
	normalized *iamclientv1.OAuthClient,
	rawSecret string,
	now time.Time,
) error {
	clientID := normalized.GetClientId()
	appType, err := biz.ApplicationTypeToStorage(normalized.GetApplicationType())
	if err != nil {
		return err
	}
	authMethod, err := biz.AuthMethodToStorage(normalized.GetTokenEndpointAuthMethod())
	if err != nil {
		return err
	}

	tx, err := initializer.storage.client.Tx(ctx)
	if err != nil {
		return fmt.Errorf("begin static OAuth client reconciliation tx for %q: %w", clientID, err)
	}
	defer func() { _ = tx.Rollback() }()

	existing, err := tx.OAuthClient.Get(ctx, clientID)
	if err != nil && !ent.IsNotFound(err) {
		return fmt.Errorf("query OAuth client %q: %w", clientID, err)
	}
	if existing != nil && existing.Source != biz.ClientSourceStatic {
		return fmt.Errorf("static OIDC client %q conflicts with existing %s OAuth client", clientID, existing.Source)
	}

	if existing != nil && existing.ApplicationType != appType {
		if err := tx.Rollback(); err != nil {
			return fmt.Errorf("rollback before static OAuth client %q type replacement: %w", clientID, err)
		}
		if err := initializer.storage.tokens.PurgeStaticClient(ctx, clientID, now); err != nil {
			return fmt.Errorf("purge static OAuth client %q for application_type change: %w", clientID, err)
		}
		tx, err = initializer.storage.client.Tx(ctx)
		if err != nil {
			return fmt.Errorf("restart static OAuth client reconciliation tx for %q: %w", clientID, err)
		}
		defer func() { _ = tx.Rollback() }()
		existing = nil
	}

	if existing == nil {
		etag, err := biz.NewID()
		if err != nil {
			return err
		}
		if _, err := tx.OAuthClient.Create().
			SetID(clientID).
			SetSource(biz.ClientSourceStatic).
			SetStatus(biz.ClientStatusActive).
			SetDisplayName(normalized.GetDisplayName()).
			SetNillableDescription(normalized.Description).
			SetNillableLogoURI(normalized.LogoUri).
			SetApplicationType(appType).
			SetTokenEndpointAuthMethod(authMethod).
			SetRedirectUris(normalized.GetRedirectUris()).
			SetPostLogoutRedirectUris(normalized.GetPostLogoutRedirectUris()).
			SetAllowedGrantTypes(normalized.GetAllowedGrantTypes()).
			SetAllowedResponseTypes(normalized.GetAllowedResponseTypes()).
			SetAllowedScopes(normalized.GetAllowedScopes()).
			SetAudiences(normalized.GetAudiences()).
			SetTrusted(normalized.GetTrusted()).
			SetEtag(etag).
			SetCreateTime(now).
			SetUpdateTime(now).
			Save(ctx); err != nil {
			return fmt.Errorf("seed OAuth client %q: %w", clientID, err)
		}
	} else if !staticClientMatches(existing, normalized, appType, authMethod) {
		etag, err := biz.NewID()
		if err != nil {
			return err
		}
		builder := tx.OAuthClient.UpdateOneID(clientID).
			SetStatus(biz.ClientStatusActive).
			SetDisplayName(normalized.GetDisplayName()).
			SetTokenEndpointAuthMethod(authMethod).
			SetRedirectUris(normalized.GetRedirectUris()).
			SetPostLogoutRedirectUris(normalized.GetPostLogoutRedirectUris()).
			SetAllowedGrantTypes(normalized.GetAllowedGrantTypes()).
			SetAllowedResponseTypes(normalized.GetAllowedResponseTypes()).
			SetAllowedScopes(normalized.GetAllowedScopes()).
			SetAudiences(normalized.GetAudiences()).
			SetTrusted(normalized.GetTrusted()).
			SetEtag(etag).
			SetUpdateTime(now)
		if normalized.Description == nil {
			builder.ClearDescription()
		} else {
			builder.SetDescription(normalized.GetDescription())
		}
		if normalized.LogoUri == nil {
			builder.ClearLogoURI()
		} else {
			builder.SetLogoURI(normalized.GetLogoUri())
		}
		if _, err := builder.Save(ctx); err != nil {
			return fmt.Errorf("reconcile OAuth client %q: %w", clientID, err)
		}
	}

	if err := reconcileStaticClientSecret(ctx, tx, clientID, authMethod, rawSecret, now); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit OAuth client %q reconciliation: %w", clientID, err)
	}
	return nil
}

func reconcileStaticClientSecret(
	ctx context.Context,
	tx *ent.Tx,
	clientID, authMethod, rawSecret string,
	now time.Time,
) error {
	if authMethod == biz.AuthMethodNone {
		if _, err := tx.OAuthClientSecret.Delete().Where(oauthclientsecret.ClientIDEQ(clientID)).Exec(ctx); err != nil {
			return fmt.Errorf("clear static public OAuth client %q secrets: %w", clientID, err)
		}
		return nil
	}

	wantHash := biz.HashOpaqueSecret(rawSecret)
	wantPrefix := biz.SecretDisplayPrefix(rawSecret)
	existingSecrets, err := tx.OAuthClientSecret.Query().
		Where(oauthclientsecret.ClientIDEQ(clientID)).
		All(ctx)
	if err != nil {
		return fmt.Errorf("query static OAuth client %q secrets: %w", clientID, err)
	}
	if len(existingSecrets) == 1 &&
		existingSecrets[0].SecretHash == wantHash &&
		existingSecrets[0].SecretPrefix == wantPrefix &&
		existingSecrets[0].ExpiresTime == nil {
		return nil
	}
	if len(existingSecrets) == 1 {
		if _, err := tx.OAuthClientSecret.UpdateOneID(existingSecrets[0].ID).
			SetSecretHash(wantHash).
			SetSecretPrefix(wantPrefix).
			ClearExpiresTime().
			Save(ctx); err != nil {
			return fmt.Errorf("rotate static OAuth client %q secret: %w", clientID, err)
		}
		return nil
	}

	if len(existingSecrets) > 0 {
		if _, err := tx.OAuthClientSecret.Delete().Where(oauthclientsecret.ClientIDEQ(clientID)).Exec(ctx); err != nil {
			return fmt.Errorf("replace static OAuth client %q secrets: %w", clientID, err)
		}
	}
	secretID, err := biz.NewID()
	if err != nil {
		return err
	}
	if _, err := tx.OAuthClientSecret.Create().
		SetID(secretID).
		SetClientID(clientID).
		SetName(biz.DefaultSecretName).
		SetSecretPrefix(wantPrefix).
		SetSecretHash(wantHash).
		SetCreateTime(now).
		Save(ctx); err != nil {
		return fmt.Errorf("seed static OAuth client %q secret: %w", clientID, err)
	}
	return nil
}

func staticClientMatches(
	existing *ent.OAuthClient,
	normalized *iamclientv1.OAuthClient,
	appType, authMethod string,
) bool {
	existingDesc := ""
	if existing.Description != nil {
		existingDesc = *existing.Description
	}
	existingLogo := ""
	if existing.LogoURI != nil {
		existingLogo = *existing.LogoURI
	}
	return existing.Status == biz.ClientStatusActive &&
		existing.DisplayName == normalized.GetDisplayName() &&
		(existing.Description != nil) == (normalized.Description != nil) &&
		existingDesc == normalized.GetDescription() &&
		(existing.LogoURI != nil) == (normalized.LogoUri != nil) &&
		existingLogo == normalized.GetLogoUri() &&
		existing.ApplicationType == appType &&
		existing.TokenEndpointAuthMethod == authMethod &&
		slices.Equal(existing.RedirectUris, normalized.GetRedirectUris()) &&
		slices.Equal(existing.PostLogoutRedirectUris, normalized.GetPostLogoutRedirectUris()) &&
		slices.Equal(existing.AllowedGrantTypes, normalized.GetAllowedGrantTypes()) &&
		slices.Equal(existing.AllowedResponseTypes, normalized.GetAllowedResponseTypes()) &&
		slices.Equal(existing.AllowedScopes, normalized.GetAllowedScopes()) &&
		slices.Equal(existing.Audiences, normalized.GetAudiences()) &&
		existing.Trusted == normalized.GetTrusted()
}

func parseAndValidateConfiguredClient(
	configured *oidcconfpb.OAuthClient,
	seenIDs map[string]struct{},
) (*iamclientv1.OAuthClient, string, error) {
	if configured == nil {
		return nil, "", fmt.Errorf("OIDC client configuration is nil")
	}
	clientID := strings.TrimSpace(configured.GetClientId())
	if clientID == "" || clientID != configured.GetClientId() {
		return nil, "", fmt.Errorf("OIDC client ID is empty")
	}
	if _, duplicate := seenIDs[clientID]; duplicate {
		return nil, "", fmt.Errorf("duplicate OIDC client ID %q", clientID)
	}
	seenIDs[clientID] = struct{}{}

	appTypeRaw := strings.TrimSpace(configured.GetApplicationType())
	if appTypeRaw == "" {
		appTypeRaw = biz.AppTypeWeb
	}
	appType, err := biz.ApplicationTypeFromStorage(appTypeRaw)
	if err != nil {
		return nil, "", fmt.Errorf("OIDC client %q: %w", clientID, err)
	}

	authMethodRaw := strings.TrimSpace(configured.GetTokenEndpointAuthMethod())
	if authMethodRaw == "" {
		authMethodRaw = biz.AuthMethodBasic
	}
	authMethod, err := biz.AuthMethodFromStorage(authMethodRaw)
	if err != nil {
		return nil, "", fmt.Errorf("OIDC client %q: %w", clientID, err)
	}

	rawSecret := configured.GetClientSecret()
	if authMethodRaw == biz.AuthMethodNone {
		if rawSecret != "" {
			return nil, "", fmt.Errorf("public OIDC client %q must not configure client_secret", clientID)
		}
	} else if len(rawSecret) < 32 {
		return nil, "", fmt.Errorf("OIDC client %q secret must contain at least 32 bytes", clientID)
	}

	displayName := strings.TrimSpace(configured.GetDisplayName())
	if displayName == "" {
		displayName = clientID
	}

	normalized, err := biz.ValidateAndNormalizeOAuthClient(
		clientID,
		&iamclientv1.OAuthClient{
			DisplayName:             &displayName,
			Description:             configured.Description,
			LogoUri:                 configured.LogoUri,
			ApplicationType:         &appType,
			TokenEndpointAuthMethod: &authMethod,
			RedirectUris:            configured.GetRedirectUris(),
			PostLogoutRedirectUris:  configured.GetPostLogoutRedirectUris(),
			AllowedGrantTypes:       configured.GetAllowedGrantTypes(),
			AllowedScopes:           configured.GetAllowedScopes(),
			Audiences:               configured.GetAudiences(),
			Trusted:                 new(configured.GetTrusted()),
		},
		iamclientv1.ClientSource_CLIENT_SOURCE_STATIC,
		iamclientv1.ClientStatus_CLIENT_STATUS_ACTIVE,
	)
	if err != nil {
		return nil, "", fmt.Errorf("OIDC client %q: %w", clientID, err)
	}
	return normalized, rawSecret, nil
}
