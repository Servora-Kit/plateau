package data

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	kafkapb "github.com/Servora-Kit/servora/api/gen/go/servora/infra/kafka/v1"
	auditconfpb "github.com/Servora-Kit/servora/api/gen/go/servora/obs/audit/v1"
	auditkafka "github.com/Servora-Kit/servora/obs/audit/kafka"
	cloudevents "github.com/cloudevents/sdk-go/v2"
	"github.com/twmb/franz-go/pkg/kgo"
)

const defaultTopic = "servora.audit.events"
const defaultConsumerGroup = "audit-consumer"

func DefaultTopic(cfg *auditconfpb.AuditContract) string {
	if cfg != nil && cfg.GetTopic() != "" {
		return cfg.GetTopic()
	}
	return defaultTopic
}

func DefaultConsumerGroup(cfg *kafkapb.Kafka) string {
	if cfg != nil && cfg.GetConsumerGroup() != "" {
		return cfg.GetConsumerGroup()
	}
	return defaultConsumerGroup
}

// Consumer polls Kafka audit records and routes decoded CloudEvents to the BatchWriter.
type Consumer struct {
	client *kgo.Client
	writer *BatchWriter
	log    *slog.Logger
	topic  string
	group  string
	cancel context.CancelFunc
	done   chan struct{}
}

func NewConsumer(client *kgo.Client, writer *BatchWriter, kafkaCfg *kafkapb.Kafka, auditCfg *auditconfpb.AuditContract, l *slog.Logger) *Consumer {
	topic := DefaultTopic(auditCfg)
	group := DefaultConsumerGroup(kafkaCfg)

	log := l.With("scope", "consumer/data/audit")
	log.Info("audit consumer configured", "topic", topic, "group", group)

	return &Consumer{
		client: client,
		writer: writer,
		log:    log,
		topic:  topic,
		group:  group,
		done:   make(chan struct{}),
	}
}

func (c *Consumer) Start(ctx context.Context) error {
	c.writer.Start(ctx)
	if c.client == nil {
		c.log.Warn("kafka client not configured, audit consumer is disabled")
		close(c.done)
		return nil
	}

	pollCtx, cancel := context.WithCancel(ctx)
	c.cancel = cancel
	go c.poll(pollCtx)
	c.log.Info("subscribed to audit topic", "topic", c.topic, "group", c.group)
	return nil
}

func (c *Consumer) Stop(_ context.Context) error {
	if c.cancel != nil {
		c.cancel()
	}
	if c.client != nil {
		c.client.Close()
		<-c.done
	}
	c.writer.Stop()
	return nil
}

func (c *Consumer) poll(ctx context.Context) {
	defer close(c.done)
	for {
		fetches := c.client.PollFetches(ctx)
		for _, fetchErr := range fetches.Errors() {
			if errors.Is(fetchErr.Err, context.Canceled) {
				return
			}
			c.log.WarnContext(ctx, "kafka fetch error", "topic", fetchErr.Topic, "partition", fetchErr.Partition, "err", fetchErr.Err)
		}
		if ctx.Err() != nil {
			return
		}
		fetches.EachRecord(func(record *kgo.Record) {
			if err := c.handle(ctx, record); err != nil {
				c.log.WarnContext(ctx, "failed to handle Kafka record", "err", err)
			}
		})
	}
}

func (c *Consumer) handle(ctx context.Context, record *kgo.Record) error {
	ce, err := auditkafka.DecodeRecord(record)
	if err != nil {
		c.log.WarnContext(ctx, "failed to decode CloudEvent", "err", err)
		_ = c.client.CommitRecords(ctx, record)
		return nil
	}

	if err := validateEvent(ce); err != nil {
		c.log.WarnContext(ctx, "invalid CloudEvent", "err", err)
		_ = c.client.CommitRecords(ctx, record)
		return nil
	}

	c.writer.Add(ce, record)
	return nil
}

func validateEvent(e *cloudevents.Event) error {
	if e.ID() == "" {
		return fmt.Errorf("validation: missing id")
	}
	if e.Type() == "" {
		return fmt.Errorf("validation: missing type")
	}
	if e.Source() == "" {
		return fmt.Errorf("validation: missing source")
	}
	if e.Time().IsZero() {
		return fmt.Errorf("validation: missing time")
	}
	return nil
}
