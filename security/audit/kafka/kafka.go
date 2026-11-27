// Package kafka provides the Kafka audit backend for security/audit and the
// CloudEvents v1.0 Kafka protocol binding (binary and structured JSON modes)
// shared by producers and consumers.
package kafka

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"time"

	auditpb "github.com/Servora-Kit/plateau/api/gen/go/plateau/security/audit/v1"
	"github.com/Servora-Kit/plateau/security/audit"
	cloudevents "github.com/cloudevents/sdk-go/v2"
	"github.com/twmb/franz-go/pkg/kgo"
)

const (
	// DefaultTopic is the platform control-plane audit Kafka topic.
	DefaultTopic = "plateau.audit.events"

	ContentTypeHeader               = "content-type"
	CloudEventsJSONContentType      = "application/cloudevents+json"
	DefaultBinaryDataContentType    = "application/octet-stream"
	CloudEventsHeaderPrefix         = "ce_"
	CloudEventsSpecVersionHeaderKey = "ce_specversion"
)

// Mode selects the CloudEvents v1.0 Kafka protocol binding format.
type Mode int

const (
	BinaryMode Mode = iota
	StructuredJSONMode
)

// Config configures a Kafka audit backend over an existing *kgo.Client whose
// connection lifecycle is managed by infra/kafka.
type Config struct {
	Topic  string
	Mode   Mode
	Logger *slog.Logger
}

// Backend returns an audit.Option that registers the "kafka" backend on audit.New.
// Connection lifecycle of client remains owned by the caller (infra/kafka).
func Backend(client *kgo.Client) audit.Option {
	return audit.WithBackend("kafka", func(cfg *auditpb.AuditEmitter, logger *slog.Logger) (audit.Auditor, error) {
		return New(client, Config{
			Topic:  cfg.GetTopic(),
			Mode:   BinaryMode,
			Logger: logger,
		})
	})
}

// NewDefault constructs an audit.Auditor from AuditEmitter configuration with
// the Kafka backend registered over client. When enabled with backend "kafka"
// and client is nil, startup fails immediately with an error.
func NewDefault(cfg *auditpb.AuditEmitter, client *kgo.Client, l *slog.Logger) (audit.Auditor, error) {
	return audit.New(cfg, l, Backend(client))
}

// New constructs a Kafka-backed audit.Auditor publishing asynchronously to
// cfg.Topic (defaulting to DefaultTopic) via non-blocking TryProduce so broker
// stalls never block business responses. The caller retains ownership of client
// and flushes/closes it on shutdown.
func New(client *kgo.Client, cfg Config) (audit.Auditor, error) {
	if client == nil {
		return nil, errors.New("audit/kafka: kgo client is nil")
	}
	if cfg.Mode != BinaryMode && cfg.Mode != StructuredJSONMode {
		return nil, fmt.Errorf("audit/kafka: unsupported mode %d", cfg.Mode)
	}
	topic := strings.TrimSpace(cfg.Topic)
	if topic == "" {
		topic = DefaultTopic
	}
	logger := cfg.Logger
	if logger == nil {
		logger = slog.Default()
	}
	return auditor{
		client: client,
		topic:  topic,
		mode:   cfg.Mode,
		logger: logger,
	}, nil
}

type auditor struct {
	client *kgo.Client
	topic  string
	mode   Mode
	logger *slog.Logger
}

func (a auditor) Emit(ctx context.Context, event cloudevents.Event) error {
	rec, err := EncodeRecord(a.topic, event, a.mode)
	if err != nil {
		return err
	}
	if ctx == nil {
		ctx = context.Background()
	}
	produceCtx := context.WithoutCancel(ctx)
	eventID := event.ID()
	operation := event.Subject()

	a.client.TryProduce(produceCtx, rec, func(_ *kgo.Record, produceErr error) {
		if produceErr == nil {
			return
		}
		a.logger.ErrorContext(produceCtx, "audit kafka produce failed",
			slog.String("topic", a.topic),
			slog.String("event_id", eventID),
			slog.String("operation", operation),
			slog.String("error", produceErr.Error()),
		)
	})
	return nil
}

