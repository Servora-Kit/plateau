package audit

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	iamauthnpb "github.com/Servora-Kit/plateau/api/gen/go/plateau/iam/authn/v1"
	userpb "github.com/Servora-Kit/plateau/api/gen/go/plateau/iam/user/v1"
	auditpb "github.com/Servora-Kit/plateau/api/gen/go/plateau/security/audit/v1"
	securityerrorspb "github.com/Servora-Kit/plateau/api/gen/go/plateau/security/errors/v1"
	security "github.com/Servora-Kit/plateau/security"
	cloudevents "github.com/cloudevents/sdk-go/v2"
	kratos "github.com/go-kratos/kratos/v3"
	kerrors "github.com/go-kratos/kratos/v3/errors"
	"github.com/go-kratos/kratos/v3/transport"
	khttp "github.com/go-kratos/kratos/v3/transport/http"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

func TestNewEventActorTypesTraceAndDualProtocolMetadata(t *testing.T) {
	traceID, _ := trace.TraceIDFromHex("4bf92f3577b34da6a3ce929d0e0e4736")
	spanID, _ := trace.SpanIDFromHex("00f067aa0ba902b7")
	traceState, err := trace.ParseTraceState("congo=t61rcWkgMzE")
	if err != nil {
		t.Fatalf("ParseTraceState: %v", err)
	}
	unsampledSC := trace.NewSpanContext(trace.SpanContextConfig{
		TraceID:    traceID,
		SpanID:     spanID,
		TraceFlags: 0,
		TraceState: traceState,
	})

	t.Run("human actor over HTTP with unsampled trace", func(t *testing.T) {
		app := kratos.New(kratos.Name("admin.service"))
		ctx := kratos.NewContext(context.Background(), app)
		ctx = security.WithActor(ctx, security.Actor{Type: security.ActorTypeHuman, ID: "01950000-0000-7000-8000-000000000001"})
		ctx = trace.ContextWithSpanContext(ctx, unsampledSC)

		httpReq := httptest.NewRequest(http.MethodPost, "http://admin.local/v1/admin/users", nil)
		httpReq.RemoteAddr = "10.0.0.8:54321"
		httpReq.Header.Set("X-Forwarded-For", "203.0.113.19, 10.0.0.8")
		httpReq.Header.Set("User-Agent", "Mozilla/5.0 PlateauTest")
		httpReq.Header.Set("X-Request-Id", "req-http-123")
		ctx = transport.NewServerContext(ctx, &fakeHTTPTransport{
			operation: "/plateau.admin.user.v1.UserService/CreateUser",
			req:       httpReq,
		})

		ev := NewEvent(ctx, WithSubject("/plateau.admin.user.v1.UserService/CreateUser"))
		parsedID, parseErr := uuid.Parse(ev.ID())
		if parseErr != nil || parsedID.Version() != 7 {
			t.Fatalf("event ID = %q (err=%v), want UUIDv7", ev.ID(), parseErr)
		}
		if ev.Source() != "//admin.service" {
			t.Fatalf("Source = %q, want //admin.service", ev.Source())
		}
		exts := ev.Extensions()
		if exts[ExtActorType] != "human" || exts[ExtActorID] != "01950000-0000-7000-8000-000000000001" {
			t.Fatalf("actor extensions = (%v, %v)", exts[ExtActorType], exts[ExtActorID])
		}
		wantTP := "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-00"
		if exts[ExtTraceParent] != wantTP {
			t.Fatalf("traceparent = %v, want %q", exts[ExtTraceParent], wantTP)
		}
		if exts[ExtTraceState] != "congo=t61rcWkgMzE" {
			t.Fatalf("tracestate = %v, want congo=t61rcWkgMzE", exts[ExtTraceState])
		}
		if exts[ExtClientIP] != "203.0.113.19" || exts[ExtUserAgent] != "Mozilla/5.0 PlateauTest" || exts[ExtRequestID] != "req-http-123" {
			t.Fatalf("HTTP client meta = ip:%v ua:%v reqid:%v", exts[ExtClientIP], exts[ExtUserAgent], exts[ExtRequestID])
		}
	})

	t.Run("service actor over gRPC transport and peer", func(t *testing.T) {
		app := kratos.New(kratos.Name("iam.service"))
		ctx := kratos.NewContext(context.Background(), app)
		ctx = security.WithActor(ctx, security.Actor{Type: security.ActorTypeService, ID: "admin-bff-client"})
		ctx = peer.NewContext(ctx, &peer.Peer{
			Addr: &net.TCPAddr{IP: net.ParseIP("172.18.0.5"), Port: 41230},
		})
		ctx = transport.NewServerContext(ctx, &fakeGRPCTransport{
			operation: "/plateau.iam.user.v1.UserService/DisableUser",
			headers: mapHeader{
				"x-request-id": "req-grpc-456",
				"user-agent":   "grpc-go/1.83.2",
			},
		})

		ev := NewEvent(ctx)
		exts := ev.Extensions()
		if ev.Source() != "//iam.service" {
			t.Fatalf("Source = %q, want //iam.service", ev.Source())
		}
		if exts[ExtActorType] != "service" || exts[ExtActorID] != "admin-bff-client" {
			t.Fatalf("service actor extensions = (%v, %v)", exts[ExtActorType], exts[ExtActorID])
		}
		if exts[ExtClientIP] != "172.18.0.5" || exts[ExtUserAgent] != "grpc-go/1.83.2" || exts[ExtRequestID] != "req-grpc-456" {
			t.Fatalf("gRPC client meta = ip:%v ua:%v reqid:%v", exts[ExtClientIP], exts[ExtUserAgent], exts[ExtRequestID])
		}
	})

	t.Run("native HTTP handler WithHTTPRequest and anonymous fallback", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodGet, "http://admin.local/v1/admin/auth/callback", nil)
		r.RemoteAddr = "198.51.100.7:60000"
		r.Header.Set("User-Agent", "CallbackBrowser/1.0")
		r.Header.Set("X-Request-Id", "req-cb-1")

		ev := NewEvent(context.Background(), WithHTTPRequest(r))
		exts := ev.Extensions()
		if exts[ExtActorType] != "anonymous" {
			t.Fatalf("actortype = %v, want anonymous", exts[ExtActorType])
		}
		if _, hasID := exts[ExtActorID]; hasID {
			t.Fatalf("anonymous event must not carry actorid, got %v", exts[ExtActorID])
		}
		if exts[ExtClientIP] != "198.51.100.7" || exts[ExtUserAgent] != "CallbackBrowser/1.0" || exts[ExtRequestID] != "req-cb-1" {
			t.Fatalf("WithHTTPRequest meta = %#v", exts)
		}
	})
}

