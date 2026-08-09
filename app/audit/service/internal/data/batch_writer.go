package data

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	auditconfv1 "github.com/Servora-Kit/servora-platform/api/gen/go/audit/service/conf/v1"
	authnauditpb "github.com/Servora-Kit/servora/api/gen/go/servora/authn/audit/v1"
	authzauditpb "github.com/Servora-Kit/servora/api/gen/go/servora/authz/audit/v1"
	cloudevents "github.com/cloudevents/sdk-go/v2"
	"github.com/twmb/franz-go/pkg/kgo"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
)

const flushOnStopTimeout = 10 * time.Second

// CE extension attribute keys used for ClickHouse column projection.
// These mirror the private constants in servora/obs/audit but are defined
// locally to avoid coupling to the framework's internal implementation.
const (
	extAuthID       = "authid"
	extAuthType     = "authtype"
	extErrorMessage = "errormessage"
	extTraceParent  = "traceparent"
)

// pendingEvent bundles a decoded CloudEvent with its Kafka event handle.
type pendingEvent struct {
	event  *cloudevents.Event
	record *kgo.Record
}

type recordCommitter interface {
	CommitRecords(context.Context, ...*kgo.Record) error
}

// BatchWriter buffers CloudEvent records and flushes them to ClickHouse in batches.
type BatchWriter struct {
	data      *Data
	log       *slog.Logger
	committer recordCommitter
	batchSize int
	interval  time.Duration

	mu     sync.Mutex
	buffer []pendingEvent

	flushCh  chan struct{}
	done     chan struct{}
	stopOnce sync.Once
}

// NewBatchWriter creates a new BatchWriter using the audit service's local
// AuditConsumerConfig (batch_size + flush_interval).
func NewBatchWriter(d *Data, auditCfg *auditconfv1.AuditConsumerConfig, client *kgo.Client, l *slog.Logger) *BatchWriter {
	batchSize := 100
	interval := time.Second

	if auditCfg != nil {
		if auditCfg.GetConsumerBatchSize() > 0 {
			batchSize = int(auditCfg.GetConsumerBatchSize())
		}
		if fi := auditCfg.GetConsumerFlushInterval(); fi != nil {
			if d := fi.AsDuration(); d > 0 {
				interval = d
			}
		}
	}

	return &BatchWriter{
		data:      d,
		log:       l.With("scope", "batch_writer/data/audit"),
		committer: client,
		batchSize: batchSize,
		interval:  interval,
		buffer:    make([]pendingEvent, 0, batchSize),
		flushCh:   make(chan struct{}, 1),
		done:      make(chan struct{}),
	}
}

// Start begins the background timer flush loop.
func (w *BatchWriter) Start(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(w.interval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				w.flush(ctx)
			case <-w.flushCh:
				w.flush(ctx)
			case <-w.done:
				// Use an independent context for the final flush so that a
				// cancelled Start ctx doesn't silently drop the last batch.
				stopCtx, cancel := context.WithTimeout(context.Background(), flushOnStopTimeout)
				defer cancel()
				w.flush(stopCtx)
				return
			}
		}
	}()
}

// Stop signals the flush loop to stop and flushes remaining events.
// Safe to call multiple times.
func (w *BatchWriter) Stop() {
	w.stopOnce.Do(func() { close(w.done) })
}

// Add appends an event to the buffer. If the buffer reaches batchSize, triggers an immediate flush.
func (w *BatchWriter) Add(evt *cloudevents.Event, record *kgo.Record) {
	w.mu.Lock()
	w.buffer = append(w.buffer, pendingEvent{event: evt, record: record})
	shouldFlush := len(w.buffer) >= w.batchSize
	w.mu.Unlock()

	if shouldFlush {
		select {
		case w.flushCh <- struct{}{}:
		default:
		}
	}
}

