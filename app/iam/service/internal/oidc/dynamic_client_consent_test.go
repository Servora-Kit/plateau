package oidc

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	iamclientv1 "github.com/Servora-Kit/plateau/api/gen/go/plateau/iam/client/v1"
	oidcconfv1 "github.com/Servora-Kit/plateau/api/gen/go/plateau/iam/oidc/conf/v1"
	iamauthn "github.com/Servora-Kit/plateau/app/iam/service/internal/authn"
	"github.com/Servora-Kit/plateau/app/iam/service/internal/biz"
	"github.com/Servora-Kit/plateau/app/iam/service/internal/data"
	"github.com/Servora-Kit/plateau/app/iam/service/internal/data/ent"
	"github.com/Servora-Kit/plateau/app/iam/service/internal/data/ent/oauthclientsecret"
	corecrud "github.com/Servora-Kit/plateau/crud"
	"github.com/alexedwards/scs/v2"
	"github.com/alexedwards/scs/v2/memstore"
	"github.com/alicebob/miniredis/v2"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
	fgaclient "github.com/openfga/go-sdk/client"
	goredis "github.com/redis/go-redis/v9"
	"google.golang.org/protobuf/proto"
)

type sqliteOIDCFixture struct {
	provider      *IAMProvider
	storage       *OIDCStorage
	bootstrap     *OIDCInitializer
	client        *ent.Client
	config        *oidcconfv1.OIDC
	clientUC      *biz.OAuthClientUsecase
	consentRepo   biz.OAuthConsentRepo
	manager       *scs.SessionManager
	sessionSecret string
	ownerToken    string
	ownerHash     string
	userID        string
	plan          *corecrud.ResourcePlan[*iamclientv1.OAuthClient]
}