// EncodeRecord encodes a CloudEvents v1.0 event into a Kafka record using the
// selected protocol binding mode.
func EncodeRecord(topic string, event cloudevents.Event, mode Mode) (*kgo.Record, error) {
	topic = strings.TrimSpace(topic)
	if topic == "" {
		return nil, errors.New("audit/kafka: topic is empty")
	}
	key := partitionKey(event)

	switch mode {
	case BinaryMode:
		headers := make([]kgo.RecordHeader, 0, 8+len(event.Extensions()))
		specVersion := event.SpecVersion()
		if specVersion == "" {
			specVersion = cloudevents.VersionV1
		}
		headers = append(headers,
			kgo.RecordHeader{Key: CloudEventsSpecVersionHeaderKey, Value: []byte(specVersion)},
			kgo.RecordHeader{Key: "ce_id", Value: []byte(event.ID())},
			kgo.RecordHeader{Key: "ce_source", Value: []byte(event.Source())},
			kgo.RecordHeader{Key: "ce_type", Value: []byte(event.Type())},
		)
		if v := event.Subject(); v != "" {
			headers = append(headers, kgo.RecordHeader{Key: "ce_subject", Value: []byte(v)})
		}
		if v := event.DataSchema(); v != "" {
			headers = append(headers, kgo.RecordHeader{Key: "ce_dataschema", Value: []byte(v)})
		}
		if t := event.Time(); !t.IsZero() {
			headers = append(headers, kgo.RecordHeader{Key: "ce_time", Value: []byte(t.UTC().Format(time.RFC3339Nano))})
		}

		exts := event.Extensions()
		extKeys := make([]string, 0, len(exts))
		for k := range exts {
			extKeys = append(extKeys, k)
		}
		slices.Sort(extKeys)
		for _, k := range extKeys {
			headers = append(headers, kgo.RecordHeader{
				Key:   CloudEventsHeaderPrefix + k,
				Value: []byte(fmt.Sprint(exts[k])),
			})
		}

		data := slices.Clone(event.Data())
		ct := event.DataContentType()
		if ct == "" && len(data) > 0 {
			ct = DefaultBinaryDataContentType
		}
		if ct != "" {
			headers = append(headers, kgo.RecordHeader{Key: ContentTypeHeader, Value: []byte(ct)})
		}
		return &kgo.Record{
			Topic:   topic,
			Key:     []byte(key),
			Value:   data,
			Headers: headers,
		}, nil

	case StructuredJSONMode:
		raw, err := json.Marshal(event)
		if err != nil {
			return nil, fmt.Errorf("audit/kafka: marshal structured event: %w", err)
		}
		return &kgo.Record{
			Topic: topic,
			Key:   []byte(key),
			Value: raw,
			Headers: []kgo.RecordHeader{
				{Key: ContentTypeHeader, Value: []byte(CloudEventsJSONContentType)},
			},
		}, nil

	default:
		return nil, fmt.Errorf("audit/kafka: unsupported mode %d", mode)
	}
}

// DecodeRecord decodes a Kafka record produced in either CloudEvents v1.0
// binary content mode or structured JSON content mode.
func DecodeRecord(record *kgo.Record) (*cloudevents.Event, error) {
	if record == nil {
		return nil, errors.New("audit/kafka: record is nil")
	}

	ct := mediaType(headerValue(record.Headers, ContentTypeHeader))
	if strings.EqualFold(ct, CloudEventsJSONContentType) {
		var ev cloudevents.Event
		if err := json.Unmarshal(record.Value, &ev); err != nil {
			return nil, fmt.Errorf("audit/kafka: unmarshal structured event: %w", err)
		}
		if ev.SpecVersion() == "" || ev.ID() == "" || ev.Source() == "" || ev.Type() == "" {
			return nil, errors.New("audit/kafka: structured event missing required attributes")
		}
		return &ev, nil
	}

	specVersion := headerValue(record.Headers, CloudEventsSpecVersionHeaderKey)
	if specVersion == "" {
		return nil, errors.New("audit/kafka: record has neither structured content-type nor ce_specversion header")
	}

	ev := cloudevents.NewEvent(specVersion)
	for _, h := range record.Headers {
		lower := strings.ToLower(h.Key)
		if !strings.HasPrefix(lower, CloudEventsHeaderPrefix) {
			continue
		}
		attr := strings.TrimPrefix(lower, CloudEventsHeaderPrefix)
		val := string(h.Value)
		switch attr {
		case "specversion":
			ev.SetSpecVersion(val)
		case "id":
			ev.SetID(val)
		case "source":
			ev.SetSource(val)
		case "type":
			ev.SetType(val)
		case "subject":
			ev.SetSubject(val)
		case "dataschema":
			ev.SetDataSchema(val)
		case "time":
			ts, err := time.Parse(time.RFC3339Nano, val)
			if err != nil {
				ts, err = time.Parse(time.RFC3339, val)
				if err != nil {
					return nil, fmt.Errorf("audit/kafka: invalid ce_time %q: %w", val, err)
				}
			}
			ev.SetTime(ts.UTC())
		default:
			if attr != "" {
				ev.SetExtension(attr, val)
			}
		}
	}

	if ev.ID() == "" || ev.Source() == "" || ev.Type() == "" {
		return nil, errors.New("audit/kafka: binary event missing required attributes")
	}
	if len(record.Value) > 0 || ct != "" {
		dataCT := headerValue(record.Headers, ContentTypeHeader)
		if dataCT == "" {
			dataCT = DefaultBinaryDataContentType
		}
		if err := ev.SetData(dataCT, slices.Clone(record.Value)); err != nil {
			return nil, fmt.Errorf("audit/kafka: set binary event data: %w", err)
		}
	}
	return &ev, nil
}

func partitionKey(event cloudevents.Event) string {
	if v, ok := event.Extensions()[audit.ExtPartitionKey]; ok {
		if s := fmt.Sprint(v); s != "" && v != nil {
			return s
		}
	}
	return event.ID()
}

func headerValue(headers []kgo.RecordHeader, key string) string {
	for _, h := range headers {
		if strings.EqualFold(h.Key, key) {
			return string(h.Value)
		}
	}
	return ""
}

func mediaType(ct string) string {
	base, _, _ := strings.Cut(ct, ";")
	return strings.TrimSpace(base)
}
