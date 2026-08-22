package oidc

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strings"

	oidcconfpb "github.com/Servora-Kit/plateau/api/gen/go/iam/oidc/conf/v1"
	"github.com/Servora-Kit/plateau/app/iam/service/internal/biz"
	httptransport "github.com/go-kratos/kratos/v3/transport/http"
	"github.com/zitadel/oidc/v3/pkg/oidc"
	"github.com/zitadel/oidc/v3/pkg/op"
)

const iamSessionCookieName = "__Host-iam_session"

var oidcRoutePaths = []string{
	"/.well-known/openid-configuration",
	"/authorize",
	"/authorize/callback",
	"/oauth/token",
	"/oauth/introspect",
	"/userinfo",
	"/revoke",
	"/end_session",
	"/keys",
}

// IAMProvider wraps the protocol engine to constrain discovery and complete browser authorization through IAM sessions.
type IAMProvider struct {
	*op.Provider
	issuer   string
	storage  *OIDCStorage
	sessions *biz.SessionUsecase
	handler  http.Handler
}

func NewIAMProvider(
	config *oidcconfpb.OIDC,
	storage *OIDCStorage,
	sessions *biz.SessionUsecase,
) (*IAMProvider, error) {
	if config == nil || storage == nil || sessions == nil {
		return nil, fmt.Errorf("OIDC provider dependencies are nil")
	}
	issuer, insecure, err := normalizeIssuer(config.GetIssuer())
	if err != nil {
		return nil, err
	}
	cryptoKey, cryptoKeyID, err := loadCryptoKey(config.GetCryptoKeyPath())
	if err != nil {
		return nil, err
	}
	protocolConfig := &op.Config{
		CryptoKey:                cryptoKey,
		CryptoKeyId:              cryptoKeyID,
		DefaultLogoutRedirectURI: issuer + "/account",
		CodeMethodS256:           true,
		GrantTypeRefreshToken:    true,
		RequestObjectSupported:   false,
		SupportedClaims:          append([]string(nil), op.DefaultSupportedClaims...),
		SupportedScopes:          append([]string(nil), supportedScopes...),
	}
	options := make([]op.Option, 0, 1)
	if insecure {
		options = append(options, op.WithAllowInsecure())
	}
	base, err := op.NewProvider(protocolConfig, storage, op.StaticIssuer(issuer), options...)
	if err != nil {
		return nil, fmt.Errorf("create OIDC provider: %w", err)
	}
	provider := &IAMProvider{Provider: base, issuer: issuer, storage: storage, sessions: sessions}
	provider.handler = op.CreateRouter(provider)
	return provider, nil
}

func (provider *IAMProvider) ServeHTTP(response http.ResponseWriter, request *http.Request) {
	if request.URL.Path == "/.well-known/openid-configuration" {
		provider.serveDiscovery(response)
		return
	}
	if request.URL.Path == "/authorize/callback" {
		provider.completeAuthorization(response, request)
		return
	}
	provider.handler.ServeHTTP(response, request)
}

func (provider *IAMProvider) HttpHandler() http.Handler { return provider }

func (provider *IAMProvider) GrantTypeJWTAuthorizationSupported() bool { return false }

func (provider *IAMProvider) ValidateAuthRequest(
	ctx context.Context,
	request *oidc.AuthRequest,
	storage op.Storage,
	verifier *op.IDTokenHintVerifier,
) (string, error) {
	client, err := storage.GetClientByClientID(ctx, request.ClientID)
	if err != nil {
		return "", err
	}
	if err := op.ValidateAuthReqRedirectURI(client, request.RedirectURI, request.ResponseType); err != nil {
		return "", err
	}
	if err := validateAuthorizationRequest(request); err != nil {
		return "", err
	}
	subject, err := op.ValidateAuthRequestClient(ctx, request, client, verifier)
	if err != nil {
		return "", err
	}
	for _, scope := range request.Scopes {
		if !client.IsScopeAllowed(scope) {
			return "", oidc.ErrInvalidScope().WithDescription("requested scope is not allowed for this client")
		}
	}
	return subject, nil
}
func validateAuthorizationRequest(request *oidc.AuthRequest) error {
	if request == nil {
		return oidc.ErrInvalidRequest().WithDescription("authorization request is missing")
	}
	if request.ResponseType != oidc.ResponseTypeCode {
		return oidc.ErrInvalidRequest().WithDescription("only authorization code flow is supported")
	}
	if request.ResponseMode != "" && request.ResponseMode != oidc.ResponseModeQuery {
		return oidc.ErrInvalidRequest().WithDescription("only query response mode is supported")
	}
	if request.State == "" || request.Nonce == "" {
		return oidc.ErrInvalidRequest().WithDescription("state and nonce are required")
	}
	if request.CodeChallenge == "" || request.CodeChallengeMethod != oidc.CodeChallengeMethodS256 {
		return oidc.ErrInvalidRequest().WithDescription("PKCE S256 is required")
	}
	if request.RequestParam != "" {
		return oidc.ErrRequestNotSupported().WithDescription("request objects are unsupported")
	}
	if !slices.Contains(request.Scopes, oidc.ScopeOpenID) {
		return oidc.ErrInvalidScope().WithDescription("openid scope is required")
	}
	for _, scope := range request.Scopes {
		if !slices.Contains(supportedScopes, scope) {
			return oidc.ErrInvalidScope().WithDescription("requested scope is unsupported")
		}
	}
	return nil
}