func newSQLiteOIDCFixture(t *testing.T, staticClients []*oidcconfv1.OAuthClient) *sqliteOIDCFixture {
	t.Helper()
	ctx := t.Context()
	dsn := os.Getenv("IAM_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("IAM_TEST_POSTGRES_DSN is required for protocol integration tests")
	}
	configPG, err := pgx.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	adminDB := stdlib.OpenDB(*configPG)
	t.Cleanup(func() { _ = adminDB.Close() })
	schema := "oidc_dyn_" + uuid.NewString()[:8]
	if _, err := adminDB.ExecContext(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = adminDB.ExecContext(context.Background(), "DROP SCHEMA "+schema+" CASCADE")
	})
	configPG.RuntimeParams["search_path"] = schema
	database := stdlib.OpenDB(*configPG)
	t.Cleanup(func() { _ = database.Close() })
	driver := entsql.OpenDB(dialect.Postgres, database)
	client, cleanupDB, err := data.NewDBClient(driver)
	if err != nil {
		t.Fatalf("create schema: %v", err)
	}
	t.Cleanup(cleanupDB)

	miniRedis := miniredis.RunT(t)
	redisClient := goredis.NewClient(&goredis.Options{Addr: miniRedis.Addr()})
	t.Cleanup(func() { _ = redisClient.Close() })

	dataStore, err := data.NewData(client, redisClient, &fgaclient.OpenFgaClient{}, slog.Default())
	if err != nil {
		t.Fatal(err)
	}
	userRepo, err := data.NewUserRepository(dataStore)
	if err != nil {
		t.Fatal(err)
	}
	sessionRepo, err := data.NewSessionRepository(dataStore)
	if err != nil {
		t.Fatal(err)
	}
	oauthRepo, err := data.NewOAuthRepository(dataStore)
	if err != nil {
		t.Fatal(err)
	}
	clientRepo, err := data.NewOAuthClientRepository(dataStore)
	if err != nil {
		t.Fatal(err)
	}
	consentRepo, err := data.NewOAuthConsentRepository(dataStore)
	if err != nil {
		t.Fatal(err)
	}
	clientUC, err := biz.NewOAuthClientUsecase(clientRepo)
	if err != nil {
		t.Fatal(err)
	}
	sessionUC, err := biz.NewSessionUsecase(userRepo, sessionRepo)
	if err != nil {
		t.Fatal(err)
	}

	now := time.Now().UTC()
	userID, err := biz.NewID()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.User.Create().SetID(userID).SetStatus(biz.UserStatusActive).SetName("Alice").SetEtag("etag-u1").Save(ctx); err != nil {
		t.Fatal(err)
	}
	idRow, _ := biz.NewID()
	if _, err := client.LoginIdentifier.Create().SetID(idRow).SetUserID(userID).SetType(biz.LoginIdentifierEmail).
		SetCanonicalValue("alice@example.com").SetDisplayValue("alice@example.com").SetVerifiedTime(now).Save(ctx); err != nil {
		t.Fatal(err)
	}
	loginID, _ := biz.NewID()
	if _, err := client.LoginSession.Create().SetID(loginID).SetUserID(userID).SetAmr([]string{"pwd"}).SetCreateTime(now).Save(ctx); err != nil {
		t.Fatal(err)
	}

	const ownerToken = "sqlite-fixture-owner-token"
	manager := scs.New()
	manager.Store = memstore.New()
	manager.Cookie.Name = "__Host-iam_session"
	browserCtx, err := manager.Load(ctx, "")
	if err != nil {
		t.Fatal(err)
	}
	manager.Put(browserCtx, iamauthn.LoginReferenceKey, loginID)
	manager.Put(browserCtx, iamauthn.AuthFlowOwnerKey, ownerToken)
	sessionSecret, _, err := manager.Commit(browserCtx)
	if err != nil {
		t.Fatal(err)
	}

	cfg := testOIDCConfig(t)
	cfg.Clients = staticClients
	storage, err := NewOIDCStorage(client, cfg, oauthRepo)
	if err != nil {
		t.Fatal(err)
	}
	bootstrap, err := NewOIDCInitializer(cfg, storage)
	if err != nil {
		t.Fatal(err)
	}
	if err := bootstrap.Initialize(ctx); err != nil {
		t.Fatalf("bootstrap Initialize: %v", err)
	}
	provider, err := NewIAMProvider(cfg, storage, sessionUC, manager)
	if err != nil {
		t.Fatal(err)
	}

	return &sqliteOIDCFixture{
		provider:      provider,
		storage:       storage,
		bootstrap:     bootstrap,
		client:        client,
		config:        cfg,
		clientUC:      clientUC,
		consentRepo:   consentRepo,
		manager:       manager,
		sessionSecret: sessionSecret,
		ownerToken:    ownerToken,
		ownerHash:     biz.HashOpaqueSecret(ownerToken),
		userID:        userID,
		plan:          corecrud.MustBuildResourcePlan[*iamclientv1.OAuthClient](iamclientv1.OAuthClientCRUDDescriptor()),
	}
}

func pkcePair(verifier string) (string, string) {
	digest := sha256.Sum256([]byte(verifier))
	return verifier, base64.RawURLEncoding.EncodeToString(digest[:])
}

