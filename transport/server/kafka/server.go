// Package kafka provides a non-generic Kratos transport.Server implementation
// for batch-oriented Kafka consumer groups with explicit cumulative offset safety.
package kafka

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"sync"
	"time"

	kafkapb "github.com/Servora-Kit/plateau/api/gen/go/plateau/infra/kafka/v1"
	infrakafka "github.com/Servora-Kit/plateau/infra/kafka"
	"github.com/go-kratos/kratos/v3/transport"
	"github.com/twmb/franz-go/pkg/kgo"
)

var _ transport.Server = (*Server)(nil)

type consumerClient interface {
	PollFetches(ctx context.Context) kgo.Fetches
	CommitRecords(ctx context.Context, rs ...*kgo.Record) error
	Close()
}

// Server consumes subscribed Kafka topics in bounded batches and commits
// partition offsets only after the topic's BatchHandler succeeds.
type Server struct {
	client        consumerClient
	handlers      map[string]BatchHandler
	topics        []string
	group         string
	batchSize     int
	flushInterval time.Duration
	retryBackoff  time.Duration
	stopTimeout   time.Duration
	log           *slog.Logger

	mu         sync.Mutex
	started    bool
	pollCancel context.CancelFunc
	stopCtx    context.Context
	done       chan struct{}
	tailErr    error
	closeOnce  sync.Once
}

// NewServer constructs a Kafka consumer transport.Server from Plateau Kafka config.
// It unconditionally enforces ConsumerGroup, ConsumeTopics, and DisableAutoCommit.
func NewServer(cfg *kafkapb.Kafka, opts ...ServerOption) (*Server, error) {
	if cfg == nil {
		return nil, fmt.Errorf("kafka server: config must not be nil")
	}

	retryDefault := defaultRetryBackoff
	if d := cfg.GetRetryBackoff(); d != nil && d.AsDuration() > 0 {
		retryDefault = d.AsDuration()
	}

	o := &serverOptions{
		consumerGroup: strings.TrimSpace(cfg.GetConsumerGroup()),
		batchSize:     defaultBatchSize,
		flushInterval: defaultFlushInterval,
		retryBackoff:  retryDefault,
		stopTimeout:   defaultStopTimeout,
	}
	for _, opt := range opts {
		if opt != nil {
			opt(o)
		}
	}

	group := strings.TrimSpace(o.consumerGroup)
	if group == "" {
		return nil, fmt.Errorf("kafka server: consumer_group is required")
	}
	if o.batchSize <= 0 {
		return nil, fmt.Errorf("kafka server: batch_size must be positive, got %d", o.batchSize)
	}
	if o.flushInterval <= 0 {
		return nil, fmt.Errorf("kafka server: flush_interval must be positive, got %s", o.flushInterval)
	}
	if o.retryBackoff < 0 {
		return nil, fmt.Errorf("kafka server: retry_backoff must not be negative, got %s", o.retryBackoff)
	}
	if o.stopTimeout <= 0 {
		return nil, fmt.Errorf("kafka server: stop_timeout must be positive, got %s", o.stopTimeout)
	}
	if len(o.registrations) == 0 {
		return nil, fmt.Errorf("kafka server: at least one topic handler is required")
	}

	handlers := make(map[string]BatchHandler, len(o.registrations))
	topics := make([]string, 0, len(o.registrations))
	for _, reg := range o.registrations {
		topic := strings.TrimSpace(reg.topic)
		if topic == "" {
			return nil, fmt.Errorf("kafka server: topic name must not be empty")
		}
		if reg.handler == nil {
			return nil, fmt.Errorf("kafka server: handler for topic %q must not be nil", topic)
		}
		if _, dup := handlers[topic]; dup {
			return nil, fmt.Errorf("kafka server: duplicate handler for topic %q", topic)
		}
		handlers[topic] = reg.handler
		topics = append(topics, topic)
	}
	slices.Sort(topics)

	log := o.logger
	if log == nil {
		log = slog.Default()
	}
	log = log.With("scope", "transport/server/kafka", "group", group)

	extra := make([]kgo.Opt, 0, len(o.clientOpts)+4)
	extra = append(extra, o.clientOpts...)
	if o.logger != nil {
		extra = append(extra, infrakafka.WithSlogLogger(o.logger))
	}
	extra = append(extra,
		kgo.ConsumerGroup(group),
		kgo.ConsumeTopics(topics...),
		kgo.DisableAutoCommit(),
	)

	kgoOpts, err := infrakafka.BuildOpts(cfg, extra...)
	if err != nil {
		return nil, err
	}
	client, err := kgo.NewClient(kgoOpts...)
	if err != nil {
		return nil, fmt.Errorf("kafka server: create client: %w", err)
	}

	return &Server{
		client:        client,
		handlers:      handlers,
		topics:        topics,
		group:         group,
		batchSize:     o.batchSize,
		flushInterval: o.flushInterval,
		retryBackoff:  o.retryBackoff,
		stopTimeout:   o.stopTimeout,
		log:           log,
		done:          make(chan struct{}),
	}, nil
}

