package data

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	auditcontractv1 "github.com/Servora-Kit/servora/api/gen/go/servora/extra/audit/v1"
	brokerv1 "github.com/Servora-Kit/servora/api/gen/go/servora/extra/broker/v1"
	"github.com/Servora-Kit/servora/infra/broker"
	logger "github.com/Servora-Kit/servora/obs/logging"
	cloudevents "github.com/cloudevents/sdk-go/v2"
)

const defaultTopic = "servora.audit.events"
const defaultConsumerGroup = "audit-consumer"

// Consumer subscribes to Kafka audit topic and routes events to the BatchWriter.
type Consumer struct {
	broker     broker.Broker
	writer     *BatchWriter
	log        *logger.Helper
	topic      string
	group      string
	subscriber broker.Subscriber
}

// NewConsumer creates a new Consumer. The topic comes from the AuditContract
// section (servora.extra.audit.v1), the consumer group from the Broker section
// (servora.extra.broker.v1 → Kafka.consumer_group). Both fall back to
// hardcoded defaults when unset.
func NewConsumer(b broker.Broker, writer *BatchWriter, brokerCfg *brokerv1.Broker, auditCfg *auditcontractv1.AuditContract, l logger.Logger) *Consumer {
	topic := defaultTopic
	group := defaultConsumerGroup

	if auditCfg != nil && auditCfg.GetTopic() != "" {
		topic = auditCfg.GetTopic()
	}
	if k := brokerCfg.GetKafka(); k != nil && k.GetConsumerGroup() != "" {
		group = k.GetConsumerGroup()
	}

	log := logger.For(l, "consumer/data/audit")
	log.Infof("audit consumer configured: topic=%s group=%s", topic, group)

	return &Consumer{
		broker: b,
		writer: writer,
		log:    log,
		topic:  topic,
		group:  group,
	}
}

// Start subscribes to the Kafka topic and begins the BatchWriter flush loop.
func (c *Consumer) Start(ctx context.Context) error {
	if c.broker == nil {
		c.log.Warn("Kafka broker not configured, audit consumer is disabled")
		c.writer.Start(ctx)
		return nil
	}

	c.writer.Start(ctx)

	sub, err := c.broker.Subscribe(ctx, c.topic, c.handle,
		broker.WithQueue(c.group),
		broker.DisableAutoAck(),
	)
	if err != nil {
		return fmt.Errorf("subscribe to audit topic %s: %w", c.topic, err)
	}

	c.subscriber = sub
	c.log.Infof("subscribed to audit topic: %s (group: %s)", c.topic, c.group)
	return nil
}

// Stop unsubscribes and flushes remaining events.
func (c *Consumer) Stop(_ context.Context) error {
	if c.subscriber != nil {
		if err := c.subscriber.Unsubscribe(true); err != nil {
			c.log.Warnf("failed to unsubscribe: %v", err)
		}
	}
	c.writer.Stop()
	return nil
}

// handle processes a single Kafka message.
func (c *Consumer) handle(ctx context.Context, evt broker.Event) error {
	msg := evt.Message()
	if msg == nil {
		c.log.WithContext(ctx).Warn("received nil message, skipping")
		_ = evt.Ack()
		return nil
	}

	ce, err := decodeCloudEvent(msg)
	if err != nil {
		c.log.WithContext(ctx).Warnf("failed to decode CloudEvent: %v", err)
		_ = evt.Ack() // skip bad messages
		return nil
	}

	if err := validateEvent(ce); err != nil {
		c.log.WithContext(ctx).Warnf("invalid CloudEvent: %v", err)
		_ = evt.Ack()
		return nil
	}

	c.writer.Add(ce, evt)
	return nil
}

// decodeCloudEvent reconstructs a cloudevents.Event from a broker.Message using
// the CloudEvents Kafka protocol binding. Structured mode is recognised by
// content-type = application/cloudevents+json; otherwise binary mode is assumed
// where ce_* headers carry the context attributes and the body is the payload.
func decodeCloudEvent(msg *broker.Message) (*cloudevents.Event, error) {
	ct := strings.ToLower(headerLookup(msg.Headers, "content-type"))
	if strings.HasPrefix(ct, "application/cloudevents+json") {
		ev := cloudevents.NewEvent()
		if err := json.Unmarshal(msg.Body, &ev); err != nil {
			return nil, fmt.Errorf("structured cloudevent: %w", err)
		}
		return &ev, nil
	}

	ev := cloudevents.NewEvent()
	dataContentType := ""
	for k, v := range msg.Headers {
		lk := strings.ToLower(k)
		switch lk {
		case "content-type":
			dataContentType = v
		case "ce_id", "ce-id":
			ev.SetID(v)
		case "ce_source", "ce-source":
			ev.SetSource(v)
		case "ce_specversion", "ce-specversion":
			ev.SetSpecVersion(v)
		case "ce_type", "ce-type":
			ev.SetType(v)
		case "ce_subject", "ce-subject":
			ev.SetSubject(v)
		case "ce_time", "ce-time":
			if t, err := time.Parse(time.RFC3339Nano, v); err == nil {
				ev.SetTime(t)
			}
		case "ce_dataschema", "ce-dataschema":
			ev.SetDataSchema(v)
		case "ce_datacontenttype", "ce-datacontenttype":
			dataContentType = v
		default:
			if name, ok := trimCEPrefix(lk); ok {
				ev.SetExtension(name, v)
			}
		}
	}

	if ev.SpecVersion() == "" {
		ev.SetSpecVersion(cloudevents.VersionV1)
	}
	if dataContentType == "" {
		dataContentType = "application/octet-stream"
	}
	if len(msg.Body) > 0 {
		if err := ev.SetData(dataContentType, msg.Body); err != nil {
			return nil, fmt.Errorf("set data: %w", err)
		}
	}
	return &ev, nil
}

func headerLookup(h broker.Headers, key string) string {
	if h == nil {
		return ""
	}
	for k, v := range h {
		if strings.EqualFold(k, key) {
			return v
		}
	}
	return ""
}

func trimCEPrefix(lowerKey string) (string, bool) {
	switch {
	case strings.HasPrefix(lowerKey, "ce_"):
		return lowerKey[3:], true
	case strings.HasPrefix(lowerKey, "ce-"):
		return lowerKey[3:], true
	default:
		return "", false
	}
}

// validateEvent checks required CloudEvents context attributes.
func validateEvent(e *cloudevents.Event) error {
	if e.ID() == "" {
		return errors.New("validation: missing id")
	}
	if e.Type() == "" {
		return errors.New("validation: missing type")
	}
	if e.Source() == "" {
		return errors.New("validation: missing source")
	}
	if e.Time().IsZero() {
		return errors.New("validation: missing time")
	}
	return nil
}