func TestOIDCInitializerZeroStaticDynamicCoexistenceAndConflict(t *testing.T) {
	// 1. Zero static clients allowed at startup.
	fx := newSQLiteOIDCFixture(t, nil)
	ctx := t.Context()
	if count := fx.client.OAuthClient.Query().CountX(ctx); count != 0 {
		t.Fatalf("expected 0 clients on empty static config, got %d", count)
	}

	// 2. Register a dynamic client, then reconcile static clients -> dynamic client MUST survive untouched!
	prepDyn, err := fx.plan.PrepareCreate(&iamclientv1.OAuthClient{
		DisplayName:             new("Dynamic SPA"),
		ApplicationType:         iamclientv1.ApplicationType_APPLICATION_TYPE_USER_AGENT.Enum(),
		TokenEndpointAuthMethod: iamclientv1.TokenEndpointAuthMethod_TOKEN_ENDPOINT_AUTH_METHOD_NONE.Enum(),
		RedirectUris:            []string{"http://localhost:5173/cb"},
		AllowedGrantTypes:       []string{biz.GrantTypeAuthorizationCode},
		AllowedScopes:           []string{"openid"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := fx.clientUC.RegisterOAuthClient(ctx, "dyn-spa", prepDyn, ""); err != nil {
		t.Fatal(err)
	}

	fx.config.Clients = []*oidcconfv1.OAuthClient{{
		ClientId:          proto.String("static-m2m"),
		ClientSecret:      proto.String("static-m2m-secret-with-at-least-32-bytes"),
		AllowedGrantTypes: []string{biz.GrantTypeClientCredentials},
		Audiences:         []string{"iam"},
	}}
	if err := fx.bootstrap.Initialize(ctx); err != nil {
		t.Fatalf("Initialize with static+dynamic: %v", err)
	}
	if fx.client.OAuthClient.Query().CountX(ctx) != 2 {
		t.Fatalf("expected both static-m2m and dyn-spa to exist")
	}

	// 3. Removing static-m2m from config removes ONLY static-m2m while preserving dyn-spa!
	fx.config.Clients = nil
	if err := fx.bootstrap.Initialize(ctx); err != nil {
		t.Fatalf("Initialize after removing static-m2m: %v", err)
	}
	if _, err := fx.client.OAuthClient.Get(ctx, "static-m2m"); !ent.IsNotFound(err) {
		t.Fatal("removed static client survived Initialize")
	}
	if dyn, err := fx.client.OAuthClient.Get(ctx, "dyn-spa"); err != nil || dyn.Source != biz.ClientSourceDynamic {
		t.Fatalf("dynamic client lost or altered during static reconciliation: %+v, %v", dyn, err)
	}

	// 4. Configuring a static client whose ID collides with an existing dynamic client MUST fail startup!
	fx.config.Clients = []*oidcconfv1.OAuthClient{{
		ClientId:          proto.String("dyn-spa"),
		ClientSecret:      proto.String("collision-secret-with-at-least-32-bytes"),
		AllowedGrantTypes: []string{biz.GrantTypeClientCredentials},
		Audiences:         []string{"iam"},
	}}
	if err := fx.bootstrap.Initialize(ctx); err == nil || !strings.Contains(err.Error(), "conflicts with existing dynamic") {
		t.Fatalf("expected static/dynamic ID conflict error, got %v", err)
	}
}

func TestFourClientArchetypesAuthMethodAntiDowngradeAndSecretRotation(t *testing.T) {
	fx := newSQLiteOIDCFixture(t, nil)
	ctx := t.Context()

	// 1. Confidential WEB client using client_secret_post.
	prepWebPost, err := fx.plan.PrepareCreate(&iamclientv1.OAuthClient{
		DisplayName:             new("Web Post Client"),
		ApplicationType:         iamclientv1.ApplicationType_APPLICATION_TYPE_WEB.Enum(),
		TokenEndpointAuthMethod: iamclientv1.TokenEndpointAuthMethod_TOKEN_ENDPOINT_AUTH_METHOD_CLIENT_SECRET_POST.Enum(),
		RedirectUris:            []string{"https://webpost.example.com/cb"},
		AllowedGrantTypes:       []string{biz.GrantTypeAuthorizationCode, biz.GrantTypeRefreshToken, biz.GrantTypeClientCredentials},
		AllowedScopes:           []string{"openid", "profile", "offline_access"},
		Audiences:               []string{"iam"},
		Trusted:                 new(true),
	})
	if err != nil {
		t.Fatal(err)
	}
	webPostClient, webPostSecret1, err := fx.clientUC.RegisterOAuthClient(ctx, "web-post", prepWebPost, "Key 1")
	if err != nil {
		t.Fatal(err)
	}

	// 2. Confidential M2M client using client_secret_basic.
	prepM2MBasic, err := fx.plan.PrepareCreate(&iamclientv1.OAuthClient{
		DisplayName:             new("M2M Basic Client"),
		ApplicationType:         iamclientv1.ApplicationType_APPLICATION_TYPE_WEB.Enum(),
		TokenEndpointAuthMethod: iamclientv1.TokenEndpointAuthMethod_TOKEN_ENDPOINT_AUTH_METHOD_CLIENT_SECRET_BASIC.Enum(),
		AllowedGrantTypes:       []string{biz.GrantTypeClientCredentials},
		Audiences:               []string{"iam"},
	})
	if err != nil {
		t.Fatal(err)
	}
	_, m2mBasicSecret, err := fx.clientUC.RegisterOAuthClient(ctx, "m2m-basic", prepM2MBasic, "")
	if err != nil {
		t.Fatal(err)
	}

	// 3. Public SPA (USER_AGENT, NONE, loopback HTTP).
	prepSPA, err := fx.plan.PrepareCreate(&iamclientv1.OAuthClient{
		DisplayName:             new("SPA Client"),
		ApplicationType:         iamclientv1.ApplicationType_APPLICATION_TYPE_USER_AGENT.Enum(),
		TokenEndpointAuthMethod: iamclientv1.TokenEndpointAuthMethod_TOKEN_ENDPOINT_AUTH_METHOD_NONE.Enum(),
		RedirectUris:            []string{"http://localhost:5173/cb"},
		AllowedGrantTypes:       []string{biz.GrantTypeAuthorizationCode, biz.GrantTypeRefreshToken},
		AllowedScopes:           []string{"openid", "offline_access"},
		Trusted:                 new(true),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := fx.clientUC.RegisterOAuthClient(ctx, "spa-none", prepSPA, ""); err != nil {
		t.Fatal(err)
	}

	// 4. Public Native client (NATIVE, NONE, custom scheme + loopback dynamic port).
	prepNative, err := fx.plan.PrepareCreate(&iamclientv1.OAuthClient{
		DisplayName:             new("Native CLI"),
		ApplicationType:         iamclientv1.ApplicationType_APPLICATION_TYPE_NATIVE.Enum(),
		TokenEndpointAuthMethod: iamclientv1.TokenEndpointAuthMethod_TOKEN_ENDPOINT_AUTH_METHOD_NONE.Enum(),
		RedirectUris:            []string{"myapp://oauth/callback", "http://127.0.0.1/callback"},
		AllowedGrantTypes:       []string{biz.GrantTypeAuthorizationCode},
		AllowedScopes:           []string{"openid"},
		Trusted:                 new(true),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := fx.clientUC.RegisterOAuthClient(ctx, "native-none", prepNative, ""); err != nil {
		t.Fatal(err)
	}

	postToken := func(form url.Values, basicID, basicSecret string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(http.MethodPost, testIssuer+"/oauth/token", strings.NewReader(form.Encode()))
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		if basicID != "" || basicSecret != "" {
			r.SetBasicAuth(basicID, basicSecret)
		}
		w := httptest.NewRecorder()
		fx.provider.ServeHTTP(w, r)
		return w
	}

	// --- Anti-Downgrade & Dual-Credential Matrix ---
	// A. m2m-basic sending secret in POST form MUST be rejected!
	assertOAuthError(t, postToken(url.Values{
		"grant_type":    {"client_credentials"},
		"client_id":     {"m2m-basic"},
		"client_secret": {m2mBasicSecret},
	}, "", ""), http.StatusUnauthorized, "invalid_client")

	// B. web-post sending secret in HTTP Basic header MUST be rejected!
	assertOAuthError(t, postToken(url.Values{
		"grant_type": {"client_credentials"},
	}, "web-post", webPostSecret1), http.StatusUnauthorized, "invalid_client")

	// C. Dual credentials (both Basic header AND POST client_secret) MUST return 400 invalid_request!
	assertOAuthError(t, postToken(url.Values{
		"grant_type":    {"client_credentials"},
		"client_id":     {"web-post"},
		"client_secret": {webPostSecret1},
	}, "web-post", webPostSecret1), http.StatusBadRequest, "invalid_request")

	// D. Confidential client sending NO secret MUST be rejected!
	assertOAuthError(t, postToken(url.Values{
		"grant_type": {"client_credentials"},
		"client_id":  {"web-post"},
	}, "", ""), http.StatusUnauthorized, "invalid_client")

	// E. GET /oauth/token -> 405 Method Not Allowed; query ?client_secret=... -> 400 invalid_request; mismatched Basic vs PostForm client_id -> 400 invalid_request!
	getTok := httptest.NewRequest(http.MethodGet, testIssuer+"/oauth/token", nil)
	getTokRec := httptest.NewRecorder()
	fx.provider.ServeHTTP(getTokRec, getTok)
	if getTokRec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("GET /oauth/token status = %d, want 405", getTokRec.Code)
	}
	querySecReq := httptest.NewRequest(http.MethodPost, testIssuer+"/oauth/token?client_secret="+webPostSecret1, strings.NewReader("grant_type=client_credentials&client_id=web-post"))
	querySecReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	querySecRec := httptest.NewRecorder()
	fx.provider.ServeHTTP(querySecRec, querySecReq)
	assertOAuthError(t, querySecRec, http.StatusBadRequest, "invalid_request")

	assertOAuthError(t, postToken(url.Values{
		"grant_type": {"client_credentials"},
		"client_id":  {"web-post"},
	}, "m2m-basic", m2mBasicSecret), http.StatusBadRequest, "invalid_request")

	// F. web-post with POST form secret succeeds!
	if resp := postToken(url.Values{
		"grant_type":    {"client_credentials"},
		"client_id":     {"web-post"},
		"client_secret": {webPostSecret1},
	}, "", ""); resp.Code != http.StatusOK {
		t.Fatalf("web-post client_credentials failed: %d %s", resp.Code, resp.Body.String())
	}

	// --- Multi-Secret Zero-Downtime Rotation ---
	sec2Meta, webPostSecret2, err := fx.clientUC.CreateOAuthClientSecret(ctx, iamclientv1.NewOAuthClientName("web-post"), "Key 2", nil)
	if err != nil {
		t.Fatal(err)
	}
	// Both Key 1 and Key 2 authenticate simultaneously and record last_used_time!
	for _, secVal := range []string{webPostSecret1, webPostSecret2} {
		if resp := postToken(url.Values{
			"grant_type":    {"client_credentials"},
			"client_id":     {"web-post"},
			"client_secret": {secVal},
		}, "", ""); resp.Code != http.StatusOK {
			t.Fatalf("dual active secret %q failed: %d %s", secVal[:12], resp.Code, resp.Body.String())
		}
	}
	sec2Row := fx.client.OAuthClientSecret.GetX(ctx, sec2Meta.GetSecretId())
	if sec2Row.LastUsedTime == nil {
		t.Fatal("AuthorizeClientIDSecret did not update last_used_time on matched secret")
	}

	// Delete Key 1 -> Key 1 immediately fails while Key 2 continues working!
	if err := fx.clientUC.DeleteOAuthClientSecret(ctx, iamclientv1.NewOAuthClientName("web-post"), webPostClient.GetSecrets()[0].GetSecretId()); err != nil {
		t.Fatal(err)
	}
	assertOAuthError(t, postToken(url.Values{
		"grant_type":    {"client_credentials"},
		"client_id":     {"web-post"},
		"client_secret": {webPostSecret1},
	}, "", ""), http.StatusUnauthorized, "invalid_client")
	if resp := postToken(url.Values{
		"grant_type":    {"client_credentials"},
		"client_id":     {"web-post"},
		"client_secret": {webPostSecret2},
	}, "", ""); resp.Code != http.StatusOK {
		t.Fatalf("surviving Key 2 failed after deleting Key 1: %d %s", resp.Code, resp.Body.String())
	}

	// --- Public SPA & Native Authorization Code + PKCE Flows ---
	authorizeAndCallback := func(clientID, redirectURI, scope, challenge string) string {
		t.Helper()
		q := url.Values{
			"client_id":             {clientID},
			"redirect_uri":          {redirectURI},
			"response_type":         {"code"},
			"scope":                 {scope},
			"state":                 {"st-1"},
			"nonce":                 {"nn-1"},
			"code_challenge":        {challenge},
			"code_challenge_method": {"S256"},
		}
		authRec := performRequest(fx.provider, http.MethodGet, testIssuer+"/authorize?"+q.Encode(), "", nil)
		if authRec.Code != http.StatusFound {
			t.Fatalf("authorize(%s) = %d %s", clientID, authRec.Code, authRec.Body.String())
		}
		cbPath := authRec.Header().Get("Location")
		cbRec := performRequest(fx.provider, http.MethodGet, testIssuer+cbPath, "", map[string]string{
			"Cookie": fx.manager.Cookie.Name + "=" + fx.sessionSecret,
		})
		if cbRec.Code != http.StatusFound {
			t.Fatalf("callback(%s) = %d %s", clientID, cbRec.Code, cbRec.Body.String())
		}
		loc, err := url.Parse(cbRec.Header().Get("Location"))
		if err != nil || loc.Query().Get("code") == "" {
			t.Fatalf("callback(%s) location = %q", clientID, cbRec.Header().Get("Location"))
		}
		return loc.Query().Get("code")
	}

	// SPA PKCE exchange + refresh without client_secret.
	spaVerifier, spaChallenge := pkcePair("spa-pkce-verifier-0123456789abcdefghijklmnopqrstuvwxyz")
	spaCode := authorizeAndCallback("spa-none", "http://localhost:5173/cb", "openid offline_access", spaChallenge)
	spaTokResp := postToken(url.Values{
		"grant_type":    {"authorization_code"},
		"client_id":     {"spa-none"},
		"code":          {spaCode},
		"redirect_uri":  {"http://localhost:5173/cb"},
		"code_verifier": {spaVerifier},
	}, "", "")
	spaTokens := decodeTokenResponse(t, spaTokResp)
	if spaTokens.RefreshToken == "" {
		t.Fatal("SPA offline_access did not receive refresh token")
	}
	spaRefreshed := postToken(url.Values{
		"grant_type":    {"refresh_token"},
		"client_id":     {"spa-none"},
		"refresh_token": {spaTokens.RefreshToken},
	}, "", "")
	decodeTokenResponse(t, spaRefreshed)

	// Native custom scheme ("myapp://oauth/callback") AND RFC 8252 dynamic loopback port ("http://127.0.0.1:54321/callback")!
	for _, nativeRedirect := range []string{"myapp://oauth/callback", "http://127.0.0.1:54321/callback"} {
		natVerifier, natChallenge := pkcePair("native-pkce-verifier-0123456789abcdefghijklmnopqrstuv")
		natCode := authorizeAndCallback("native-none", nativeRedirect, "openid", natChallenge)
		natTokResp := postToken(url.Values{
			"grant_type":    {"authorization_code"},
			"client_id":     {"native-none"},
			"code":          {natCode},
			"redirect_uri":  {nativeRedirect},
			"code_verifier": {natVerifier},
		}, "", "")
		decodeTokenResponse(t, natTokResp)
	}

	// Disabling spa-none immediately blocks refresh token exchange and new /authorize calls!
	if _, err := fx.clientUC.DisableOAuthClient(ctx, iamclientv1.NewOAuthClientName("spa-none"), ""); err != nil {
		t.Fatal(err)
	}
	var refreshedPayload tokenResponse
	_ = json.Unmarshal(spaRefreshed.Body.Bytes(), &refreshedPayload)
	disabledRefresh := postToken(url.Values{
		"grant_type":    {"refresh_token"},
		"client_id":     {"spa-none"},
		"refresh_token": {refreshedPayload.RefreshToken},
	}, "", "")
	if disabledRefresh.Code < http.StatusBadRequest {
		t.Fatalf("disabled SPA refresh token was accepted: %d %s", disabledRefresh.Code, disabledRefresh.Body.String())
	}

	// Ensure expired secret is rejected even when present in DB.
	expiredAt := time.Now().UTC().Add(-time.Minute)
	if _, err := fx.client.OAuthClientSecret.Update().
		Where(oauthclientsecret.IDEQ(sec2Meta.GetSecretId())).
		SetExpiresTime(expiredAt).
		Save(ctx); err != nil {
		t.Fatal(err)
	}
	assertOAuthError(t, postToken(url.Values{
		"grant_type":    {"client_credentials"},
		"client_id":     {"web-post"},
		"client_secret": {webPostSecret2},
	}, "", ""), http.StatusUnauthorized, "invalid_client")
}

func TestThirdPartyConsentSkipExpandAndPromptConsentNoLoop(t *testing.T) {
	fx := newSQLiteOIDCFixture(t, nil)
	ctx := t.Context()

	// Register an untrusted third-party Web client (trusted = false).
	prepUntrusted, err := fx.plan.PrepareCreate(&iamclientv1.OAuthClient{
		DisplayName:             new("Untrusted Partner"),
		ApplicationType:         iamclientv1.ApplicationType_APPLICATION_TYPE_WEB.Enum(),
		TokenEndpointAuthMethod: iamclientv1.TokenEndpointAuthMethod_TOKEN_ENDPOINT_AUTH_METHOD_NONE.Enum(),
		RedirectUris:            []string{"https://third.example.com/cb"},
		AllowedGrantTypes:       []string{biz.GrantTypeAuthorizationCode},
		AllowedScopes:           []string{"openid", "profile", "email"},
		Trusted:                 new(false),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := fx.clientUC.RegisterOAuthClient(ctx, "third-party", prepUntrusted, ""); err != nil {
		t.Fatal(err)
	}

	startAuthorize := func(scope, prompt string) (reqID, cbLocation string) {
		t.Helper()
		_, challenge := pkcePair("consent-pkce-verifier-0123456789abcdefghijklmnopqrstuv")
		q := url.Values{
			"client_id":             {"third-party"},
			"redirect_uri":          {"https://third.example.com/cb"},
			"response_type":         {"code"},
			"scope":                 {scope},
			"state":                 {"st-consent"},
			"nonce":                 {"nn-consent"},
			"code_challenge":        {challenge},
			"code_challenge_method": {"S256"},
		}
		if prompt != "" {
			q.Set("prompt", prompt)
		}
		rec := performRequest(fx.provider, http.MethodGet, testIssuer+"/authorize?"+q.Encode(), "", nil)
		if rec.Code != http.StatusFound {
			t.Fatalf("authorize = %d %s", rec.Code, rec.Body.String())
		}
		loc := rec.Header().Get("Location")
		parsed, err := url.Parse(loc)
		if err != nil {
			t.Fatal(err)
		}
		return parsed.Query().Get("id"), loc
	}

	callCallback := func(cbLocation string) *httptest.ResponseRecorder {
		return performRequest(fx.provider, http.MethodGet, testIssuer+cbLocation, "", map[string]string{
			"Cookie": fx.manager.Cookie.Name + "=" + fx.sessionSecret,
		})
	}

	// 1. First authorization (openid profile) -> redirects to /consent/?request_id=<id> without iss!
	req1, cb1 := startAuthorize("openid profile", "")
	rec1 := callCallback(cb1)
	if rec1.Code != http.StatusFound || rec1.Header().Get("Location") != "/consent/?request_id="+url.QueryEscape(req1) {
		t.Fatalf("first untrusted callback = %d loc=%q", rec1.Code, rec1.Header().Get("Location"))
	}

	// Approve consent -> subsequent callback issues authorization code!
	if _, err := fx.consentRepo.ApproveConsent(ctx, fx.userID, fx.ownerHash, req1, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	rec1After := callCallback(cb1)
	if rec1After.Code != http.StatusFound || !strings.HasPrefix(rec1After.Header().Get("Location"), "https://third.example.com/cb?") {
		t.Fatalf("callback after approval = %d loc=%q", rec1After.Code, rec1After.Header().Get("Location"))
	}

	// 2. Second authorization with subset/same scopes ("openid profile") -> skips consent and issues code directly!
	_, cb2 := startAuthorize("openid profile", "")
	rec2 := callCallback(cb2)
	if rec2.Code != http.StatusFound || !strings.HasPrefix(rec2.Header().Get("Location"), "https://third.example.com/cb?") {
		t.Fatalf("repeat covered-scope callback did not skip consent: %d loc=%q", rec2.Code, rec2.Header().Get("Location"))
	}

	// 3. Third authorization requesting new scope ("openid profile email") -> prompts /consent/ again!
	req3, cb3 := startAuthorize("openid profile email", "")
	rec3 := callCallback(cb3)
	if rec3.Code != http.StatusFound || rec3.Header().Get("Location") != "/consent/?request_id="+url.QueryEscape(req3) {
		t.Fatalf("expanded-scope callback did not prompt consent: %d loc=%q", rec3.Code, rec3.Header().Get("Location"))
	}
	if _, err := fx.consentRepo.ApproveConsent(ctx, fx.userID, fx.ownerHash, req3, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	if rec3After := callCallback(cb3); rec3After.Code != http.StatusFound || !strings.HasPrefix(rec3After.Header().Get("Location"), "https://third.example.com/cb?") {
		t.Fatalf("expanded-scope callback after approval = %d loc=%q", rec3After.Code, rec3After.Header().Get("Location"))
	}

	// 4. Authorization with prompt=consent -> forces /consent/ even though scopes are granted, AND after approval does NOT loop!
	req4, cb4 := startAuthorize("openid profile", "consent")
	rec4 := callCallback(cb4)
	if rec4.Code != http.StatusFound || rec4.Header().Get("Location") != "/consent/?request_id="+url.QueryEscape(req4) {
		t.Fatalf("prompt=consent callback did not prompt consent: %d loc=%q", rec4.Code, rec4.Header().Get("Location"))
	}
	if _, err := fx.consentRepo.ApproveConsent(ctx, fx.userID, fx.ownerHash, req4, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	rec4After := callCallback(cb4)
	if rec4After.Code != http.StatusFound || !strings.HasPrefix(rec4After.Header().Get("Location"), "https://third.example.com/cb?") {
		t.Fatalf("prompt=consent looped instead of issuing code after approval: %d loc=%q", rec4After.Code, rec4After.Header().Get("Location"))
	}

	// 5. Cross-user consent bypass defense: User A approves req5 (prompt=consent), then victim User B visits /authorize/callback?id=req5 -> MUST NOT bypass User B's consent!
	req5, cb5 := startAuthorize("openid profile", "consent")
	if r := callCallback(cb5); r.Code != http.StatusFound {
		t.Fatalf("req5 initial callback = %d", r.Code)
	}
	if _, err := fx.consentRepo.ApproveConsent(ctx, fx.userID, fx.ownerHash, req5, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	victimID, _ := biz.NewID()
	now := time.Now().UTC()
	if _, err := fx.client.User.Create().SetID(victimID).SetStatus(biz.UserStatusActive).SetName("Bob").SetEtag("etag-u2").Save(ctx); err != nil {
		t.Fatal(err)
	}
	victimIdentID, _ := biz.NewID()
	if _, err := fx.client.LoginIdentifier.Create().SetID(victimIdentID).SetUserID(victimID).SetType(biz.LoginIdentifierEmail).
		SetCanonicalValue("bob@example.com").SetDisplayValue("bob@example.com").SetVerifiedTime(now).Save(ctx); err != nil {
		t.Fatal(err)
	}
	victimLoginID, _ := biz.NewID()
	if _, err := fx.client.LoginSession.Create().SetID(victimLoginID).SetUserID(victimID).SetAmr([]string{"pwd"}).SetCreateTime(now).Save(ctx); err != nil {
		t.Fatal(err)
	}
	victimCtx, _ := fx.manager.Load(ctx, "")
	fx.manager.Put(victimCtx, iamauthn.LoginReferenceKey, victimLoginID)
	victimSessionSecret, _, _ := fx.manager.Commit(victimCtx)

	victimCallback := performRequest(fx.provider, http.MethodGet, testIssuer+cb5, "", map[string]string{
		"Cookie": fx.manager.Cookie.Name + "=" + victimSessionSecret,
	})
	if victimCallback.Code != http.StatusFound || victimCallback.Header().Get("Location") != "/consent/?request_id="+url.QueryEscape(req5) {
		t.Fatalf("victim cross-user consent bypass NOT blocked: code=%d loc=%q", victimCallback.Code, victimCallback.Header().Get("Location"))
	}
}
