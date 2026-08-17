package openfga

import (
	"context"
	"errors"
	"net"
	"net/http"
	"sync/atomic"
	"testing"

	authzpb "github.com/Servora-Kit/servora-platform/api/gen/go/platform/security/authz/v1"
	securityerrorspb "github.com/Servora-Kit/servora-platform/api/gen/go/platform/security/errors/v1"
	security "github.com/Servora-Kit/servora-platform/security"
	"github.com/go-kratos/kratos/v3/middleware"
	"github.com/go-kratos/kratos/v3/transport"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

const middlewareOperation = "/test.v1.ResourceService/Get"

type authzMiddlewareTransport struct{ operation string }

func (*authzMiddlewareTransport) Kind() transport.Kind            { return transport.KindHTTP }
func (*authzMiddlewareTransport) Endpoint() string                { return "" }
func (value *authzMiddlewareTransport) Operation() string         { return value.operation }
func (*authzMiddlewareTransport) RequestHeader() transport.Header { return authzMiddlewareHeader{} }
func (*authzMiddlewareTransport) ReplyHeader() transport.Header   { return authzMiddlewareHeader{} }

type authzMiddlewareHeader struct{}

func (authzMiddlewareHeader) Get(string) string      { return "" }
func (authzMiddlewareHeader) Set(string, string)     {}
func (authzMiddlewareHeader) Add(string, string)     {}
func (authzMiddlewareHeader) Keys() []string         { return nil }
func (authzMiddlewareHeader) Values(string) []string { return nil }

func authzMiddlewareContext() context.Context {
	return transport.NewServerContext(context.Background(), &authzMiddlewareTransport{operation: middlewareOperation})
}

func authzMiddlewareRules(rule *authzpb.AuthzRule) map[string]*authzpb.AuthzRule {
	return map[string]*authzpb.AuthzRule{middlewareOperation: rule}
}

func invokeAuthzMiddleware(t *testing.T, value middleware.Middleware, ctx context.Context, request any) (any, error) {
	t.Helper()
	return value(func(context.Context, any) (any, error) { return "ok", nil })(ctx, request)
}

func staticRule() *authzpb.AuthzRule {
	return &authzpb.AuthzRule{
		Mode: authzpb.AuthzMode_AUTHZ_MODE_REQUIRED, Action: "read", ResourceType: "document",
		Target: &authzpb.AuthzRule_ResourceId{ResourceId: "default"},
	}
}

func fieldRule(field string) *authzpb.AuthzRule {
	return &authzpb.AuthzRule{
		Mode: authzpb.AuthzMode_AUTHZ_MODE_REQUIRED, Action: "read", ResourceType: "document",
		Target: &authzpb.AuthzRule_ResourceIdField{ResourceIdField: field},
	}
}

func middlewareAuthorizer(t *testing.T, status int, body string) (*Authorizer, *atomic.Int32) {
	t.Helper()
	client, calls := sdkClient(t, func(response http.ResponseWriter, _ *http.Request) {
		response.Header().Set("Content-Type", "application/json")
		if status != 0 {
			response.WriteHeader(status)
		}
		_, _ = response.Write([]byte(body))
	})
	authorizer, err := New(client)
	if err != nil {
		t.Fatal(err)
	}
	return authorizer, calls
}

func TestServerNoneSkipsOpenFGA(t *testing.T) {
	authorizer, calls := middlewareAuthorizer(t, 0, `{"allowed":true}`)
	result, err := invokeAuthzMiddleware(t, Server(authorizer, authzMiddlewareRules(&authzpb.AuthzRule{Mode: authzpb.AuthzMode_AUTHZ_MODE_NONE})), authzMiddlewareContext(), nil)
	if err != nil || result != "ok" || calls.Load() != 0 {
		t.Fatalf("result=%v error=%v calls=%d", result, err, calls.Load())
	}
}

func TestServerUsesStaticResourceID(t *testing.T) {
	authorizer, _ := middlewareAuthorizer(t, 0, `{"allowed":true}`)
	ctx := security.WithActor(authzMiddlewareContext(), security.Actor{Type: security.ActorTypeHuman, ID: "user-1"})
	result, err := invokeAuthzMiddleware(t, Server(authorizer, authzMiddlewareRules(staticRule())), ctx, nil)
	if err != nil || result != "ok" {
		t.Fatalf("result=%v error=%v", result, err)
	}
}

func TestServerResolvesDirectResourceID(t *testing.T) {
	authorizer, _ := middlewareAuthorizer(t, 0, `{"allowed":true}`)
	ctx := security.WithActor(authzMiddlewareContext(), security.Actor{Type: security.ActorTypeService, ID: "worker-1"})
	result, err := invokeAuthzMiddleware(t, Server(authorizer, authzMiddlewareRules(fieldRule("value"))), ctx, &wrapperspb.StringValue{Value: "doc-1"})
	if err != nil || result != "ok" {
		t.Fatalf("result=%v error=%v", result, err)
	}
}

func TestServerFailsClosedBeforeProvider(t *testing.T) {
	authorizer, calls := middlewareAuthorizer(t, 0, `{"allowed":true}`)
	server := Server(authorizer, authzMiddlewareRules(fieldRule("value")))
	if _, err := invokeAuthzMiddleware(t, server, authzMiddlewareContext(), &wrapperspb.StringValue{Value: "doc-1"}); !securityerrorspb.IsSecurityErrorReasonUnauthenticated(err) || calls.Load() != 0 {
		t.Fatalf("missing Actor error=%v calls=%d", err, calls.Load())
	}
	ctx := security.WithActor(authzMiddlewareContext(), security.Actor{Type: security.ActorTypeHuman, ID: "user-1"})
	if _, err := invokeAuthzMiddleware(t, server, ctx, &wrapperspb.StringValue{}); !securityerrorspb.IsSecurityErrorReasonInvalidArgument(err) || calls.Load() != 0 {
		t.Fatalf("empty resource error=%v calls=%d", err, calls.Load())
	}
	oneofServer := Server(authorizer, authzMiddlewareRules(fieldRule("number_value")))
	oneofRequest := &structpb.Value{Kind: &structpb.Value_NumberValue{NumberValue: 1}}
	if _, err := invokeAuthzMiddleware(t, oneofServer, ctx, oneofRequest); !securityerrorspb.IsSecurityErrorReasonInvalidArgument(err) || calls.Load() != 0 {
		t.Fatalf("oneof resource error=%v calls=%d", err, calls.Load())
	}
}

func TestServerMapsDeniedAndUnavailable(t *testing.T) {
	denied, _ := middlewareAuthorizer(t, 0, `{"allowed":false}`)
	ctx := security.WithActor(authzMiddlewareContext(), security.Actor{Type: security.ActorTypeHuman, ID: "user-1"})
	if _, err := invokeAuthzMiddleware(t, Server(denied, authzMiddlewareRules(staticRule())), ctx, nil); !securityerrorspb.IsSecurityErrorReasonPermissionDenied(err) {
		t.Fatalf("denied error=%v", err)
	}
	unavailable, _ := middlewareAuthorizer(t, http.StatusServiceUnavailable, `{"code":"internal_error","message":"secret"}`)
	if _, err := invokeAuthzMiddleware(t, Server(unavailable, authzMiddlewareRules(staticRule())), ctx, nil); !securityerrorspb.IsSecurityErrorReasonUnavailable(err) {
		t.Fatalf("unavailable error=%v", err)
	}
}

func TestAPIErrorMapsNetworkFailure(t *testing.T) {
	cause := &net.DNSError{Err: "connection refused", Name: "openfga.invalid"}
	mapped := apiError(cause)
	if !securityerrorspb.IsSecurityErrorReasonUnavailable(mapped) || !errors.Is(mapped, cause) {
		t.Fatalf("mapped error=%v", mapped)
	}
}

func TestServerRequiresTransportAndRule(t *testing.T) {
	authorizer, _ := middlewareAuthorizer(t, 0, `{"allowed":true}`)
	if _, err := invokeAuthzMiddleware(t, Server(authorizer, map[string]*authzpb.AuthzRule{}), nil, nil); !securityerrorspb.IsSecurityErrorReasonInternal(err) {
		t.Fatalf("nil context error=%v", err)
	}
	if _, err := invokeAuthzMiddleware(t, Server(authorizer, map[string]*authzpb.AuthzRule{}), context.Background(), nil); !securityerrorspb.IsSecurityErrorReasonInternal(err) {
		t.Fatalf("missing transport error=%v", err)
	}
	if _, err := invokeAuthzMiddleware(t, Server(authorizer, map[string]*authzpb.AuthzRule{}), authzMiddlewareContext(), nil); !securityerrorspb.IsSecurityErrorReasonInternal(err) {
		t.Fatalf("missing rule error=%v", err)
	}
}

func TestServerRejectsIntermediateOneof(t *testing.T) {
	authorizer, calls := middlewareAuthorizer(t, 0, `{"allowed":true}`)
	ctx := security.WithActor(authzMiddlewareContext(), security.Actor{Type: security.ActorTypeHuman, ID: "user-1"})
	request := &structpb.Value{Kind: &structpb.Value_StructValue{StructValue: &structpb.Struct{Fields: map[string]*structpb.Value{}}}}
	if _, err := invokeAuthzMiddleware(t, Server(authorizer, authzMiddlewareRules(fieldRule("struct_value.fields"))), ctx, request); !securityerrorspb.IsSecurityErrorReasonInvalidArgument(err) || calls.Load() != 0 {
		t.Fatalf("intermediate oneof error=%v calls=%d", err, calls.Load())
	}
}
