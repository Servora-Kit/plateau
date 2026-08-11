package authz

import (
	"bytes"
	"context"
	"encoding/json"
	stderrors "errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	authnpb "github.com/Servora-Kit/servora-platform/api/gen/go/platform/authn/v1"
	authzpb "github.com/Servora-Kit/servora-platform/api/gen/go/platform/authz/v1"
	authn "github.com/Servora-Kit/servora-platform/security/authn"
	kerrors "github.com/go-kratos/kratos/v3/errors"
	"github.com/go-kratos/kratos/v3/middleware"
	"github.com/go-kratos/kratos/v3/transport"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

const testOperation = "/test.v1.ResourceService/Get"

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

func serverContext(operation string) context.Context {
	return transport.NewServerContext(context.Background(), &fakeTransport{operation: operation})
}

type fakeAuthorizer struct {
	mu         sync.Mutex
	allowed    bool
	err        error
	check      func(context.Context, CheckRequest) (bool, error)
	requests   []CheckRequest
	setLoggers int
}

func (authorizer *fakeAuthorizer) Check(ctx context.Context, request CheckRequest) (bool, error) {
	authorizer.mu.Lock()
	authorizer.requests = append(authorizer.requests, request)
	authorizer.mu.Unlock()
	if authorizer.check != nil {
		return authorizer.check(ctx, request)
	}
	return authorizer.allowed, authorizer.err
}

func (authorizer *fakeAuthorizer) SetLogger(*slog.Logger) {
	authorizer.mu.Lock()
	authorizer.setLoggers++
	authorizer.mu.Unlock()
}

func (authorizer *fakeAuthorizer) requestCount() int {
	authorizer.mu.Lock()
	defer authorizer.mu.Unlock()
	return len(authorizer.requests)
}

func (authorizer *fakeAuthorizer) lastRequest() CheckRequest {
	authorizer.mu.Lock()
	defer authorizer.mu.Unlock()
	return authorizer.requests[len(authorizer.requests)-1]
}

type capabilityAuthorizer struct{ fakeAuthorizer }

func (*capabilityAuthorizer) BatchCheck(context.Context, []CheckRequest) ([]bool, error) {
	return []bool{}, nil
}

func (*capabilityAuthorizer) ListAllowed(context.Context, string, string, string) ([]string, error) {
	return []string{}, nil
}

type staticAuthenticator struct{ subject string }

func (staticAuthenticator) Scheme() authn.Scheme { return "test" }
func (authenticator staticAuthenticator) Authenticate(context.Context) (authn.Authentication, error) {
	return authn.Authentication{Subject: authenticator.subject}, nil
}

func checkRule() *authzpb.AuthzRule {
	return &authzpb.AuthzRule{
		Mode:            authzpb.AuthzMode_AUTHZ_MODE_CHECK,
		Action:          "read",
		ResourceType:    "document",
		ResourceIdField: "value",
	}
}

func rules(rule *authzpb.AuthzRule) Option {
	return WithRulesFuncs(func() map[string]*authzpb.AuthzRule {
		return map[string]*authzpb.AuthzRule{testOperation: rule}
	})
}

func subject(value string) Option {
	return WithSubjectFunc(func(context.Context) (string, bool) { return value, value != "" })
}

func invoke(t *testing.T, middleware middleware.Middleware, ctx context.Context, request any, handler middleware.Handler) (any, error) {
	t.Helper()
	if handler == nil {
		handler = func(context.Context, any) (any, error) { return "ok", nil }
	}
	return middleware(handler)(ctx, request)
}

func assertPanicContains(t *testing.T, want string, function func()) {
	t.Helper()
	defer func() {
		recovered := recover()
		if recovered == nil || !strings.Contains(fmt.Sprint(recovered), want) {
			t.Fatalf("panic = %v, want substring %q", recovered, want)
		}
	}()
	function()
}

func TestServerConstructionValidation(t *testing.T) {
	assertPanicContains(t, "authorizer is nil", func() { Server(nil) })
	var typedNil *fakeAuthorizer
	assertPanicContains(t, "authorizer is nil", func() { Server(typedNil) })
	assertPanicContains(t, "option[0] is nil", func() { Server(&fakeAuthorizer{}, nil) })
	assertPanicContains(t, "option[0]", func() { Server(&fakeAuthorizer{}, WithSubjectFunc(nil)) })
}

func TestServerFailsClosedWithoutTransportOrRule(t *testing.T) {
	called := false
	_, err := invoke(t, Server(&fakeAuthorizer{}), context.Background(), nil, func(context.Context, any) (any, error) {
		called = true
		return nil, nil
	})
	if !authzpb.IsAuthzErrorReasonInternal(err) || called {
		t.Fatalf("called = %v, error = %v", called, err)
	}
	_, err = invoke(t, Server(&fakeAuthorizer{}), serverContext(testOperation), nil, nil)
	if !authzpb.IsAuthzErrorReasonInternal(err) {
		t.Fatalf("error = %v, want INTERNAL for missing rule", err)
	}
}

func TestServerModeNonePassesThrough(t *testing.T) {
	called := false
	_, err := invoke(t, Server(&fakeAuthorizer{}, rules(&authzpb.AuthzRule{Mode: authzpb.AuthzMode_AUTHZ_MODE_NONE})), serverContext(testOperation), nil, func(context.Context, any) (any, error) {
		called = true
		return nil, nil
	})
	if err != nil || !called {
		t.Fatalf("called = %v, error = %v", called, err)
	}
}

func TestServerUsesStandardAuthnSubjectAndStructuredRequest(t *testing.T) {
	authorizer := &fakeAuthorizer{allowed: true}
	authzMiddleware := Server(authorizer, rules(checkRule()))
	authnMiddleware := authn.Server([]authn.Authenticator{staticAuthenticator{subject: "user:alice"}})
	called := false
	chain := authnMiddleware(authzMiddleware(func(context.Context, any) (any, error) {
		called = true
		return nil, nil
	}))
	_, err := chain(serverContext(testOperation), &wrapperspb.StringValue{Value: "doc-1"})
	if err != nil || !called {
		t.Fatalf("called = %v, error = %v", called, err)
	}
	request := authorizer.lastRequest()
	if request.Subject != "user:alice" || request.Action != "read" || request.Resource != (Resource{Type: "document", ID: "doc-1"}) {
		t.Fatalf("request = %#v", request)
	}
}

func TestServerSubjectOverrideAndMissingSubject(t *testing.T) {
	authorizer := &fakeAuthorizer{allowed: true}
	_, err := invoke(t, Server(authorizer, rules(checkRule()), subject("workload:worker")), serverContext(testOperation), &wrapperspb.StringValue{Value: "doc-1"}, nil)
	if err != nil || authorizer.lastRequest().Subject != "workload:worker" {
		t.Fatalf("subject = %q, error = %v", authorizer.lastRequest().Subject, err)
	}

	missing := &fakeAuthorizer{allowed: true}
	_, err = invoke(t, Server(missing, rules(checkRule()), subject("")), serverContext(testOperation), &wrapperspb.StringValue{Value: "doc-1"}, nil)
	if !authnpb.IsAuthnErrorReasonUnauthenticated(err) || missing.requestCount() != 0 {
		t.Fatalf("error = %v, calls = %d", err, missing.requestCount())
	}
}

func TestServerInvalidResourceDoesNotCallBackend(t *testing.T) {
	authorizer := &fakeAuthorizer{allowed: true}
	_, err := invoke(t, Server(authorizer, rules(checkRule()), subject("user:alice")), serverContext(testOperation), &wrapperspb.StringValue{}, nil)
	if !authzpb.IsAuthzErrorReasonInvalidRequest(err) || authorizer.requestCount() != 0 {
		t.Fatalf("error = %v, calls = %d", err, authorizer.requestCount())
	}
}

func TestServerDeniedAndBackendErrorMappingConcealCause(t *testing.T) {
	_, err := invoke(t, Server(&fakeAuthorizer{allowed: false}, rules(checkRule()), subject("user:alice")), serverContext(testOperation), &wrapperspb.StringValue{Value: "doc-1"}, nil)
	if !authzpb.IsAuthzErrorReasonDenied(err) {
		t.Fatalf("error = %v, want DENIED", err)
	}

	providerCause := stderrors.New("provider endpoint token-secret detail")
	tests := []struct {
		name    string
		err     error
		matcher func(error) bool
		code    int32
		message string
	}{
		{name: "unavailable", err: stderrors.Join(ErrUnavailable, providerCause), matcher: authzpb.IsAuthzErrorReasonUnavailable, code: 503, message: "authorization service unavailable"},
		{name: "internal", err: providerCause, matcher: authzpb.IsAuthzErrorReasonInternal, code: 500, message: "internal authorization error"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := invoke(t, Server(&fakeAuthorizer{err: test.err}, rules(checkRule()), subject("user:alice")), serverContext(testOperation), &wrapperspb.StringValue{Value: "doc-1"}, nil)
			if !test.matcher(err) || !stderrors.Is(err, providerCause) {
				t.Fatalf("error = %v", err)
			}
			status := kerrors.FromError(err)
			if status.Code != test.code || status.Message != test.message {
				t.Fatalf("status = (%d, %q)", status.Code, status.Message)
			}
			grpcWire, marshalErr := json.Marshal(status.GRPCStatus().Proto())
			if marshalErr != nil {
				t.Fatal(marshalErr)
			}
			if bytes.Contains(grpcWire, []byte("token-secret")) || strings.Contains(err.Error(), "token-secret") {
				t.Fatalf("wire or error leaked cause: %v", err)
			}
		})
	}
}

