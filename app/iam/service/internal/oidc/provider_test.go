package oidc

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	oidcconfv1 "github.com/Servora-Kit/plateau/api/gen/go/plateau/iam/oidc/conf/v1"
	jwtpb "github.com/Servora-Kit/plateau/api/gen/go/plateau/security/authn/jwt/v1"
	sessionconfig "github.com/Servora-Kit/plateau/api/gen/go/plateau/security/session/v1"
	iamauthn "github.com/Servora-Kit/plateau/app/iam/service/internal/authn"
	"github.com/Servora-Kit/plateau/app/iam/service/internal/biz"
	"github.com/Servora-Kit/plateau/app/iam/service/internal/data"
	"github.com/Servora-Kit/plateau/app/iam/service/internal/data/ent"
	"github.com/Servora-Kit/plateau/app/iam/service/internal/data/ent/oauthclient"
	"github.com/Servora-Kit/plateau/app/iam/service/internal/data/ent/oauthclientsecret"
	"github.com/Servora-Kit/plateau/security"
	authnjwt "github.com/Servora-Kit/plateau/security/authn/jwt"
	httpsession "github.com/Servora-Kit/plateau/security/session"
	"github.com/alexedwards/scs/v2"
	"github.com/alicebob/miniredis/v2"
	jwtlib "github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
	fgaclient "github.com/openfga/go-sdk/client"
	goredis "github.com/redis/go-redis/v9"
	oidcprotocol "github.com/zitadel/oidc/v3/pkg/oidc"
	"google.golang.org/protobuf/proto"
)

const (
	testIssuer       = "http://localhost:8000"
	testClientID     = "test-web"
	testClientSecret = "test-web-secret-with-at-least-32-bytes"
	testRedirectURI  = "http://localhost:3001/callback"
)

type providerFixture struct {
	provider      *IAMProvider
	storage       *OIDCStorage
	bootstrap     *OIDCInitializer
	client        *ent.Client
	config        *oidcconfv1.OIDC
	sessions      *biz.SessionUsecase
	sessionSecret string
	manager       *scs.SessionManager
	userID        string
}