func TestMiddlewareCreateFallbackRedactionErrorsAndAuthzDenied(t *testing.T) {
	const (
		createOp  = "/plateau.iam.user.v1.UserService/CreateUser"
		setPwdOp  = "/plateau.iam.user.v1.UserService/SetUserPassword"
		loginOp   = "/plateau.iam.authn.v1.AuthnService/SubmitPassword"
		listOp    = "/plateau.iam.user.v1.UserService/ListUsers"
		disableOp = "/plateau.iam.user.v1.UserService/DisableUser"
	)

	rulesFn := func() map[string]*auditpb.AuditRule {
		return map[string]*auditpb.AuditRule{
			createOp: {
				Mode:     auditpb.AuditMode_AUDIT_MODE_ENABLED,
				Category: auditpb.AuditCategory_AUDIT_CATEGORY_RESOURCE,
			},
			setPwdOp: {
				Mode:     auditpb.AuditMode_AUDIT_MODE_ENABLED,
				Category: auditpb.AuditCategory_AUDIT_CATEGORY_RESOURCE,
			},
			loginOp: {
				Mode:     auditpb.AuditMode_AUDIT_MODE_ENABLED,
				Category: auditpb.AuditCategory_AUDIT_CATEGORY_AUTHN,
			},
			listOp: {
				Mode: auditpb.AuditMode_AUDIT_MODE_DISABLED,
			},
			disableOp: {
				Mode:     auditpb.AuditMode_AUDIT_MODE_ENABLED,
				Category: auditpb.AuditCategory_AUDIT_CATEGORY_RESOURCE,
			},
		}
	}

	t.Run("CreateUser extracts resource from response and redacts password without mutating request", func(t *testing.T) {
		rec := &recordingAuditor{}
		mw := Server(rec, security.WithRulesFuncs(rulesFn))

		const rawCreatePwd = "test-create-cred-val"
		req := &userpb.CreateUserRequest{
			User:     &userpb.User{Email: new("alice@example.com")},
			Password: rawCreatePwd,
		}
		ctx := serverCtx(createOp, security.Actor{Type: security.ActorTypeService, ID: "admin-service"})

		resp, err := mw(func(context.Context, any) (any, error) {
			return &userpb.User{
				Name:   "users/01950000-0000-7000-8000-000000000099",
				UserId: "01950000-0000-7000-8000-000000000099",
			}, nil
		})(ctx, req)
		if err != nil || resp == nil {
			t.Fatalf("handler returned (%v, %v)", resp, err)
		}
		if req.GetPassword() != rawCreatePwd {
			t.Fatalf("middleware mutated caller request password: %q", req.GetPassword())
		}

		events := rec.Events()
		if len(events) != 1 {
			t.Fatalf("emitted events = %d, want 1", len(events))
		}
		ev := events[0]
		exts := ev.Extensions()
		if exts[ExtCategory] != CategoryResource || exts[ExtOutcome] != OutcomeSuccess {
			t.Fatalf("category/outcome = (%v, %v)", exts[ExtCategory], exts[ExtOutcome])
		}
		if exts[ExtResourceType] != "iam.plateau.dev/User" || exts[ExtResourceName] != "users/01950000-0000-7000-8000-000000000099" {
			t.Fatalf("resource = (%v, %v)", exts[ExtResourceType], exts[ExtResourceName])
		}
		if strings.Contains(string(ev.Data()), rawCreatePwd) {
			t.Fatalf("sensitive password leaked into CloudEvent data: %s", string(ev.Data()))
		}
		var payload map[string]any
		if err := json.Unmarshal(ev.Data(), &payload); err != nil {
			t.Fatalf("unmarshal event JSON data: %v", err)
		}
	})

	t.Run("SetUserPassword extracts resource_reference and redacts new_password", func(t *testing.T) {
		rec := &recordingAuditor{}
		mw := Server(rec, security.WithRulesFuncs(rulesFn))

		const rawNewPwd = "test-reset-cred-val"
		req := &userpb.SetUserPasswordRequest{
			Name:        "users/01950000-0000-7000-8000-000000000077",
			NewPassword: rawNewPwd,
		}
		ctx := serverCtx(setPwdOp, security.Actor{Type: security.ActorTypeHuman, ID: "admin-1"})
		if _, err := mw(func(context.Context, any) (any, error) {
			return &userpb.SetUserPasswordResponse{}, nil
		})(ctx, req); err != nil {
			t.Fatalf("unexpected err: %v", err)
		}

		ev := rec.Events()[0]
		exts := ev.Extensions()
		if exts[ExtResourceType] != "iam.plateau.dev/User" || exts[ExtResourceName] != "users/01950000-0000-7000-8000-000000000077" {
			t.Fatalf("resource_reference extraction = (%v, %v)", exts[ExtResourceType], exts[ExtResourceName])
		}
		if strings.Contains(string(ev.Data()), rawNewPwd) {
			t.Fatalf("new_password leaked into CloudEvent data: %s", string(ev.Data()))
		}
	})

	t.Run("public SubmitPassword enriches Actor and Resource via context slot and redacts password", func(t *testing.T) {
		rec := &recordingAuditor{}
		mw := Server(rec, security.WithRulesFuncs(rulesFn))

		const rawLoginPwd = "test-login-cred-val"
		req := &iamauthnpb.SubmitPasswordRequest{
			FlowId:   "flow-1",
			Email:    "alice@example.com",
			Password: rawLoginPwd,
		}
		ctx := serverCtx(loginOp, security.Actor{Type: security.ActorTypeAnonymous})
		_, err := mw(func(hCtx context.Context, _ any) (any, error) {
			SetActor(hCtx, security.Actor{Type: security.ActorTypeHuman, ID: "user-alice"})
			SetResource(hCtx, "iam.plateau.dev/User", "users/user-alice")
			return &iamauthnpb.SubmitPasswordResponse{}, nil
		})(ctx, req)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}

		ev := rec.Events()[0]
		exts := ev.Extensions()
		if ev.Type() != DefaultRPCEventType || exts[ExtCategory] != CategoryAuthn {
			t.Fatalf("type/category = (%q, %v)", ev.Type(), exts[ExtCategory])
		}
		if exts[ExtActorType] != "human" || exts[ExtActorID] != "user-alice" {
			t.Fatalf("enriched actor = (%v, %v)", exts[ExtActorType], exts[ExtActorID])
		}
		if exts[ExtResourceName] != "users/user-alice" {
			t.Fatalf("enriched resource = %v", exts[ExtResourceName])
		}
		if strings.Contains(string(ev.Data()), rawLoginPwd) {
			t.Fatalf("login password leaked: %s", string(ev.Data()))
		}
	})

	t.Run("Kratos reason, gRPC ErrorInfo reason, and sanitized internal DB error", func(t *testing.T) {
		rec := &recordingAuditor{}
		mw := Server(rec, security.WithRulesFuncs(rulesFn))
		ctx := serverCtx(disableOp, security.Actor{Type: security.ActorTypeHuman, ID: "admin-1"})
		req := &userpb.DisableUserRequest{Name: "users/u-404"}

		wantDomainErr := kerrors.NotFound("USER_ERROR_REASON_NOT_FOUND", "user not found")
		_, gotErr := mw(func(context.Context, any) (any, error) {
			return nil, wantDomainErr
		})(ctx, req)
		if !errors.Is(gotErr, wantDomainErr) {
			t.Fatalf("gotErr = %v, want %v", gotErr, wantDomainErr)
		}
		ev1 := rec.Events()[0]
		if ev1.Extensions()[ExtOutcome] != OutcomeFailure ||
			ev1.Extensions()[ExtErrorCode] != "USER_ERROR_REASON_NOT_FOUND" ||
			ev1.Extensions()[ExtErrorMessage] != "user not found" {
			t.Fatalf("domain error extensions = %#v", ev1.Extensions())
		}

		st, stErr := status.New(codes.FailedPrecondition, "account already disabled").WithDetails(
			&errdetails.ErrorInfo{Reason: "USER_ERROR_REASON_INVALID_STATE"},
		)
		if stErr != nil {
			t.Fatalf("WithDetails: %v", stErr)
		}
		_, _ = mw(func(context.Context, any) (any, error) {
			return nil, st.Err()
		})(ctx, req)
		ev2 := rec.Events()[1]
		if ev2.Extensions()[ExtErrorCode] != "USER_ERROR_REASON_INVALID_STATE" ||
			ev2.Extensions()[ExtErrorMessage] != "account already disabled" {
			t.Fatalf("gRPC ErrorInfo extensions = %#v", ev2.Extensions())
		}

		rawDBErr := errors.New("pq: authentication failed for postgres at 10.0.1.9:5432")
		_, gotDBErr := mw(func(context.Context, any) (any, error) {
			return nil, rawDBErr
		})(ctx, req)
		if !errors.Is(gotDBErr, rawDBErr) {
			t.Fatalf("gotDBErr = %v, want %v", gotDBErr, rawDBErr)
		}
		ev3 := rec.Events()[2]
		if ev3.Extensions()[ExtErrorCode] != "INTERNAL" ||
			strings.Contains(ev3.Extensions()[ExtErrorMessage].(string), "postgres") {
			t.Fatalf("raw DB details leaked into errormessage: %#v", ev3.Extensions())
		}
	})

	t.Run("DISABLED read endpoint skips 200 OK but audits 403 PERMISSION_DENIED", func(t *testing.T) {
		rec := &recordingAuditor{}
		mw := Server(rec, security.WithRulesFuncs(rulesFn))
		ctx := serverCtx(listOp, security.Actor{Type: security.ActorTypeHuman, ID: "viewer-1"})

		if _, err := mw(func(context.Context, any) (any, error) {
			return &userpb.ListUsersResponse{}, nil
		})(ctx, &userpb.ListUsersRequest{}); err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if len(rec.Events()) != 0 {
			t.Fatalf("expected 0 events on 200 OK DISABLED method, got %d", len(rec.Events()))
		}

		deniedErr := securityerrorspb.ErrorSecurityErrorReasonPermissionDenied("permission denied")
		_, gotErr := mw(func(context.Context, any) (any, error) {
			return nil, deniedErr
		})(ctx, &userpb.ListUsersRequest{})
		if !securityerrorspb.IsSecurityErrorReasonPermissionDenied(gotErr) {
			t.Fatalf("gotErr = %v, want PERMISSION_DENIED", gotErr)
		}
		if len(rec.Events()) != 1 {
			t.Fatalf("expected 1 authz denied event on 403 DISABLED method, got %d", len(rec.Events()))
		}
		ev := rec.Events()[0]
		if ev.Type() != AuthzDeniedEventType ||
			ev.Extensions()[ExtCategory] != CategoryAuthz ||
			ev.Extensions()[ExtOutcome] != OutcomeFailure ||
			ev.Extensions()[ExtErrorCode] != "SECURITY_ERROR_REASON_PERMISSION_DENIED" ||
			ev.Extensions()[ExtActorID] != "viewer-1" {
			t.Fatalf("unexpected 403 denied event: type=%q exts=%#v", ev.Type(), ev.Extensions())
		}
	})

	t.Run("auditor Emit failure never blocks or alters business response", func(t *testing.T) {
		rec := &recordingAuditor{err: errors.New("kafka broker unreachable")}
		mw := Server(rec, security.WithRulesFuncs(rulesFn))
		ctx := serverCtx(disableOp, security.Actor{Type: security.ActorTypeHuman, ID: "admin-1"})

		wantResp := &userpb.DisableUserResponse{User: &userpb.User{Name: "users/u-1"}}
		gotResp, gotErr := mw(func(context.Context, any) (any, error) {
			return wantResp, nil
		})(ctx, &userpb.DisableUserRequest{Name: "users/u-1"})
		if gotErr != nil {
			t.Fatalf("business call failed due to audit error: %v", gotErr)
		}
		if !proto.Equal(gotResp.(proto.Message), wantResp) {
			t.Fatalf("gotResp = %v, want %v", gotResp, wantResp)
		}
	})
}

