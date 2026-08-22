package oidc

import (
	"context"
	"fmt"
	"net/url"
	"slices"
	"strings"
	"time"

	oidcconfpb "github.com/Servora-Kit/plateau/api/gen/go/iam/oidc/conf/v1"
	"github.com/Servora-Kit/plateau/app/iam/service/internal/biz"
	"github.com/Servora-Kit/plateau/app/iam/service/internal/data/ent"
	"github.com/Servora-Kit/plateau/app/iam/service/internal/data/ent/oauthclient"
	"github.com/go-jose/go-jose/v4"
	"github.com/zitadel/oidc/v3/pkg/oidc"
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
	if _, _, err := normalizeIssuer(config.GetIssuer()); err != nil {
		return nil, err
	}
	if len(config.GetClients()) == 0 {
		return nil, fmt.Errorf("OIDC requires at least one static OAuth client")
	}
	return &OIDCInitializer{config: config, storage: storage}, nil
}

func (initializer *OIDCInitializer) Initialize(ctx context.Context) error {
	if err := initializer.reconcileSigningKey(ctx); err != nil {
		return err
	}
	configuredIDs := make([]string, 0, len(initializer.config.GetClients()))
	seenIDs := make(map[string]struct{}, len(initializer.config.GetClients()))
	for _, configured := range initializer.config.GetClients() {
		clientID, redirectURIs, scopes, err := validateConfiguredClient(configured, seenIDs)
		if err != nil {
			return err
		}
		configuredIDs = append(configuredIDs, clientID)
		if err := initializer.reconcileClient(ctx, configured, clientID, redirectURIs, scopes); err != nil {
			return err
		}
	}
	if _, err := initializer.storage.client.OAuthClient.Delete().
		Where(oauthclient.IDNotIn(configuredIDs...)).
		Exec(ctx); err != nil {
		return fmt.Errorf("delete OAuth clients removed from static configuration: %w", err)
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
	configured *oidcconfpb.OAuthClient,
	clientID string,
	redirectURIs, scopes []string,
) error {
	existing, err := initializer.storage.client.OAuthClient.Get(ctx, clientID)
	if ent.IsNotFound(err) {
		secretHash := biz.HashOpaqueSecret(configured.GetClientSecret())
		if _, err := initializer.storage.client.OAuthClient.Create().
			SetID(clientID).
			SetSecretHash(secretHash).
			SetRedirectUris(redirectURIs).
			SetAllowedGrantTypes([]string{string(oidc.GrantTypeCode), string(oidc.GrantTypeRefreshToken)}).
			SetAllowedResponseTypes([]string{string(oidc.ResponseTypeCode)}).
			SetAllowedScopes(scopes).
			SetTrusted(configured.GetTrusted()).
			Save(ctx); err != nil {
			return fmt.Errorf("seed OAuth client %q: %w", clientID, err)
		}
		return nil
	}
	if err != nil {
		return fmt.Errorf("query OAuth client %q: %w", clientID, err)
	}
	if biz.HashOpaqueSecret(configured.GetClientSecret()) != existing.SecretHash {
		return fmt.Errorf("configured OAuth client %q secret conflicts with persisted seed", clientID)
	}
	if _, err := initializer.storage.client.OAuthClient.UpdateOneID(clientID).
		SetRedirectUris(redirectURIs).
		SetAllowedGrantTypes([]string{string(oidc.GrantTypeCode), string(oidc.GrantTypeRefreshToken)}).
		SetAllowedResponseTypes([]string{string(oidc.ResponseTypeCode)}).
		SetAllowedScopes(scopes).
		SetTrusted(configured.GetTrusted()).
		Save(ctx); err != nil {
		return fmt.Errorf("reconcile OAuth client %q: %w", clientID, err)
	}
	return nil
}

func validateConfiguredClient(
	configured *oidcconfpb.OAuthClient,
	seenIDs map[string]struct{},
) (string, []string, []string, error) {
	if configured == nil {
		return "", nil, nil, fmt.Errorf("OIDC client configuration is nil")
	}
	clientID := strings.TrimSpace(configured.GetClientId())
	if clientID == "" {
		return "", nil, nil, fmt.Errorf("OIDC client ID is empty")
	}
	if _, duplicate := seenIDs[clientID]; duplicate {
		return "", nil, nil, fmt.Errorf("duplicate OIDC client ID %q", clientID)
	}
	seenIDs[clientID] = struct{}{}
	if len(configured.GetClientSecret()) < 32 {
		return "", nil, nil, fmt.Errorf("OIDC client %q secret must contain at least 32 bytes", clientID)
	}
	if !configured.GetTrusted() {
		return "", nil, nil, fmt.Errorf("OIDC client %q must be trusted because consent is unsupported", clientID)
	}
	redirectURIs := deduplicate(configured.GetRedirectUris())
	if len(redirectURIs) == 0 {
		return "", nil, nil, fmt.Errorf("OIDC client %q requires a redirect URI", clientID)
	}
	for _, redirectURI := range redirectURIs {
		if err := validateRedirectURI(redirectURI); err != nil {
			return "", nil, nil, fmt.Errorf("OIDC client %q: %w", clientID, err)
		}
	}
	scopes := deduplicate(configured.GetAllowedScopes())
	if !slices.Contains(scopes, oidc.ScopeOpenID) {
		return "", nil, nil, fmt.Errorf("OIDC client %q must allow the openid scope", clientID)
	}
	for _, scope := range scopes {
		if !slices.Contains(supportedScopes, scope) {
			return "", nil, nil, fmt.Errorf("OIDC client %q contains unsupported scope %q", clientID, scope)
		}
	}
	return clientID, redirectURIs, scopes, nil
}

func validateRedirectURI(raw string) error {
	if strings.Contains(raw, "*") {
		return fmt.Errorf("redirect URI wildcards are forbidden")
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" || parsed.Fragment != "" {
		return fmt.Errorf("redirect URI %q must be absolute and omit fragments", raw)
	}
	if parsed.Scheme == "https" {
		return nil
	}
	if parsed.Scheme == "http" && isLoopbackHost(parsed.Hostname()) {
		return nil
	}
	return fmt.Errorf("redirect URI %q must use HTTPS except on localhost", raw)
}

func deduplicate(values []string) []string {
	result := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}