func TestAuthorizationCodeFlowWithRefreshRotation(t *testing.T) {
	fixture := newProviderFixture(t)
	ctx := context.Background()

	seeded, err := fixture.client.OAuthClient.Query().Where(oauthclient.IDEQ(testClientID)).Only(ctx)
	if err != nil {
		t.Fatalf("query seeded OAuth client: %v", err)
	}
	if seeded.Source != biz.ClientSourceStatic || seeded.Status != biz.ClientStatusActive {
		t.Fatalf("seeded OAuth client source=%q status=%q", seeded.Source, seeded.Status)
	}
	seededSecret, err := fixture.client.OAuthClientSecret.Query().Where(oauthclientsecret.ClientIDEQ(testClientID)).Only(ctx)
	if err != nil {
		t.Fatalf("query seeded OAuth client secret: %v", err)
	}
	originalHash := seededSecret.SecretHash
	if originalHash == "" || originalHash == testClientSecret {
		t.Fatal("OAuth client secret was stored in plaintext or empty")
	}
	if err := fixture.bootstrap.Initialize(ctx); err != nil {
		t.Fatalf("run OIDC initializer twice: %v", err)
	}
	seededSecretAgain, err := fixture.client.OAuthClientSecret.Query().Where(oauthclientsecret.ClientIDEQ(testClientID)).Only(ctx)
	if err != nil {
		t.Fatalf("query reconciled OAuth client secret: %v", err)
	}
	if seededSecretAgain.SecretHash != originalHash || seededSecretAgain.ID != seededSecret.ID {
		t.Fatal("idempotent bootstrap replaced the persisted client-secret record or hash")
	}
	fixture.config.Clients[0].ClientSecret = proto.String("different-client-secret-with-at-least-32-bytes")
	if err := fixture.bootstrap.Initialize(ctx); err != nil {
		t.Fatalf("rotate client secret: %v", err)
	}
	rotatedSecret := fixture.client.OAuthClientSecret.Query().Where(oauthclientsecret.ClientIDEQ(testClientID)).OnlyX(ctx)
	if rotatedSecret.SecretHash == originalHash {
		t.Fatal("secret rotation was not persisted")
	}
	fixture.config.Clients[0].ClientSecret = proto.String(testClientSecret)
	if err := fixture.bootstrap.Initialize(ctx); err != nil {
		t.Fatal(err)
	}
	keyCount, err := fixture.client.OIDCSigningKey.Query().Count(ctx)
	if err != nil || keyCount != 1 {
		t.Fatalf("signing key metadata count = %d, err = %v; want 1", keyCount, err)
	}

	verifier := "pkce-verifier-with-sufficient-entropy-0123456789"
	challengeDigest := sha256.Sum256([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(challengeDigest[:])
	authorizeValues := url.Values{
		"client_id":             {testClientID},
		"redirect_uri":          {testRedirectURI},
		"response_type":         {"code"},
		"scope":                 {"openid profile email offline_access"},
		"state":                 {"state-value"},
		"nonce":                 {"nonce-value"},
		"code_challenge":        {challenge},
		"code_challenge_method": {"S256"},
	}
	authorizeURL := testIssuer + "/authorize?" + authorizeValues.Encode()

	badRedirectValues := cloneValues(authorizeValues)
	badRedirectValues.Set("redirect_uri", "https://attacker.example/callback")
	badRedirect := performRequest(fixture.provider, http.MethodGet, testIssuer+"/authorize?"+badRedirectValues.Encode(), "", nil)
	if strings.Contains(badRedirect.Header().Get("Location"), "attacker.example") {
		t.Fatalf("unregistered redirect leaked to attacker: %q", badRedirect.Header().Get("Location"))
	}
	if badRedirect.Code != http.StatusBadRequest {
		t.Fatalf("unregistered redirect status = %d, body = %s", badRedirect.Code, badRedirect.Body.String())
	}

	if _, err := fixture.client.OAuthClient.UpdateOneID(testClientID).SetAllowedScopes([]string{"openid"}).Save(ctx); err != nil {
		t.Fatalf("restrict test client scopes: %v", err)
	}
	scopeRejected := performRequest(fixture.provider, http.MethodGet, authorizeURL, "", nil)
	if scopeRejected.Code != http.StatusFound {
		t.Fatalf("disallowed client scope status = %d, body = %s", scopeRejected.Code, scopeRejected.Body.String())
	}
	if got := scopeRejected.Header().Get("Cache-Control"); got != "no-store" {
		t.Fatalf("authorize error Cache-Control = %q, want no-store", got)
	}
	scopeErrorLocation, err := url.Parse(scopeRejected.Header().Get("Location"))
	if err != nil || scopeErrorLocation.Query().Get("error") != "invalid_scope" || scopeErrorLocation.Query().Get("iss") != testIssuer {
		t.Fatalf("disallowed client scope redirect = %q, err = %v", scopeRejected.Header().Get("Location"), err)
	}
	if _, err := fixture.client.OAuthClient.UpdateOneID(testClientID).SetAllowedScopes(supportedScopes).Save(ctx); err != nil {
		t.Fatalf("restore test client scopes: %v", err)
	}

	authorize := performRequest(fixture.provider, http.MethodGet, authorizeURL, "", nil)
	if authorize.Code != http.StatusFound {
		t.Fatalf("authorize status = %d, body = %s", authorize.Code, authorize.Body.String())
	}
	if got := authorize.Header().Get("Cache-Control"); got != "no-store" {
		t.Fatalf("authorize Cache-Control = %q, want no-store", got)
	}
	callbackLocation := authorize.Header().Get("Location")
	callbackURL, err := url.Parse(callbackLocation)
	if err != nil || callbackURL.Path != "/authorize/callback" || callbackURL.Query().Get("id") == "" || callbackURL.Query().Has("iss") {
		t.Fatalf("authorize location = %q, err = %v", callbackLocation, err)
	}
	requestID := callbackURL.Query().Get("id")
	if strings.Contains(callbackLocation, testRedirectURI) {
		t.Fatalf("login interaction leaked redirect URI: %q", callbackLocation)
	}
	fixture.restartProvider(t)

	withoutSession := performRequest(fixture.provider, http.MethodGet, testIssuer+callbackLocation, "", nil)
	if withoutSession.Code != http.StatusFound || withoutSession.Header().Get("Location") != "/login?request_id="+url.QueryEscape(requestID) || withoutSession.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("unauthenticated callback = %d %q (cc=%q)", withoutSession.Code, withoutSession.Header().Get("Location"), withoutSession.Header().Get("Cache-Control"))
	}

	callbackRequest := httptest.NewRequest(http.MethodGet, testIssuer+callbackLocation, nil)
	callbackRequest.AddCookie(&http.Cookie{Name: fixture.manager.Cookie.Name, Value: fixture.sessionSecret})
	callback := httptest.NewRecorder()
	httpsession.LoadAndSave(fixture.manager)(fixture.provider).ServeHTTP(callback, callbackRequest)
	if callback.Code != http.StatusFound {
		t.Fatalf("authenticated callback status = %d, body = %s", callback.Code, callback.Body.String())
	}
	if got := callback.Header().Get("Cache-Control"); got != "no-store" {
		t.Fatalf("authenticated callback Cache-Control = %q, want no-store", got)
	}
	clientRedirect, err := url.Parse(callback.Header().Get("Location"))
	if err != nil || clientRedirect.Scheme+"://"+clientRedirect.Host+clientRedirect.Path != testRedirectURI {
		t.Fatalf("client redirect = %q, err = %v", callback.Header().Get("Location"), err)
	}
	if clientRedirect.Query().Get("state") != "state-value" || clientRedirect.Query().Get("code") == "" || clientRedirect.Query().Get("iss") != testIssuer {
		t.Fatalf("client redirect query = %q", clientRedirect.RawQuery)
	}
	authorizationCode := clientRedirect.Query().Get("code")
	fixture.restartProvider(t)

	wrongRedirect := exchangeTokenRaw(fixture.provider, url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {authorizationCode},
		"redirect_uri":  {"http://localhost:3001/wrong"},
		"code_verifier": {verifier},
	})
	assertOAuthError(t, wrongRedirect, http.StatusBadRequest, "invalid_grant")
	wrongVerifier := exchangeTokenRaw(fixture.provider, url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {authorizationCode},
		"redirect_uri":  {testRedirectURI},
		"code_verifier": {"wrong-pkce-verifier-with-sufficient-length"},
	})
	assertOAuthError(t, wrongVerifier, http.StatusBadRequest, "invalid_grant")

	tokens := exchangeCode(t, fixture.provider, authorizationCode, verifier)
	assertRS256JWT(t, tokens.AccessToken)
	assertRS256JWT(t, tokens.IDToken)
	if tokens.RefreshToken == "" {
		t.Fatal("offline_access exchange did not issue a refresh token")
	}
	jwtVerifier, err := NewJWTVerifier(fixture.storage)
	if err != nil {
		t.Fatalf("NewJWTVerifier: %v", err)
	}
	humanAuthn, closeHumanAuthn, err := authnjwt.New(ctx, &jwtpb.JwtAuthnConfig{
		Issuer:   testIssuer,
		Audience: testClientID,
	}, authnjwt.WithVerifier(jwtVerifier))
	if err != nil {
		t.Fatalf("authnjwt.New: %v", err)
	}
	t.Cleanup(closeHumanAuthn)
	humanActor, err := authnjwt.AuthenticateFor[authnjwt.HumanClaims](ctx, humanAuthn, "Bearer "+tokens.AccessToken)
	if err != nil || humanActor != (security.Actor{Type: security.ActorTypeHuman, ID: fixture.userID}) {
		t.Fatalf("AuthenticateFor[HumanClaims](access_token) = %+v, err = %v", humanActor, err)
	}
	if _, err := authnjwt.AuthenticateFor[authnjwt.ServiceClaims](ctx, humanAuthn, "Bearer "+tokens.AccessToken); err == nil {
		t.Fatal("human access_token was unexpectedly accepted by ServiceClaims")
	}
	idClaimsMap := jwtlib.MapClaims{}
	if _, err := jwtVerifier.VerifySignature(ctx, tokens.IDToken, &idClaimsMap); err != nil {
		t.Fatalf("verify id_token signature: %v", err)
	}
	if _, hasTokenUse := idClaimsMap["token_use"]; hasTokenUse {
		t.Fatalf("id_token unexpectedly carried token_use claim: %+v", idClaimsMap)
	}
	rawAMR, _ := idClaimsMap["amr"].([]any)
	var gotAMR []string
	for _, item := range rawAMR {
		if s, ok := item.(string); ok {
			gotAMR = append(gotAMR, s)
		}
	}
	if !sameStrings(gotAMR, []string{"hwk", "mfa", "user"}) {
		t.Fatalf("id_token amr = %v, want [hwk mfa user]", gotAMR)
	}
	if _, err := authnjwt.AuthenticateFor[authnjwt.HumanClaims](ctx, humanAuthn, "Bearer "+tokens.IDToken); err == nil {
		t.Fatal("id_token without token_use=access was unexpectedly accepted by HumanClaims")
	}

	replayedCode := exchangeTokenRaw(fixture.provider, url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {authorizationCode},
		"redirect_uri":  {testRedirectURI},
		"code_verifier": {verifier},
	})
	assertOAuthError(t, replayedCode, http.StatusBadRequest, "invalid_grant")

	userinfo := performRequest(
		fixture.provider,
		http.MethodGet,
		testIssuer+"/userinfo",
		"",
		map[string]string{"Authorization": "Bearer " + tokens.AccessToken},
	)
	if userinfo.Code != http.StatusOK {
		t.Fatalf("userinfo status = %d, body = %s", userinfo.Code, userinfo.Body.String())
	}
	var claims struct {
		Subject       string `json:"sub"`
		Email         string `json:"email"`
		EmailVerified bool   `json:"email_verified"`
		Name          string `json:"name"`
	}
	if err := json.Unmarshal(userinfo.Body.Bytes(), &claims); err != nil {
		t.Fatalf("decode userinfo: %v", err)
	}
	if claims.Subject != fixture.userID || claims.Email != "person@example.com" || !claims.EmailVerified || claims.Name != "Person" {
		t.Fatalf("userinfo claims = %+v", claims)
	}
	activeToken := introspectToken(t, fixture.provider, tokens.AccessToken)
	if !activeToken.Active || activeToken.Subject != fixture.userID || activeToken.ClientID != testClientID {
		t.Fatalf("active token introspection = %+v", activeToken)
	}

	discovery := performRequest(fixture.provider, http.MethodGet, testIssuer+"/.well-known/openid-configuration", "", nil)
	if discovery.Code != http.StatusOK {
		t.Fatalf("discovery status = %d, body = %s", discovery.Code, discovery.Body.String())
	}
	var metadata struct {
		Issuer                                     string   `json:"issuer"`
		Scopes                                     []string `json:"scopes_supported"`
		GrantTypes                                 []string `json:"grant_types_supported"`
		CodeChallengeMethods                       []string `json:"code_challenge_methods_supported"`
		TokenEndpointAuthenticationMethods         []string `json:"token_endpoint_auth_methods_supported"`
		AuthorizationResponseIssParameterSupported bool     `json:"authorization_response_iss_parameter_supported"`
	}
	if err := json.Unmarshal(discovery.Body.Bytes(), &metadata); err != nil {
		t.Fatalf("decode discovery: %v", err)
	}
	if metadata.Issuer != testIssuer || !sameStrings(metadata.Scopes, supportedScopes) || !metadata.AuthorizationResponseIssParameterSupported {
		t.Fatalf("discovery metadata = %+v", metadata)
	}
	if !contains(metadata.GrantTypes, string(oidcprotocol.GrantTypeClientCredentials)) || !contains(metadata.GrantTypes, string(oidcprotocol.GrantTypeCode)) || !contains(metadata.GrantTypes, string(oidcprotocol.GrantTypeRefreshToken)) {
		t.Fatalf("discovery grant types = %v", metadata.GrantTypes)
	}
	if !sameStrings(metadata.CodeChallengeMethods, []string{"S256"}) || !sameStrings(metadata.TokenEndpointAuthenticationMethods, []string{"client_secret_basic", "client_secret_post", "none"}) {
		t.Fatalf("discovery PKCE/auth methods = %v / %v", metadata.CodeChallengeMethods, metadata.TokenEndpointAuthenticationMethods)
	}

	jwks := performRequest(fixture.provider, http.MethodGet, testIssuer+"/keys", "", nil)
	if jwks.Code != http.StatusOK {
		t.Fatalf("JWKS status = %d, body = %s", jwks.Code, jwks.Body.String())
	}
	var keySet struct {
		Keys []struct {
			KeyID     string `json:"kid"`
			Algorithm string `json:"alg"`
			Use       string `json:"use"`
		} `json:"keys"`
	}
	if err := json.Unmarshal(jwks.Body.Bytes(), &keySet); err != nil {
		t.Fatalf("decode JWKS: %v", err)
	}
	if len(keySet.Keys) != 1 || keySet.Keys[0].KeyID == "" || keySet.Keys[0].Algorithm != "RS256" || keySet.Keys[0].Use != "sig" {
		t.Fatalf("JWKS = %+v", keySet.Keys)
	}

	rotated := exchangeRefreshScope(t, fixture.provider, tokens.RefreshToken, "openid email")
	if rotated.RefreshToken == "" || rotated.RefreshToken == tokens.RefreshToken || !sameStrings(strings.Fields(rotated.Scope), []string{"openid", "email"}) {
		t.Fatalf("refresh rotation = %+v", rotated)
	}
	rotatedAgain := exchangeRefresh(t, fixture.provider, rotated.RefreshToken)
	if rotatedAgain.RefreshToken == "" || rotatedAgain.RefreshToken == rotated.RefreshToken || !sameStrings(strings.Fields(rotatedAgain.Scope), []string{"openid", "email"}) {
		t.Fatalf("persisted refresh scope rotation = %+v", rotatedAgain)
	}
	rotatedClaims := jwtlib.MapClaims{}
	if _, err := jwtVerifier.VerifySignature(ctx, rotatedAgain.IDToken, &rotatedClaims); err != nil {
		t.Fatalf("verify rotated id_token: %v", err)
	}
	var rotatedAMR []string
	if arr, ok := rotatedClaims["amr"].([]any); ok {
		for _, item := range arr {
			if s, ok := item.(string); ok {
				rotatedAMR = append(rotatedAMR, s)
			}
		}
	}
	if !sameStrings(rotatedAMR, []string{"hwk", "mfa", "user"}) {
		t.Fatalf("rotated id_token amr = %v, want [hwk mfa user]", rotatedAMR)
	}

	oldReplay := exchangeTokenRaw(fixture.provider, url.Values{
		"grant_type":    {"refresh_token"},
		"refresh_token": {tokens.RefreshToken},
	})
	assertOAuthError(t, oldReplay, http.StatusBadRequest, "invalid_grant")
	familyAfterReplay := exchangeTokenRaw(fixture.provider, url.Values{
		"grant_type":    {"refresh_token"},
		"refresh_token": {rotatedAgain.RefreshToken},
	})
	assertOAuthError(t, familyAfterReplay, http.StatusBadRequest, "invalid_grant")
	inactiveToken := introspectToken(t, fixture.provider, tokens.AccessToken)
	if inactiveToken.Active {
		t.Fatalf("replayed refresh family left access token active: %+v", inactiveToken)
	}
}

