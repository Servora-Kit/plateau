package session

import (
	"context"
	"errors"
	"testing"

	authnpb "github.com/Servora-Kit/plateau/api/gen/go/plateau/security/authn/v1"
	securityerrorspb "github.com/Servora-Kit/plateau/api/gen/go/plateau/security/errors/v1"
	security "github.com/Servora-Kit/plateau/security"
	authnruntime "github.com/Servora-Kit/plateau/security/authn"
	"github.com/go-kratos/kratos/v3/middleware"
	"github.com/go-kratos/kratos/v3/transport"
)

const sessionMiddlewareOperation = "/test.v1.SessionService/Get"

type sessionMiddlewareTransport struct {
	header sessionMiddlewareHeader
}

func (*sessionMiddlewareTransport) Kind() transport.Kind                  { return transport.KindHTTP }
func (*sessionMiddlewareTransport) Endpoint() string                      { return "" }
func (*sessionMiddlewareTransport) Operation() string                     { return sessionMiddlewareOperation }
func (value *sessionMiddlewareTransport) RequestHeader() transport.Header { return value.header }
func (*sessionMiddlewareTransport) ReplyHeader() transport.Header         { return sessionMiddlewareHeader{} }

type sessionMiddlewareHeader map[string][]string

func (header sessionMiddlewareHeader) Get(key string) string {
	values := header[key]
	if len(values) == 0 {
		return ""
	}
	return values[0]
}
func (header sessionMiddlewareHeader) Set(key, value string) { header[key] = []string{value} }
func (header sessionMiddlewareHeader) Add(key, value string) {
	header[key] = append(header[key], value)
}
func (header sessionMiddlewareHeader) Keys() []string {
	keys := make([]string, 0, len(header))
	for key := range header {
		keys = append(keys, key)
	}
	return keys
}
func (header sessionMiddlewareHeader) Values(key string) []string { return header[key] }

func TestServerAppliesPublicAndRequiredRules(t *testing.T) {
	resolveCalls := 0
	authenticator, err := New("__Host-session", func(_ context.Context, credential string) (testIdentity, error) {
		resolveCalls++
		if credential != "secret" {
			return testIdentity{}, ErrInvalidCredentials
		}
		return testIdentity{userID: "user-1"}, nil
	}, func(identity testIdentity) (security.Actor, error) {
		return security.Actor{Type: security.ActorTypeHuman, ID: identity.userID}, nil
	}, nil)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	publicActor, err := invokeSessionMiddleware(t, Server(authenticator, sessionRules(authnpb.AuthnMode_AUTHN_MODE_PUBLIC)), sessionMiddlewareContext(""))
	if err != nil || publicActor != (security.Actor{Type: security.ActorTypeAnonymous}) || resolveCalls != 0 {
		t.Fatalf("public Actor = %+v, calls = %d, error = %v", publicActor, resolveCalls, err)
	}
	requiredActor, err := invokeSessionMiddleware(t, Server(authenticator, sessionRules(authnpb.AuthnMode_AUTHN_MODE_REQUIRED)), sessionMiddlewareContext("__Host-session=secret"))
	if err != nil || requiredActor != (security.Actor{Type: security.ActorTypeHuman, ID: "user-1"}) || resolveCalls != 1 {
		t.Fatalf("required Actor = %+v, calls = %d, error = %v", requiredActor, resolveCalls, err)
	}
}

func TestServerFailsClosedAndMapsResolverAvailability(t *testing.T) {
	authenticator, err := New("__Host-session", func(context.Context, string) (testIdentity, error) {
		return testIdentity{}, ErrDependencyUnavailable
	}, func(identity testIdentity) (security.Actor, error) {
		return security.Actor{Type: security.ActorTypeHuman, ID: identity.userID}, nil
	}, nil)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	required := Server(authenticator, sessionRules(authnpb.AuthnMode_AUTHN_MODE_REQUIRED))
	if _, err := invokeSessionMiddleware(t, required, sessionMiddlewareContext("")); !securityerrorspb.IsSecurityErrorReasonUnauthenticated(err) {
		t.Fatalf("missing cookie error = %v", err)
	}
	if _, err := invokeSessionMiddleware(t, required, sessionMiddlewareContext("__Host-session=secret")); !securityerrorspb.IsSecurityErrorReasonUnavailable(err) {
		t.Fatalf("resolver unavailable error = %v", err)
	}
	if _, err := invokeSessionMiddleware(t, Server(authenticator), sessionMiddlewareContext("")); !securityerrorspb.IsSecurityErrorReasonInternal(err) {
		t.Fatalf("missing rule error = %v", err)
	}
	mapped := apiError(errors.New("unexpected result"))
	if !securityerrorspb.IsSecurityErrorReasonInternal(mapped) {
		t.Fatalf("internal error = %v", mapped)
	}
}

func sessionRules(mode authnpb.AuthnMode) authnruntime.Option {
	return authnruntime.WithRulesFuncs(func() map[string]*authnpb.AuthnRule {
		return map[string]*authnpb.AuthnRule{sessionMiddlewareOperation: {Mode: mode}}
	})
}

func sessionMiddlewareContext(cookie string) context.Context {
	header := sessionMiddlewareHeader{}
	if cookie != "" {
		header.Set("Cookie", cookie)
	}
	return transport.NewServerContext(context.Background(), &sessionMiddlewareTransport{header: header})
}

func invokeSessionMiddleware(t *testing.T, value middleware.Middleware, ctx context.Context) (security.Actor, error) {
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
