package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"testing"
	"time"

	accountpb "github.com/Servora-Kit/plateau/api/gen/go/plateau/iam/account/v1"
	authnpb "github.com/Servora-Kit/plateau/api/gen/go/plateau/iam/authn/v1"
	iamclientv1 "github.com/Servora-Kit/plateau/api/gen/go/plateau/iam/client/v1"
	iamconfpb "github.com/Servora-Kit/plateau/api/gen/go/plateau/iam/conf/v1"
	oidcconfpb "github.com/Servora-Kit/plateau/api/gen/go/plateau/iam/oidc/conf/v1"
	userpb "github.com/Servora-Kit/plateau/api/gen/go/plateau/iam/user/v1"
	iamauthn "github.com/Servora-Kit/plateau/app/iam/service/internal/authn"
	"github.com/Servora-Kit/plateau/app/iam/service/internal/biz"
	corecrud "github.com/Servora-Kit/plateau/crud"
	"github.com/Servora-Kit/plateau/security"
	sessionauthn "github.com/Servora-Kit/plateau/security/authn/session"
	sessions "github.com/Servora-Kit/plateau/security/session"
	corepb "github.com/Servora-Kit/servora/api/gen/go/servora/core/v1"
	"github.com/alexedwards/scs/v2"
	"github.com/alexedwards/scs/v2/memstore"
	"github.com/go-kratos/kratos/v3/transport"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/fieldmaskpb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type stubOAuthClientRepo struct {
	clients map[string]*iamclientv1.OAuthClient
}

func newStubOAuthClientRepo() *stubOAuthClientRepo {
	return &stubOAuthClientRepo{clients: make(map[string]*iamclientv1.OAuthClient)}
}

func (r *stubOAuthClientRepo) Create(_ context.Context, c *iamclientv1.OAuthClient, sec *iamclientv1.OAuthClientSecret, _ string, now time.Time) (*iamclientv1.OAuthClient, error) {
	if _, exists := r.clients[c.GetClientId()]; exists {
		return nil, biz.ErrAlreadyExists
	}
	cloned := proto.CloneOf(c)
	cloned.Etag = "etag-1"
	cloned.CreateTime = timestamppb.New(now)
	cloned.UpdateTime = timestamppb.New(now)
	if sec != nil {
		s := proto.CloneOf(sec)
		s.CreateTime = timestamppb.New(now)
		cloned.Secrets = []*iamclientv1.OAuthClientSecret{s}
	}
	r.clients[cloned.GetClientId()] = cloned
	return proto.CloneOf(cloned), nil
}

func (r *stubOAuthClientRepo) Get(_ context.Context, id string) (*iamclientv1.OAuthClient, error) {
	c, ok := r.clients[id]
	if !ok {
		return nil, biz.ErrNotFound
	}
	return proto.CloneOf(c), nil
}

func (r *stubOAuthClientRepo) List(_ context.Context, q corecrud.ListQuery) (corecrud.ListResult[*iamclientv1.OAuthClient], error) {
	items := make([]*iamclientv1.OAuthClient, 0, len(r.clients))
	for _, c := range r.clients {
		items = append(items, proto.CloneOf(c))
	}
	total := int64(len(items))
	return corecrud.NewListResult(q, items, "", &total)
}

func (r *stubOAuthClientRepo) Update(_ context.Context, id string, patch *iamclientv1.OAuthClient, mask *fieldmaskpb.FieldMask, etag string, now time.Time) (*iamclientv1.OAuthClient, error) {
	c, ok := r.clients[id]
	if !ok {
		return nil, biz.ErrNotFound
	}
	if c.GetEtag() != etag {
		return nil, biz.ErrMutationMiss
	}
	for _, p := range mask.GetPaths() {
		if p == "display_name" {
			c.DisplayName = patch.DisplayName
		}
	}
	c.Etag = "etag-2"
	c.UpdateTime = timestamppb.New(now)
	return proto.CloneOf(c), nil
}

func (r *stubOAuthClientRepo) UpdateStatus(_ context.Context, id, etag string, status iamclientv1.ClientStatus, now time.Time) (*iamclientv1.OAuthClient, error) {
	c, ok := r.clients[id]
	if !ok {
		return nil, biz.ErrNotFound
	}
	if c.GetEtag() != etag {
		return nil, biz.ErrMutationMiss
	}
	c.Status = status
	c.Etag = "etag-3"
	c.UpdateTime = timestamppb.New(now)
	return proto.CloneOf(c), nil
}

