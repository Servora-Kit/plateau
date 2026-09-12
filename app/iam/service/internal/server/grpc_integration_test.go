package server

import (
	"context"
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	userpb "github.com/Servora-Kit/plateau/api/gen/go/iam/user/v1"
	jwtpb "github.com/Servora-Kit/plateau/api/gen/go/plateau/security/authn/jwt/v1"
	iamauthn "github.com/Servora-Kit/plateau/app/iam/service/internal/authn"
	jwtsecurity "github.com/Servora-Kit/plateau/security/authn/jwt"
	jwtlib "github.com/golang-jwt/jwt/v5"
	fgaclient "github.com/openfga/go-sdk/client"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/clientcredentials"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	grpcoauth "google.golang.org/grpc/credentials/oauth"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

func TestServiceOAuthHTTPAndGRPC(t *testing.T) {
	f := newServerFixture(t)
	ctx := t.Context()
	_, preflightErr := f.fga.Check(ctx).Body(fgaclient.ClientCheckRequest{User: "service:admin", Relation: "manage_users", Object: "iam:global"}).Execute()
	check(t, preflightErr)
	// One application-lifetime source is shared by HTTP and gRPC.
	tokenCtx := context.WithValue(ctx, oauth2.HTTPClient, &http.Client{Timeout: 5 * time.Second})
	config := clientcredentials.Config{ClientID: "admin", ClientSecret: serviceSecret, TokenURL: f.web.URL + "/oauth/token", AuthStyle: oauth2.AuthStyleInHeader}
	source := config.TokenSource(tokenCtx)
	token, err := source.Token()
	check(t, err)
	if f.ent.IAMLoginSession.Query().CountX(ctx) != 0 || f.ent.OAuthTokenSession.Query().CountX(ctx) != 0 {
		t.Fatal("service token created a user login")
	}

	// A business receiver obtains its public keys from IAM Discovery/JWKS.
	verifier, closeVerifier, err := jwtsecurity.New(ctx, &jwtpb.JwtAuthnConfig{Issuer: f.config.Issuer, Audience: "iam"})
	check(t, err)
	t.Cleanup(closeVerifier)
	var httpCalls atomic.Int32
	receiver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		httpCalls.Add(1)
		actor, err := jwtsecurity.Authenticate(r.Context(), verifier, r.Header.Get("Authorization"), iamauthn.NewServiceClaims, iamauthn.ServiceActor)
		if err != nil || actor.ID != "admin" {
			w.WriteHeader(401)
			return
		}
		if r.URL.Path == "/denied" {
			w.WriteHeader(403)
			return
		}
		w.WriteHeader(204)
	}))
	t.Cleanup(receiver.Close)
	httpClient := oauth2.NewClient(tokenCtx, source)
	for path, want := range map[string]int{"/ok": 204, "/denied": 403} {
		before := httpCalls.Load()
		response, err := httpClient.Get(receiver.URL + path)
		check(t, err)
		_ = response.Body.Close()
		if response.StatusCode != want || httpCalls.Load() != before+1 {
			t.Fatalf("HTTP status/replay: %d calls=%d", response.StatusCode, httpCalls.Load()-before)
		}
	}

	var rpcCalls atomic.Int32
	endpoint := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { rpcCalls.Add(1); f.grpc.ServeHTTP(w, r) }))
	endpoint.EnableHTTP2 = true
	endpoint.StartTLS()
	t.Cleanup(endpoint.Close)
	roots := endpoint.Client().Transport.(*http.Transport).TLSClientConfig.RootCAs
	dial := func(source oauth2.TokenSource) *grpc.ClientConn {
		conn, err := grpc.NewClient(strings.TrimPrefix(endpoint.URL, "https://"), grpc.WithTransportCredentials(credentials.NewTLS(&tls.Config{RootCAs: roots, MinVersion: tls.VersionTLS12})), grpc.WithPerRPCCredentials(grpcoauth.TokenSource{TokenSource: source}), grpc.WithDisableRetry())
		check(t, err)
		t.Cleanup(func() { _ = conn.Close() })
		return conn
	}
	rpc := userpb.NewUserServiceClient(dial(source))
	services := f.grpc.GetServiceInfo()
	if _, ok := services[userpb.UserService_ServiceDesc.ServiceName]; !ok {
		t.Fatal("UserService is not registered on gRPC")
	}
	for name := range services {
		if strings.HasPrefix(name, "iam.") && name != userpb.UserService_ServiceDesc.ServiceName {
			t.Fatalf("browser service registered on gRPC: %s", name)
		}
	}
	request := &userpb.GetUserRequest{Name: f.user.GetName()}
	call := func(want codes.Code) {
		t.Helper()
		before := rpcCalls.Load()
		_, err := rpc.GetUser(ctx, request)
		if status.Code(err) != want || rpcCalls.Load() != before+1 {
			t.Fatalf("gRPC code=%s want=%s calls=%d error=%v", status.Code(err), want, rpcCalls.Load()-before, err)
		}
	}
	call(codes.PermissionDenied)
	tuple := fgaclient.ClientTupleKey{User: "service:admin", Relation: "manage_users", Object: "iam:global"}
	_, err = f.fga.WriteTuples(ctx).Body([]fgaclient.ClientTupleKey{tuple}).Execute()
	check(t, err)
	t.Cleanup(func() {
		_, _ = f.fga.DeleteTuples(context.Background()).Body([]fgaclient.ClientTupleKeyWithoutCondition{{User: tuple.User, Relation: tuple.Relation, Object: tuple.Object}}).Execute()
	})
	call(codes.OK)
	_, err = f.fga.DeleteTuples(ctx).Body([]fgaclient.ClientTupleKeyWithoutCondition{{User: tuple.User, Relation: tuple.Relation, Object: tuple.Object}}).Execute()
	check(t, err)
	call(codes.PermissionDenied)
	if f.ent.OAuthAccessToken.Query().CountX(ctx) != 1 {
		t.Fatal("valid cached token was reacquired after denied RPC")
	}
	if f.ent.HTTPSession.Query().CountX(ctx) != 0 {
		t.Fatal("machine HTTP/gRPC touched browser sessions")
	}

	badRPC := userpb.NewUserServiceClient(dial(oauth2.StaticTokenSource(&oauth2.Token{AccessToken: "invalid", TokenType: "Bearer"})))
	before := rpcCalls.Load()
	_, err = badRPC.GetUser(metadata.AppendToOutgoingContext(ctx, "cookie", "__Host-iam_session=fake", "x-user", f.user.GetUserId(), "x-actor", "service:admin"), request)
	if status.Code(err) != codes.Unauthenticated || rpcCalls.Load() != before+1 {
		t.Fatalf("forged metadata/replay: %v", err)
	}
	key, err := f.oidc.SigningKey(ctx)
	check(t, err)
	for _, use := range []string{"access", "id"} {
		t.Run("reject_user_"+use+"_token", func(t *testing.T) {
			claims := jwtlib.MapClaims{
				"iss": f.config.Issuer, "sub": f.user.GetUserId(), "client_id": "admin", "aud": "iam",
				"iat": time.Now().Unix(), "exp": time.Now().Add(5 * time.Minute).Unix(), "jti": "user-token",
				"token_use": use, "actor_type": "human",
			}
			if use == "id" {
				delete(claims, "token_use")
				delete(claims, "actor_type")
			}
			signed := jwtlib.NewWithClaims(jwtlib.SigningMethodRS256, claims)
			signed.Header["kid"] = key.ID()
			value, err := signed.SignedString(key.Key())
			check(t, err)
			client := userpb.NewUserServiceClient(dial(oauth2.StaticTokenSource(&oauth2.Token{AccessToken: value, TokenType: "Bearer"})))
			before := rpcCalls.Load()
			_, err = client.GetUser(ctx, request)
			if status.Code(err) != codes.Unauthenticated || rpcCalls.Load() != before+1 {
				t.Fatalf("signed user token accepted or replayed: %v", err)
			}
		})
	}

	// Registration changes only affect future issuance. Old tokens retain identity validity.
	f.config.Clients[0].Audiences = []string{"different-service"}
	check(t, f.initializer.Initialize(ctx))
	wrongSource := config.TokenSource(tokenCtx)
	wrongRPC := userpb.NewUserServiceClient(dial(wrongSource))
	_, err = wrongRPC.GetUser(ctx, request)
	if status.Code(err) != codes.Unauthenticated {
		t.Fatalf("wrong audience accepted: %v", err)
	}
	check(t, f.ent.OAuthClient.DeleteOneID("admin").Exec(ctx))
	if _, err := config.TokenSource(tokenCtx).Token(); err == nil {
		t.Fatal("removed client acquired token")
	}
	_, err = jwtsecurity.Authenticate(ctx, verifier, "Bearer "+token.AccessToken, iamauthn.NewServiceClaims, iamauthn.ServiceActor)
	check(t, err)
	call(codes.PermissionDenied)
}