func TestValidateAuthorizationRequest(t *testing.T) {
	valid := func() *oidcprotocol.AuthRequest {
		return &oidcprotocol.AuthRequest{
			Scopes:              []string{"openid", "email"},
			ResponseType:        oidcprotocol.ResponseTypeCode,
			State:               "state",
			Nonce:               "nonce",
			CodeChallenge:       "challenge",
			CodeChallengeMethod: oidcprotocol.CodeChallengeMethodS256,
		}
	}
	if err := validateAuthorizationRequest(valid()); err != nil {
		t.Fatalf("valid request rejected: %v", err)
	}
	cases := map[string]func(*oidcprotocol.AuthRequest){
		"implicit response": func(request *oidcprotocol.AuthRequest) { request.ResponseType = oidcprotocol.ResponseTypeIDToken },
		"fragment response": func(request *oidcprotocol.AuthRequest) { request.ResponseMode = oidcprotocol.ResponseModeFragment },
		"missing state":     func(request *oidcprotocol.AuthRequest) { request.State = "" },
		"missing nonce":     func(request *oidcprotocol.AuthRequest) { request.Nonce = "" },
		"plain PKCE": func(request *oidcprotocol.AuthRequest) {
			request.CodeChallengeMethod = oidcprotocol.CodeChallengeMethodPlain
		},
		"request object": func(request *oidcprotocol.AuthRequest) { request.RequestParam = "jwt" },
		"missing openid": func(request *oidcprotocol.AuthRequest) { request.Scopes = []string{"email"} },
		"unknown scope":  func(request *oidcprotocol.AuthRequest) { request.Scopes = append(request.Scopes, "admin") },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			request := valid()
			mutate(request)
			if err := validateAuthorizationRequest(request); err == nil {
				t.Fatal("invalid authorization request was accepted")
			}
		})
	}
}

