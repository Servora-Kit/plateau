package audit

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	security "github.com/Servora-Kit/plateau/security"
	cloudevents "github.com/cloudevents/sdk-go/v2"
	kratos "github.com/go-kratos/kratos/v3"
	kerrors "github.com/go-kratos/kratos/v3/errors"
	"github.com/go-kratos/kratos/v3/transport"
	khttp "github.com/go-kratos/kratos/v3/transport/http"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc/peer"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

const (
	// DefaultRPCEventType is the default CloudEvents type emitted by RPC middleware.
	DefaultRPCEventType = "plateau.audit.rpc.v1"
	// AuthzDeniedEventType is the CloudEvents type emitted when authorization denies access.
	AuthzDeniedEventType = "plateau.security.authz.denied.v1"

	// Control-plane audit category values.
	CategoryAuthn    = "authn"
	CategoryResource = "resource"
	CategoryAuthz    = "authz"

	// Outcome values.
	OutcomeSuccess = "success"
	OutcomeFailure = "failure"

	// CloudEvents v1.0 extension attribute names (lowercase alphanumeric).
	ExtCategory     = "category"
	ExtActorType    = "actortype"
	ExtActorID      = "actorid"
	ExtResourceType = "resourcetype"
	ExtResourceName = "resourcename"
	ExtOutcome      = "outcome"
	ExtErrorCode    = "errorcode"
	ExtErrorMessage = "errormessage"
	ExtTraceParent  = "traceparent"
	ExtTraceState   = "tracestate"
	ExtRequestID    = "requestid"
	ExtClientIP     = "clientip"
	ExtUserAgent    = "useragent"
	ExtPartitionKey = "partitionkey"
)

// EventOption customizes a CloudEvents audit event during construction.
type EventOption func(*cloudevents.Event)

// WithType sets the CloudEvents type attribute.
func WithType(t string) EventOption {
	return func(e *cloudevents.Event) {
		if t != "" {
			e.SetType(t)
		}
	}
}

// WithSource sets the CloudEvents source attribute.
func WithSource(s string) EventOption {
	return func(e *cloudevents.Event) {
		if s != "" {
			e.SetSource(s)
		}
	}
}

// WithSubject sets the CloudEvents subject attribute.
func WithSubject(s string) EventOption {
	return func(e *cloudevents.Event) {
		if s != "" {
			e.SetSubject(s)
		}
	}
}

// WithCategory sets the control-plane audit category extension.
func WithCategory(category string) EventOption {
	return func(e *cloudevents.Event) {
		if category != "" {
			e.SetExtension(ExtCategory, category)
		}
	}
}

// WithActor sets actortype and actorid from a validated security.Actor.
func WithActor(actor security.Actor) EventOption {
	return func(e *cloudevents.Event) {
		if !actor.Valid() {
			actor = security.Actor{Type: security.ActorTypeAnonymous}
		}
		e.SetExtension(ExtActorType, string(actor.Type))
		if actor.ID != "" {
			e.SetExtension(ExtActorID, actor.ID)
		} else {
			e.SetExtension(ExtActorID, nil)
		}
	}
}

// WithResource sets resourcetype and resourcename extensions when non-empty.
func WithResource(resourceType, resourceName string) EventOption {
	return func(e *cloudevents.Event) {
		if resourceType != "" {
			e.SetExtension(ExtResourceType, resourceType)
		}
		if resourceName != "" {
			e.SetExtension(ExtResourceName, resourceName)
		}
	}
}

// WithOutcome sets the outcome extension ("success" or "failure").
func WithOutcome(outcome string) EventOption {
	return func(e *cloudevents.Event) {
		if outcome != "" {
			e.SetExtension(ExtOutcome, outcome)
		}
	}
}

// WithError populates outcome="failure", errorcode, and a sanitized errormessage
// from err without leaking internal database or system error details.
func WithError(err error) EventOption {
	return func(e *cloudevents.Event) {
		if err == nil {
			e.SetExtension(ExtOutcome, OutcomeSuccess)
			return
		}
		e.SetExtension(ExtOutcome, OutcomeFailure)
		code, message := extractErrorInfo(err)
		e.SetExtension(ExtErrorCode, code)
		e.SetExtension(ExtErrorMessage, message)
	}
}

// WithPartitionKey sets the CloudEvents partitionkey extension used by Kafka
// record encoding (cleared when key is empty so encoding falls back to event.ID()).
func WithPartitionKey(key string) EventOption {
	return func(e *cloudevents.Event) {
		if key != "" {
			e.SetExtension(ExtPartitionKey, key)
		} else {
			e.SetExtension(ExtPartitionKey, nil)
		}
	}
}

// WithHTTPRequest extracts requestid, clientip, and useragent from a native
// *http.Request (useful for non-Kratos HTTP handlers such as OIDC endpoints).
func WithHTTPRequest(r *http.Request) EventOption {
	return func(e *cloudevents.Event) {
		if r == nil {
			return
		}
		if reqID := strings.TrimSpace(r.Header.Get("X-Request-Id")); reqID != "" {
			e.SetExtension(ExtRequestID, reqID)
		}
		if ua := strings.TrimSpace(r.UserAgent()); ua != "" {
			e.SetExtension(ExtUserAgent, ua)
		}
		ip := forwardedIP(r.Header.Get("X-Forwarded-For"), r.Header.Get("X-Real-Ip"))
		if ip == "" {
			ip = stripPort(r.RemoteAddr)
		}
		if ip != "" {
			e.SetExtension(ExtClientIP, ip)
		}
	}
}

