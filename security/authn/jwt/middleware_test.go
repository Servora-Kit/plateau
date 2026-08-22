package jwt

import (
	"context"
	"testing"

	authnpb "github.com/Servora-Kit/plateau/api/gen/go/plateau/security/authn/v1"
	securityerrorspb "github.com/Servora-Kit/plateau/api/gen/go/plateau/security/errors/v1"
	security "github.com/Servora-Kit/plateau/security"
	authnruntime "github.com/Servora-Kit/plateau/security/authn"
	"github.com/go-kratos/kratos/v3/middleware"
	"github.com/go-kratos/kratos/v3/transport"
)

const middlewareOperation = "/test.v1.AuthService/Get"

type middlewareTransport struct {
	operation string
	header    middlewareHeader
}

func (*middlewareTransport) Kind() transport.Kind                  { return transport.KindHTTP }
func (*middlewareTransport) Endpoint() string                      { return "" }
func (value *middlewareTransport) Operation() string               { return value.operation }
func (value *middlewareTransport) RequestHeader() transport.Header { return value.header }
func (*middlewareTransport) ReplyHeader() transport.Header         { return middlewareHeader{} }

type middlewareHeader map[string][]string

func (header middlewareHeader) Get(key string) string {
	values := header[key]
	if len(values) == 0 {
		return ""
	}
	return values[0]
}
func (header middlewareHeader) Set(key, value string) { header[key] = []string{value} }
func (header middlewareHeader) Add(key, value string) { header[key] = append(header[key], value) }
func (header middlewareHeader) Keys() []string {
	keys := make([]string, 0, len(header))
	for key := range header {
		keys = append(keys, key)
	}
	return keys
}
func (header middlewareHeader) Values(key string) []string { return header[key] }

func middlewareContext(authorization string) context.Context {
	header := middlewareHeader{}
	if authorization != "" {
		header.Set("Authorization", authorization)
	}
	return transport.NewServerContext(context.Background(), &middlewareTransport{operation: middlewareOperation, header: header})
}

func middlewareRules(mode authnpb.AuthnMode) authnruntime.Option {
	return authnruntime.WithRulesFuncs(func() map[string]*authnpb.AuthnRule {
		return map[string]*authnpb.AuthnRule{middlewareOperation: {Mode: mode}}
	})
}

func invokeMiddleware(t *testing.T, value middleware.Middleware, ctx context.Context) (security.Actor, error) {
	t.Helper()
	result, err := value(func(ctx context.Context, _ any) (any, error) {
		actor, _ := security.ActorFrom(ctx)
		return actor, nil
	})(ctx, nil)
	if err != nil {
		return security.Actor{}, err
	}
	return result.(security.Actor), nil
}

func TestServerPublicRouteWritesAnonymousActor(t *testing.T) {
	_, _, authenticator := newAuthenticator(t)
	actor, err := invokeMiddleware(t, Server(authenticator, newTestClaims, mapTestActor, middlewareRules(authnpb.AuthnMode_AUTHN_MODE_PUBLIC)), middlewareContext(""))
	if err != nil || actor != (security.Actor{Type: security.ActorTypeAnonymous}) {
		t.Fatalf("actor=%+v error=%v", actor, err)
	}
}

func TestServerRequiredRouteAuthenticatesAndWritesActor(t *testing.T) {
	signer, _, authenticator := newAuthenticator(t)
	claims := validTestClaims("human", "user-1")
	actor, err := invokeMiddleware(t, Server(authenticator, newTestClaims, mapTestActor, middlewareRules(authnpb.AuthnMode_AUTHN_MODE_REQUIRED)), middlewareContext("Bearer "+mustToken(t, signer, claims)))
	if err != nil || actor != (security.Actor{Type: security.ActorTypeHuman, ID: "user-1"}) {
		t.Fatalf("actor=%+v error=%v", actor, err)
	}
}

func TestServerRequiredRouteFailsClosed(t *testing.T) {
	_, _, authenticator := newAuthenticator(t)
	server := Server(authenticator, newTestClaims, mapTestActor, middlewareRules(authnpb.AuthnMode_AUTHN_MODE_REQUIRED))
	if _, err := invokeMiddleware(t, server, middlewareContext("")); !securityerrorspb.IsSecurityErrorReasonUnauthenticated(err) {
		t.Fatalf("missing credentials error=%v", err)
	}
	if _, err := invokeMiddleware(t, server, middlewareContext("Bearer malformed")); !securityerrorspb.IsSecurityErrorReasonUnauthenticated(err) {
		t.Fatalf("invalid token error=%v", err)
	}
	if _, err := invokeMiddleware(t, Server(authenticator, newTestClaims, mapTestActor), middlewareContext("")); !securityerrorspb.IsSecurityErrorReasonInternal(err) {
		t.Fatalf("missing rule error=%v", err)
	}
}