func (r *stubOAuthClientRepo) Delete(_ context.Context, id, etag string, _ time.Time) error {
	c, ok := r.clients[id]
	if !ok {
		return biz.ErrNotFound
	}
	if c.GetEtag() != etag {
		return biz.ErrMutationMiss
	}
	delete(r.clients, id)
	return nil
}

func (r *stubOAuthClientRepo) CreateSecret(_ context.Context, id string, seed *iamclientv1.OAuthClientSecret, _ string, now time.Time) (*iamclientv1.OAuthClientSecret, error) {
	c, ok := r.clients[id]
	if !ok {
		return nil, biz.ErrNotFound
	}
	if len(c.Secrets) >= biz.MaxClientSecrets {
		return nil, biz.ErrSecretLimitExceeded
	}
	sec := proto.CloneOf(seed)
	sec.CreateTime = timestamppb.New(now)
	c.Secrets = append(c.Secrets, sec)
	return proto.CloneOf(sec), nil
}

func (r *stubOAuthClientRepo) DeleteSecret(_ context.Context, id, secID string, _ time.Time) error {
	c, ok := r.clients[id]
	if !ok {
		return biz.ErrNotFound
	}
	if len(c.Secrets) <= 1 {
		return biz.ErrLastActiveSecret
	}
	for i, s := range c.Secrets {
		if s.GetSecretId() == secID {
			c.Secrets = slices.Delete(c.Secrets, i, i+1)
			return nil
		}
	}
	return biz.ErrNotFound
}