// Start begins polling subscribed topics and blocks until Stop is invoked or
// ctx is canceled.
func (s *Server) Start(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}

	s.mu.Lock()
	if s.started {
		s.mu.Unlock()
		return fmt.Errorf("kafka server: already started")
	}
	pollCtx, pollCancel := context.WithCancel(ctx)
	s.started = true
	s.pollCancel = pollCancel
	s.mu.Unlock()

	s.log.InfoContext(ctx, "starting kafka consumer server", "topics", s.topics)

	fetchCh := make(chan []*kgo.Record, 1)
	go s.pollLoop(pollCtx, fetchCh)

	s.runLoop(pollCtx, fetchCh)
	return nil
}

// Stop halts polling, flushes buffered tail batches with an independent bounded
// context, commits offsets for succeeded batches, and closes the client.
func (s *Server) Stop(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}

	s.mu.Lock()
	started := s.started
	if started && s.stopCtx == nil {
		s.stopCtx = context.WithoutCancel(ctx)
		if deadline, hasDeadline := ctx.Deadline(); hasDeadline {
			var cancel context.CancelFunc
			s.stopCtx, cancel = context.WithDeadline(s.stopCtx, deadline)
			defer cancel()
		}
	}
	cancel := s.pollCancel
	s.mu.Unlock()

	if !started {
		s.closeClient()
		return nil
	}
	if cancel != nil {
		cancel()
	}

	select {
	case <-s.done:
	case <-ctx.Done():
		return ctx.Err()
	}

	s.mu.Lock()
	err := s.tailErr
	s.mu.Unlock()
	return err
}

func (s *Server) pollLoop(ctx context.Context, fetchCh chan<- []*kgo.Record) {
	defer close(fetchCh)
	for {
		if ctx.Err() != nil {
			return
		}
		fetches := s.client.PollFetches(ctx)
		for _, fetchErr := range fetches.Errors() {
			if errors.Is(fetchErr.Err, context.Canceled) || errors.Is(fetchErr.Err, context.DeadlineExceeded) {
				continue
			}
			s.log.WarnContext(ctx, "kafka fetch error",
				"topic", fetchErr.Topic,
				"partition", fetchErr.Partition,
				"err", fetchErr.Err,
			)
		}
		n := fetches.NumRecords()
		if n > 0 {
			records := make([]*kgo.Record, 0, n)
			fetches.EachRecord(func(r *kgo.Record) {
				if r != nil {
					records = append(records, r)
				}
			})
			if len(records) > 0 {
				fetchCh <- records
			}
		}
		if ctx.Err() != nil {
			return
		}
	}
}