// flush writes all buffered events to ClickHouse and Ack/Nack their Kafka handles.
//
// Column projection from CloudEvents to audit_events:
//   - id/type/specversion/time/subject → event_id/event_type/event_version/occurred_at/target_id
//   - source ("//app-name") → service (app-name)
//   - subject or type → operation
//   - typed AuthN/AuthZ payload or legacy extensions → actor/target/error columns
//   - CE type → success (type-based: authn.success/authz.allowed/rpc(no error) → true)
//   - data payload → detail column as JSON (see detailJSON)
func (w *BatchWriter) flush(ctx context.Context) {
	w.mu.Lock()
	if len(w.buffer) == 0 {
		w.mu.Unlock()
		return
	}
	batch := w.buffer
	w.buffer = make([]pendingEvent, 0, w.batchSize)
	w.mu.Unlock()

	if w.data.ClickHouse() == nil {
		w.commitAll(ctx, batch)
		return
	}

	chBatch, err := w.data.ClickHouse().PrepareBatch(ctx, "INSERT INTO audit_events")
	if err != nil {
		w.log.Warn("failed to prepare batch", "err", err)
		return
	}

	for _, p := range batch {
		e := p.event
		exts := e.Extensions()
		service := serviceFromSource(e.Source())
		operation := operationFromEvent(e)
		projection := projectEvent(e)
		success := successFromCEType(e.Type(), projection.errorMessage)

		if err := chBatch.Append(
			e.ID(),
			e.Type(),
			e.SpecVersion(),
			e.Time(),
			service,
			operation,
			projection.actorID,
			projection.actorType,
			"", // actor_display_name — not carried in framework Audit payloads
			projection.targetType,
			projection.targetID,
			"", // target_name — not carried in framework Audit payloads
			success,
			projection.errorCode,
			projection.errorMessage,
			traceIDFromTraceparent(extString(exts, extTraceParent)),
			"", // request_id — not carried in CloudEvents context
			detailJSON(e),
		); err != nil {
			w.log.Warn("append failed for event, aborting batch", "event_id", e.ID(), "err", err)
			_ = chBatch.Abort()
			return
		}
	}

	if err := chBatch.Send(); err != nil {
		w.log.Warn("failed to send batch", "err", err)
		return
	}

	w.log.Info("flushed events to ClickHouse", "count", len(batch))
	w.commitAll(ctx, batch)
}

func (w *BatchWriter) commitAll(ctx context.Context, batch []pendingEvent) {
	if w.committer == nil {
		return
	}
	records := make([]*kgo.Record, 0, len(batch))
	for _, p := range batch {
		if p.record != nil {
			records = append(records, p.record)
		}
	}
	if len(records) == 0 {
		return
	}
	if err := w.committer.CommitRecords(ctx, records...); err != nil {
		w.log.Warn("failed to commit Kafka records", "err", err)
	}
}

func extString(exts map[string]any, name string) string {
	if v, ok := exts[name]; ok {
		if s, ok := v.(string); ok {
			return s
		}
		return fmt.Sprint(v)
	}
	return ""
}

type eventProjection struct {
	actorID      string
	actorType    string
	targetType   string
	targetID     string
	errorCode    string
	errorMessage string
}

func projectEvent(event *cloudevents.Event) eventProjection {
	if event == nil {
		return eventProjection{}
	}

	switch event.Type() {
	case "servora.authn.success.v1":
		payload := new(authnauditpb.AuthnSuccess)
		if err := proto.Unmarshal(event.Data(), payload); err != nil {
			return eventProjection{}
		}
		return eventProjection{
			actorID:   payload.GetSubject(),
			actorType: payload.GetScheme(),
		}
	case "servora.authn.failure.v1":
		payload := new(authnauditpb.AuthnFailure)
		if err := proto.Unmarshal(event.Data(), payload); err != nil {
			return eventProjection{}
		}
		return eventProjection{errorCode: payload.GetReason().String()}
	case "servora.authz.allowed.v1", "servora.authz.denied.v1", "servora.authz.error.v1":
		payload := new(authzauditpb.AuthzDecision)
		if err := proto.Unmarshal(event.Data(), payload); err != nil {
			return eventProjection{}
		}
		projection := eventProjection{
			actorID:    payload.GetSubject(),
			targetType: payload.GetResourceType(),
			targetID:   payload.GetResourceId(),
		}
		if payload.GetDecision() != authzauditpb.AuthzDecision_DECISION_ALLOWED {
			projection.errorCode = payload.GetReason().String()
		}
		return projection
	default:
		extensions := event.Extensions()
		return eventProjection{
			actorID:      extString(extensions, extAuthID),
			actorType:    extString(extensions, extAuthType),
			targetID:     event.Subject(),
			errorMessage: extString(extensions, extErrorMessage),
		}
	}
}

