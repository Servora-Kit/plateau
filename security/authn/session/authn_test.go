package session

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	authnpb "github.com/Servora-Kit/plateau/api/gen/go/plateau/security/authn/v1"
	securitypb "github.com/Servora-Kit/plateau/api/gen/go/plateau/security/errors/v1"
	"github.com/Servora-Kit/plateau/security"
	authnruntime "github.com/Servora-Kit/plateau/security/authn"
	sessions "github.com/Servora-Kit/plateau/security/session"
	"github.com/alexedwards/scs/v2"
	"github.com/go-kratos/kratos/v3/transport"
)

type testIdentity struct{ userID string }
type localIdentityKey struct{}

func loadedContext(manager *scs.SessionManager) context.Context {
	var result context.Context
	sessions.LoadAndSave(manager)(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) { result = r.Context() })).ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))
	return result
}

func identityActor(identity testIdentity) (security.Actor, error) {
	return security.Actor{Type: security.ActorTypeHuman, ID: identity.userID}, nil
}

func TestAuthenticatorResolvesAndBindsIdentity(t *testing.T) {
	manager := scs.New()
	authenticator, err := New(manager, func(context.Context) (testIdentity, error) { return testIdentity{userID: "user-1"}, nil }, identityActor, func(ctx context.Context, identity testIdentity) context.Context {
		return context.WithValue(ctx, localIdentityKey{}, identity)
	})
	if err != nil {
		t.Fatal(err)
	}
	trusted, err := authenticator.authenticate(loadedContext(manager))
	if err != nil {
		t.Fatal(err)
	}
	actor, ok := security.ActorFrom(trusted)
	if !ok || actor != (security.Actor{Type: security.ActorTypeHuman, ID: "user-1"}) {
		t.Fatalf("actor=%+v present=%v", actor, ok)
	}
	if got := trusted.Value(localIdentityKey{}); got != (testIdentity{userID: "user-1"}) {
		t.Fatalf("identity=%+v", got)
	}
}

func TestAuthenticatorPreservesResolverClassification(t *testing.T) {
	manager := scs.New()
	cause := errors.New("store offline")
	authenticator, err := New(manager, func(context.Context) (testIdentity, error) {
		return testIdentity{}, errors.Join(ErrDependencyUnavailable, cause)
	}, identityActor, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := authenticator.authenticate(loadedContext(manager)); !errors.Is(err, ErrDependencyUnavailable) || !errors.Is(err, cause) {
		t.Fatalf("error=%v", err)
	}
}

type testTransport struct{}

func (testTransport) Kind() transport.Kind { return transport.KindHTTP }
func (testTransport) Endpoint() string     { return "" }
func (testTransport) Operation() string    { return "/test/Get" }
func (testTransport) RequestHeader() transport.Header {
	panic("session authentication must not read headers")
}
func (testTransport) ReplyHeader() transport.Header { return nil }

func TestMiddlewareRulesAndFailures(t *testing.T) {
	for _, tc := range []struct {
		name       string
		mode       authnpb.AuthnMode
		loaded     bool
		resolveErr error
		wantActor  security.ActorType
		checkError func(error) bool
	}{
		{"public", authnpb.AuthnMode_AUTHN_MODE_PUBLIC, false, nil, security.ActorTypeAnonymous, nil},
		{"authenticated", authnpb.AuthnMode_AUTHN_MODE_REQUIRED, true, nil, security.ActorTypeHuman, nil},
		{"missing context", authnpb.AuthnMode_AUTHN_MODE_REQUIRED, false, nil, "", securitypb.IsSecurityErrorReasonInternal},
		{"invalid identity", authnpb.AuthnMode_AUTHN_MODE_REQUIRED, true, ErrInvalidCredentials, "", securitypb.IsSecurityErrorReasonUnauthenticated},
		{"dependency failure", authnpb.AuthnMode_AUTHN_MODE_REQUIRED, true, ErrDependencyUnavailable, "", securitypb.IsSecurityErrorReasonUnavailable},
		{"missing rule", authnpb.AuthnMode_AUTHN_MODE_UNSPECIFIED, true, nil, "", securitypb.IsSecurityErrorReasonInternal},
	} {
		t.Run(tc.name, func(t *testing.T) {
			manager := scs.New()
			resolveCalls := 0
			authenticator, err := New(manager, func(context.Context) (testIdentity, error) {
				resolveCalls++
				return testIdentity{userID: "user-1"}, tc.resolveErr
			}, identityActor, nil)
			if err != nil {
				t.Fatal(err)
			}
			ctx := context.Background()
			if tc.loaded {
				ctx = loadedContext(manager)
			}
			ctx = transport.NewServerContext(ctx, testTransport{})
			rules := authnruntime.WithRulesFuncs(func() map[string]*authnpb.AuthnRule {
				if tc.mode == authnpb.AuthnMode_AUTHN_MODE_UNSPECIFIED {
					return nil
				}
				return map[string]*authnpb.AuthnRule{"/test/Get": {Mode: tc.mode}}
			})
			called := false
			_, err = Server(authenticator, rules)(func(ctx context.Context, _ any) (any, error) {
				called = true
				actor, ok := security.ActorFrom(ctx)
				if !ok || actor.Type != tc.wantActor {
					t.Fatalf("actor=%+v", actor)
				}
				return nil, nil
			})(ctx, nil)
			if tc.checkError != nil {
				if called || !tc.checkError(err) {
					t.Fatalf("called=%v error=%v", called, err)
				}
			} else if !called || err != nil {
				t.Fatalf("called=%v error=%v", called, err)
			}
			if tc.mode == authnpb.AuthnMode_AUTHN_MODE_PUBLIC && resolveCalls != 0 {
				t.Fatal("public route resolved identity")
			}
		})
	}
}

func TestAuthenticatorRejectsOtherManagerContext(t *testing.T) {
	called := false
	authenticator, err := New(scs.New(), func(context.Context) (testIdentity, error) { called = true; return testIdentity{}, nil }, identityActor, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := authenticator.authenticate(loadedContext(scs.New())); err == nil || called {
		t.Fatalf("called=%v error=%v", called, err)
	}
}