func TestServerPreservesIncomingDeadlineAndLoggerBoundary(t *testing.T) {
	var seen time.Time
	authorizer := &fakeAuthorizer{check: func(ctx context.Context, _ CheckRequest) (bool, error) {
		seen, _ = ctx.Deadline()
		return true, nil
	}}
	ctx, cancel := context.WithTimeout(serverContext(testOperation), time.Second)
	defer cancel()
	want, _ := ctx.Deadline()
	_, err := invoke(t, Server(authorizer, rules(checkRule()), subject("user:alice")), ctx, &wrapperspb.StringValue{Value: "doc-1"}, nil)
	if err != nil || !seen.Equal(want) {
		t.Fatalf("deadline = %v, want %v, error = %v", seen, want, err)
	}

	var buffer bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buffer, nil))
	backend := &fakeAuthorizer{err: stderrors.New("backend token-secret")}
	_, err = invoke(t, Server(backend, rules(checkRule()), subject("identity-token-secret"), WithLogger(logger)), serverContext(testOperation), &wrapperspb.StringValue{Value: "resource-token-secret"}, nil)
	if err == nil || backend.setLoggers != 0 {
		t.Fatalf("error = %v, backend logger injections = %d", err, backend.setLoggers)
	}
	if !strings.Contains(buffer.String(), "AUTHZ_ERROR_REASON_INTERNAL") || strings.Contains(buffer.String(), "token-secret") {
		t.Fatalf("log = %q", buffer.String())
	}
}

func TestExtractProtoFieldNestedScalar(t *testing.T) {
	request := &descriptorpb.FileDescriptorProto{Options: &descriptorpb.FileOptions{GoPackage: proto.String("example.com/pkg")}}
	got, err := extractProtoField(request, "options.go_package")
	if err != nil || got != "example.com/pkg" {
		t.Fatalf("value = %q, error = %v", got, err)
	}
}

func TestOptionalCapabilitiesRemainDiscoverable(t *testing.T) {
	var authorizer Authorizer = &capabilityAuthorizer{}
	if _, ok := authorizer.(BatchAuthorizer); !ok {
		t.Fatal("BatchAuthorizer capability missing")
	}
	if _, ok := authorizer.(Lister); !ok {
		t.Fatal("Lister capability missing")
	}
}