func TestNewAndStdlibBackendsNoopStdoutLogMulti(t *testing.T) {
	// 1. Disabled or nil AuditEmitter -> Noop
	disabled, err := New(nil, nil)
	if err != nil {
		t.Fatalf("New(nil) error = %v", err)
	}
	if err := disabled.Emit(context.Background(), NewEvent(context.Background())); err != nil {
		t.Fatalf("Noop Emit() error = %v", err)
	}

	// 2. Stdout + Log + Multi fanout
	var stdoutBuf, logBuf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logBuf, nil))
	logAud, err := New(&auditpb.AuditEmitter{Enabled: true, Backend: new("log")}, logger)
	if err != nil {
		t.Fatalf("New(log) error = %v", err)
	}
	failing := &recordingAuditor{err: errors.New("sink failed")}
	fanout := Multi(NewStdout(&stdoutBuf), nil, logAud, failing)

	ev := NewEvent(context.Background(), WithSubject("/plateau.iam.authn.v1.AuthnService/Login"), WithCategory(CategoryAuthn))
	if emitErr := fanout.Emit(context.Background(), ev); !errors.Is(emitErr, failing.err) {
		t.Fatalf("Multi Emit() error = %v, want %v", emitErr, failing.err)
	}
	if !strings.HasSuffix(stdoutBuf.String(), "\n") || !strings.Contains(stdoutBuf.String(), ev.ID()) {
		t.Fatalf("unexpected stdout output: %s", stdoutBuf.String())
	}
	if !strings.Contains(logBuf.String(), `"audit_event"`) || !strings.Contains(logBuf.String(), `"cloudevent"`) {
		t.Fatalf("unexpected slog output: %s", logBuf.String())
	}

	// 3. Registered external backend via WithBackend and unknown backend rejection
	rec := &recordingAuditor{}
	var gotTopic string
	extAud, err := New(
		&auditpb.AuditEmitter{Enabled: true, Backend: new("custom")},
		logger,
		WithBackend("custom", func(cfg *auditpb.AuditEmitter) (Auditor, error) {
			gotTopic = cfg.GetTopic()
			return rec, nil
		}),
	)
	if err != nil {
		t.Fatalf("New(custom) error = %v", err)
	}
	if gotTopic != "plateau.audit.events" {
		t.Fatalf("Apply() default topic = %q, want plateau.audit.events", gotTopic)
	}
	if err := extAud.Emit(context.Background(), ev); err != nil || len(rec.Events()) != 1 {
		t.Fatalf("custom backend Emit err = %v, events = %d", err, len(rec.Events()))
	}

	if _, err := New(&auditpb.AuditEmitter{Enabled: true, Backend: new("unknown")}, nil); err == nil {
		t.Fatal("New(unknown) error = nil, want error")
	}
}

