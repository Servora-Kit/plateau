package kafka

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"reflect"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	kafkapb "github.com/Servora-Kit/plateau/api/gen/go/plateau/infra/kafka/v1"
	"github.com/twmb/franz-go/pkg/kgo"
)

func TestNewServerEnforcesDisableAutoCommitAndValidatesOptions(t *testing.T) {
	cfg := &kafkapb.Kafka{
		Brokers:       []string{"127.0.0.1:9092"},
		ConsumerGroup: "audit-group",
	}

	noopHandler := func(context.Context, []*kgo.Record) error { return nil }
	srv, err := NewServer(
		cfg,
		WithLogger(testLogger()),
		WithTopicHandler("topic.b", noopHandler),
		WithTopicHandler("topic.a", noopHandler),
	)
	if err != nil {
		t.Fatalf("NewServer() error = %v", err)
	}
	defer func() { _ = srv.Stop(context.Background()) }()

	rawClient, ok := srv.client.(*kgo.Client)
	if !ok {
		t.Fatalf("expected *kgo.Client, got %T", srv.client)
	}
	if disabled, _ := rawClient.OptValue(kgo.DisableAutoCommit).(bool); !disabled {
		t.Fatalf("DisableAutoCommit = %v, want true", rawClient.OptValue(kgo.DisableAutoCommit))
	}
	if got := rawClient.OptValue(kgo.ConsumerGroup); got != "audit-group" {
		t.Fatalf("ConsumerGroup = %v, want audit-group", got)
	}
	wantTopics := []string{"topic.a", "topic.b"}
	gotTopics := rawClient.GetConsumeTopics()
	slices.Sort(gotTopics)
	if !reflect.DeepEqual(gotTopics, wantTopics) {
		t.Fatalf("GetConsumeTopics() = %v, want %v", gotTopics, wantTopics)
	}

	if _, autoCommitErr := NewServer(
		cfg,
		WithClientOpts(kgo.AutoCommitMarks()),
		WithTopicHandler("topic.a", noopHandler),
	); autoCommitErr == nil || !strings.Contains(autoCommitErr.Error(), "autocommit") {
		t.Fatalf("NewServer(AutoCommitMarks) error = %v, want autocommit conflict error", autoCommitErr)
	}
	errCases := []struct {
		name string
		cfg  *kafkapb.Kafka
		opts []ServerOption
		want string
	}{
		{
			name: "nil config",
			cfg:  nil,
			want: "config must not be nil",
		},
		{
			name: "missing consumer group",
			cfg:  &kafkapb.Kafka{Brokers: []string{"127.0.0.1:9092"}},
			opts: []ServerOption{WithTopicHandler("t", noopHandler)},
			want: "consumer_group is required",
		},
		{
			name: "no topic handlers",
			cfg:  cfg,
			want: "at least one topic handler",
		},
		{
			name: "empty topic",
			cfg:  cfg,
			opts: []ServerOption{WithTopicHandler(" ", noopHandler)},
			want: "topic name must not be empty",
		},
		{
			name: "nil handler",
			cfg:  cfg,
			opts: []ServerOption{WithTopicHandler("t", nil)},
			want: "must not be nil",
		},
		{
			name: "duplicate topic",
			cfg:  cfg,
			opts: []ServerOption{
				WithTopicHandler("t", noopHandler),
				WithTopicHandler("t", noopHandler),
			},
			want: "duplicate handler",
		},
	}

	for _, tc := range errCases {
		t.Run(tc.name, func(t *testing.T) {
			_, gotErr := NewServer(tc.cfg, tc.opts...)
			if gotErr == nil || !strings.Contains(gotErr.Error(), tc.want) {
				t.Fatalf("NewServer() error = %v, want containing %q", gotErr, tc.want)
			}
		})
	}
}