func TestOAuthClientServiceAllRPCsAndErrorMappings(t *testing.T) {
	repo := newStubOAuthClientRepo()
	repo.clients["admin"] = &iamclientv1.OAuthClient{
		Name:                    "oauthClients/admin",
		ClientId:                "admin",
		Source:                  iamclientv1.ClientSource_CLIENT_SOURCE_STATIC,
		Status:                  iamclientv1.ClientStatus_CLIENT_STATUS_ACTIVE,
		DisplayName:             new("Static Admin"),
		ApplicationType:         iamclientv1.ApplicationType_APPLICATION_TYPE_WEB.Enum(),
		TokenEndpointAuthMethod: iamclientv1.TokenEndpointAuthMethod_TOKEN_ENDPOINT_AUTH_METHOD_CLIENT_SECRET_BASIC.Enum(),
		AllowedGrantTypes:       []string{biz.GrantTypeClientCredentials},
		Audiences:               []string{"iam"},
		Etag:                    "etag-static",
	}

	uc, err := biz.NewOAuthClientUsecase(repo)
	if err != nil {
		t.Fatal(err)
	}
	svc, err := NewOAuthClientService(uc)
	if err != nil {
		t.Fatal(err)
	}
	ctx := t.Context()

	// Invalid slug -> INVALID_CONFIG (400).
	if _, err := svc.RegisterOAuthClient(ctx, &iamclientv1.RegisterOAuthClientRequest{
		OauthClientId: "INVALID_SLUG",
		OauthClient: &iamclientv1.OAuthClient{
			DisplayName:             new("Bad"),
			ApplicationType:         iamclientv1.ApplicationType_APPLICATION_TYPE_WEB.Enum(),
			TokenEndpointAuthMethod: iamclientv1.TokenEndpointAuthMethod_TOKEN_ENDPOINT_AUTH_METHOD_NONE.Enum(),
			RedirectUris:            []string{"https://app.example.com/cb"},
			AllowedGrantTypes:       []string{biz.GrantTypeAuthorizationCode},
			AllowedScopes:           []string{"openid"},
		},
	}); !iamclientv1.IsClientErrorReasonInvalidConfig(err) {
		t.Fatalf("Register(bad slug) = %v, want INVALID_CONFIG", err)
	}

	// Register confidential client -> returns Client + one-time ClientSecret.
	regResp, err := svc.RegisterOAuthClient(ctx, &iamclientv1.RegisterOAuthClientRequest{
		OauthClientId: "partner-app",
		OauthClient: &iamclientv1.OAuthClient{
			DisplayName:             new("Partner App"),
			ApplicationType:         iamclientv1.ApplicationType_APPLICATION_TYPE_WEB.Enum(),
			TokenEndpointAuthMethod: iamclientv1.TokenEndpointAuthMethod_TOKEN_ENDPOINT_AUTH_METHOD_CLIENT_SECRET_BASIC.Enum(),
			RedirectUris:            []string{"https://partner.example.com/cb"},
			AllowedGrantTypes:       []string{biz.GrantTypeAuthorizationCode, biz.GrantTypeRefreshToken},
			AllowedScopes:           []string{"openid", "profile"},
		},
	})
	if err != nil || !strings.HasPrefix(regResp.GetClientSecret(), "pcs_") || regResp.GetClient().GetName() != "oauthClients/partner-app" {
		t.Fatalf("RegisterOAuthClient() = (%+v, %v)", regResp, err)
	}

	// Duplicate ID -> ALREADY_EXISTS (409).
	if _, err := svc.RegisterOAuthClient(ctx, &iamclientv1.RegisterOAuthClientRequest{
		OauthClientId: "partner-app",
		OauthClient: &iamclientv1.OAuthClient{
			DisplayName:             new("Dup"),
			ApplicationType:         iamclientv1.ApplicationType_APPLICATION_TYPE_WEB.Enum(),
			TokenEndpointAuthMethod: iamclientv1.TokenEndpointAuthMethod_TOKEN_ENDPOINT_AUTH_METHOD_CLIENT_SECRET_BASIC.Enum(),
			RedirectUris:            []string{"https://partner.example.com/cb"},
			AllowedGrantTypes:       []string{biz.GrantTypeAuthorizationCode},
			AllowedScopes:           []string{"openid"},
		},
	}); !iamclientv1.IsClientErrorReasonAlreadyExists(err) {
		t.Fatalf("Register(duplicate) = %v, want ALREADY_EXISTS", err)
	}

	// Static client modification -> STATIC_CLIENT_IMMUTABLE (403).
	if _, err := svc.DisableOAuthClient(ctx, &iamclientv1.DisableOAuthClientRequest{Name: "oauthClients/admin"}); !iamclientv1.IsClientErrorReasonStaticClientImmutable(err) {
		t.Fatalf("Disable(static) = %v, want STATIC_CLIENT_IMMUTABLE", err)
	}

	// Get & List.
	got, err := svc.GetOAuthClient(ctx, &iamclientv1.GetOAuthClientRequest{Name: "oauthClients/partner-app"})
	if err != nil || got.GetDisplayName() != "Partner App" {
		t.Fatalf("GetOAuthClient() = (%+v, %v)", got, err)
	}
	listed, err := svc.ListOAuthClients(ctx, &iamclientv1.ListOAuthClientsRequest{IncludeTotal: true})
	if err != nil || listed.GetTotalSize() != 2 {
		t.Fatalf("ListOAuthClients() = (%+v, %v)", listed, err)
	}

	// Update with stale etag -> ETAG_MISMATCH (409); valid etag succeeds.
	if _, err := svc.UpdateOAuthClient(ctx, &iamclientv1.UpdateOAuthClientRequest{
		OauthClient: &iamclientv1.OAuthClient{Name: "oauthClients/partner-app", DisplayName: new("New"), Etag: "stale"},
		UpdateMask:  &fieldmaskpb.FieldMask{Paths: []string{"display_name"}},
	}); !iamclientv1.IsClientErrorReasonEtagMismatch(err) {
		t.Fatalf("Update(stale etag) = %v, want ETAG_MISMATCH", err)
	}
	updated, err := svc.UpdateOAuthClient(ctx, &iamclientv1.UpdateOAuthClientRequest{
		OauthClient: &iamclientv1.OAuthClient{Name: "oauthClients/partner-app", DisplayName: new("Partner App v2"), Etag: got.GetEtag()},
		UpdateMask:  &fieldmaskpb.FieldMask{Paths: []string{"display_name"}},
	})
	if err != nil || updated.GetDisplayName() != "Partner App v2" {
		t.Fatalf("UpdateOAuthClient() = (%+v, %v)", updated, err)
	}

	// Secret rotation: deleting last active -> LAST_ACTIVE_SECRET (409); create 2nd then delete 1st succeeds.
	firstSecID := updated.GetSecrets()[0].GetSecretId()
	if _, err := svc.DeleteOAuthClientSecret(ctx, &iamclientv1.DeleteOAuthClientSecretRequest{
		Name: "oauthClients/partner-app", SecretId: firstSecID,
	}); !iamclientv1.IsClientErrorReasonLastActiveSecret(err) {
		t.Fatalf("DeleteSecret(last) = %v, want LAST_ACTIVE_SECRET", err)
	}
	newSecResp, err := svc.CreateOAuthClientSecret(ctx, &iamclientv1.CreateOAuthClientSecretRequest{
		Name: "oauthClients/partner-app", SecretName: "Rotated",
	})
	if err != nil || !strings.HasPrefix(newSecResp.GetSecretValue(), "pcs_") {
		t.Fatalf("CreateOAuthClientSecret() = (%+v, %v)", newSecResp, err)
	}
	if _, err := svc.DeleteOAuthClientSecret(ctx, &iamclientv1.DeleteOAuthClientSecretRequest{
		Name: "oauthClients/partner-app", SecretId: firstSecID,
	}); err != nil {
		t.Fatalf("DeleteOAuthClientSecret() = %v", err)
	}

	// Disable -> Enable -> Delete -> Get returns NOT_FOUND (404).
	disabled, err := svc.DisableOAuthClient(ctx, &iamclientv1.DisableOAuthClientRequest{Name: "oauthClients/partner-app", Etag: updated.GetEtag()})
	if err != nil || disabled.GetStatus() != iamclientv1.ClientStatus_CLIENT_STATUS_DISABLED {
		t.Fatalf("DisableOAuthClient() = (%+v, %v)", disabled, err)
	}
	enabled, err := svc.EnableOAuthClient(ctx, &iamclientv1.EnableOAuthClientRequest{Name: "oauthClients/partner-app", Etag: disabled.GetEtag()})
	if err != nil || enabled.GetStatus() != iamclientv1.ClientStatus_CLIENT_STATUS_ACTIVE {
		t.Fatalf("EnableOAuthClient() = (%+v, %v)", enabled, err)
	}
	if _, err := svc.DeleteOAuthClient(ctx, &iamclientv1.DeleteOAuthClientRequest{Name: "oauthClients/partner-app", Etag: enabled.GetEtag()}); err != nil {
		t.Fatalf("DeleteOAuthClient() = %v", err)
	}
	if _, err := svc.GetOAuthClient(ctx, &iamclientv1.GetOAuthClientRequest{Name: "oauthClients/partner-app"}); !iamclientv1.IsClientErrorReasonNotFound(err) {
		t.Fatalf("Get(deleted) = %v, want NOT_FOUND", err)
	}
}