func serverCtx(operation string, actor security.Actor) context.Context {
	app := kratos.New(kratos.Name("iam.service"))
	ctx := kratos.NewContext(context.Background(), app)
	ctx = security.WithActor(ctx, actor)
	return transport.NewServerContext(ctx, &fakeGRPCTransport{
		operation: operation,
		headers:   mapHeader{"X-Request-Id": "req-1"},
	})
}

type recordingAuditor struct {
	mu     sync.Mutex
	events []cloudevents.Event
	err    error
}

func (r *recordingAuditor) Emit(_ context.Context, event cloudevents.Event) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.err != nil {
		return r.err
	}
	r.events = append(r.events, event)
	return nil
}

func (r *recordingAuditor) Events() []cloudevents.Event {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]cloudevents.Event, len(r.events))
	copy(out, r.events)
	return out
}

type mapHeader map[string]string

func (m mapHeader) Get(key string) string {
	for k, v := range m {
		if strings.EqualFold(k, key) {
			return v
		}
	}
	return ""
}
func (m mapHeader) Set(key, value string) { m[key] = value }
func (m mapHeader) Add(key, value string) { m[key] = value }
func (m mapHeader) Keys() []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}
func (m mapHeader) Values(key string) []string {
	if v := m.Get(key); v != "" {
		return []string{v}
	}
	return nil
}