func (provider *IAMProvider) serveDiscovery(response http.ResponseWriter) {
	metadata := &oidc.DiscoveryConfiguration{
		Issuer:                                    provider.issuer,
		AuthorizationEndpoint:                     provider.issuer + "/authorize",
		TokenEndpoint:                             provider.issuer + "/oauth/token",
		IntrospectionEndpoint:                     provider.issuer + "/oauth/introspect",
		UserinfoEndpoint:                          provider.issuer + "/userinfo",
		RevocationEndpoint:                        provider.issuer + "/revoke",
		EndSessionEndpoint:                        provider.issuer + "/end_session",
		JwksURI:                                   provider.issuer + "/keys",
		ScopesSupported:                           append([]string(nil), supportedScopes...),
		ResponseTypesSupported:                    []string{string(oidc.ResponseTypeCode)},
		GrantTypesSupported:                       []oidc.GrantType{oidc.GrantTypeCode, oidc.GrantTypeRefreshToken},
		SubjectTypesSupported:                     []string{"public"},
		IDTokenSigningAlgValuesSupported:          []string{"RS256"},
		TokenEndpointAuthMethodsSupported:         []oidc.AuthMethod{oidc.AuthMethodBasic},
		IntrospectionEndpointAuthMethodsSupported: []oidc.AuthMethod{oidc.AuthMethodBasic},
		RevocationEndpointAuthMethodsSupported:    []oidc.AuthMethod{oidc.AuthMethodBasic},
		ClaimsSupported: []string{
			"sub", "aud", "exp", "iat", "iss", "auth_time", "nonce", "amr", "c_hash", "at_hash",
			"name", "family_name", "given_name", "nickname", "preferred_username", "picture", "locale", "updated_at",
			"email", "email_verified",
		},
		CodeChallengeMethodsSupported: []oidc.CodeChallengeMethod{oidc.CodeChallengeMethodS256},
		RequestParameterSupported:     false,
	}
	response.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(response).Encode(metadata); err != nil {
		http.Error(response, "encode discovery metadata", http.StatusInternalServerError)
	}
}

func (provider *IAMProvider) completeAuthorization(response http.ResponseWriter, request *http.Request) {
	requestID := request.URL.Query().Get("id")
	if requestID == "" {
		http.Error(response, "authorization request rejected", http.StatusBadRequest)
		return
	}
	cookie, err := request.Cookie(iamSessionCookieName)
	if err != nil || cookie.Value == "" {
		redirectToLogin(response, request, requestID)
		return
	}
	user, session, err := provider.sessions.Resolve(request.Context(), cookie.Value)
	if err != nil || user == nil || session == nil || session.GetCreateTime() == nil {
		redirectToLogin(response, request, requestID)
		return
	}
	if err := provider.storage.CompleteAuthRequest(
		request.Context(),
		requestID,
		user.GetUserId(),
		session.GetSessionId(),
		session.GetCreateTime().AsTime(),
	); err != nil {
		http.Error(response, "authorization request rejected", http.StatusBadRequest)
		return
	}
	provider.handler.ServeHTTP(response, request)
}

func redirectToLogin(response http.ResponseWriter, request *http.Request, requestID string) {
	location := "/login?request_id=" + url.QueryEscape(requestID)
	http.Redirect(response, request, location, http.StatusFound)
}

func RegisterHTTPServer(server *httptransport.Server, provider *IAMProvider) {
	for _, path := range oidcRoutePaths {
		server.Handle(path, provider)
	}
}

func normalizeIssuer(raw string) (string, bool, error) {
	issuer := strings.TrimSuffix(strings.TrimSpace(raw), "/")
	parsed, err := url.Parse(issuer)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", false, fmt.Errorf("OIDC issuer must be an absolute URL without query or fragment")
	}
	if parsed.Path != "" {
		return "", false, fmt.Errorf("OIDC issuer path is unsupported")
	}
	if parsed.Scheme == "https" {
		return issuer, false, nil
	}
	if parsed.Scheme == "http" && isLoopbackHost(parsed.Hostname()) {
		return issuer, true, nil
	}
	return "", false, fmt.Errorf("OIDC issuer must use HTTPS except on localhost")
}

func isLoopbackHost(host string) bool {
	switch host {
	case "localhost", "127.0.0.1", "::1":
		return true
	default:
		return false
	}
}