func newProviderFixture(t *testing.T) *providerFixture {
	t.Helper()
	ctx := context.Background()
	dsn := os.Getenv("IAM_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("IAM_TEST_POSTGRES_DSN is required for protocol integration tests")
	}
	configPG, err := pgx.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	adminDB := stdlib.OpenDB(*configPG)
	t.Cleanup(func() {
		if err := adminDB.Close(); err != nil {
			t.Error(err)
		}
	})
	schema := "oidc_" + uuid.NewString()[:8]
	if _, err := adminDB.ExecContext(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, err := adminDB.ExecContext(context.Background(), "DROP SCHEMA "+schema+" CASCADE")
		if err != nil {
			t.Error(err)
		}
	})
	configPG.RuntimeParams["search_path"] = schema
	database := stdlib.OpenDB(*configPG)
	t.Cleanup(func() {
		if err := database.Close(); err != nil {
			t.Error(err)
		}
	})
	driver := entsql.OpenDB(dialect.Postgres, database)
	client, cleanupDB, err := data.NewDBClient(driver)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(cleanupDB)

	miniRedis := miniredis.RunT(t)
	redisClient := goredis.NewClient(&goredis.Options{Addr: miniRedis.Addr()})
	t.Cleanup(func() { _ = redisClient.Close() })
	dataStore, err := data.NewData(client, redisClient, &fgaclient.OpenFgaClient{}, slog.Default())
	if err != nil {
		t.Fatalf("create data store: %v", err)
	}
	userRepo, err := data.NewUserRepository(dataStore)
	if err != nil {
		t.Fatalf("create User repository: %v", err)
	}
	sessionRepo, err := data.NewSessionRepository(dataStore)
	if err != nil {
		t.Fatalf("create Session repository: %v", err)
	}
	tokenSessionRepo, err := data.NewOAuthRepository(dataStore)
	if err != nil {
		t.Fatalf("create Token Session repository: %v", err)
	}
	sessionUsecase, err := biz.NewSessionUsecase(userRepo, sessionRepo)
	if err != nil {
		t.Fatalf("create Session usecase: %v", err)
	}

	userID, err := biz.NewID()
	if err != nil {
		t.Fatalf("generate test User ID: %v", err)
	}
	now := time.Now().UTC()
	if _, err := client.User.Create().
		SetID(userID).
		SetStatus(biz.UserStatusActive).
		SetName("Person").
		SetEtag("test-etag").
		Save(ctx); err != nil {
		t.Fatalf("seed active User: %v", err)
	}
	identifierID, err := biz.NewID()
	if err != nil {
		t.Fatalf("generate Login Identifier ID: %v", err)
	}
	if _, err := client.LoginIdentifier.Create().
		SetID(identifierID).
		SetUserID(userID).
		SetType(biz.LoginIdentifierEmail).
		SetCanonicalValue("person@example.com").
		SetDisplayValue("person@example.com").
		SetVerifiedTime(now).
		Save(ctx); err != nil {
		t.Fatalf("seed Login Identifier: %v", err)
	}
	authID, err := biz.NewID()
	if err != nil {
		t.Fatalf("generate Authenticator ID: %v", err)
	}
	if _, err := client.Authenticator.Create().
		SetID(authID).
		SetUserID(userID).
		SetType(biz.AuthenticatorPassword).
		SetState(biz.AuthenticatorActive).
		Save(ctx); err != nil {
		t.Fatalf("seed Authenticator: %v", err)
	}
	passAuthID, err := biz.NewID()
	if err != nil {
		t.Fatalf("generate Password Authenticator ID: %v", err)
	}
	if _, err := client.PasswordAuthenticator.Create().
		SetID(passAuthID).
		SetAuthenticatorID(authID).
		SetPasswordHash("password-hash").
		SetChangedTime(now).
		Save(ctx); err != nil {
		t.Fatalf("seed Password Authenticator: %v", err)
	}
	loginID, err := biz.NewID()
	if err != nil {
		t.Fatalf("generate LoginSession ID: %v", err)
	}
	login, err := client.LoginSession.Create().
		SetID(loginID).
		SetUserID(userID).
		SetAmr([]string{"hwk", "mfa", "user"}).
		SetCreateTime(now).
		Save(ctx)
	if err != nil {
		t.Fatalf("seed LoginSession: %v", err)
	}
	manager, cleanupSession, err := data.NewHTTPSessionManager(&sessionconfig.Session{Cookie: &sessionconfig.Cookie{Name: proto.String("__Host-iam_session")}}, database, client)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(cleanupSession)
	browserCtx, err := manager.Load(ctx, "")
	if err != nil {
		t.Fatal(err)
	}
	manager.Put(browserCtx, iamauthn.LoginReferenceKey, login.ID)
	sessionSecret, _, err := manager.Commit(browserCtx)
	if err != nil {
		t.Fatal(err)
	}

	config := testOIDCConfig(t)
	storage, err := NewOIDCStorage(client, config, tokenSessionRepo)
	if err != nil {
		t.Fatalf("create OIDC storage: %v", err)
	}
	bootstrap, err := NewOIDCInitializer(config, storage)
	if err != nil {
		t.Fatalf("create OIDC initializer: %v", err)
	}
	if err := bootstrap.Initialize(ctx); err != nil {
		t.Fatalf("run OIDC initializer: %v", err)
	}
	provider, err := NewIAMProvider(config, storage, sessionUsecase, manager)
	if err != nil {
		t.Fatalf("create OIDC provider: %v", err)
	}
	return &providerFixture{
		provider: provider, storage: storage, bootstrap: bootstrap, client: client, config: config,
		sessions: sessionUsecase, manager: manager, sessionSecret: sessionSecret, userID: userID,
	}
}
func (fixture *providerFixture) restartProvider(t *testing.T) {
	t.Helper()
	provider, err := NewIAMProvider(fixture.config, fixture.storage, fixture.sessions, fixture.manager)
	if err != nil {
		t.Fatalf("restart OIDC provider: %v", err)
	}
	fixture.provider = provider
}