// NewEvent constructs a CloudEvents v1.0 audit event populated from ctx:
//   - UUIDv7 id and UTC timestamp
//   - "//" + appName source from Kratos AppInfo (fallback "//unknown")
//   - actortype / actorid from security.ActorFrom(ctx) (fallback "anonymous")
//   - traceparent / tracestate whenever SpanContext.IsValid(), independent of sampling
//   - requestid, clientip, and useragent from Kratos HTTP or gRPC server context
func NewEvent(ctx context.Context, opts ...EventOption) cloudevents.Event {
	if ctx == nil {
		ctx = context.Background()
	}

	e := cloudevents.NewEvent()
	e.SetSpecVersion(cloudevents.VersionV1)
	e.SetID(uuid.Must(uuid.NewV7()).String())
	e.SetTime(time.Now().UTC())
	e.SetType(DefaultRPCEventType)
	e.SetExtension(ExtOutcome, OutcomeSuccess)

	if info, ok := kratos.FromContext(ctx); ok && info != nil && info.Name() != "" {
		e.SetSource("//" + info.Name())
	} else {
		e.SetSource("//unknown")
	}

	actor, _ := security.ActorFrom(ctx)
	WithActor(actor)(&e)

	if span := trace.SpanFromContext(ctx); span != nil {
		sc := span.SpanContext()
		if sc.IsValid() {
			e.SetExtension(ExtTraceParent, fmt.Sprintf("00-%s-%s-%02x",
				sc.TraceID().String(),
				sc.SpanID().String(),
				uint8(sc.TraceFlags()),
			))
			if ts := sc.TraceState().String(); ts != "" {
				e.SetExtension(ExtTraceState, ts)
			}
		}
	}

	reqID, clientIP, userAgent := extractClientMeta(ctx)
	if reqID != "" {
		e.SetExtension(ExtRequestID, reqID)
	}
	if clientIP != "" {
		e.SetExtension(ExtClientIP, clientIP)
	}
	if userAgent != "" {
		e.SetExtension(ExtUserAgent, userAgent)
	}

	for _, opt := range opts {
		if opt != nil {
			opt(&e)
		}
	}
	return e
}

// SetJSONProtoData clones and redacts msg, then marshals it as canonical
// ProtoJSON (using proto field names) into e's data payload.
func SetJSONProtoData(e *cloudevents.Event, msg proto.Message) error {
	if e == nil || msg == nil || !msg.ProtoReflect().IsValid() {
		return nil
	}
	safe := RedactClone(msg)
	raw, err := protojson.MarshalOptions{UseProtoNames: true}.Marshal(safe)
	if err != nil {
		return err
	}
	e.SetDataSchema("type.googleapis.com/" + string(safe.ProtoReflect().Descriptor().FullName()))
	return e.SetData(cloudevents.ApplicationJSON, raw)
}

func extractErrorInfo(err error) (code, safeMsg string) {
	if errors.Is(err, context.Canceled) {
		return "CANCELED", "request canceled"
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "DEADLINE_EXCEEDED", "deadline exceeded"
	}

	se := kerrors.FromError(err)
	if se == nil {
		return "INTERNAL", "internal server error"
	}
	if se.Reason != "" && se.Reason != kerrors.UnknownReason {
		code = se.Reason
	} else if text := http.StatusText(int(se.Code)); text != "" && se.Code < 500 {
		code = strings.ReplaceAll(strings.ToUpper(text), " ", "_")
	} else if se.Code == http.StatusServiceUnavailable {
		code = "UNAVAILABLE"
	} else {
		code = "INTERNAL"
	}

	if se.Code > 0 && se.Code < 500 && se.Message != "" {
		return code, se.Message
	}
	return code, "internal server error"
}

func extractClientMeta(ctx context.Context) (requestID, clientIP, userAgent string) {
	if tr, ok := transport.FromServerContext(ctx); ok && tr != nil {
		if hdr := tr.RequestHeader(); hdr != nil {
			requestID = strings.TrimSpace(hdr.Get("X-Request-Id"))
			userAgent = strings.TrimSpace(hdr.Get("User-Agent"))
			clientIP = forwardedIP(hdr.Get("X-Forwarded-For"), hdr.Get("X-Real-Ip"))
		}
		if clientIP == "" {
			if ht, isHTTP := tr.(khttp.Transporter); isHTTP && ht.Request() != nil {
				clientIP = stripPort(ht.Request().RemoteAddr)
			}
		}
	}
	if clientIP == "" {
		if p, ok := peer.FromContext(ctx); ok && p != nil && p.Addr != nil {
			clientIP = stripPort(p.Addr.String())
		}
	}
	return requestID, clientIP, userAgent
}

func forwardedIP(xff, xri string) string {
	if xff != "" {
		first, _, _ := strings.Cut(xff, ",")
		if ip := stripPort(first); ip != "" {
			return ip
		}
	}
	return stripPort(xri)
}

func stripPort(addr string) string {
	addr = strings.TrimSpace(addr)
	if host, _, err := net.SplitHostPort(addr); err == nil {
		return host
	}
	return addr
}
