package biz

import (
	"context"
	"errors"
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
	corecrud "github.com/Servora-Kit/plateau/crud"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/fieldmaskpb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type fakeOAuthClients struct {
	byID map[string]*iamclientv1.OAuthClient
}

func newFakeOAuthClients() *fakeOAuthClients {
	return &fakeOAuthClients{byID: make(map[string]*iamclientv1.OAuthClient)}
}

func (r *fakeOAuthClients) Create(_ context.Context, resource *iamclientv1.OAuthClient, initialSecret *iamclientv1.OAuthClientSecret, _ string, now time.Time) (*iamclientv1.OAuthClient, error) {
	if _, exists := r.byID[resource.GetClientId()]; exists {
		return nil, ErrAlreadyExists
	}
	cloned := proto.CloneOf(resource)
	cloned.Etag = "etag-1"
	cloned.CreateTime = timestamppb.New(now)
	cloned.UpdateTime = timestamppb.New(now)
	if initialSecret != nil {
		sec := proto.CloneOf(initialSecret)
		sec.CreateTime = timestamppb.New(now)
		cloned.Secrets = []*iamclientv1.OAuthClientSecret{sec}
	}
	r.byID[cloned.GetClientId()] = cloned
	return proto.CloneOf(cloned), nil
}

func (r *fakeOAuthClients) Get(_ context.Context, clientID string) (*iamclientv1.OAuthClient, error) {
	item, ok := r.byID[clientID]
	if !ok {
		return nil, ErrNotFound
	}
	return proto.CloneOf(item), nil
}

func (r *fakeOAuthClients) List(_ context.Context, query corecrud.ListQuery) (corecrud.ListResult[*iamclientv1.OAuthClient], error) {
	items := make([]*iamclientv1.OAuthClient, 0, len(r.byID))
	for _, v := range r.byID {
		items = append(items, proto.CloneOf(v))
	}
	return corecrud.NewListResult(query, items, "", nil)
}

func (r *fakeOAuthClients) Update(_ context.Context, clientID string, resource *iamclientv1.OAuthClient, mask *fieldmaskpb.FieldMask, expectedEtag string, now time.Time) (*iamclientv1.OAuthClient, error) {
	current, ok := r.byID[clientID]
	if !ok {
		return nil, ErrNotFound
	}
	if current.GetSource() == iamclientv1.ClientSource_CLIENT_SOURCE_STATIC {
		return nil, ErrStaticClientImmutable
	}
	if current.GetEtag() != expectedEtag {
		return nil, ErrMutationMiss
	}
	merged := mergeOAuthClientUpdate(current, resource, mask)
	if slices.Contains(mask.GetPaths(), iamclientv1.OAuthClientFields.AllowedResponseTypes.String()) {
		merged.AllowedResponseTypes = slices.Clone(resource.GetAllowedResponseTypes())
	}
	if merged.GetTokenEndpointAuthMethod() == iamclientv1.TokenEndpointAuthMethod_TOKEN_ENDPOINT_AUTH_METHOD_NONE {
		merged.Secrets = nil
	}
	merged.Etag = "etag-updated"
	merged.UpdateTime = timestamppb.New(now)
	r.byID[clientID] = merged
	return proto.CloneOf(merged), nil
}

func (r *fakeOAuthClients) UpdateStatus(_ context.Context, clientID, expectedEtag string, status iamclientv1.ClientStatus, now time.Time) (*iamclientv1.OAuthClient, error) {
	current, ok := r.byID[clientID]
	if !ok {
		return nil, ErrNotFound
	}
	if current.GetEtag() != expectedEtag {
		return nil, ErrMutationMiss
	}
	current.Status = status
	current.Etag = "etag-status"
	current.UpdateTime = timestamppb.New(now)
	return proto.CloneOf(current), nil
}

func (r *fakeOAuthClients) Delete(_ context.Context, clientID, expectedEtag string, _ time.Time) error {
	current, ok := r.byID[clientID]
	if !ok {
		return ErrNotFound
	}
	if current.GetEtag() != expectedEtag {
		return ErrMutationMiss
	}
	delete(r.byID, clientID)
	return nil
}

func (r *fakeOAuthClients) CreateSecret(_ context.Context, clientID string, secret *iamclientv1.OAuthClientSecret, _ string, now time.Time) (*iamclientv1.OAuthClientSecret, error) {
	current, ok := r.byID[clientID]
	if !ok {
		return nil, ErrNotFound
	}
	if len(current.Secrets) >= MaxClientSecrets {
		return nil, ErrSecretLimitExceeded
	}
	sec := proto.CloneOf(secret)
	sec.CreateTime = timestamppb.New(now)
	current.Secrets = append(current.Secrets, sec)
	return proto.CloneOf(sec), nil
}

func (r *fakeOAuthClients) DeleteSecret(_ context.Context, clientID, secretID string, now time.Time) error {
	current, ok := r.byID[clientID]
	if !ok {
		return ErrNotFound
	}
	idx := -1
	targetActive := false
	active := 0
	for i, s := range current.Secrets {
		isActive := s.GetExpiresTime() == nil || s.GetExpiresTime().AsTime().After(now)
		if s.GetSecretId() == secretID {
			idx = i
			targetActive = isActive
			continue
		}
		if isActive {
			active++
		}
	}
	if idx < 0 {
		return ErrNotFound
	}
	if (targetActive && active < 1) || len(current.Secrets) <= 1 {
		return ErrLastActiveSecret
	}
	current.Secrets = slices.Delete(current.Secrets, idx, idx+1)
	return nil
}

func TestClientIDSlugAndSecretGeneration(t *testing.T) {
	for _, valid := range []string{"a", "ab", "admin", "my-web-app-01", strings.Repeat("a", 63)} {
		if !ValidateClientIDSlug(valid) {
			t.Errorf("ValidateClientIDSlug(%q) = false, want true", valid)
		}
	}
	for _, invalid := range []string{"", "-abc", "abc-", "MyApp", "my_app", "app/1", strings.Repeat("a", 64)} {
		if ValidateClientIDSlug(invalid) {
			t.Errorf("ValidateClientIDSlug(%q) = true, want false", invalid)
		}
	}

	genID, err := NewGeneratedClientID()
	if err != nil || !strings.HasPrefix(genID, "cli_") || len(genID) != 28 {
		t.Fatalf("NewGeneratedClientID() = (%q, %v)", genID, err)
	}

	plain, prefix, hash, err := NewClientSecret()
	if err != nil {
		t.Fatalf("NewClientSecret() error = %v", err)
	}
	if !strings.HasPrefix(plain, "pcs_") || len(plain) < 44 || len(prefix) != 12 || !strings.HasPrefix(plain, prefix) {
		t.Fatalf("NewClientSecret() plain=%q prefix=%q", plain, prefix)
	}
	if hash != HashOpaqueSecret(plain) || hash == plain {
		t.Fatalf("NewClientSecret() hash mismatch: %q", hash)
	}
}

func TestValidateAndNormalizeOAuthClientMatrix(t *testing.T) {
	baseWeb := func() *iamclientv1.OAuthClient {
		return &iamclientv1.OAuthClient{
			DisplayName:             new(" Partner Web "),
			ApplicationType:         iamclientv1.ApplicationType_APPLICATION_TYPE_WEB.Enum(),
			TokenEndpointAuthMethod: iamclientv1.TokenEndpointAuthMethod_TOKEN_ENDPOINT_AUTH_METHOD_CLIENT_SECRET_BASIC.Enum(),
			RedirectUris:            []string{"https://partner.example.com/cb", "https://partner.example.com/cb"},
			PostLogoutRedirectUris:  []string{"https://partner.example.com/logout"},
			AllowedGrantTypes:       []string{GrantTypeAuthorizationCode, GrantTypeRefreshToken},
			AllowedScopes:           []string{"openid", "profile", "email"},
			Trusted:                 new(false),
		}
	}

	normWeb, err := ValidateAndNormalizeOAuthClient(
		"partner-web",
		baseWeb(),
		iamclientv1.ClientSource_CLIENT_SOURCE_DYNAMIC,
		iamclientv1.ClientStatus_CLIENT_STATUS_ACTIVE,
	)
	if err != nil {
		t.Fatalf("valid web config rejected: %v", err)
	}
	if normWeb.GetDisplayName() != "Partner Web" || len(normWeb.GetRedirectUris()) != 1 || !slices.Equal(normWeb.GetAllowedResponseTypes(), []string{"code"}) {
		t.Fatalf("normalized web = %+v", normWeb)
	}

	// SPA (user_agent) with loopback HTTP succeeds; confidential SPA fails.
	spa := &iamclientv1.OAuthClient{
		DisplayName:             new("SPA"),
		ApplicationType:         iamclientv1.ApplicationType_APPLICATION_TYPE_USER_AGENT.Enum(),
		TokenEndpointAuthMethod: iamclientv1.TokenEndpointAuthMethod_TOKEN_ENDPOINT_AUTH_METHOD_NONE.Enum(),
		RedirectUris:            []string{"http://localhost:5173/callback"},
		AllowedGrantTypes:       []string{GrantTypeAuthorizationCode},
		AllowedScopes:           []string{"openid"},
	}
	if _, err := ValidateAndNormalizeOAuthClient("spa-app", spa, iamclientv1.ClientSource_CLIENT_SOURCE_DYNAMIC, iamclientv1.ClientStatus_CLIENT_STATUS_ACTIVE); err != nil {
		t.Fatalf("valid SPA rejected: %v", err)
	}
	spaConfidential := proto.CloneOf(spa)
	spaConfidential.TokenEndpointAuthMethod = iamclientv1.TokenEndpointAuthMethod_TOKEN_ENDPOINT_AUTH_METHOD_CLIENT_SECRET_POST.Enum()
	if _, err := ValidateAndNormalizeOAuthClient("spa-app", spaConfidential, iamclientv1.ClientSource_CLIENT_SOURCE_DYNAMIC, iamclientv1.ClientStatus_CLIENT_STATUS_ACTIVE); !errors.Is(err, ErrInvalidClientConfig) {
		t.Fatalf("confidential SPA err = %v, want ErrInvalidClientConfig", err)
	}

	// Native client accepts custom schemes & loopback HTTP, rejects dangerous schemes and client_credentials.
	native := &iamclientv1.OAuthClient{
		DisplayName:             new("Mobile"),
		ApplicationType:         iamclientv1.ApplicationType_APPLICATION_TYPE_NATIVE.Enum(),
		TokenEndpointAuthMethod: iamclientv1.TokenEndpointAuthMethod_TOKEN_ENDPOINT_AUTH_METHOD_NONE.Enum(),
		RedirectUris:            []string{"myapp://oauth/callback", "com.example.app:/oauth2redirect", "http://127.0.0.1:8080/cb"},
		AllowedGrantTypes:       []string{GrantTypeAuthorizationCode, GrantTypeRefreshToken},
		AllowedScopes:           []string{"openid", "offline_access"},
	}
	if _, err := ValidateAndNormalizeOAuthClient("mobile-app", native, iamclientv1.ClientSource_CLIENT_SOURCE_DYNAMIC, iamclientv1.ClientStatus_CLIENT_STATUS_ACTIVE); err != nil {
		t.Fatalf("valid native config rejected: %v", err)
	}
	for _, badURI := range []string{
		"javascript:alert(1)",
		"data:text/html,hello",
		"file:///etc/passwd",
		"vbscript:msgbox(1)",
		"http://evil.example.com/cb",
		"myapp://oauth/callback#frag",
		"myapp://*/callback",
		"1invalid://callback",
		"myapp:",
	} {
		badNative := proto.CloneOf(native)
		badNative.RedirectUris = []string{badURI}
		if _, err := ValidateAndNormalizeOAuthClient("mobile-app", badNative, iamclientv1.ClientSource_CLIENT_SOURCE_DYNAMIC, iamclientv1.ClientStatus_CLIENT_STATUS_ACTIVE); !errors.Is(err, ErrInvalidClientConfig) {
			t.Errorf("native bad URI %q accepted: err=%v", badURI, err)
		}
	}

	// M2M client_credentials validation (WEB allowed; NATIVE confidential rejected).
	m2m := &iamclientv1.OAuthClient{
		DisplayName:             new("Billing Worker"),
		ApplicationType:         iamclientv1.ApplicationType_APPLICATION_TYPE_WEB.Enum(),
		TokenEndpointAuthMethod: iamclientv1.TokenEndpointAuthMethod_TOKEN_ENDPOINT_AUTH_METHOD_CLIENT_SECRET_POST.Enum(),
		AllowedGrantTypes:       []string{GrantTypeClientCredentials},
		Audiences:               []string{"iam", "billing"},
	}
	normM2M, err := ValidateAndNormalizeOAuthClient("billing-worker", m2m, iamclientv1.ClientSource_CLIENT_SOURCE_DYNAMIC, iamclientv1.ClientStatus_CLIENT_STATUS_ACTIVE)
	if err != nil || len(normM2M.GetAllowedResponseTypes()) != 0 {
		t.Fatalf("valid M2M = %+v, err=%v", normM2M, err)
	}
	nativeM2M := proto.CloneOf(m2m)
	nativeM2M.ApplicationType = iamclientv1.ApplicationType_APPLICATION_TYPE_NATIVE.Enum()
	if _, err := ValidateAndNormalizeOAuthClient("native-m2m", nativeM2M, iamclientv1.ClientSource_CLIENT_SOURCE_DYNAMIC, iamclientv1.ClientStatus_CLIENT_STATUS_ACTIVE); !errors.Is(err, ErrInvalidClientConfig) {
		t.Fatalf("native client_credentials = %v, want ErrInvalidClientConfig", err)
	}

	invalidCases := []struct {
		name   string
		mutate func(*iamclientv1.OAuthClient)
	}{
		{name: "empty display_name", mutate: func(s *iamclientv1.OAuthClient) { s.DisplayName = new("   ") }},
		{name: "refresh without code", mutate: func(s *iamclientv1.OAuthClient) { s.AllowedGrantTypes = []string{GrantTypeRefreshToken} }},
		{name: "public client_credentials", mutate: func(s *iamclientv1.OAuthClient) {
			s.TokenEndpointAuthMethod = iamclientv1.TokenEndpointAuthMethod_TOKEN_ENDPOINT_AUTH_METHOD_NONE.Enum()
			s.AllowedGrantTypes = []string{GrantTypeClientCredentials}
			s.Audiences = []string{"iam"}
		}},
		{name: "client_credentials without audiences", mutate: func(s *iamclientv1.OAuthClient) {
			s.AllowedGrantTypes = []string{GrantTypeClientCredentials}
			s.Audiences = nil
		}},
		{name: "code without openid scope", mutate: func(s *iamclientv1.OAuthClient) { s.AllowedScopes = []string{"profile"} }},
		{name: "code with unsupported scope", mutate: func(s *iamclientv1.OAuthClient) { s.AllowedScopes = []string{"openid", "root"} }},
		{name: "javascript logo_uri", mutate: func(s *iamclientv1.OAuthClient) { s.LogoUri = new("javascript:alert(1)") }},
		{name: "non-loopback http logo_uri", mutate: func(s *iamclientv1.OAuthClient) { s.LogoUri = new("http://cdn.example.com/logo.png") }},
		{name: "web custom scheme redirect_uri", mutate: func(s *iamclientv1.OAuthClient) { s.RedirectUris = []string{"myapp://cb"} }},
	}
	for _, tc := range invalidCases {
		t.Run(tc.name, func(t *testing.T) {
			spec := baseWeb()
			tc.mutate(spec)
			if _, err := ValidateAndNormalizeOAuthClient("partner-web", spec, iamclientv1.ClientSource_CLIENT_SOURCE_DYNAMIC, iamclientv1.ClientStatus_CLIENT_STATUS_ACTIVE); !errors.Is(err, ErrInvalidClientConfig) {
				t.Fatalf("expected ErrInvalidClientConfig, got %v", err)
			}
		})
	}
}

func TestOAuthClientUsecaseLifecycleAndStaticGuard(t *testing.T) {
	repo := newFakeOAuthClients()
	uc, err := NewOAuthClientUsecase(repo)
	if err != nil {
		t.Fatal(err)
	}
	plan := corecrud.MustBuildResourcePlan[*iamclientv1.OAuthClient](iamclientv1.OAuthClientCRUDDescriptor())

	repo.byID["admin"] = &iamclientv1.OAuthClient{
		Name:                    "oauthClients/admin",
		ClientId:                "admin",
		Source:                  iamclientv1.ClientSource_CLIENT_SOURCE_STATIC,
		Status:                  iamclientv1.ClientStatus_CLIENT_STATUS_ACTIVE,
		DisplayName:             new("Admin"),
		ApplicationType:         iamclientv1.ApplicationType_APPLICATION_TYPE_WEB.Enum(),
		TokenEndpointAuthMethod: iamclientv1.TokenEndpointAuthMethod_TOKEN_ENDPOINT_AUTH_METHOD_CLIENT_SECRET_BASIC.Enum(),
		AllowedGrantTypes:       []string{GrantTypeClientCredentials},
		Audiences:               []string{"iam"},
		Etag:                    "etag-static",
	}
	staticName := iamclientv1.NewOAuthClientName("admin")
	updStatic, err := plan.PrepareUpdate(&iamclientv1.OAuthClient{
		Name:        "oauthClients/admin",
		DisplayName: new("Hacked"),
	}, &fieldmaskpb.FieldMask{Paths: []string{"display_name"}}, corecrud.UpdateOptions{Etag: "etag-static"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := uc.UpdateOAuthClient(t.Context(), staticName, updStatic); !errors.Is(err, ErrStaticClientImmutable) {
		t.Fatalf("Update static = %v, want ErrStaticClientImmutable", err)
	}
	if _, err := uc.DisableOAuthClient(t.Context(), staticName, "etag-static"); !errors.Is(err, ErrStaticClientImmutable) {
		t.Fatalf("Disable static = %v, want ErrStaticClientImmutable", err)
	}
	if _, err := uc.EnableOAuthClient(t.Context(), staticName, "etag-static"); !errors.Is(err, ErrStaticClientImmutable) {
		t.Fatalf("Enable static = %v, want ErrStaticClientImmutable", err)
	}
	if err := uc.DeleteOAuthClient(t.Context(), staticName, "etag-static"); !errors.Is(err, ErrStaticClientImmutable) {
		t.Fatalf("Delete static = %v, want ErrStaticClientImmutable", err)
	}
	if _, _, err := uc.CreateOAuthClientSecret(t.Context(), staticName, "rot", nil); !errors.Is(err, ErrStaticClientImmutable) {
		t.Fatalf("CreateSecret static = %v, want ErrStaticClientImmutable", err)
	}
	if err := uc.DeleteOAuthClientSecret(t.Context(), staticName, "sec-1"); !errors.Is(err, ErrStaticClientImmutable) {
		t.Fatalf("DeleteSecret static = %v, want ErrStaticClientImmutable", err)
	}

	prepCreate, err := plan.PrepareCreate(&iamclientv1.OAuthClient{
		DisplayName:             new("Dynamic Web"),
		ApplicationType:         iamclientv1.ApplicationType_APPLICATION_TYPE_WEB.Enum(),
		TokenEndpointAuthMethod: iamclientv1.TokenEndpointAuthMethod_TOKEN_ENDPOINT_AUTH_METHOD_CLIENT_SECRET_BASIC.Enum(),
		RedirectUris:            []string{"https://app.example.com/cb"},
		AllowedGrantTypes:       []string{GrantTypeAuthorizationCode, GrantTypeRefreshToken},
		AllowedScopes:           []string{"openid", "email"},
	})
	if err != nil {
		t.Fatal(err)
	}
	created, initialPlain, err := uc.RegisterOAuthClient(t.Context(), "dyn-web", prepCreate, "")
	if err != nil || !strings.HasPrefix(initialPlain, "pcs_") || len(created.GetSecrets()) != 1 || created.GetSecrets()[0].GetName() != DefaultSecretName {
		t.Fatalf("RegisterOAuthClient() = (%+v, %q, %v)", created, initialPlain, err)
	}
	dynName := iamclientv1.NewOAuthClientName("dyn-web")

	if _, err := uc.EnableOAuthClient(t.Context(), dynName, created.GetEtag()); !errors.Is(err, ErrClientInvalidState) {
		t.Fatalf("Enable active client = %v, want ErrClientInvalidState", err)
	}

	past := time.Now().UTC().Add(-time.Minute)
	if _, _, err := uc.CreateOAuthClientSecret(t.Context(), dynName, "expired", &past); !errors.Is(err, ErrInvalidClientConfig) {
		t.Fatalf("CreateSecret(past) = %v, want ErrInvalidClientConfig", err)
	}
	for i := 2; i <= MaxClientSecrets; i++ {
		if _, plain, err := uc.CreateOAuthClientSecret(t.Context(), dynName, "key", nil); err != nil || !strings.HasPrefix(plain, "pcs_") {
			t.Fatalf("CreateSecret #%d err = %v", i, err)
		}
	}
	if _, _, err := uc.CreateOAuthClientSecret(t.Context(), dynName, "key-6", nil); !errors.Is(err, ErrSecretLimitExceeded) {
		t.Fatalf("CreateSecret #6 = %v, want ErrSecretLimitExceeded", err)
	}

	prepPublic, err := plan.PrepareCreate(&iamclientv1.OAuthClient{
		DisplayName:             new("Public Native"),
		ApplicationType:         iamclientv1.ApplicationType_APPLICATION_TYPE_NATIVE.Enum(),
		TokenEndpointAuthMethod: iamclientv1.TokenEndpointAuthMethod_TOKEN_ENDPOINT_AUTH_METHOD_NONE.Enum(),
		RedirectUris:            []string{"myapp://callback"},
		AllowedGrantTypes:       []string{GrantTypeAuthorizationCode},
		AllowedScopes:           []string{"openid"},
	})
	if err != nil {
		t.Fatal(err)
	}
	pubClient, pubSecret, err := uc.RegisterOAuthClient(t.Context(), "", prepPublic, "")
	if err != nil || pubSecret != "" || !strings.HasPrefix(pubClient.GetClientId(), "cli_") {
		t.Fatalf("Register public client = (%+v, %q, %v)", pubClient, pubSecret, err)
	}
	switchToBasic, err := plan.PrepareUpdate(&iamclientv1.OAuthClient{
		Name:                    pubClient.GetName(),
		TokenEndpointAuthMethod: iamclientv1.TokenEndpointAuthMethod_TOKEN_ENDPOINT_AUTH_METHOD_CLIENT_SECRET_BASIC.Enum(),
	}, &fieldmaskpb.FieldMask{Paths: []string{"token_endpoint_auth_method"}}, corecrud.UpdateOptions{Etag: pubClient.GetEtag()})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := uc.UpdateOAuthClient(t.Context(), iamclientv1.NewOAuthClientName(pubClient.GetClientId()), switchToBasic); !errors.Is(err, ErrInvalidClientConfig) {
		t.Fatalf("Switch NONE->BASIC with 0 secrets = %v, want ErrInvalidClientConfig", err)
	}
}

func TestConsentAndAuthorizedAppsUsecase(t *testing.T) {
	u := activeUser()
	users := &fakeAccountUsers{user: u}
	creds := new(fakeCredentials)
	sessions := new(fakeSessions)
	sessionUC := newSessionForTest(t, users, sessions)
	const ownerTok = "consent-owner-token-1"
	consents := &fakeConsents{
		pending: &fakeConsentPending{
			view: &authnpb.ConsentView{
				RequestId:       "req-consent-1",
				ClientId:        "third-party",
				ClientName:      "Third Party App",
				LogoUri:         "https://third.example.com/logo.png",
				RequestedScopes: []string{"openid", "email", "profile"},
			},
			ownerHash:   HashOpaqueSecret(ownerTok),
			redirectURI: "https://third.example.com/callback",
			state:       "st-99",
		},
		granted: map[string][]string{"user-1:third-party": {"openid"}},
		apps: []*accountpb.AuthorizedApp{
			{ClientId: "third-party", ClientName: "Third Party App", GrantedScopes: []string{"openid"}},
		},
	}
	authnUC, err := NewAuthenticationUsecase(&iamconfpb.IAM{}, &oidcconfpb.OIDC{Issuer: new("https://iam.example.com")}, users, creds, sessionUC, newFakeAuthFlows(users, creds, sessions), consents)
	if err != nil {
		t.Fatal(err)
	}

	// Wrong browser owner token rejected.
	if _, err := authnUC.GetConsentView(t.Context(), u, "wrong-owner", "req-consent-1"); !errors.Is(err, ErrOIDCRequestInvalid) {
		t.Fatalf("GetConsentView(wrong owner) = %v, want ErrOIDCRequestInvalid", err)
	}

	view, err := authnUC.GetConsentView(t.Context(), u, ownerTok, "req-consent-1")
	if err != nil || view.GetClientName() != "Third Party App" || view.GetUserEmail() != u.GetEmail() || !slices.Equal(view.GetGrantedScopes(), []string{"openid"}) {
		t.Fatalf("GetConsentView() = (%+v, %v)", view, err)
	}

	approveURI, approveCID, err := authnUC.SubmitConsentDecision(t.Context(), u, ownerTok, "req-consent-1", true)
	if err != nil || approveCID != "third-party" || approveURI != "/authorize/callback?id=req-consent-1" {
		t.Fatalf("SubmitConsentDecision(approve) = (%q, %q, %v)", approveURI, approveCID, err)
	}
	if !slices.Equal(consents.granted["user-1:third-party"], []string{"email", "openid", "profile"}) {
		t.Fatalf("merged scopes = %v", consents.granted["user-1:third-party"])
	}

	denyURI, denyCID, err := authnUC.SubmitConsentDecision(t.Context(), u, ownerTok, "req-consent-1", false)
	if err != nil || denyCID != "third-party" {
		t.Fatalf("SubmitConsentDecision(deny) = (%q, %q, %v)", denyURI, denyCID, err)
	}
	parsedDeny, err := url.Parse(denyURI)
	if err != nil || parsedDeny.Query().Get("error") != "access_denied" || parsedDeny.Query().Get("state") != "st-99" || parsedDeny.Query().Get("iss") != "https://iam.example.com" {
		t.Fatalf("deny redirect = %q", denyURI)
	}

	accUC, err := NewAccountUsecase(users, creds, new(fakeVerificationTokens), new(fakePasswordResetTokens), consents, &fakeCAP{valid: true}, new(fakeMailSender), accountTestRuntime())
	if err != nil {
		t.Fatal(err)
	}
	apps, err := accUC.ListAuthorizedApps(t.Context(), "user-1")
	if err != nil || len(apps) != 1 || apps[0].GetClientId() != "third-party" {
		t.Fatalf("ListAuthorizedApps() = (%+v, %v)", apps, err)
	}
	if err := accUC.RevokeAuthorizedApp(t.Context(), "user-1", "third-party"); err != nil || !slices.Equal(consents.revoked, []string{"third-party"}) {
		t.Fatalf("RevokeAuthorizedApp() err=%v revoked=%v", err, consents.revoked)
	}
}
