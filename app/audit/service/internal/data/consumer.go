package data

import (
	"context"
	"fmt"

	auditv1 "github.com/Servora-Kit/servora/api/gen/go/servora/audit/v1"
	conf "github.com/Servora-Kit/servora/api/gen/go/servora/conf/v1"
	"github.com/Servora-Kit/servora/infra/broker"
	"github.com/Servora-Kit/servora/obs/logging"
	"google.golang.org/protobuf/proto"
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

// NewConsumer creates a new Consumer. The topic comes from conf.App.Audit.topic,
// the consumer group from conf.Data.Kafka.consumer_group. Both fall back to
// hardcoded defaults when unset.
func NewConsumer(b broker.Broker, writer *BatchWriter, dataCfg *conf.Data, appCfg *conf.App, l logger.Logger) *Consumer {
	topic := defaultTopic
	group := defaultConsumerGroup

	if appCfg != nil && appCfg.Audit != nil && appCfg.Audit.Topic != "" {
		topic = appCfg.Audit.Topic
	}
	if dataCfg != nil && dataCfg.Kafka != nil && dataCfg.Kafka.ConsumerGroup != "" {
		group = dataCfg.Kafka.ConsumerGroup
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

	var auditEvt auditv1.AuditEvent
	if err := proto.Unmarshal(msg.Body, &auditEvt); err != nil {
		c.log.WithContext(ctx).Warnf("failed to unmarshal audit event: %v", err)
		_ = evt.Ack() // skip bad messages
		return nil
	}

	if err := validateEvent(&auditEvt); err != nil {
		c.log.WithContext(ctx).Warnf("invalid audit event: %v", err)
		_ = evt.Ack()
		return nil
	}

	c.writer.Add(&auditEvt, evt)
	return nil
}

// validateEvent checks required fields.
func validateEvent(e *auditv1.AuditEvent) error {
	if e.EventId == "" {
		return errorf("missing event_id")
	}
	if e.EventType == 0 {
		return errorf("missing event_type")
	}
	if e.OccurredAt == nil {
		return errorf("missing occurred_at")
	}
	if e.Service == "" {
		return errorf("missing service")
	}
	return nil
}

func errorf(msg string) error {
	return &validationError{msg: msg}
}

type validationError struct{ msg string }

func (e *validationError) Error() string { return "validation: " + e.msg }
