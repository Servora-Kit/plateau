package kafka

import (
	"context"
	"log/slog"
	"time"

	"github.com/twmb/franz-go/pkg/kgo"
)

const (
	defaultBatchSize     = 100
	defaultFlushInterval = time.Second
	defaultRetryBackoff  = 500 * time.Millisecond
	defaultStopTimeout   = 10 * time.Second
)

// BatchHandler processes a batch of raw Kafka records for a topic.
// Offsets for records are committed if and only if BatchHandler returns nil.
type BatchHandler func(ctx context.Context, records []*kgo.Record) error

// ServerOption configures a Kafka transport Server.
type ServerOption func(*serverOptions)

type topicRegistration struct {
	topic   string
	handler BatchHandler
}

type serverOptions struct {
	consumerGroup string
	batchSize     int
	flushInterval time.Duration
	retryBackoff  time.Duration
	stopTimeout   time.Duration
	logger        *slog.Logger
	clientOpts    []kgo.Opt
	registrations []topicRegistration
}

// WithTopicHandler registers a BatchHandler for topic.
func WithTopicHandler(topic string, handler BatchHandler) ServerOption {
	return func(o *serverOptions) {
		o.registrations = append(o.registrations, topicRegistration{
			topic:   topic,
			handler: handler,
		})
	}
}

// WithConsumerGroup overrides the consumer_group field from Kafka proto config.
func WithConsumerGroup(group string) ServerOption {
	return func(o *serverOptions) {
		o.consumerGroup = group
	}
}

// WithBatchSize sets the per-topic record count threshold that triggers an
// immediate batch flush.
func WithBatchSize(size int) ServerOption {
	return func(o *serverOptions) {
		o.batchSize = size
	}
}

// WithFlushInterval sets the periodic timer interval for flushing partial batches.
func WithFlushInterval(d time.Duration) ServerOption {
	return func(o *serverOptions) {
		o.flushInterval = d
	}
}

// WithRetryBackoff sets the delay before retrying a failed BatchHandler invocation.
func WithRetryBackoff(d time.Duration) ServerOption {
	return func(o *serverOptions) {
		o.retryBackoff = d
	}
}

// WithStopTimeout bounds the independent context used to flush buffered tail
// records during graceful Stop when the caller's stop context has no deadline.
func WithStopTimeout(d time.Duration) ServerOption {
	return func(o *serverOptions) {
		o.stopTimeout = d
	}
}

// WithLogger binds a structured logger to both the Server and underlying franz-go client.
func WithLogger(l *slog.Logger) ServerOption {
	return func(o *serverOptions) {
		o.logger = l
	}
}

// WithClientOpts appends additional franz-go client options prior to the
// Server's mandatory consumer group, topic subscription, and DisableAutoCommit options.
func WithClientOpts(opts ...kgo.Opt) ServerOption {
	return func(o *serverOptions) {
		o.clientOpts = append(o.clientOpts, opts...)
	}
}