func TestServerMultiTopicBatchSizeAndFlushInterval(t *testing.T) {
	var (
		mu       sync.Mutex
		gotA     []string
		gotB     []string
		batchACh = make(chan struct{}, 2)
		batchBCh = make(chan struct{}, 2)
	)

	fake := newFakeConsumerClient()
	srv := newTestServer(t, fake,
		WithBatchSize(2),
		WithFlushInterval(40*time.Millisecond),
		WithTopicHandler("topic.a", func(_ context.Context, records []*kgo.Record) error {
			mu.Lock()
			for _, r := range records {
				gotA = append(gotA, string(r.Value))
			}
			mu.Unlock()
			batchACh <- struct{}{}
			return nil
		}),
		WithTopicHandler("topic.b", func(_ context.Context, records []*kgo.Record) error {
			mu.Lock()
			for _, r := range records {
				gotB = append(gotB, string(r.Value))
			}
			mu.Unlock()
			batchBCh <- struct{}{}
			return nil
		}),
	)

	startErrCh := make(chan error, 1)
	go func() { startErrCh <- srv.Start(context.Background()) }()

	// Push 2 records to topic.a (hits batch_size=2 immediately) and 1 record to topic.b (waits for flush_interval).
	fake.push(
		&kgo.Record{Topic: "topic.a", Partition: 0, Offset: 0, Value: []byte("a0")},
		&kgo.Record{Topic: "topic.b", Partition: 0, Offset: 0, Value: []byte("b0")},
		&kgo.Record{Topic: "topic.a", Partition: 0, Offset: 1, Value: []byte("a1")},
	)

	waitSignal(t, batchACh, "topic.a batch_size flush")
	waitSignal(t, batchBCh, "topic.b flush_interval flush")

	if err := srv.Stop(context.Background()); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
	if err := <-startErrCh; err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if !reflect.DeepEqual(gotA, []string{"a0", "a1"}) {
		t.Fatalf("topic.a records = %v, want [a0 a1]", gotA)
	}
	if !reflect.DeepEqual(gotB, []string{"b0"}) {
		t.Fatalf("topic.b records = %v, want [b0]", gotB)
	}
	if got := fake.committedValues(); !reflect.DeepEqual(got, []string{"a0", "a1", "b0"}) {
		t.Fatalf("committed records = %v, want [a0 a1 b0]", got)
	}
}