func (s *Server) runLoop(ctx context.Context, fetchCh <-chan []*kgo.Record) {
	defer func() {
		s.closeClient()
		close(s.done)
	}()

	buffers := make(map[string][]*kgo.Record, len(s.topics))
	ticker := time.NewTicker(s.flushInterval)
	defer ticker.Stop()

	var (
		retryTimer *time.Timer
		retryCh    <-chan time.Time
	)
	defer func() {
		if retryTimer != nil {
			retryTimer.Stop()
		}
	}()

	armRetry := func() {
		if s.retryBackoff <= 0 || retryCh != nil {
			return
		}
		if retryTimer == nil {
			retryTimer = time.NewTimer(s.retryBackoff)
		} else {
			retryTimer.Reset(s.retryBackoff)
		}
		retryCh = retryTimer.C
	}

	for {
		select {
		case records, ok := <-fetchCh:
			if !ok {
				tailErr := s.flushAllTail(buffers)
				s.mu.Lock()
				s.tailErr = tailErr
				s.mu.Unlock()
				return
			}
			for _, rec := range records {
				if _, known := s.handlers[rec.Topic]; !known {
					continue
				}
				buffers[rec.Topic] = append(buffers[rec.Topic], rec)
				for len(buffers[rec.Topic]) >= s.batchSize {
					if err := s.flushTopicBatch(ctx, buffers, rec.Topic, s.batchSize); err != nil {
						armRetry()
						break
					}
				}
			}

		case <-ticker.C:
			for _, topic := range s.topics {
				for len(buffers[topic]) > 0 {
					limit := min(len(buffers[topic]), s.batchSize)
					if err := s.flushTopicBatch(ctx, buffers, topic, limit); err != nil {
						armRetry()
						break
					}
				}
			}

		case <-retryCh:
			retryCh = nil
			for _, topic := range s.topics {
				for len(buffers[topic]) > 0 {
					limit := min(len(buffers[topic]), s.batchSize)
					if err := s.flushTopicBatch(ctx, buffers, topic, limit); err != nil {
						armRetry()
						break
					}
				}
			}
		}
	}
}

func (s *Server) flushTopicBatch(ctx context.Context, buffers map[string][]*kgo.Record, topic string, count int) error {
	buf := buffers[topic]
	if len(buf) == 0 || count <= 0 {
		return nil
	}
	if count > len(buf) {
		count = len(buf)
	}
	batch := slices.Clone(buf[:count])
	handler := s.handlers[topic]

	if err := handler(ctx, batch); err != nil {
		s.log.ErrorContext(ctx, "kafka batch handler failed",
			"topic", topic,
			"records", len(batch),
			"err", err,
		)
		return err
	}

	if err := s.client.CommitRecords(ctx, batch...); err != nil {
		s.log.WarnContext(ctx, "kafka commit records failed",
			"topic", topic,
			"records", len(batch),
			"err", err,
		)
	}
	buffers[topic] = slices.Delete(buf, 0, count)
	return nil
}

func (s *Server) flushAllTail(buffers map[string][]*kgo.Record) error {
	baseCtx := context.Background()
	s.mu.Lock()
	if s.stopCtx != nil {
		baseCtx = s.stopCtx
	}
	s.mu.Unlock()

	flushCtx := baseCtx
	var cancel context.CancelFunc
	if _, hasDeadline := baseCtx.Deadline(); !hasDeadline {
		flushCtx, cancel = context.WithTimeout(baseCtx, s.stopTimeout)
		defer cancel()
	}

	var errs []error
	for _, topic := range s.topics {
		for len(buffers[topic]) > 0 {
			limit := min(len(buffers[topic]), s.batchSize)
			if err := s.flushTopicBatch(flushCtx, buffers, topic, limit); err != nil {
				errs = append(errs, fmt.Errorf("topic %q tail flush: %w", topic, err))
				break
			}
		}
	}
	return errors.Join(errs...)
}

func (s *Server) closeClient() {
	s.closeOnce.Do(func() {
		if s.client != nil {
			s.client.Close()
		}
	})
}