func testOIDCConfig(t *testing.T) *oidcconfv1.OIDC {
	t.Helper()
	directory := t.TempDir()
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate RSA key: %v", err)
	}
	signingPath := filepath.Join(directory, "signing.pem")
	pemData := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(privateKey)})
	if err := os.WriteFile(signingPath, pemData, 0o600); err != nil {
		t.Fatalf("write signing key: %v", err)
	}
	cryptoKey := make([]byte, 32)
	if _, err := rand.Read(cryptoKey); err != nil {
		t.Fatalf("generate crypto key: %v", err)
	}
	cryptoPath := filepath.Join(directory, "crypto.key")
	if err := os.WriteFile(cryptoPath, cryptoKey, 0o600); err != nil {
		t.Fatalf("write crypto key: %v", err)
	}
	return &oidcconfv1.OIDC{
		Issuer:         proto.String(testIssuer),
		SigningKeyPath: proto.String(signingPath),
		CryptoKeyPath:  proto.String(cryptoPath),
		Clients: []*oidcconfv1.OAuthClient{{
			ClientId:          proto.String(testClientID),
			ClientSecret:      proto.String(testClientSecret),
			RedirectUris:      []string{testRedirectURI},
			AllowedScopes:     append([]string(nil), supportedScopes...),
			Trusted:           true,
			AllowedGrantTypes: []string{"authorization_code", "refresh_token"},
		}},
	}
}

type tokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	IDToken      string `json:"id_token"`
	Scope        string `json:"scope"`
}

type introspectionResponse struct {
	Active   bool   `json:"active"`
	Subject  string `json:"sub"`
	ClientID string `json:"client_id"`
}

func introspectToken(t *testing.T, provider *IAMProvider, token string) introspectionResponse {
	t.Helper()
	values := url.Values{"token": {token}, "token_type_hint": {"access_token"}}
	request := httptest.NewRequest(http.MethodPost, testIssuer+"/oauth/introspect", strings.NewReader(values.Encode()))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.SetBasicAuth(testClientID, testClientSecret)
	response := httptest.NewRecorder()
	provider.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("introspection status = %d, body = %s", response.Code, response.Body.String())
	}
	var result introspectionResponse
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatalf("decode introspection: %v", err)
	}
	return result
}

func exchangeCode(t *testing.T, provider *IAMProvider, code, verifier string) tokenResponse {
	t.Helper()
	response := exchangeTokenRaw(provider, url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"redirect_uri":  {testRedirectURI},
		"code_verifier": {verifier},
	})
	return decodeTokenResponse(t, response)
}

func exchangeRefresh(t *testing.T, provider *IAMProvider, refreshToken string) tokenResponse {
	t.Helper()
	response := exchangeTokenRaw(provider, url.Values{
		"grant_type":    {"refresh_token"},
		"refresh_token": {refreshToken},
	})
	return decodeTokenResponse(t, response)
}

