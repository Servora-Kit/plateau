package authn

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"testing"

	authnpb "github.com/Servora-Kit/servora-platform/api/gen/go/platform/authn/v1"
	kerrors "github.com/go-kratos/kratos/v3/errors"
	"github.com/go-kratos/kratos/v3/middleware"
	"github.com/go-kratos/kratos/v3/transport"
)

type fakeTransport struct{ operation string }

func (*fakeTransport) Kind() transport.Kind            { return transport.KindHTTP }
func (*fakeTransport) Endpoint() string                { return "" }
func (transport *fakeTransport) Operation() string     { return transport.operation }
func (*fakeTransport) RequestHeader() transport.Header { return fakeHeader{} }
func (*fakeTransport) ReplyHeader() transport.Header   { return fakeHeader{} }

type fakeHeader struct{}

func (fakeHeader) Get(string) string      { return "" }
func (fakeHeader) Set(string, string)     {}
func (fakeHeader) Add(string, string)     {}
func (fakeHeader) Keys() []string         { return nil }
func (fakeHeader) Values(string) []string { return nil }

type fakeAuthenticator struct {
	scheme      Scheme
	schemeCalls int
	calls       int
	loggerSets  int
	auth        Authentication
	err         error
}

func (authenticator *fakeAuthenticator) Scheme() Scheme {
	authenticator.schemeCalls++
	return authenticator.scheme
}

func (authenticator *fakeAuthenticator) Authenticate(context.Context) (Authentication, error) {
	authenticator.calls++
	return authenticator.auth, authenticator.err
}

func (authenticator *fakeAuthenticator) SetLogger(*slog.Logger) { authenticator.loggerSets++ }

func serverContext(operation string) context.Context {
	return transport.NewServerContext(context.Background(), &fakeTransport{operation: operation})
}

func rules(entries map[string]*authnpb.AuthnRule) func() map[string]*authnpb.AuthnRule {
	return func() map[string]*authnpb.AuthnRule { return entries }
}

func invoke(t *testing.T, middleware middleware.Middleware, ctx context.Context) (context.Context, error) {
	t.Helper()
	var handlerContext context.Context
	handler := middleware(func(ctx context.Context, _ any) (any, error) {
		handlerContext = ctx
		return "ok", nil
	})
	_, err := handler(ctx, nil)
	return handlerContext, err
}

func TestAuthenticationContextAccessors(t *testing.T) {
	ctx := withAuthentication(context.Background(), Authentication{Subject: "user:123"})
	authentication, ok := AuthenticationFrom(ctx)
	if !ok || authentication.Subject != "user:123" {
		t.Fatalf("AuthenticationFrom = (%+v, %v)", authentication, ok)
	}
	subject, ok := SubjectFrom(ctx)
	if !ok || subject != authentication.Subject {
		t.Fatalf("SubjectFrom = (%q, %v)", subject, ok)
	}
	if _, ok := SubjectFrom(context.Background()); ok {
		t.Fatal("SubjectFrom accepted context without runtime authentication")
	}
}

func TestServerConstructionValidation(t *testing.T) {
	tests := []struct {
		name string
		fn   func()
		want string
	}{
		{name: "empty", fn: func() { Server(nil) }, want: "authenticator collection is empty"},
		{name: "nil", fn: func() { Server([]Authenticator{nil}) }, want: "authenticator[0] is nil"},
		{name: "typed nil", fn: func() { var authenticator *fakeAuthenticator; Server([]Authenticator{authenticator}) }, want: "authenticator[0] is nil"},
		{name: "empty scheme", fn: func() { Server([]Authenticator{&fakeAuthenticator{}}) }, want: "empty scheme"},
		{name: "duplicate scheme", fn: func() { Server([]Authenticator{&fakeAuthenticator{scheme: "jwt"}, &fakeAuthenticator{scheme: "jwt"}}) }, want: "duplicate scheme"},
		{name: "nil option", fn: func() { Server([]Authenticator{&fakeAuthenticator{scheme: "jwt"}}, nil) }, want: "option[0] is nil"},
		{name: "unknown rule scheme", fn: func() {
			Server([]Authenticator{&fakeAuthenticator{scheme: "jwt"}}, WithRulesFuncs(rules(map[string]*authnpb.AuthnRule{
				"/svc/Get": {Mode: authnpb.AuthnMode_AUTHN_MODE_REQUIRED, Schemes: []string{"mtls"}},
			})))
		}, want: `references unknown scheme "mtls"`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			message := panicMessage(t, test.fn)
			if !strings.Contains(message, test.want) {
				t.Fatalf("panic = %q, want %q", message, test.want)
			}
		})
	}
}

