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
	"github.com/Servora-Kit/servora/obs/audit"
	cloudevents "github.com/cloudevents/sdk-go/v2"
	"github.com/twmb/franz-go/pkg/kgo"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
)

const flushOnStopTimeout = 10 * time.Second

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
//   - source ("/pkg.Service/Method") → service (pkg.Service) + operation (full path)
//   - extensions authid/authtype/severitytext/errormessage/traceparent → actor_*/error_*/trace_id
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
		service, operation := splitOperation(e.Source())
		errMsg := extString(exts, audit.ExtErrorMessage)
		success := errMsg == "" && !strings.EqualFold(extString(exts, audit.ExtSeverityText), "ERROR")

		if err := chBatch.Append(
			e.ID(),
			e.Type(),
			e.SpecVersion(),
			e.Time(),
			service,
			operation,
			extString(exts, audit.ExtAuthID),
			extString(exts, audit.ExtAuthType),
			"", // actor_display_name — not carried in CloudEvents context
			"", // target_type — not carried in CloudEvents context
			e.Subject(),
			"", // target_name — not carried in CloudEvents context
			success,
			"", // error_code — not carried in CloudEvents context
			errMsg,
			traceIDFromTraceparent(extString(exts, audit.ExtTraceParent)),
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

// splitOperation parses an RPC operation path "/pkg.Service/Method" into
// (service="pkg.Service", operation=original). When the source does not match
// that shape both fields fall back to the raw source string.
func splitOperation(source string) (string, string) {
	if source == "" {
		return "", ""
	}
	trimmed := strings.TrimPrefix(source, "/")
	if i := strings.Index(trimmed, "/"); i > 0 {
		return trimmed[:i], source
	}
	return source, source
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