type fakeGRPCTransport struct {
	operation string
	headers   mapHeader
}

func (f *fakeGRPCTransport) Kind() transport.Kind            { return transport.KindGRPC }
func (f *fakeGRPCTransport) Endpoint() string                { return "grpc://127.0.0.1:10001" }
func (f *fakeGRPCTransport) Operation() string               { return f.operation }
func (f *fakeGRPCTransport) RequestHeader() transport.Header { return f.headers }
func (f *fakeGRPCTransport) ReplyHeader() transport.Header   { return mapHeader{} }

type fakeHTTPTransport struct {
	operation string
	req       *http.Request
}

var _ khttp.Transporter = (*fakeHTTPTransport)(nil)

func (f *fakeHTTPTransport) Kind() transport.Kind            { return transport.KindHTTP }
func (f *fakeHTTPTransport) Endpoint() string                { return "http://127.0.0.1:10010" }
func (f *fakeHTTPTransport) Operation() string               { return f.operation }
func (f *fakeHTTPTransport) RequestHeader() transport.Header { return httpHeaderWrap{f.req.Header} }
func (f *fakeHTTPTransport) ReplyHeader() transport.Header   { return mapHeader{} }
func (f *fakeHTTPTransport) Request() *http.Request          { return f.req }
func (f *fakeHTTPTransport) PathTemplate() string            { return "/v1/admin/users" }

type httpHeaderWrap struct{ http.Header }

func (h httpHeaderWrap) Keys() []string {
	keys := make([]string, 0, len(h.Header))
	for k := range h.Header {
		keys = append(keys, k)
	}
	return keys
}
