package oidc

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strings"

	oidcconfpb "github.com/Servora-Kit/plateau/api/gen/go/plateau/iam/oidc/conf/v1"
	iamauthn "github.com/Servora-Kit/plateau/app/iam/service/internal/authn"
	"github.com/Servora-Kit/plateau/app/iam/service/internal/biz"
	"github.com/Servora-Kit/plateau/security/audit"
	httpsession "github.com/Servora-Kit/plateau/security/session"
	"github.com/alexedwards/scs/v2"
	httphelper "github.com/zitadel/oidc/v3/pkg/http"
	"github.com/zitadel/oidc/v3/pkg/oidc"
	"github.com/zitadel/oidc/v3/pkg/op"
)

const maxTokenRequestBodyBytes = 64 << 10

var supportedDiscoveryClaims = []string{
	"sub", "aud", "exp", "iat", "iss", "auth_time", "nonce", "amr", "c_hash", "at_hash",
	"name", "family_name", "given_name", "nickname", "preferred_username", "picture", "locale", "updated_at",
	"email", "email_verified",
}

// IAMProvider wraps ZITADEL's op.Provider to enforce OAuth 2.1 constraints,
// RFC 9207 issuer identification, strict client auth method matching, and IAM browser session / consent flows.
type IAMProvider struct {
	*op.Provider
	issuer   string
	storage  *OIDCStorage
	sessions *biz.SessionUsecase
	manager  *scs.SessionManager
	handler  http.Handler
}

func NewIAMProvider(
	config *oidcconfpb.OIDC,
	storage *OIDCStorage,
	sessions *biz.SessionUsecase,
	manager *scs.SessionManager,
) (*IAMProvider, error) {
	if config == nil || storage == nil || sessions == nil || manager == nil {
		return nil, fmt.Errorf("OIDC provider dependencies are nil")
	}
	if err := config.Apply(); err != nil {
		return nil, fmt.Errorf("OIDC provider config: %w", err)
	}
	issuer, insecure, err := biz.NormalizeOIDCIssuer(config.GetIssuer())
	if err != nil {
		return nil, err
	}
	cryptoKey, cryptoKeyID, err := LoadCryptoKey(config.GetCryptoKeyPath())
	if err != nil {
		return nil, err
	}
	protocolConfig := &op.Config{
		CryptoKey:                cryptoKey,
		CryptoKeyId:              cryptoKeyID,
		DefaultLogoutRedirectURI: issuer + "/account",
		CodeMethodS256:           true,
		AuthMethodPost:           true,
		GrantTypeRefreshToken:    true,
		RequestObjectSupported:   false,
		SupportedClaims:          slices.Clone(supportedDiscoveryClaims),
		SupportedScopes:          slices.Clone(supportedScopes),
	}
	var options []op.Option
	if insecure {
		options = append(options, op.WithAllowInsecure())
	}
	base, err := op.NewProvider(protocolConfig, storage, op.StaticIssuer(issuer), options...)
	if err != nil {
		return nil, fmt.Errorf("create OIDC provider: %w", err)
	}
	provider := &IAMProvider{Provider: base, issuer: issuer, storage: storage, sessions: sessions, manager: manager}
	provider.handler = op.CreateRouter(provider)
	return provider, nil
}

func (provider *IAMProvider) HttpHandler() http.Handler { return provider }

func (provider *IAMProvider) GrantTypeJWTAuthorizationSupported() bool { return false }

func (provider *IAMProvider) ServeHTTP(response http.ResponseWriter, request *http.Request) {
	switch request.URL.Path {
	case oidc.DiscoveryEndpoint:
		provider.serveDiscovery(response, request)
	case "/authorize":
		response.Header().Set("Cache-Control", "no-store")
		provider.handler.ServeHTTP(&issRedirectWriter{ResponseWriter: response, issuer: provider.issuer}, request)
	case "/authorize/callback":
		response.Header().Set("Cache-Control", "no-store")
		provider.completeAuthorization(&issRedirectWriter{ResponseWriter: response, issuer: provider.issuer}, request)
	case "/oauth/token", "/oauth/introspect", "/revoke":
		preparedReq, ok := prepareClientAuthEndpointRequest(response, request)
		if !ok {
			return
		}
		provider.handler.ServeHTTP(response, preparedReq)
	default:
		provider.handler.ServeHTTP(response, request)
	}
}