func TestServerDispatchesInAssemblyOrderAndSnapshotsSchemes(t *testing.T) {
	first := &fakeAuthenticator{scheme: "jwt", err: fmt.Errorf("missing: %w", ErrNoCredentials)}
	second := &fakeAuthenticator{scheme: "api_key", auth: Authentication{Subject: "service:worker"}}
	authenticators := []Authenticator{first, second}
	middleware := Server(authenticators, WithRulesFuncs(rules(map[string]*authnpb.AuthnRule{
		"/svc/Get": {Mode: authnpb.AuthnMode_AUTHN_MODE_REQUIRED, Schemes: []string{"api_key", "jwt"}},
	})))
	authenticators[0] = second
	ctx, err := invoke(t, middleware, serverContext("/svc/Get"))
	if err != nil {
		t.Fatal(err)
	}
	subject, _ := SubjectFrom(ctx)
	if subject != "service:worker" || first.calls != 1 || second.calls != 1 {
		t.Fatalf("subject=%q calls=(%d,%d)", subject, first.calls, second.calls)
	}
	if first.schemeCalls != 1 || second.schemeCalls != 1 {
		t.Fatalf("Scheme calls=(%d,%d), want one snapshot each", first.schemeCalls, second.schemeCalls)
	}
}

func TestServerPublicAndEmptySchemesRules(t *testing.T) {
	first := &fakeAuthenticator{scheme: "jwt", auth: Authentication{Subject: "user:first"}}
	second := &fakeAuthenticator{scheme: "api_key", auth: Authentication{Subject: "user:second"}}
	middleware := Server([]Authenticator{first, second}, WithRulesFuncs(rules(map[string]*authnpb.AuthnRule{
		"/svc/Public": {Mode: authnpb.AuthnMode_AUTHN_MODE_PUBLIC},
		"/svc/All":    {Mode: authnpb.AuthnMode_AUTHN_MODE_REQUIRED},
	})))
	if _, err := invoke(t, middleware, serverContext("/svc/Public")); err != nil {
		t.Fatal(err)
	}
	if first.calls != 0 || second.calls != 0 {
		t.Fatal("PUBLIC invoked authenticators")
	}
	ctx, err := invoke(t, middleware, serverContext("/svc/All"))
	if err != nil {
		t.Fatal(err)
	}
	if subject, _ := SubjectFrom(ctx); subject != "user:first" {
		t.Fatalf("subject = %q", subject)
	}
}

func TestServerStopsAfterRejectedCredentials(t *testing.T) {
	first := &fakeAuthenticator{scheme: "jwt", err: fmt.Errorf("invalid token-secret: %w", ErrCredentialsRejected)}
	second := &fakeAuthenticator{scheme: "api_key", auth: Authentication{Subject: "service:worker"}}
	_, err := invoke(t, Server([]Authenticator{first, second}), serverContext("/svc/Get"))
	if !authnpb.IsAuthnErrorReasonUnauthenticated(err) || second.calls != 0 {
		t.Fatalf("error=%v second calls=%d", err, second.calls)
	}
}

