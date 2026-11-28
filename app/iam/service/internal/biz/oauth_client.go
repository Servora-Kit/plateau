package biz

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"time"

	iamclientv1 "github.com/Servora-Kit/plateau/api/gen/go/plateau/iam/client/v1"
	corecrud "github.com/Servora-Kit/plateau/crud"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/fieldmaskpb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

var (
	clientIDSlugPattern = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$`)
	customSchemePattern = regexp.MustCompile(`^[a-z][a-z0-9+.-]*$`)
)

// OAuthClientRepo is the persistence port for OAuth 2.0 / OIDC client management.
type OAuthClientRepo interface {
	Create(ctx context.Context, resource *iamclientv1.OAuthClient, initialSecret *iamclientv1.OAuthClientSecret, secretHash string, now time.Time) (*iamclientv1.OAuthClient, error)
	Get(ctx context.Context, clientID string) (*iamclientv1.OAuthClient, error)
	List(ctx context.Context, query corecrud.ListQuery) (corecrud.ListResult[*iamclientv1.OAuthClient], error)
	Update(ctx context.Context, clientID string, resource *iamclientv1.OAuthClient, mask *fieldmaskpb.FieldMask, expectedEtag string, now time.Time) (*iamclientv1.OAuthClient, error)
	UpdateStatus(ctx context.Context, clientID, expectedEtag string, status iamclientv1.ClientStatus, now time.Time) (*iamclientv1.OAuthClient, error)
	Delete(ctx context.Context, clientID, expectedEtag string, now time.Time) error
	CreateSecret(ctx context.Context, clientID string, secret *iamclientv1.OAuthClientSecret, secretHash string, now time.Time) (*iamclientv1.OAuthClientSecret, error)
	DeleteSecret(ctx context.Context, clientID, secretID string, now time.Time) error
}

// OAuthClientUsecase owns dynamic OAuth client CRUD, secret lifecycle, and static client immutability.
type OAuthClientUsecase struct {
	clients OAuthClientRepo
}

// NewOAuthClientUsecase constructs the dynamic OAuth client management usecase.
func NewOAuthClientUsecase(clients OAuthClientRepo) (*OAuthClientUsecase, error) {
	if clients == nil {
		return nil, fmt.Errorf("oauth client: repository dependency is nil")
	}
	return &OAuthClientUsecase{clients: clients}, nil
}

// RegisterOAuthClient validates and persists a dynamic OAuth client, generating a one-time secret when confidential.
func (uc *OAuthClientUsecase) RegisterOAuthClient(
	ctx context.Context,
	customClientID string,
	prepared corecrud.PreparedCreate[*iamclientv1.OAuthClient],
	initialSecretName string,
) (*iamclientv1.OAuthClient, string, error) {
	now := time.Now().UTC()

	clientID := customClientID
	if clientID != "" {
		if !ValidateClientIDSlug(clientID) {
			return nil, "", fmt.Errorf("%w: client_id %q must be 1-63 lowercase alphanumeric or hyphen characters", ErrInvalidClientConfig, clientID)
		}
	} else {
		generated, err := NewGeneratedClientID()
		if err != nil {
			return nil, "", err
		}
		clientID = generated
	}

	toCreate, err := ValidateAndNormalizeOAuthClient(
		clientID,
		prepared.Resource(),
		iamclientv1.ClientSource_CLIENT_SOURCE_DYNAMIC,
		iamclientv1.ClientStatus_CLIENT_STATUS_ACTIVE,
	)
	if err != nil {
		return nil, "", err
	}

	var (
		initialSecret *iamclientv1.OAuthClientSecret
		secretHash    string
		rawSecret     string
	)
	if toCreate.GetTokenEndpointAuthMethod() != iamclientv1.TokenEndpointAuthMethod_TOKEN_ENDPOINT_AUTH_METHOD_NONE {
		initialSecret, rawSecret, secretHash, err = newClientSecretRecord(initialSecretName, nil, now)
		if err != nil {
			return nil, "", err
		}
	}

	created, err := uc.clients.Create(ctx, toCreate, initialSecret, secretHash, now)
	if err != nil {
		return nil, "", err
	}
	return created, rawSecret, nil
}

// GetOAuthClient returns one static or dynamic OAuth client by resource name.
func (uc *OAuthClientUsecase) GetOAuthClient(ctx context.Context, name iamclientv1.OAuthClientName) (*iamclientv1.OAuthClient, error) {
	return uc.clients.Get(ctx, name.OauthClient)
}

// ListOAuthClients lists static and dynamic OAuth clients.
func (uc *OAuthClientUsecase) ListOAuthClients(ctx context.Context, query corecrud.ListQuery) (corecrud.ListResult[*iamclientv1.OAuthClient], error) {
	return uc.clients.List(ctx, query)
}

// UpdateOAuthClient updates mutable fields of a dynamic OAuth client while enforcing optimistic concurrency and matrix invariants.
func (uc *OAuthClientUsecase) UpdateOAuthClient(
	ctx context.Context,
	name iamclientv1.OAuthClientName,
	prepared corecrud.PreparedUpdate[*iamclientv1.OAuthClient],
) (*iamclientv1.OAuthClient, error) {
	now := time.Now().UTC()
	current, err := uc.clients.Get(ctx, name.OauthClient)
	if err != nil {
		return nil, err
	}
	if current.GetSource() == iamclientv1.ClientSource_CLIENT_SOURCE_STATIC {
		return nil, ErrStaticClientImmutable
	}
	expectedEtag := prepared.Options().Etag
	if expectedEtag == "" || expectedEtag != current.GetEtag() {
		return nil, ErrClientEtagMismatch
	}
	if err := prepared.ValidateImmutable(current); err != nil {
		return nil, err
	}

	writeMask := prepared.WriteMask()
	merged := mergeOAuthClientUpdate(current, prepared.Resource(), writeMask)
	normalizedProto, err := ValidateAndNormalizeOAuthClient(
		current.GetClientId(),
		merged,
		current.GetSource(),
		current.GetStatus(),
	)
	if err != nil {
		return nil, err
	}

	if normalizedProto.GetTokenEndpointAuthMethod() != iamclientv1.TokenEndpointAuthMethod_TOKEN_ENDPOINT_AUTH_METHOD_NONE &&
		countActiveSecrets(current.GetSecrets(), now) == 0 {
		return nil, fmt.Errorf("%w: confidential client requires at least one active secret", ErrInvalidClientConfig)
	}

	effectiveMask := proto.CloneOf(writeMask)
	if slices.Contains(effectiveMask.GetPaths(), iamclientv1.OAuthClientFields.AllowedGrantTypes.String()) &&
		!slices.Contains(effectiveMask.GetPaths(), iamclientv1.OAuthClientFields.AllowedResponseTypes.String()) {
		effectiveMask.Paths = append(effectiveMask.Paths, iamclientv1.OAuthClientFields.AllowedResponseTypes.String())
	}

	updated, err := uc.clients.Update(ctx, name.OauthClient, normalizedProto, effectiveMask, expectedEtag, now)
	if errors.Is(err, ErrMutationMiss) {
		return nil, ErrClientEtagMismatch
	}
	return updated, err
}

// DisableOAuthClient disables a dynamic OAuth client and cascades token/flow revocation.
func (uc *OAuthClientUsecase) DisableOAuthClient(ctx context.Context, name iamclientv1.OAuthClientName, expectedEtag string) (*iamclientv1.OAuthClient, error) {
	return uc.updateStatus(ctx, name, expectedEtag, iamclientv1.ClientStatus_CLIENT_STATUS_DISABLED)
}

// EnableOAuthClient re-enables a disabled dynamic OAuth client.
func (uc *OAuthClientUsecase) EnableOAuthClient(ctx context.Context, name iamclientv1.OAuthClientName, expectedEtag string) (*iamclientv1.OAuthClient, error) {
	return uc.updateStatus(ctx, name, expectedEtag, iamclientv1.ClientStatus_CLIENT_STATUS_ACTIVE)
}

func (uc *OAuthClientUsecase) updateStatus(
	ctx context.Context,
	name iamclientv1.OAuthClientName,
	expectedEtag string,
	status iamclientv1.ClientStatus,
) (*iamclientv1.OAuthClient, error) {
	now := time.Now().UTC()
	current, err := uc.clients.Get(ctx, name.OauthClient)
	if err != nil {
		return nil, err
	}
	if current.GetSource() == iamclientv1.ClientSource_CLIENT_SOURCE_STATIC {
		return nil, ErrStaticClientImmutable
	}
	if status == iamclientv1.ClientStatus_CLIENT_STATUS_ACTIVE && current.GetStatus() != iamclientv1.ClientStatus_CLIENT_STATUS_DISABLED {
		return nil, ErrClientInvalidState
	}
	if expectedEtag != "" && expectedEtag != current.GetEtag() {
		return nil, ErrClientEtagMismatch
	}
	updated, err := uc.clients.UpdateStatus(ctx, name.OauthClient, current.GetEtag(), status, now)
	if errors.Is(err, ErrMutationMiss) {
		return nil, ErrClientEtagMismatch
	}
	return updated, err
}

// DeleteOAuthClient permanently deletes a dynamic OAuth client and cascades revocation of all associated artifacts.
func (uc *OAuthClientUsecase) DeleteOAuthClient(ctx context.Context, name iamclientv1.OAuthClientName, expectedEtag string) error {
	now := time.Now().UTC()
	current, err := uc.clients.Get(ctx, name.OauthClient)
	if err != nil {
		return err
	}
	if current.GetSource() == iamclientv1.ClientSource_CLIENT_SOURCE_STATIC {
		return ErrStaticClientImmutable
	}
	if expectedEtag != "" && expectedEtag != current.GetEtag() {
		return ErrClientEtagMismatch
	}
	err = uc.clients.Delete(ctx, name.OauthClient, current.GetEtag(), now)
	if errors.Is(err, ErrMutationMiss) {
		return ErrClientEtagMismatch
	}
	return err
}

// CreateOAuthClientSecret generates a CSPRNG secret for a confidential dynamic OAuth client and returns plaintext once.
func (uc *OAuthClientUsecase) CreateOAuthClientSecret(
	ctx context.Context,
	name iamclientv1.OAuthClientName,
	secretName string,
	expiresTime *time.Time,
) (*iamclientv1.OAuthClientSecret, string, error) {
	now := time.Now().UTC()
	current, err := uc.clients.Get(ctx, name.OauthClient)
	if err != nil {
		return nil, "", err
	}
	if current.GetSource() == iamclientv1.ClientSource_CLIENT_SOURCE_STATIC {
		return nil, "", ErrStaticClientImmutable
	}
	if current.GetTokenEndpointAuthMethod() == iamclientv1.TokenEndpointAuthMethod_TOKEN_ENDPOINT_AUTH_METHOD_NONE ||
		current.GetTokenEndpointAuthMethod() == iamclientv1.TokenEndpointAuthMethod_TOKEN_ENDPOINT_AUTH_METHOD_UNSPECIFIED {
		return nil, "", fmt.Errorf("%w: public client cannot hold secrets", ErrInvalidClientConfig)
	}
	if len(current.GetSecrets()) >= MaxClientSecrets {
		return nil, "", ErrSecretLimitExceeded
	}
	secret, plaintext, secretHash, err := newClientSecretRecord(secretName, expiresTime, now)
	if err != nil {
		return nil, "", err
	}
	created, err := uc.clients.CreateSecret(ctx, name.OauthClient, secret, secretHash, now)
	if err != nil {
		return nil, "", err
	}
	return created, plaintext, nil
}

// DeleteOAuthClientSecret removes one secret from a confidential dynamic OAuth client while keeping >=1 active secret.
func (uc *OAuthClientUsecase) DeleteOAuthClientSecret(
	ctx context.Context,
	name iamclientv1.OAuthClientName,
	secretID string,
) error {
	secretID = strings.TrimSpace(secretID)
	if secretID == "" {
		return ErrNotFound
	}
	now := time.Now().UTC()
	current, err := uc.clients.Get(ctx, name.OauthClient)
	if err != nil {
		return err
	}
	if current.GetSource() == iamclientv1.ClientSource_CLIENT_SOURCE_STATIC {
		return ErrStaticClientImmutable
	}
	if current.GetTokenEndpointAuthMethod() == iamclientv1.TokenEndpointAuthMethod_TOKEN_ENDPOINT_AUTH_METHOD_NONE {
		return fmt.Errorf("%w: public client has no secrets", ErrInvalidClientConfig)
	}
	return uc.clients.DeleteSecret(ctx, name.OauthClient, secretID, now)
}

func newClientSecretRecord(
	secretName string,
	expiresTime *time.Time,
	now time.Time,
) (*iamclientv1.OAuthClientSecret, string, string, error) {
	var expiresProto *timestamppb.Timestamp
	if expiresTime != nil {
		exp := expiresTime.UTC()
		if !exp.After(now) {
			return nil, "", "", fmt.Errorf("%w: secret expires_time must be in the future", ErrInvalidClientConfig)
		}
		expiresProto = timestamppb.New(exp)
	}
	secretID, err := NewID()
	if err != nil {
		return nil, "", "", err
	}
	plaintext, prefix, hash, err := NewClientSecret()
	if err != nil {
		return nil, "", "", err
	}
	label := strings.TrimSpace(secretName)
	if label == "" {
		label = DefaultSecretName
	}
	return &iamclientv1.OAuthClientSecret{
		SecretId:     secretID,
		Name:         label,
		SecretPrefix: prefix,
		ExpiresTime:  expiresProto,
		CreateTime:   timestamppb.New(now),
	}, plaintext, hash, nil
}

// ValidateClientIDSlug checks custom dynamic client_id syntax (1-63 lowercase alphanumeric or hyphen chars, no leading/trailing hyphen).
func ValidateClientIDSlug(id string) bool {
	return clientIDSlugPattern.MatchString(id)
}

// NewGeneratedClientID generates a random dynamic client_id ("cli_" + 24 hex chars).
func NewGeneratedClientID() (string, error) {
	var raw [12]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", fmt.Errorf("generate OAuth client ID: %w", err)
	}
	return "cli_" + hex.EncodeToString(raw[:]), nil
}

// NewClientSecret generates a 256-bit high-entropy client secret ("pcs_" + base64url), its 12-char prefix, and its SHA-256 digest.
func NewClientSecret() (plaintext, prefix, secretHash string, err error) {
	var raw [32]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", "", "", fmt.Errorf("generate OAuth client secret: %w", err)
	}
	plaintext = "pcs_" + base64.RawURLEncoding.EncodeToString(raw[:])
	return plaintext, SecretDisplayPrefix(plaintext), HashOpaqueSecret(plaintext), nil
}

// SecretDisplayPrefix extracts at most the first 12 ASCII characters of a client secret as a non-sensitive identifier prefix.
func SecretDisplayPrefix(secret string) string {
	if len(secret) <= 12 {
		return secret
	}
	return secret[:12]
}

// ValidateAndNormalizeOAuthClient validates and normalizes an OAuthClient protobuf message in-place into a new canonical instance.
func ValidateAndNormalizeOAuthClient(
	clientID string,
	input *iamclientv1.OAuthClient,
	source iamclientv1.ClientSource,
	status iamclientv1.ClientStatus,
) (*iamclientv1.OAuthClient, error) {
	trimmedID := strings.TrimSpace(clientID)
	if trimmedID == "" || trimmedID != clientID || strings.Contains(trimmedID, "/") {
		return nil, fmt.Errorf("%w: client_id is empty or invalid", ErrInvalidClientConfig)
	}
	displayName := strings.TrimSpace(input.GetDisplayName())
	if displayName == "" {
		return nil, fmt.Errorf("%w: display_name is required", ErrInvalidClientConfig)
	}

	var description *string
	if trimmed := strings.TrimSpace(input.GetDescription()); trimmed != "" {
		description = &trimmed
	}

	var logoURI *string
	if trimmed := strings.TrimSpace(input.GetLogoUri()); trimmed != "" {
		if err := ValidateLogoURI(trimmed); err != nil {
			return nil, err
		}
		logoURI = &trimmed
	}

	appType := input.GetApplicationType()
	appTypeStorage, err := ApplicationTypeToStorage(appType)
	if err != nil {
		return nil, err
	}
	authMethod := input.GetTokenEndpointAuthMethod()
	authMethodStorage, err := AuthMethodToStorage(authMethod)
	if err != nil {
		return nil, err
	}
	if appTypeStorage == AppTypeUserAgent && authMethodStorage != AuthMethodNone {
		return nil, fmt.Errorf("%w: user_agent client must use token_endpoint_auth_method=none", ErrInvalidClientConfig)
	}

	grants := deduplicateStrings(input.GetAllowedGrantTypes())
	if len(grants) == 0 {
		return nil, fmt.Errorf("%w: allowed_grant_types must not be empty", ErrInvalidClientConfig)
	}
	for _, grant := range grants {
		switch grant {
		case GrantTypeAuthorizationCode, GrantTypeRefreshToken, GrantTypeClientCredentials:
		default:
			return nil, fmt.Errorf("%w: unsupported grant type %q", ErrInvalidClientConfig, grant)
		}
	}

	userFlow := slices.Contains(grants, GrantTypeAuthorizationCode)
	if slices.Contains(grants, GrantTypeRefreshToken) && !userFlow {
		return nil, fmt.Errorf("%w: refresh_token grant requires authorization_code grant", ErrInvalidClientConfig)
	}

	hasClientCredentials := slices.Contains(grants, GrantTypeClientCredentials)
	if hasClientCredentials {
		if authMethodStorage == AuthMethodNone {
			return nil, fmt.Errorf("%w: public client cannot use client_credentials grant", ErrInvalidClientConfig)
		}
		if appTypeStorage != AppTypeWeb {
			return nil, fmt.Errorf("%w: client_credentials grant is only supported for web clients", ErrInvalidClientConfig)
		}
	}

	for _, audience := range input.GetAudiences() {
		if audience == "" || strings.TrimSpace(audience) != audience {
			return nil, fmt.Errorf("%w: audience is empty or contains surrounding whitespace", ErrInvalidClientConfig)
		}
	}
	audiences := deduplicateStrings(input.GetAudiences())
	if hasClientCredentials && len(audiences) == 0 {
		return nil, fmt.Errorf("%w: client_credentials grant requires non-empty audiences", ErrInvalidClientConfig)
	}

	redirectURIs := deduplicateStrings(input.GetRedirectUris())
	if userFlow && len(redirectURIs) == 0 {
		return nil, fmt.Errorf("%w: authorization_code grant requires redirect_uris", ErrInvalidClientConfig)
	}
	for _, rawURI := range redirectURIs {
		if err := ValidateClientRedirectURI(appTypeStorage, rawURI); err != nil {
			return nil, err
		}
	}

	postLogoutURIs := deduplicateStrings(input.GetPostLogoutRedirectUris())
	for _, rawURI := range postLogoutURIs {
		if err := ValidateClientRedirectURI(appTypeStorage, rawURI); err != nil {
			return nil, err
		}
	}

	for _, scope := range input.GetAllowedScopes() {
		if scope == "" || strings.TrimSpace(scope) != scope {
			return nil, fmt.Errorf("%w: scope is empty or contains surrounding whitespace", ErrInvalidClientConfig)
		}
	}
	scopes := deduplicateStrings(input.GetAllowedScopes())
	if userFlow {
		if !slices.Contains(scopes, "openid") {
			return nil, fmt.Errorf("%w: authorization_code client must allow the openid scope", ErrInvalidClientConfig)
		}
		for _, scope := range scopes {
			if !slices.Contains(SupportedOIDCScopes, scope) {
				return nil, fmt.Errorf("%w: unsupported scope %q", ErrInvalidClientConfig, scope)
			}
		}
	}

	responseTypes := []string{}
	if userFlow {
		responseTypes = []string{ResponseTypeCode}
	}

	return &iamclientv1.OAuthClient{
		Name:                    "oauthClients/" + trimmedID,
		ClientId:                trimmedID,
		Source:                  source,
		Status:                  status,
		DisplayName:             &displayName,
		Description:             description,
		LogoUri:                 logoURI,
		ApplicationType:         &appType,
		TokenEndpointAuthMethod: &authMethod,
		RedirectUris:            redirectURIs,
		PostLogoutRedirectUris:  postLogoutURIs,
		AllowedGrantTypes:       grants,
		AllowedResponseTypes:    responseTypes,
		AllowedScopes:           scopes,
		Audiences:               audiences,
		Trusted:                 new(input.GetTrusted()),
	}, nil
}

// ValidateClientRedirectURI enforces RFC 6749 / RFC 8252 redirect URI rules per application_type.
func ValidateClientRedirectURI(applicationType, raw string) error {
	if raw == "" || strings.TrimSpace(raw) != raw {
		return fmt.Errorf("%w: redirect URI %q must not be empty or padded with whitespace", ErrInvalidClientConfig, raw)
	}
	if strings.Contains(raw, "*") {
		return fmt.Errorf("%w: redirect URI %q wildcards are forbidden", ErrInvalidClientConfig, raw)
	}
	if strings.Contains(raw, "#") {
		return fmt.Errorf("%w: redirect URI %q must omit fragments", ErrInvalidClientConfig, raw)
	}
	colonIdx := strings.IndexByte(raw, ':')
	if colonIdx <= 0 {
		return fmt.Errorf("%w: redirect URI %q must include a scheme", ErrInvalidClientConfig, raw)
	}
	rawScheme := raw[:colonIdx]
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme == "" || parsed.Fragment != "" {
		return fmt.Errorf("%w: redirect URI %q is malformed", ErrInvalidClientConfig, raw)
	}

	switch applicationType {
	case AppTypeWeb, AppTypeUserAgent:
		if parsed.Host == "" {
			return fmt.Errorf("%w: redirect URI %q must be an absolute URL", ErrInvalidClientConfig, raw)
		}
		if !isHTTPSOrLoopbackHTTP(parsed) {
			return fmt.Errorf("%w: redirect URI %q must use HTTPS except on loopback hosts", ErrInvalidClientConfig, raw)
		}
		return nil

	case AppTypeNative:
		if strings.EqualFold(rawScheme, "https") || strings.EqualFold(rawScheme, "http") {
			if parsed.Host == "" || !isHTTPSOrLoopbackHTTP(parsed) {
				return fmt.Errorf("%w: %s native redirect URI %q requires HTTPS or a loopback host", ErrInvalidClientConfig, parsed.Scheme, raw)
			}
			return nil
		}
		if !customSchemePattern.MatchString(rawScheme) {
			return fmt.Errorf("%w: native redirect URI %q has invalid custom scheme %q", ErrInvalidClientConfig, raw, rawScheme)
		}
		switch rawScheme {
		case "javascript", "data", "file", "vbscript", "about", "blob":
			return fmt.Errorf("%w: native redirect URI scheme %q is forbidden", ErrInvalidClientConfig, rawScheme)
		}
		if len(raw) <= colonIdx+1 {
			return fmt.Errorf("%w: native custom-scheme redirect URI %q is incomplete", ErrInvalidClientConfig, raw)
		}
		return nil

	default:
		return fmt.Errorf("%w: unsupported application_type %q", ErrInvalidClientConfig, applicationType)
	}
}

// ValidateLogoURI ensures logo_uri uses HTTPS or loopback HTTP and never allows dangerous URI schemes.
func ValidateLogoURI(raw string) error {
	if strings.Contains(raw, "#") || strings.Contains(raw, "*") {
		return fmt.Errorf("%w: logo_uri %q must not contain fragments or wildcards", ErrInvalidClientConfig, raw)
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" || parsed.Fragment != "" {
		return fmt.Errorf("%w: logo_uri %q must be an absolute URL", ErrInvalidClientConfig, raw)
	}
	if !isHTTPSOrLoopbackHTTP(parsed) {
		return fmt.Errorf("%w: logo_uri %q must use HTTPS except on loopback hosts", ErrInvalidClientConfig, raw)
	}
	return nil
}

// ClientSourceToStorage maps Proto ClientSource to canonical storage string.
func ClientSourceToStorage(value iamclientv1.ClientSource) (string, error) {
	switch value {
	case iamclientv1.ClientSource_CLIENT_SOURCE_STATIC:
		return ClientSourceStatic, nil
	case iamclientv1.ClientSource_CLIENT_SOURCE_DYNAMIC:
		return ClientSourceDynamic, nil
	default:
		return "", fmt.Errorf("%w: unsupported client source %v", ErrInvalidClientConfig, value)
	}
}

// ClientSourceFromStorage maps canonical storage string to Proto ClientSource.
func ClientSourceFromStorage(value string) (iamclientv1.ClientSource, error) {
	switch value {
	case ClientSourceStatic:
		return iamclientv1.ClientSource_CLIENT_SOURCE_STATIC, nil
	case ClientSourceDynamic:
		return iamclientv1.ClientSource_CLIENT_SOURCE_DYNAMIC, nil
	default:
		return iamclientv1.ClientSource_CLIENT_SOURCE_UNSPECIFIED, fmt.Errorf("unknown stored client source %q", value)
	}
}

// ClientStatusToStorage maps Proto ClientStatus to canonical storage string.
func ClientStatusToStorage(value iamclientv1.ClientStatus) (string, error) {
	switch value {
	case iamclientv1.ClientStatus_CLIENT_STATUS_ACTIVE:
		return ClientStatusActive, nil
	case iamclientv1.ClientStatus_CLIENT_STATUS_DISABLED:
		return ClientStatusDisabled, nil
	default:
		return "", fmt.Errorf("%w: unsupported client status %v", ErrInvalidClientConfig, value)
	}
}

// ClientStatusFromStorage maps canonical storage string to Proto ClientStatus.
func ClientStatusFromStorage(value string) (iamclientv1.ClientStatus, error) {
	switch value {
	case ClientStatusActive:
		return iamclientv1.ClientStatus_CLIENT_STATUS_ACTIVE, nil
	case ClientStatusDisabled:
		return iamclientv1.ClientStatus_CLIENT_STATUS_DISABLED, nil
	default:
		return iamclientv1.ClientStatus_CLIENT_STATUS_UNSPECIFIED, fmt.Errorf("unknown stored client status %q", value)
	}
}

// ApplicationTypeToStorage maps Proto ApplicationType to canonical storage string.
func ApplicationTypeToStorage(value iamclientv1.ApplicationType) (string, error) {
	switch value {
	case iamclientv1.ApplicationType_APPLICATION_TYPE_WEB:
		return AppTypeWeb, nil
	case iamclientv1.ApplicationType_APPLICATION_TYPE_USER_AGENT:
		return AppTypeUserAgent, nil
	case iamclientv1.ApplicationType_APPLICATION_TYPE_NATIVE:
		return AppTypeNative, nil
	default:
		return "", fmt.Errorf("%w: unsupported application_type %v", ErrInvalidClientConfig, value)
	}
}

// ApplicationTypeFromStorage maps canonical storage string to Proto ApplicationType.
func ApplicationTypeFromStorage(value string) (iamclientv1.ApplicationType, error) {
	switch value {
	case AppTypeWeb:
		return iamclientv1.ApplicationType_APPLICATION_TYPE_WEB, nil
	case AppTypeUserAgent:
		return iamclientv1.ApplicationType_APPLICATION_TYPE_USER_AGENT, nil
	case AppTypeNative:
		return iamclientv1.ApplicationType_APPLICATION_TYPE_NATIVE, nil
	default:
		return iamclientv1.ApplicationType_APPLICATION_TYPE_UNSPECIFIED, fmt.Errorf("%w: unknown application_type %q", ErrInvalidClientConfig, value)
	}
}

// AuthMethodToStorage maps Proto TokenEndpointAuthMethod to canonical storage string.
func AuthMethodToStorage(value iamclientv1.TokenEndpointAuthMethod) (string, error) {
	switch value {
	case iamclientv1.TokenEndpointAuthMethod_TOKEN_ENDPOINT_AUTH_METHOD_CLIENT_SECRET_BASIC:
		return AuthMethodBasic, nil
	case iamclientv1.TokenEndpointAuthMethod_TOKEN_ENDPOINT_AUTH_METHOD_CLIENT_SECRET_POST:
		return AuthMethodPost, nil
	case iamclientv1.TokenEndpointAuthMethod_TOKEN_ENDPOINT_AUTH_METHOD_NONE:
		return AuthMethodNone, nil
	default:
		return "", fmt.Errorf("%w: unsupported token_endpoint_auth_method %v", ErrInvalidClientConfig, value)
	}
}

// AuthMethodFromStorage maps canonical storage string to Proto TokenEndpointAuthMethod.
func AuthMethodFromStorage(value string) (iamclientv1.TokenEndpointAuthMethod, error) {
	switch value {
	case AuthMethodBasic:
		return iamclientv1.TokenEndpointAuthMethod_TOKEN_ENDPOINT_AUTH_METHOD_CLIENT_SECRET_BASIC, nil
	case AuthMethodPost:
		return iamclientv1.TokenEndpointAuthMethod_TOKEN_ENDPOINT_AUTH_METHOD_CLIENT_SECRET_POST, nil
	case AuthMethodNone:
		return iamclientv1.TokenEndpointAuthMethod_TOKEN_ENDPOINT_AUTH_METHOD_NONE, nil
	default:
		return iamclientv1.TokenEndpointAuthMethod_TOKEN_ENDPOINT_AUTH_METHOD_UNSPECIFIED, fmt.Errorf("%w: unknown token_endpoint_auth_method %q", ErrInvalidClientConfig, value)
	}
}

func mergeOAuthClientUpdate(
	current *iamclientv1.OAuthClient,
	patch *iamclientv1.OAuthClient,
	mask *fieldmaskpb.FieldMask,
) *iamclientv1.OAuthClient {
	merged := proto.CloneOf(current)
	for _, path := range mask.GetPaths() {
		switch path {
		case iamclientv1.OAuthClientFields.DisplayName.String():
			merged.DisplayName = patch.DisplayName
		case iamclientv1.OAuthClientFields.Description.String():
			merged.Description = patch.Description
		case iamclientv1.OAuthClientFields.LogoUri.String():
			merged.LogoUri = patch.LogoUri
		case iamclientv1.OAuthClientFields.TokenEndpointAuthMethod.String():
			merged.TokenEndpointAuthMethod = patch.TokenEndpointAuthMethod
		case iamclientv1.OAuthClientFields.RedirectUris.String():
			merged.RedirectUris = slices.Clone(patch.GetRedirectUris())
		case iamclientv1.OAuthClientFields.PostLogoutRedirectUris.String():
			merged.PostLogoutRedirectUris = slices.Clone(patch.GetPostLogoutRedirectUris())
		case iamclientv1.OAuthClientFields.AllowedGrantTypes.String():
			merged.AllowedGrantTypes = slices.Clone(patch.GetAllowedGrantTypes())
		case iamclientv1.OAuthClientFields.AllowedScopes.String():
			merged.AllowedScopes = slices.Clone(patch.GetAllowedScopes())
		case iamclientv1.OAuthClientFields.Audiences.String():
			merged.Audiences = slices.Clone(patch.GetAudiences())
		case iamclientv1.OAuthClientFields.Trusted.String():
			merged.Trusted = new(patch.GetTrusted())
		}
	}
	return merged
}

func countActiveSecrets(secrets []*iamclientv1.OAuthClientSecret, now time.Time) int {
	count := 0
	for _, s := range secrets {
		if s.GetExpiresTime() == nil || s.GetExpiresTime().AsTime().After(now) {
			count++
		}
	}
	return count
}

func deduplicateStrings(values []string) []string {
	if len(values) == 0 {
		return []string{}
	}
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