func prepareClientAuthEndpointRequest(response http.ResponseWriter, request *http.Request) (*http.Request, bool) {
	if request.Method != http.MethodPost {
		response.Header().Set("Allow", http.MethodPost)
		http.Error(response, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
		return nil, false
	}
	if request.URL.Query().Has("client_secret") {
		rejectClientAuthRequest(response, request, "client_secret must not be sent in the URI query")
		return nil, false
	}
	if request.PostForm == nil {
		if request.Body != nil && request.Body != http.NoBody {
			request.Body = http.MaxBytesReader(response, request.Body, maxTokenRequestBodyBytes)
		}
		if err := request.ParseForm(); err != nil {
			var maxErr *http.MaxBytesError
			if errors.As(err, &maxErr) {
				http.Error(response, http.StatusText(http.StatusRequestEntityTooLarge), http.StatusRequestEntityTooLarge)
				return nil, false
			}
		}
	}

	authHeader := strings.TrimSpace(request.Header.Get("Authorization"))
	hasBasic := len(authHeader) >= 6 && strings.EqualFold(authHeader[:6], "Basic ")
	hasPostSecret := request.PostForm.Get("client_secret") != ""
	if hasBasic && hasPostSecret {
		rejectClientAuthRequest(response, request, "multiple client authentication methods are not allowed")
		return nil, false
	}
	if hasBasic {
		if formClientID := strings.TrimSpace(request.PostForm.Get("client_id")); formClientID != "" {
			basicID, _, ok := request.BasicAuth()
			if decoded, err := url.QueryUnescape(basicID); err == nil && decoded != "" {
				basicID = decoded
			}
			if !ok || basicID != formClientID {
				rejectClientAuthRequest(response, request, "conflicting client_id values in Authorization header and body")
				return nil, false
			}
		}
	}

	transportMethod := biz.AuthMethodNone
	switch {
	case hasBasic:
		transportMethod = biz.AuthMethodBasic
	case hasPostSecret:
		transportMethod = biz.AuthMethodPost
	}
	return request.WithContext(withClientAuthTransport(request.Context(), transportMethod)), true
}

func rejectClientAuthRequest(response http.ResponseWriter, request *http.Request, description string) {
	response.Header().Set("Cache-Control", "no-store")
	op.RequestError(response, request, oidc.ErrInvalidRequest().WithDescription("%s", description), nil)
}

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
	for _, scope := range request.Scopes {
		if !client.IsScopeAllowed(scope) {
			return "", oidc.ErrInvalidScope().WithDescription("requested scope is not allowed for this client")
		}
	}
	return op.ValidateAuthRequestClient(ctx, request, client, verifier)
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

type discoveryDocument struct {
	*oidc.DiscoveryConfiguration
	AuthorizationResponseIssParameterSupported bool `json:"authorization_response_iss_parameter_supported"`
}

func (provider *IAMProvider) serveDiscovery(response http.ResponseWriter, request *http.Request) {
	cfg := op.CreateDiscoveryConfig(op.ContextWithIssuer(request.Context(), provider.issuer), provider.Provider, provider.storage)
	cfg.DeviceAuthorizationEndpoint = ""
	cfg.ResponseTypesSupported = []string{string(oidc.ResponseTypeCode)}
	cfg.GrantTypesSupported = []oidc.GrantType{oidc.GrantTypeCode, oidc.GrantTypeRefreshToken, oidc.GrantTypeClientCredentials}
	cfg.IntrospectionEndpointAuthMethodsSupported = []oidc.AuthMethod{oidc.AuthMethodBasic, oidc.AuthMethodPost}
	httphelper.MarshalJSON(response, discoveryDocument{
		DiscoveryConfiguration:                     cfg,
		AuthorizationResponseIssParameterSupported: true,
	})
}

func (provider *IAMProvider) completeAuthorization(response http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodGet {
		response.Header().Set("Allow", http.MethodGet)
		http.Error(response, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
		return
	}
	ctx := request.Context()
	if !httpsession.Loaded(ctx, provider.manager) {
		http.Error(response, "browser session is unavailable", http.StatusInternalServerError)
		return
	}
	requestID := request.URL.Query().Get("id")
	if requestID == "" {
		http.Error(response, "authorization request rejected", http.StatusBadRequest)
		return
	}
	loginID := provider.manager.GetString(ctx, iamauthn.LoginReferenceKey)
	if loginID == "" {
		redirectWithoutAudit(response, request, "/login?request_id="+url.QueryEscape(requestID))
		return
	}
	user, session, err := provider.sessions.Resolve(ctx, loginID)
	if err != nil || session.AuthTime.IsZero() || len(session.AMR) == 0 {
		redirectWithoutAudit(response, request, "/login?request_id="+url.QueryEscape(requestID))
		return
	}

	ownerToken := provider.manager.GetString(ctx, iamauthn.AuthFlowOwnerKey)
	var candidateToken, ownerHash string
	if ownerToken != "" {
		ownerHash = biz.HashOpaqueSecret(ownerToken)
	} else {
		generatedToken, generatedHash, err := biz.NewOpaqueSecret()
		if err != nil {
			http.Error(response, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
			return
		}
		candidateToken, ownerHash = generatedToken, generatedHash
	}

	needsConsent, err := provider.storage.CompleteAuthRequest(ctx, requestID, user.GetUserId(), session.ID, ownerHash)
	if err != nil {
		http.Error(response, "authorization request rejected", http.StatusBadRequest)
		return
	}
	if needsConsent {
		if candidateToken != "" {
			provider.manager.Put(ctx, iamauthn.AuthFlowOwnerKey, candidateToken)
		}
		redirectWithoutAudit(response, request, "/consent/?request_id="+url.QueryEscape(requestID))
		return
	}
	provider.handler.ServeHTTP(response, request)
	if errCode := redirectOAuthErrorCode(response.Header().Get("Location")); errCode != "" {
		audit.SetError(ctx, errCode, "authorization callback failed")
	}
}

func redirectWithoutAudit(response http.ResponseWriter, request *http.Request, location string) {
	audit.Skip(request.Context())
	http.Redirect(response, request, location, http.StatusFound)
}