func TestServerErrorMappingConcealsCause(t *testing.T) {
	providerCause := errors.New("provider token-secret detail")
	tests := []struct {
		name    string
		auth    Authentication
		err     error
		matcher func(error) bool
		code    int32
		message string
	}{
		{name: "no credentials", err: fmt.Errorf("missing: %w", ErrNoCredentials), matcher: authnpb.IsAuthnErrorReasonUnauthenticated, code: 401, message: "authentication failed"},
		{name: "rejected", err: fmt.Errorf("rejected: %w: %w", ErrCredentialsRejected, providerCause), matcher: authnpb.IsAuthnErrorReasonUnauthenticated, code: 401, message: "authentication failed"},
		{name: "unavailable", err: fmt.Errorf("jwks: %w: %w", ErrUnavailable, providerCause), matcher: authnpb.IsAuthnErrorReasonUnavailable, code: 503, message: "authentication service unavailable"},
		{name: "internal", err: providerCause, matcher: authnpb.IsAuthnErrorReasonInternal, code: 500, message: "internal authentication error"},
		{name: "empty subject", auth: Authentication{}, matcher: authnpb.IsAuthnErrorReasonInternal, code: 500, message: "internal authentication error"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			authenticator := &fakeAuthenticator{scheme: "jwt", auth: test.auth, err: test.err}
			_, err := invoke(t, Server([]Authenticator{authenticator}), serverContext("/svc/Get"))
			if !test.matcher(err) {
				t.Fatalf("error = %v", err)
			}
			status := kerrors.FromError(err)
			if status.Code != test.code || status.Message != test.message {
				t.Fatalf("status=(%d,%q)", status.Code, status.Message)
			}
			wire, marshalErr := json.Marshal(status.GRPCStatus().Proto())
			if marshalErr != nil {
				t.Fatal(marshalErr)
			}
			if bytes.Contains(wire, []byte("token-secret")) || strings.Contains(err.Error(), "token-secret") {
				t.Fatalf("public error leaked provider cause: %v", err)
			}
			if test.err != nil && !errors.Is(err, test.err) {
				t.Fatalf("cause chain lost %v", test.err)
			}
		})
	}
}

func TestServerLoggerIsOptionalSafeAndNotInjected(t *testing.T) {
	var logs bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logs, nil))
	authenticator := &fakeAuthenticator{scheme: "jwt", err: fmt.Errorf("token-secret: %w", ErrCredentialsRejected)}
	_, _ = invoke(t, Server([]Authenticator{authenticator}), serverContext("/svc/Get"))
	if logs.Len() != 0 {
		t.Fatalf("unexpected logs without logger: %q", logs.String())
	}
	_, _ = invoke(t, Server([]Authenticator{authenticator}, WithLogger(logger)), serverContext("/svc/Get"))
	if !strings.Contains(logs.String(), "authentication failed") || !strings.Contains(logs.String(), "UNAUTHENTICATED") {
		t.Fatalf("logs = %q", logs.String())
	}
	if strings.Contains(logs.String(), "token-secret") || authenticator.loggerSets != 0 {
		t.Fatalf("unsafe logs or dynamic logger injection: %q sets=%d", logs.String(), authenticator.loggerSets)
	}

	logs.Reset()
	success := &fakeAuthenticator{scheme: "jwt", auth: Authentication{Subject: "identity-token-secret"}}
	if _, err := invoke(t, Server([]Authenticator{success}, WithLogger(logger)), serverContext("/svc/Get")); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(logs.String(), "identity-token-secret") {
		t.Fatalf("success log leaked subject: %q", logs.String())
	}
}

func panicMessage(t *testing.T, fn func()) (message string) {
	t.Helper()
	defer func() {
		if recovered := recover(); recovered != nil {
			message = fmt.Sprint(recovered)
		}
	}()
	fn()
	t.Fatal("expected panic")
	return ""
}

var _ Authenticator = (*fakeAuthenticator)(nil)