// successFromCEType determines whether an audit event represents a successful
// operation based on its CloudEvents type. This replaces the legacy severitytext
// extension-based approach.
func successFromCEType(ceType, errMsg string) bool {
	switch ceType {
	case "servora.authn.success.v1",
		"servora.authz.allowed.v1":
		return true
	case "servora.authn.failure.v1",
		"servora.authz.denied.v1",
		"servora.authz.error.v1":
		return false
	default:
		// Generic RPC events and unknown types: treat as success when no error message.
		return errMsg == ""
	}
}

// serviceFromSource projects the CloudEvents source into the service column.
// New Servora events use source="//app-name"; legacy RPC-path sources still
// fall back to the service segment so old records remain readable.
func serviceFromSource(source string) string {
	if source == "" {
		return ""
	}
	if strings.HasPrefix(source, "//") {
		return strings.TrimPrefix(source, "//")
	}
	trimmed := strings.TrimPrefix(source, "/")
	if i := strings.Index(trimmed, "/"); i > 0 {
		return trimmed[:i]
	}
	return source
}

// operationFromEvent stores the actionable event dimension. RPC audit events
// set subject to the transport operation; non-RPC events fall back to CE type.
func operationFromEvent(e *cloudevents.Event) string {
	if e == nil {
		return ""
	}
	if subject := e.Subject(); subject != "" {
		return subject
	}
	return e.Type()
}

// traceIDFromTraceparent extracts the trace-id segment from a W3C traceparent
// header ("00-<32hex trace-id>-<16hex span-id>-<flags>"). Returns the raw input
// when the header doesn't match the expected shape so debugging stays possible.
func traceIDFromTraceparent(tp string) string {
	if tp == "" {
		return ""
	}
	parts := strings.Split(tp, "-")
	if len(parts) >= 2 && len(parts[1]) == 32 {
		return parts[1]
	}
	return tp
}

// detailJSON renders the CloudEvents data payload as a JSON string for the
// audit_events.detail column. Proto payloads are looked up via the global type
// registry and re-encoded with protojson; JSON payloads pass through; anything
// else is wrapped as a base64 envelope so the bytes survive.
func detailJSON(e *cloudevents.Event) string {
	data := e.Data()
	if len(data) == 0 {
		return "{}"
	}
	contentType := strings.ToLower(e.DataContentType())
	switch {
	case strings.HasPrefix(contentType, "application/json"),
		strings.HasPrefix(contentType, "application/cloudevents+json"):
		return string(data)
	case strings.HasPrefix(contentType, "application/protobuf"),
		strings.HasPrefix(contentType, "application/x-protobuf"):
		if s, ok := protoToJSON(e.DataSchema(), data); ok {
			return s
		}
	}
	wrapper := map[string]string{
		"_content_type": e.DataContentType(),
		"_schema":       e.DataSchema(),
		"_raw_base64":   base64.StdEncoding.EncodeToString(data),
	}
	b, err := json.Marshal(wrapper)
	if err != nil {
		return "{}"
	}
	return string(b)
}

// TODO(upstream): move to servora/core/ — this is a generic CloudEvents helper
// (dataschema URL → registered proto type → JSON), not audit-specific. Reused
// by any consumer of CloudEvents with application/protobuf payloads.
func protoToJSON(schemaURL string, data []byte) (string, bool) {
	fullName := schemaURL
	if i := strings.LastIndex(schemaURL, "/"); i >= 0 {
		fullName = schemaURL[i+1:]
	}
	if fullName == "" {
		return "", false
	}
	mt, err := protoregistry.GlobalTypes.FindMessageByName(protoreflect.FullName(fullName))
	if err != nil {
		return "", false
	}
	msg := mt.New().Interface()
	if err := proto.Unmarshal(data, msg); err != nil {
		return "", false
	}
	b, err := protojson.Marshal(msg)
	if err != nil {
		return "", false
	}
	return string(b), true
}
