package server

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io"
	"log/slog"
	"net"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	auditconfv1 "github.com/Servora-Kit/plateau/api/gen/go/audit/conf/v1"
	auditeventpb "github.com/Servora-Kit/plateau/api/gen/go/audit/event/v1"
	kafkapb "github.com/Servora-Kit/plateau/api/gen/go/plateau/infra/kafka/v1"
	"github.com/Servora-Kit/plateau/app/audit/service/internal/biz"
	"github.com/Servora-Kit/plateau/app/audit/service/internal/service"
	"github.com/Servora-Kit/plateau/security/audit"
	auditkafka "github.com/Servora-Kit/plateau/security/audit/kafka"
	cloudevents "github.com/cloudevents/sdk-go/v2"
	"github.com/twmb/franz-go/pkg/kgo"
	"google.golang.org/protobuf/types/known/durationpb"
)

type captureRepo struct {
	mu        sync.Mutex
	events    []*auditeventpb.Event
	failFirst atomic.Bool
	attempts  atomic.Int32
}

func (c *captureRepo) SaveBatch(_ context.Context, batch []*auditeventpb.Event) error {
	c.attempts.Add(1)
	if c.failFirst.CompareAndSwap(true, false) {
		return errors.New("simulated transient clickhouse write error")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.events = append(c.events, batch...)
	return nil
}

func (c *captureRepo) GetByID(context.Context, string) (*auditeventpb.Event, error) {
	return nil, biz.ErrEventNotFound
}

func (c *captureRepo) List(context.Context, biz.ListEventsFilter) ([]*auditeventpb.Event, error) {
	return nil, nil
}

func (c *captureRepo) ListServices(context.Context) ([]string, error) {
	return nil, nil
}

func (c *captureRepo) snapshot() []*auditeventpb.Event {
	c.mu.Lock()
	defer c.mu.Unlock()
	cp := make([]*auditeventpb.Event, len(c.events))
	copy(cp, c.events)
	return cp
}

func TestNewKafkaServerFailFast(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	uc, _ := biz.NewEventUsecase(&captureRepo{}, logger)
	svc, _ := service.NewEventService(uc)

	if _, err := NewKafkaServer(nil, &auditconfv1.Audit{}, svc, logger); err == nil {
		t.Fatal("NewKafkaServer(nil kafkaCfg) succeeded, want error")
	}
	if _, err := NewKafkaServer(&kafkapb.Kafka{}, &auditconfv1.Audit{}, svc, logger); err == nil {
		t.Fatal("NewKafkaServer(empty brokers) succeeded, want error")
	}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	deadAddr := ln.Addr().String()
	_ = ln.Close()

	if _, err := NewKafkaServer(&kafkapb.Kafka{
		Brokers:       []string{deadAddr},
		ConsumerGroup: "audit-consumer",
		DialTimeout:   durationpb.New(300 * time.Millisecond),
	}, &auditconfv1.Audit{}, svc, logger); err == nil {
		t.Fatal("NewKafkaServer(unreachable broker) succeeded, want error")
	}
}

func TestKafkaServerDualModeDecodePoisonSkipAndRetryIntegration(t *testing.T) {
	broker := os.Getenv("AUDIT_TEST_KAFKA_BROKER")
	if broker == "" {
		broker = "127.0.0.1:29092"
	}
	probe, err := net.DialTimeout("tcp", broker, 500*time.Millisecond)
	if err != nil {
		t.Skipf("skipping Kafka integration test (%s not reachable: %v)", broker, err)
	}
	_ = probe.Close()

	var suffix [6]byte
	if _, err := rand.Read(suffix[:]); err != nil {
		t.Fatalf("rand.Read: %v", err)
	}
	tag := hex.EncodeToString(suffix[:])
	topic := "plateau.audit.it." + tag
	group := "audit-it-" + tag

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	repo := &captureRepo{}
	repo.failFirst.Store(true)

	uc, err := biz.NewEventUsecase(repo, logger)
	if err != nil {
		t.Fatalf("NewEventUsecase: %v", err)
	}
	svc, err := service.NewEventService(uc)
	if err != nil {
		t.Fatalf("NewEventService: %v", err)
	}

	batchSize := int32(10)
	ks, err := NewKafkaServer(
		&kafkapb.Kafka{
			Brokers:       []string{broker},
			ClientId:      "audit-it-consumer",
			ConsumerGroup: group,
			DialTimeout:   durationpb.New(5 * time.Second),
			RetryBackoff:  durationpb.New(100 * time.Millisecond),
		},
		&auditconfv1.Audit{
			Topic:         &topic,
			BatchSize:     &batchSize,
			FlushInterval: durationpb.New(150 * time.Millisecond),
		},
		svc,
		logger,
	)
	if err != nil {
		t.Skipf("skipping Kafka integration test (broker not ready: %v)", err)
	}

	producer, err := kgo.NewClient(
		kgo.SeedBrokers(broker),
		kgo.AllowAutoTopicCreation(),
	)
	if err != nil {
		t.Fatalf("kgo.NewClient(producer): %v", err)
	}
	t.Cleanup(producer.Close)

	now := time.Now().UTC().Truncate(time.Microsecond)
	makeCE := func(id string) cloudevents.Event {
		ev := cloudevents.NewEvent()
		ev.SetID(id)
		ev.SetType(audit.DefaultRPCEventType)
		ev.SetSource("iam.service")
		ev.SetSubject("/iam.authn.v1.AuthnService/Login")
		ev.SetTime(now)
		ev.SetExtension(audit.ExtCategory, audit.CategoryAuthn)
		ev.SetExtension(audit.ExtActorType, "human")
		ev.SetExtension(audit.ExtActorID, "u-"+id)
		ev.SetExtension(audit.ExtOutcome, audit.OutcomeSuccess)
		_ = ev.SetData(cloudevents.ApplicationJSON, []byte(`{"ok":true}`))
		return ev
	}

	binRec, err := auditkafka.EncodeRecord(topic, makeCE("bin-1"), auditkafka.BinaryMode)
	if err != nil {
		t.Fatalf("EncodeRecord(binary): %v", err)
	}
	structRec, err := auditkafka.EncodeRecord(topic, makeCE("struct-1"), auditkafka.StructuredJSONMode)
	if err != nil {
		t.Fatalf("EncodeRecord(structured): %v", err)
	}
	poisonRec := &kgo.Record{
		Topic: topic,
		Key:   []byte("poison"),
		Value: []byte("{corrupted-json"),
		Headers: []kgo.RecordHeader{{
			Key:   auditkafka.ContentTypeHeader,
			Value: []byte(auditkafka.CloudEventsJSONContentType),
		}},
	}

	if err := producer.ProduceSync(context.Background(), poisonRec, binRec, structRec).FirstErr(); err != nil {
		t.Fatalf("ProduceSync: %v", err)
	}

	srvCtx, cancelSrv := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() { errCh <- ks.Start(srvCtx) }()

	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		got := repo.snapshot()
		if len(got) >= 2 {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	stopCtx, cancelStop := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelStop()
	if err := ks.Stop(stopCtx); err != nil {
		t.Fatalf("ks.Stop: %v", err)
	}
	cancelSrv()
	if startErr := <-errCh; startErr != nil && !errors.Is(startErr, context.Canceled) {
		t.Fatalf("ks.Start: %v", startErr)
	}

	got := repo.snapshot()
	if len(got) != 2 {
		t.Fatalf("persisted events = %d, want 2 (attempts=%d, events=%+v)", len(got), repo.attempts.Load(), got)
	}
	if repo.attempts.Load() < 2 {
		t.Fatalf("SaveBatch attempts = %d, want >= 2 after transient write failure retry", repo.attempts.Load())
	}
}