func TestConsentAndAuthorizedAppsServiceEndpoints(t *testing.T) {
	userID := "01912345-6789-7abc-8def-0123456789ab"
	email := "alice@example.com"
	users := &stubUserRepo{user: &userpb.User{
		Name: "users/" + userID, UserId: userID, Email: &email,
		Status: userpb.UserStatus_USER_STATUS_ACTIVE, EmailVerified: true, Etag: "etag-1",
	}}
	creds := new(stubCredentialRepo)
	sessRepo := &stubSessionRepo{session: &biz.LoginSession{
		ID: "login-1", UserID: userID, AMR: []string{"pwd"}, AuthTime: time.Now().UTC(),
	}}
	const ownerToken = "svc-consent-owner"
	consents := &stubConsentRepo{
		pending: &stubConsentPending{
			view: &authnpb.ConsentView{
				RequestId:       "req-svc-1",
				ClientId:        "ext-app",
				ClientName:      "External App",
				LogoUri:         "https://ext.example.com/logo.png",
				RequestedScopes: []string{"openid", "profile"},
			},
			ownerHash:   biz.HashOpaqueSecret(ownerToken),
			redirectURI: "https://ext.example.com/cb",
			state:       "st-svc",
		},
		apps: []*accountpb.AuthorizedApp{
			{ClientId: "ext-app", ClientName: "External App", GrantedScopes: []string{"openid", "profile"}},
		},
	}

	sessUC, err := biz.NewSessionUsecase(users, sessRepo)
	if err != nil {
		t.Fatal(err)
	}
	authnUC, err := biz.NewAuthenticationUsecase(&iamconfpb.IAM{}, &oidcconfpb.OIDC{Issuer: proto.String("https://iam.example")}, users, creds, sessUC, newStubAuthFlowRepo(users, creds, sessRepo), consents)
	if err != nil {
		t.Fatal(err)
	}
	accUC, err := biz.NewAccountUsecase(users, creds, stubVerifyRepo{}, stubResetRepo{}, consents, stubCAP{}, stubMailer{}, &corepb.App{ExternalUrl: "https://iam.example"})
	if err != nil {
		t.Fatal(err)
	}

	manager := scs.New()
	manager.Store = memstore.New()
	authnSvc, err := NewAuthnService(authnUC, manager)
	if err != nil {
		t.Fatal(err)
	}
	accSvc, err := NewAccountService(accUC, manager)
	if err != nil {
		t.Fatal(err)
	}

	var baseCtx context.Context
	sessions.LoadAndSave(manager)(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		baseCtx = r.Context()
	})).ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))
	manager.Put(baseCtx, iamauthn.LoginReferenceKey, "login-1")
	manager.Put(baseCtx, iamauthn.AuthFlowOwnerKey, ownerToken)

	sessionAuthn, err := iamauthn.NewSessionAuthenticator(sessUC, manager)
	if err != nil {
		t.Fatal(err)
	}
	authnMW := sessionauthn.Server(sessionAuthn, security.WithRulesFuncs(authnpb.AuthnRules, accountpb.AuthnRules))

	callAuthed := func(op string, fn func(context.Context) (any, error)) (any, error) {
		trCtx := transport.NewServerContext(baseCtx, stubServerTransport{op: op})
		return authnMW(func(authedCtx context.Context, _ any) (any, error) {
			return fn(authedCtx)
		})(trCtx, nil)
	}

	viewAny, err := callAuthed(authnpb.OperationAuthnServiceGetConsentView, func(c context.Context) (any, error) {
		return authnSvc.GetConsentView(c, &authnpb.GetConsentViewRequest{RequestId: "req-svc-1"})
	})
	if err != nil {
		t.Fatalf("GetConsentView() error = %v", err)
	}
	if viewAny.(*authnpb.GetConsentViewResponse).GetConsent().GetClientName() != "External App" {
		t.Fatalf("GetConsentView() = %+v", viewAny)
	}

	approveAny, err := callAuthed(authnpb.OperationAuthnServiceSubmitConsentDecision, func(c context.Context) (any, error) {
		return authnSvc.SubmitConsentDecision(c, &authnpb.SubmitConsentDecisionRequest{RequestId: "req-svc-1", Approved: true})
	})
	if err != nil || approveAny.(*authnpb.SubmitConsentDecisionResponse).GetRedirectUri() != "/authorize/callback?id=req-svc-1" {
		t.Fatalf("SubmitConsentDecision(approve) = (%+v, %v)", approveAny, err)
	}

	denyAny, err := callAuthed(authnpb.OperationAuthnServiceSubmitConsentDecision, func(c context.Context) (any, error) {
		return authnSvc.SubmitConsentDecision(c, &authnpb.SubmitConsentDecisionRequest{RequestId: "req-svc-1", Approved: false})
	})
	if err != nil {
		t.Fatalf("SubmitConsentDecision(deny) error = %v", err)
	}
	parsedDeny, err := url.Parse(denyAny.(*authnpb.SubmitConsentDecisionResponse).GetRedirectUri())
	if err != nil || parsedDeny.Query().Get("error") != "access_denied" || parsedDeny.Query().Get("iss") != "https://iam.example" {
		t.Fatalf("SubmitConsentDecision(deny) = %+v", denyAny)
	}

	listAny, err := callAuthed(accountpb.OperationAccountServiceListAuthorizedApps, func(c context.Context) (any, error) {
		return accSvc.ListAuthorizedApps(c, &accountpb.ListAuthorizedAppsRequest{})
	})
	if err != nil || len(listAny.(*accountpb.ListAuthorizedAppsResponse).GetApplications()) != 1 {
		t.Fatalf("ListAuthorizedApps() = (%+v, %v)", listAny, err)
	}

	if _, err := callAuthed(accountpb.OperationAccountServiceRevokeAuthorizedApp, func(c context.Context) (any, error) {
		return accSvc.RevokeAuthorizedApp(c, &accountpb.RevokeAuthorizedAppRequest{ClientId: "ext-app"})
	}); err != nil || !slices.Equal(consents.revoked, []string{"ext-app"}) {
		t.Fatalf("RevokeAuthorizedApp() err=%v revoked=%v", err, consents.revoked)
	}
}