func exchangeRefreshScope(t *testing.T, provider *IAMProvider, refreshToken, scope string) tokenResponse {
	t.Helper()
	response := exchangeTokenRaw(provider, url.Values{
		"grant_type":    {"refresh_token"},
		"refresh_token": {refreshToken},
		"scope":         {scope},
	})
	return decodeTokenResponse(t, response)
}

func exchangeTokenRaw(provider *IAMProvider, values url.Values) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodPost, testIssuer+"/oauth/token", strings.NewReader(values.Encode()))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.SetBasicAuth(testClientID, testClientSecret)
	response := httptest.NewRecorder()
	provider.ServeHTTP(response, request)
	return response
}

func decodeTokenResponse(t *testing.T, response *httptest.ResponseRecorder) tokenResponse {
	t.Helper()
	if response.Code != http.StatusOK {
		t.Fatalf("token endpoint status = %d, body = %s", response.Code, response.Body.String())
	}
	var tokens tokenResponse
	if err := json.Unmarshal(response.Body.Bytes(), &tokens); err != nil {
		t.Fatalf("decode token response: %v", err)
	}
	if tokens.AccessToken == "" || tokens.IDToken == "" {
		t.Fatalf("incomplete token response: %+v", tokens)
	}
	return tokens
}

func assertOAuthError(t *testing.T, response *httptest.ResponseRecorder, status int, errorType string) {
	t.Helper()
	if response.Code != status {
		t.Fatalf("OAuth error status = %d, body = %s; want %d", response.Code, response.Body.String(), status)
	}
	var payload struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode OAuth error: %v", err)
	}
	if payload.Error != errorType {
		t.Fatalf("OAuth error = %q, body = %s; want %q", payload.Error, response.Body.String(), errorType)
	}
}

