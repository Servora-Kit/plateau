package oidc

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"testing"
	"time"

	oidcpb "github.com/Servora-Kit/plateau/api/gen/go/iam/oidc/conf/v1"
	iamauthn "github.com/Servora-Kit/plateau/app/iam/service/internal/authn"
	authnjwt "github.com/Servora-Kit/plateau/security/authn/jwt"
	jwtlib "github.com/golang-jwt/jwt/v5"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/durationpb"
)

func TestServiceTokenProtocolAndClientReload(t *testing.T) {
	fixture := newProviderFixture(t)
	const secret = "service-secret-with-at-least-32-bytes"
	machine := &oidcpb.OAuthClient{ClientId: proto.String("admin"), ClientSecret: proto.String(secret),
		AllowedGrantTypes: []string{"client_credentials"}, Audiences: []string{"iam", "service-b"}}
	fixture.config.Clients = append(fixture.config.Clients, machine)
	if err := fixture.bootstrap.Initialize(t.Context()); err != nil {
		t.Fatal(err)
	}
	requestToken := func(id, password string, values url.Values) *httptest.ResponseRecorder {
		request := httptest.NewRequest(http.MethodPost, testIssuer+"/oauth/token", strings.NewReader(values.Encode()))
		request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		request.SetBasicAuth(id, password)
		response := httptest.NewRecorder()
		fixture.provider.ServeHTTP(response, request)
		return response
	}
	values := url.Values{"grant_type": {"client_credentials"}}
	response := requestToken("admin", secret, values)
	if response.Code != http.StatusOK {
		t.Fatalf("machine token: %d %s", response.Code, response.Body.String())
	}
	var payload struct {
		AccessToken  string `json:"access_token"`
		TokenType    string `json:"token_type"`
		ExpiresIn    int    `json:"expires_in"`
		RefreshToken string `json:"refresh_token"`
		IDToken      string `json:"id_token"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.AccessToken == "" || payload.RefreshToken != "" || payload.IDToken != "" || payload.TokenType != "Bearer" || payload.ExpiresIn > 300 || payload.ExpiresIn < 298 {
		t.Fatal("unexpected machine token response")
	}
	verifier, err := NewJWTVerifier(fixture.storage)
	if err != nil {
		t.Fatal(err)
	}
	authenticator, cleanup, err := iamauthn.NewServiceAuthenticator(fixture.config, verifier)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(cleanup)
	claims := new(iamauthn.ServiceClaims)
	parsed, err := verifier.VerifySignature(t.Context(), payload.AccessToken, claims)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Header["kid"] != fixture.storage.signingPrivate.id || parsed.Header["typ"] != "JWT" || claims.ClientID != "admin" || claims.Subject != "admin" || !slices.Equal([]string(claims.Audience), []string{"iam", "service-b"}) {
		t.Fatalf("machine claims=%+v header=%v", claims, parsed.Header)
	}
	if claims.ExpiresAt.Sub(claims.IssuedAt.Time) != 5*time.Minute {
		t.Fatal("wrong machine TTL")
	}
	actor, err := authnjwt.Authenticate(t.Context(), authenticator, "Bearer "+payload.AccessToken, iamauthn.NewServiceClaims, iamauthn.ServiceActor)
	if err != nil || actor.ID != "admin" {
		t.Fatalf("service actor=%v error=%v", actor, err)
	}
	stored := fixture.client.OAuthAccessToken.GetX(t.Context(), claims.ID)
	if stored.TokenSessionID != "" || string(stored.ActorType) != "service" {
		t.Fatal("machine token attached to a user session")
	}
	if count := fixture.client.OAuthTokenSession.Query().CountX(t.Context()); count != 0 {
		t.Fatal("machine grant created a user OAuth session")
	}
	assertOAuthError(t, requestToken("admin", "wrong", values), http.StatusUnauthorized, "invalid_client")
	assertOAuthError(t, requestToken(testClientID, testClientSecret, values), http.StatusBadRequest, "unauthorized_client")
	values.Set("resource", "attacker")
	values.Set("audience", "attacker")
	next := requestToken("admin", secret, values)
	if next.Code == http.StatusOK {
		var body struct {
			AccessToken string `json:"access_token"`
		}
		if err := json.Unmarshal(next.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		mapped := new(iamauthn.ServiceClaims)
		if _, err := verifier.VerifySignature(t.Context(), body.AccessToken, mapped); err != nil {
			t.Fatal(err)
		}
		if !slices.Equal([]string(mapped.Audience), []string{"iam", "service-b"}) {
			t.Fatal("caller expanded token audience")
		}
	}
	machine.ClientSecret = proto.String("rotated-service-secret-with-at-least-32-bytes")
	if err := fixture.bootstrap.Initialize(t.Context()); err != nil {
		t.Fatal(err)
	}
	assertOAuthError(t, requestToken("admin", secret, url.Values{"grant_type": {"client_credentials"}}), http.StatusUnauthorized, "invalid_client")
	if got := requestToken("admin", machine.GetClientSecret(), url.Values{"grant_type": {"client_credentials"}}); got.Code != 200 {
		t.Fatalf("rotated secret rejected: %s", got.Body.String())
	}
	fixture.config.Clients = fixture.config.Clients[:1]
	if err := fixture.bootstrap.Initialize(t.Context()); err != nil {
		t.Fatal(err)
	}
	assertOAuthError(t, requestToken("admin", machine.GetClientSecret(), url.Values{"grant_type": {"client_credentials"}}), http.StatusUnauthorized, "invalid_client")
	if _, err := authnjwt.Authenticate(t.Context(), authenticator, "Bearer "+payload.AccessToken, iamauthn.NewServiceClaims, iamauthn.ServiceActor); err != nil {
		t.Fatalf("disabled client prematurely invalidated issued token: %v", err)
	}
	future := jwtlib.NewValidator(jwtlib.WithTimeFunc(func() time.Time { return claims.ExpiresAt.Add(time.Second) }), jwtlib.WithExpirationRequired())
	if err := future.Validate(claims); err == nil {
		t.Fatal("expired service token accepted")
	}
}

func TestServiceConfigValidation(t *testing.T) {
	valid := &oidcpb.OAuthClient{ClientId: proto.String("service-a"), ClientSecret: proto.String("service-secret-with-at-least-32-bytes"), AllowedGrantTypes: []string{"client_credentials"}, Audiences: []string{"iam"}}
	if _, _, _, err := validateConfiguredClient(valid, map[string]struct{}{}); err != nil {
		t.Fatal(err)
	}
	for _, mutate := range []func(*oidcpb.OAuthClient){
		func(c *oidcpb.OAuthClient) { c.AllowedGrantTypes = nil },
		func(c *oidcpb.OAuthClient) { c.AllowedGrantTypes = []string{"password"} },
		func(c *oidcpb.OAuthClient) { c.Audiences = nil },
		func(c *oidcpb.OAuthClient) { c.Audiences = []string{" "} },
		func(c *oidcpb.OAuthClient) { c.AllowedGrantTypes = []string{"refresh_token"} },
	} {
		c := proto.Clone(valid).(*oidcpb.OAuthClient)
		mutate(c)
		if _, _, _, err := validateConfiguredClient(c, map[string]struct{}{}); err == nil {
			t.Fatal("invalid machine configuration accepted")
		}
	}
	f := newProviderFixture(t)
	for _, ttl := range []time.Duration{0, -time.Second, 90 * time.Second} {
		config := proto.Clone(f.config).(*oidcpb.OIDC)
		config.ServiceAccessTokenTtl = durationpb.New(ttl)
		storage, err := NewOIDCStorage(f.client, config, f.storage.tokens)
		if ttl <= 0 && err == nil {
			t.Fatal("nonpositive token lifetime accepted")
		}
		if ttl > 0 && (err != nil || storage.serviceAccessTokenTTL != ttl) {
			t.Fatalf("explicit lifetime failed: %v", err)
		}
	}
}

func TestServiceClaimsAndOverlappingSigningKeys(t *testing.T) {
	f := newProviderFixture(t)
	verifier, err := NewJWTVerifier(f.storage)
	if err != nil {
		t.Fatal(err)
	}
	authenticator, cleanup, err := iamauthn.NewServiceAuthenticator(f.config, verifier)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(cleanup)
	claims := func() jwtlib.MapClaims {
		return jwtlib.MapClaims{
			"iss": testIssuer, "sub": "admin", "client_id": "admin", "aud": "iam", "jti": "unique-token",
			"iat": time.Now().Unix(), "exp": time.Now().Add(5 * time.Minute).Unix(), "token_use": "access", "actor_type": "service",
		}
	}
	sign := func(key *signingKey, claims jwtlib.MapClaims) string {
		t.Helper()
		token := jwtlib.NewWithClaims(jwtlib.SigningMethodRS256, claims)
		token.Header["kid"] = key.id
		value, err := token.SignedString(key.key)
		if err != nil {
			t.Fatal(err)
		}
		return value
	}
	oldToken := sign(f.storage.signingPrivate, claims())
	for _, mutate := range []func(jwtlib.MapClaims){
		func(c jwtlib.MapClaims) { c["actor_type"] = "human"; c["sub"] = f.userID },
		func(c jwtlib.MapClaims) { delete(c, "actor_type"); delete(c, "token_use") },
		func(c jwtlib.MapClaims) { c["token_use"] = "id" },
		func(c jwtlib.MapClaims) { c["sub"] = "different-service" },
		func(c jwtlib.MapClaims) { delete(c, "iat") },
		func(c jwtlib.MapClaims) { delete(c, "jti") },
	} {
		c := claims()
		mutate(c)
		if _, err := authnjwt.Authenticate(t.Context(), authenticator, "Bearer "+sign(f.storage.signingPrivate, c), iamauthn.NewServiceClaims, iamauthn.ServiceActor); err == nil {
			t.Fatal("invalid service claims accepted")
		}
	}
	// A new mounted signing key creates a second public record; old tokens remain verifiable.
	config := testOIDCConfig(t)
	newStorage, err := NewOIDCStorage(f.client, config, f.storage.tokens)
	if err != nil {
		t.Fatal(err)
	}
	initializer, err := NewOIDCInitializer(config, newStorage)
	if err != nil {
		t.Fatal(err)
	}
	if err := initializer.Initialize(t.Context()); err != nil {
		t.Fatal(err)
	}
	newToken := sign(newStorage.signingPrivate, claims())
	keys, err := newStorage.KeySet(t.Context())
	if err != nil || len(keys) != 2 {
		t.Fatalf("overlap key count=%d error=%v", len(keys), err)
	}
	for _, token := range []string{oldToken, newToken} {
		if _, err := authnjwt.Authenticate(t.Context(), authenticator, "Bearer "+token, iamauthn.NewServiceClaims, iamauthn.ServiceActor); err != nil {
			t.Fatalf("overlapping key rejected: %v", err)
		}
	}
	if _, err := f.client.OIDCSigningKey.UpdateOneID(f.storage.signingPrivate.id).SetRevokedTime(time.Now()).Save(t.Context()); err != nil {
		t.Fatal(err)
	}
	if _, err := authnjwt.Authenticate(t.Context(), authenticator, "Bearer "+oldToken, iamauthn.NewServiceClaims, iamauthn.ServiceActor); err == nil {
		t.Fatal("retired key accepted")
	}
}