func TestServerPreservesUncommittedOffsetsOnFailureAndRetries(t *testing.T) {
	var attempts atomic.Int32
	succeededCh := make(chan []string, 1)

	fake := newFakeConsumerClient()
	srv := newTestServer(t, fake,
		WithBatchSize(2),
		WithFlushInterval(time.Hour),
		WithRetryBackoff(25*time.Millisecond),
		WithTopicHandler("audit.events", func(_ context.Context, records []*kgo.Record) error {
			n := attempts.Add(1)
			if n == 1 {
				return errors.New("clickhouse unavailable")
			}
			vals := make([]string, 0, len(records))
			for _, r := range records {
				vals = append(vals, string(r.Value))
			}
			succeededCh <- vals
			return nil
		}),
	)

	startErrCh := make(chan error, 1)
	go func() { startErrCh <- srv.Start(context.Background()) }()

	fake.push(
		&kgo.Record{Topic: "audit.events", Partition: 0, Offset: 10, Value: []byte("ev-10")},
		&kgo.Record{Topic: "audit.events", Partition: 0, Offset: 11, Value: []byte("ev-11")},
	)

	select {
	case got := <-succeededCh:
		if !reflect.DeepEqual(got, []string{"ev-10", "ev-11"}) {
			t.Fatalf("retried batch = %v, want [ev-10 ev-11]", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for retry batch")
	}

	if err := srv.Stop(context.Background()); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
	if err := <-startErrCh; err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	if attempts.Load() != 2 {
		t.Fatalf("attempts = %d, want 2", attempts.Load())
	}
	// Offsets must be committed exactly once (after attempt 2 succeeded), never on attempt 1.
	if got := fake.committedValues(); !reflect.DeepEqual(got, []string{"ev-10", "ev-11"}) {
		t.Fatalf("committed values = %v, want [ev-10 ev-11]", got)
	}
}

func TestServerGracefulStopFlushesTailBatchBeforeClientClose(t *testing.T) {
	var (
		mu            sync.Mutex
		flushedInStop []string
		closedAtFlush bool
	)

	fake := newFakeConsumerClient()
	enqueuedCh := make(chan struct{})
	fake.onPolled = func() {
		select {
		case <-enqueuedCh:
		default:
			close(enqueuedCh)
		}
	}

	srv := newTestServer(t, fake,
		WithBatchSize(10),               // higher than pushed count so only Stop flushes
		WithFlushInterval(10*time.Hour), // disable periodic timer during test
		WithStopTimeout(2*time.Second),
		WithTopicHandler("audit.events", func(ctx context.Context, records []*kgo.Record) error {
			if _, hasDeadline := ctx.Deadline(); !hasDeadline {
				t.Errorf("expected tail flush context to carry deadline")
			}
			mu.Lock()
			closedAtFlush = fake.isClosed()
			for _, r := range records {
				flushedInStop = append(flushedInStop, string(r.Value))
			}
			mu.Unlock()
			return nil
		}),
	)

	startErrCh := make(chan error, 1)
	go func() { startErrCh <- srv.Start(context.Background()) }()

	fake.push(
		&kgo.Record{Topic: "audit.events", Partition: 0, Offset: 1, Value: []byte("tail-1")},
		&kgo.Record{Topic: "audit.events", Partition: 0, Offset: 2, Value: []byte("tail-2")},
	)
	waitSignal(t, enqueuedCh, "records polled")
	// Yield briefly so runLoop buffers the polled records before Stop cancels pollCtx.
	time.Sleep(15 * time.Millisecond)

	// Pass an already-canceled context's parent? No, pass a normal background context without deadline
	// so Stop applies WithStopTimeout and flushes before closing client.
	if err := srv.Stop(context.Background()); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
	if err := <-startErrCh; err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if closedAtFlush {
		t.Fatal("underlying client was closed before tail batch flush completed")
	}
	if !fake.isClosed() {
		t.Fatal("underlying client was not closed after Stop returned")
	}
	if !reflect.DeepEqual(flushedInStop, []string{"tail-1", "tail-2"}) {
		t.Fatalf("tail flushed = %v, want [tail-1 tail-2]", flushedInStop)
	}
	if got := fake.committedValues(); !reflect.DeepEqual(got, []string{"tail-1", "tail-2"}) {
		t.Fatalf("committed values = %v, want [tail-1 tail-2]", got)
	}
}

func newTestServer(t *testing.T, fake *fakeConsumerClient, opts ...ServerOption) *Server {
	t.Helper()
	cfg := &kafkapb.Kafka{
		Brokers:       []string{"127.0.0.1:9092"},
		ConsumerGroup: "test-group",
	}
	allOpts := append([]ServerOption{WithLogger(testLogger())}, opts...)
	srv, err := NewServer(cfg, allOpts...)
	if err != nil {
		t.Fatalf("NewServer() error = %v", err)
	}
	// Close the idle real client created by NewServer and bind the deterministic fake client.
	srv.client.Close()
	srv.client = fake
	return srv
}

type fakeConsumerClient struct {
	ch       chan []*kgo.Record
	onPolled func()

	mu        sync.Mutex
	committed []*kgo.Record
	closed    bool
}

func newFakeConsumerClient() *fakeConsumerClient {
	return &fakeConsumerClient{
		ch: make(chan []*kgo.Record, 8),
	}
}

func (f *fakeConsumerClient) push(records ...*kgo.Record) {
	f.ch <- records
}

func (f *fakeConsumerClient) PollFetches(ctx context.Context) kgo.Fetches {
	select {
	case <-ctx.Done():
		return kgo.Fetches{{
			Topics: []kgo.FetchTopic{{
				Topic: "canceled",
				Partitions: []kgo.FetchPartition{{
					Err: ctx.Err(),
				}},
			}},
		}}
	case records := <-f.ch:
		if f.onPolled != nil {
			f.onPolled()
		}
		byTopic := make(map[string][]*kgo.Record)
		var order []string
		for _, r := range records {
			if _, ok := byTopic[r.Topic]; !ok {
				order = append(order, r.Topic)
			}
			byTopic[r.Topic] = append(byTopic[r.Topic], r)
		}
		topics := make([]kgo.FetchTopic, 0, len(order))
		for _, topic := range order {
			topics = append(topics, kgo.FetchTopic{
				Topic: topic,
				Partitions: []kgo.FetchPartition{{
					Partition: 0,
					Records:   byTopic[topic],
				}},
			})
		}
		return kgo.Fetches{{Topics: topics}}
	}
}

func (f *fakeConsumerClient) CommitRecords(_ context.Context, rs ...*kgo.Record) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.closed {
		return errors.New("client already closed")
	}
	f.committed = append(f.committed, rs...)
	return nil
}

func (f *fakeConsumerClient) Close() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.closed = true
}

func (f *fakeConsumerClient) isClosed() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.closed
}

func (f *fakeConsumerClient) committedValues() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]string, 0, len(f.committed))
	for _, r := range f.committed {
		out = append(out, string(r.Value))
	}
	return out
}

func waitSignal(t *testing.T, ch <-chan struct{}, label string) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for %s", label)
	}
}

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}