func performRequest(handler http.Handler, method, target, body string, headers map[string]string) *httptest.ResponseRecorder {
	var reader io.Reader
	if body != "" {
		reader = strings.NewReader(body)
	}
	request := httptest.NewRequest(method, target, reader)
	for key, value := range headers {
		request.Header.Set(key, value)
	}
	response := httptest.NewRecorder()
	if provider, ok := handler.(*IAMProvider); ok {
		switch request.URL.Path {
		case "/authorize", "/authorize/callback", "/end_session":
			handler = httpsession.LoadAndSave(provider.manager)(handler)
		}
	}
	handler.ServeHTTP(response, request)
	return response
}

func assertRS256JWT(t *testing.T, token string) {
	t.Helper()
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		t.Fatalf("token is not a compact JWT: %q", token)
	}
	headerBytes, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		t.Fatalf("decode JWT header: %v", err)
	}
	var header struct {
		Algorithm string `json:"alg"`
		KeyID     string `json:"kid"`
	}
	if err := json.Unmarshal(headerBytes, &header); err != nil {
		t.Fatalf("decode JWT header JSON: %v", err)
	}
	if header.Algorithm != "RS256" || header.KeyID == "" {
		t.Fatalf("JWT header = %+v", header)
	}
}

func cloneValues(values url.Values) url.Values {
	clone := make(url.Values, len(values))
	for key, entries := range values {
		clone[key] = append([]string(nil), entries...)
	}
	return clone
}

func contains(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}

func sameStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for _, value := range left {
		if !contains(right, value) {
			return false
		}
	}
	return true
}

func TestLoopbackHostIssuerAndRedirectURI(t *testing.T) {
	allowedIssuers := []string{
		"http://localhost:10002",
		"http://LOCALHOST:8080",
		"http://127.0.0.1:10000",
		"http://[::1]:10000",
		"http://iam.plateau.localhost:8080",
		"http://IAM.Plateau.LocalHost:8080/",
		"https://iam.example.com",
	}
	for _, raw := range allowedIssuers {
		if _, _, err := biz.NormalizeOIDCIssuer(raw); err != nil {
			t.Errorf("NormalizeOIDCIssuer(%q) unexpected error: %v", raw, err)
		}
	}

	rejectedIssuers := []string{
		"http://.localhost:8080",
		"http://..localhost:8080",
		"http://foo..localhost:8080",
		"http://.iam.plateau.localhost:8080",
		"http://evil-localhost.com",
		"http://iam.plateau.localhost.evil.com",
		"http://localhost.:8080",
	}
	for _, raw := range rejectedIssuers {
		if _, _, err := biz.NormalizeOIDCIssuer(raw); err == nil {
			t.Errorf("NormalizeOIDCIssuer(%q) succeeded; expected HTTPS requirement error", raw)
		}
	}

	allowedRedirects := []string{
		"http://localhost:10012/v1/admin/auth/callback",
		"http://admin.plateau.localhost:8080/v1/admin/auth/callback",
		"https://admin.example.com/v1/admin/auth/callback",
	}
	for _, raw := range allowedRedirects {
		if err := biz.ValidateClientRedirectURI(biz.AppTypeWeb, raw); err != nil {
			t.Errorf("ValidateClientRedirectURI(%q) unexpected error: %v", raw, err)
		}
	}

	rejectedRedirects := []string{
		"http://.localhost:8080/callback",
		"http://foo..localhost:8080/callback",
		"http://evil-localhost.com/callback",
		"http://admin.plateau.localhost.evil.com/callback",
	}
	for _, raw := range rejectedRedirects {
		if err := biz.ValidateClientRedirectURI(biz.AppTypeWeb, raw); err == nil {
			t.Errorf("ValidateClientRedirectURI(%q) succeeded; expected HTTPS requirement error", raw)
		}
	}
}
