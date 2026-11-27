package kafka

import (
	"bytes"
	"context"
	"encoding/binary"
	"io"
	"log/slog"
	"net"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	auditpb "github.com/Servora-Kit/plateau/api/gen/go/plateau/security/audit/v1"
	"github.com/Servora-Kit/plateau/security"
	"github.com/Servora-Kit/plateau/security/audit"
	cloudevents "github.com/cloudevents/sdk-go/v2"
	"github.com/twmb/franz-go/pkg/kerr"
	"github.com/twmb/franz-go/pkg/kgo"
	"github.com/twmb/franz-go/pkg/kmsg"
)

func TestEncodeDecodeBinaryAndStructuredRoundTrip(t *testing.T) {
	original := sampleEvent(t)

	for _, tc := range []struct {
		name string
		mode Mode
	}{
		{name: "binary", mode: BinaryMode},
		{name: "structured-json", mode: StructuredJSONMode},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec, err := EncodeRecord(DefaultTopic, original, tc.mode)
			if err != nil {
				t.Fatalf("EncodeRecord() error = %v", err)
			}
			if string(rec.Key) != "actor-key-1" {
				t.Fatalf("record key = %q, want actor-key-1", string(rec.Key))
			}

			decoded, err := DecodeRecord(rec)
			if err != nil {
				t.Fatalf("DecodeRecord() error = %v", err)
			}
			if decoded.ID() != original.ID() ||
				decoded.Source() != original.Source() ||
				decoded.Type() != original.Type() ||
				decoded.Subject() != original.Subject() ||
				!decoded.Time().Equal(original.Time()) {
				t.Fatalf("decoded envelope mismatch: got %+v, want %+v", decoded, original)
			}
			if got := decoded.Extensions()[audit.ExtActorID]; got != "user-1" {
				t.Fatalf("actorid = %v, want user-1", got)
			}
			if string(decoded.Data()) != `{"name":"users/user-1"}` {
				t.Fatalf("data = %s, want {\"name\":\"users/user-1\"}", string(decoded.Data()))
			}
		})
	}

	t.Run("validation errors", func(t *testing.T) {
		if _, err := EncodeRecord("", original, BinaryMode); err == nil {
			t.Fatal("EncodeRecord(empty topic) = nil, want error")
		}
		if _, err := EncodeRecord(DefaultTopic, original, Mode(99)); err == nil {
			t.Fatal("EncodeRecord(invalid mode) = nil, want error")
		}
		if _, err := DecodeRecord(nil); err == nil {
			t.Fatal("DecodeRecord(nil) = nil, want error")
		}
		if _, err := DecodeRecord(&kgo.Record{Value: []byte("plain")}); err == nil {
			t.Fatal("DecodeRecord(non-cloudevent) = nil, want error")
		}
	})
}

func TestBackendRegistrationAndRealKgoClientEmit(t *testing.T) {
	// 1. Backend(nil) does not fail when AuditEmitter selects a stdlib backend,
	//    and returns an explicit error when AuditEmitter selects "kafka".
	if _, err := audit.New(&auditpb.AuditEmitter{Enabled: true, Backend: new("noop")}, nil, Backend(nil)); err != nil {
		t.Fatalf("audit.New(noop, Backend(nil)) error = %v", err)
	}
	if _, err := audit.New(&auditpb.AuditEmitter{Enabled: true, Backend: new("kafka")}, nil, Backend(nil)); err == nil {
		t.Fatal("audit.New(kafka, Backend(nil)) error = nil, want error")
	}

	// 2. Real *kgo.Client publishing over Kafka wire protocol via audit.New + Backend(client).
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("net.Listen: %v", err)
	}
	defer func() { _ = ln.Close() }()

	host, portStr, _ := net.SplitHostPort(ln.Addr().String())
	port, _ := strconv.Atoi(portStr)
	producedCh := make(chan *kgo.Record, 4)

	go func() {
		for {
			conn, acceptErr := ln.Accept()
			if acceptErr != nil {
				return
			}
			go serveProduceBroker(conn, host, int32(port), producedCh)
		}
	}()

	client, err := kgo.NewClient(
		kgo.SeedBrokers(ln.Addr().String()),
		kgo.DisableIdempotentWrite(),
		kgo.ProducerBatchCompression(kgo.NoCompression()),
	)
	if err != nil {
		t.Fatalf("kgo.NewClient: %v", err)
	}
	defer client.Close()

	const customTopic = "tenant.orders.audit"
	auditor, err := audit.New(
		&auditpb.AuditEmitter{
			Enabled: true,
			Backend: new("kafka"),
			Topic:   new(customTopic),
		},
		nil,
		Backend(client),
	)
	if err != nil {
		t.Fatalf("audit.New(kafka) error = %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	reqCtx, cancelReq := context.WithCancel(ctx)
	ev := audit.NewEvent(
		reqCtx,
		audit.WithSource("//iam.service"),
		audit.WithSubject("/plateau.iam.user.v1.UserService/DisableUser"),
		audit.WithCategory(audit.CategoryResource),
		audit.WithActor(security.Actor{Type: security.ActorTypeHuman, ID: "admin-1"}),
		audit.WithResource("iam.plateau.dev/User", "users/u-9"),
	)
	if err := auditor.Emit(reqCtx, ev); err != nil {
		t.Fatalf("Emit() error = %v", err)
	}
	// Canceling the request context immediately after Emit returns must not abort
	// in-flight asynchronous delivery.
	cancelReq()

	select {
	case rec := <-producedCh:
		if rec.Topic != customTopic {
			t.Fatalf("produced topic = %q, want %q", rec.Topic, customTopic)
		}
		decoded, decErr := DecodeRecord(rec)
		if decErr != nil {
			t.Fatalf("DecodeRecord() error = %v", decErr)
		}
		if decoded.ID() != ev.ID() || decoded.Subject() != ev.Subject() {
			t.Fatalf("decoded event = (%q, %q), want (%q, %q)", decoded.ID(), decoded.Subject(), ev.ID(), ev.Subject())
		}
		if got := decoded.Extensions()[audit.ExtResourceName]; got != "users/u-9" {
			t.Fatalf("resourcename = %v, want users/u-9", got)
		}
	case <-ctx.Done():
		t.Fatal("timed out waiting for produced Kafka record")
	}
}

func TestEmitNonBlockingOnUnknownTopicAndLogsAsyncError(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("net.Listen: %v", err)
	}
	defer func() { _ = ln.Close() }()

	host, portStr, _ := net.SplitHostPort(ln.Addr().String())
	port, _ := strconv.Atoi(portStr)
	go func() {
		for {
			conn, acceptErr := ln.Accept()
			if acceptErr != nil {
				return
			}
			go serveUnknownTopicBroker(conn, host, int32(port))
		}
	}()

	client, err := kgo.NewClient(
		kgo.SeedBrokers(ln.Addr().String()),
		kgo.UnknownTopicRetries(1),
		kgo.MetadataMinAge(20*time.Millisecond),
		kgo.RecordDeliveryTimeout(time.Second),
		kgo.DisableIdempotentWrite(),
	)
	if err != nil {
		t.Fatalf("kgo.NewClient: %v", err)
	}
	defer client.Close()

	logs := &lockedBuffer{}
	logger := slog.New(slog.NewJSONHandler(logs, nil))
	auditor, err := audit.New(&auditpb.AuditEmitter{
		Enabled: true,
		Backend: new("kafka"),
		Topic:   new("missing.audit.topic"),
	}, logger, Backend(client))
	if err != nil {
		t.Fatalf("audit.New(kafka): %v", err)
	}

	ev := sampleEvent(t)
	start := time.Now()
	if err := auditor.Emit(context.Background(), ev); err != nil {
		t.Fatalf("Emit() error = %v, want nil (async enqueue)", err)
	}
	if elapsed := time.Since(start); elapsed > 100*time.Millisecond {
		t.Fatalf("Emit() blocked for %s on missing topic, want non-blocking enqueue", elapsed)
	}

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		out := logs.String()
		if strings.Contains(out, "audit kafka produce failed") &&
			strings.Contains(out, "missing.audit.topic") &&
			strings.Contains(out, ev.ID()) {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("expected async produce failure log, got:\n%s", logs.String())
}

func TestEmitFailsFastWhenBufferFull(t *testing.T) {
	client, err := kgo.NewClient(
		kgo.SeedBrokers("127.0.0.1:9092"),
		kgo.MaxBufferedRecords(1),
		kgo.ManualFlushing(),
	)
	if err != nil {
		t.Fatalf("kgo.NewClient: %v", err)
	}
	defer client.Close()

	logs := &lockedBuffer{}
	logger := slog.New(slog.NewJSONHandler(logs, nil))
	auditor, err := New(client, Config{
		Topic:  DefaultTopic,
		Mode:   BinaryMode,
		Logger: logger,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ev := sampleEvent(t)
	if err := auditor.Emit(context.Background(), ev); err != nil {
		t.Fatalf("first Emit() error = %v, want nil", err)
	}

	start := time.Now()
	if err := auditor.Emit(context.Background(), ev); err != nil {
		t.Fatalf("second Emit() error = %v, want nil (async promise reporting)", err)
	}
	if elapsed := time.Since(start); elapsed > 100*time.Millisecond {
		t.Fatalf("full-buffer Emit() blocked for %s, want non-blocking drop", elapsed)
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		out := logs.String()
		if strings.Contains(out, "audit kafka produce failed") &&
			strings.Contains(out, kgo.ErrMaxBuffered.Error()) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("expected ErrMaxBuffered async log, got:\n%s", logs.String())
}

func TestNewDefault(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	disabledAuditor, err := NewDefault(&auditpb.AuditEmitter{Enabled: false}, nil, logger)
	if err != nil || disabledAuditor == nil {
		t.Fatalf("NewDefault(disabled, nil client) = (%v, %v), want non-nil Noop auditor", disabledAuditor, err)
	}

	if _, err := NewDefault(&auditpb.AuditEmitter{
		Enabled: true,
		Backend: new("kafka"),
	}, nil, logger); err == nil {
		t.Fatal("NewDefault(enabled kafka, nil client) succeeded, want error")
	}

	client, err := kgo.NewClient(kgo.SeedBrokers("127.0.0.1:9092"))
	if err != nil {
		t.Fatalf("kgo.NewClient: %v", err)
	}
	defer client.Close()

	kafkaAuditor, err := NewDefault(&auditpb.AuditEmitter{
		Enabled: true,
		Backend: new("kafka"),
		Topic:   new(DefaultTopic),
	}, client, logger)
	if err != nil || kafkaAuditor == nil {
		t.Fatalf("NewDefault(enabled kafka, valid client) = (%v, %v), want non-nil auditor", kafkaAuditor, err)
	}
}

func sampleEvent(t *testing.T) cloudevents.Event {
	t.Helper()
	ev := cloudevents.NewEvent()
	ev.SetID("0196ed7a-0000-7000-8000-000000000001")
	ev.SetSource("//iam.service")
	ev.SetType(audit.DefaultRPCEventType)
	ev.SetSubject("/plateau.iam.user.v1.UserService/CreateUser")
	ev.SetTime(time.Date(2026, 11, 24, 8, 0, 0, 123456789, time.UTC))
	ev.SetExtension(audit.ExtCategory, audit.CategoryResource)
	ev.SetExtension(audit.ExtActorType, string(security.ActorTypeHuman))
	ev.SetExtension(audit.ExtActorID, "user-1")
	ev.SetExtension(audit.ExtPartitionKey, "actor-key-1")
	if err := ev.SetData(cloudevents.ApplicationJSON, []byte(`{"name":"users/user-1"}`)); err != nil {
		t.Fatalf("SetData: %v", err)
	}
	return ev
}

type lockedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func serveUnknownTopicBroker(conn net.Conn, host string, port int32) {
	defer func() { _ = conn.Close() }()
	var sizeBuf [4]byte
	for {
		if _, err := io.ReadFull(conn, sizeBuf[:]); err != nil {
			return
		}
		size := binary.BigEndian.Uint32(sizeBuf[:])
		payload := make([]byte, size)
		if _, err := io.ReadFull(conn, payload); err != nil {
			return
		}
		if len(payload) < 10 {
			return
		}
		apiKey := int16(binary.BigEndian.Uint16(payload[0:2]))
		apiVersion := int16(binary.BigEndian.Uint16(payload[2:4]))
		corrID := binary.BigEndian.Uint32(payload[4:8])
		clientIDLen := int16(binary.BigEndian.Uint16(payload[8:10]))
		pos := 10
		if clientIDLen > 0 {
			pos += int(clientIDLen)
		}
		body := payload[pos:]

		var resp kmsg.Response
		switch kmsg.Key(apiKey) {
		case kmsg.ApiVersions:
			av := kmsg.NewPtrApiVersionsResponse()
			av.SetVersion(apiVersion)
			av.ApiKeys = []kmsg.ApiVersionsResponseApiKey{
				{ApiKey: int16(kmsg.ApiVersions), MinVersion: 0, MaxVersion: 2},
				{ApiKey: int16(kmsg.Metadata), MinVersion: 0, MaxVersion: 8},
			}
			resp = av
		case kmsg.Metadata:
			req := kmsg.NewPtrMetadataRequest()
			req.SetVersion(apiVersion)
			_ = req.ReadFrom(body)
			md := kmsg.NewPtrMetadataResponse()
			md.SetVersion(apiVersion)
			broker := kmsg.NewMetadataResponseBroker()
			broker.NodeID = 0
			broker.Host = host
			broker.Port = port
			md.Brokers = append(md.Brokers, broker)
			for _, rt := range req.Topics {
				if rt.Topic != nil {
					topic := kmsg.NewMetadataResponseTopic()
					topic.Topic = rt.Topic
					topic.ErrorCode = kerr.UnknownTopicOrPartition.Code
					md.Topics = append(md.Topics, topic)
				}
			}
			resp = md
		default:
			return
		}

		var wire []byte
		wire = binary.BigEndian.AppendUint32(wire, corrID)
		wire = resp.AppendTo(wire)
		var frame []byte
		frame = binary.BigEndian.AppendUint32(frame, uint32(len(wire)))
		frame = append(frame, wire...)
		if _, err := conn.Write(frame); err != nil {
			return
		}
	}
}

func serveProduceBroker(conn net.Conn, host string, port int32, outCh chan<- *kgo.Record) {
	defer func() { _ = conn.Close() }()
	var sizeBuf [4]byte
	for {
		if _, err := io.ReadFull(conn, sizeBuf[:]); err != nil {
			return
		}
		size := binary.BigEndian.Uint32(sizeBuf[:])
		payload := make([]byte, size)
		if _, err := io.ReadFull(conn, payload); err != nil {
			return
		}
		if len(payload) < 8 {
			return
		}
		apiKey := int16(binary.BigEndian.Uint16(payload[0:2]))
		apiVersion := int16(binary.BigEndian.Uint16(payload[2:4]))
		corrID := binary.BigEndian.Uint32(payload[4:8])
		clientIDLen := int16(binary.BigEndian.Uint16(payload[8:10]))
		pos := 10
		if clientIDLen > 0 {
			pos += int(clientIDLen)
		}
		body := payload[pos:]

		var resp kmsg.Response
		switch kmsg.Key(apiKey) {
		case kmsg.ApiVersions:
			av := kmsg.NewPtrApiVersionsResponse()
			av.SetVersion(apiVersion)
			for _, k := range []struct {
				key int16
				max int16
			}{
				{int16(kmsg.ApiVersions), 2},
				{int16(kmsg.Metadata), 8},
				{int16(kmsg.Produce), 8},
			} {
				entry := kmsg.NewApiVersionsResponseApiKey()
				entry.ApiKey = k.key
				entry.MinVersion = 0
				entry.MaxVersion = k.max
				av.ApiKeys = append(av.ApiKeys, entry)
			}
			resp = av

		case kmsg.Metadata:
			req := kmsg.NewPtrMetadataRequest()
			req.SetVersion(apiVersion)
			_ = req.ReadFrom(body)
			md := kmsg.NewPtrMetadataResponse()
			md.SetVersion(apiVersion)
			broker := kmsg.NewMetadataResponseBroker()
			broker.NodeID = 0
			broker.Host = host
			broker.Port = port
			md.Brokers = append(md.Brokers, broker)
			for _, rt := range req.Topics {
				if rt.Topic != nil {
					topic := kmsg.NewMetadataResponseTopic()
					topic.Topic = rt.Topic
					part := kmsg.NewMetadataResponseTopicPartition()
					part.Partition = 0
					part.Leader = 0
					part.Replicas = []int32{0}
					part.ISR = []int32{0}
					topic.Partitions = append(topic.Partitions, part)
					md.Topics = append(md.Topics, topic)
				}
			}
			resp = md

		case kmsg.Produce:
			req := kmsg.NewPtrProduceRequest()
			req.SetVersion(apiVersion)
			_ = req.ReadFrom(body)
			pr := kmsg.NewPtrProduceResponse()
			pr.SetVersion(apiVersion)
			for _, tp := range req.Topics {
				respTopic := kmsg.NewProduceResponseTopic()
				respTopic.Topic = tp.Topic
				for _, part := range tp.Partitions {
					extractProducedRecords(tp.Topic, part.Records, outCh)
					p := kmsg.NewProduceResponseTopicPartition()
					p.Partition = part.Partition
					respTopic.Partitions = append(respTopic.Partitions, p)
				}
				pr.Topics = append(pr.Topics, respTopic)
			}
			resp = pr

		default:
			return
		}

		var wire []byte
		wire = binary.BigEndian.AppendUint32(wire, corrID)
		wire = resp.AppendTo(wire)
		var frame []byte
		frame = binary.BigEndian.AppendUint32(frame, uint32(len(wire)))
		frame = append(frame, wire...)
		if _, err := conn.Write(frame); err != nil {
			return
		}
	}
}

func extractProducedRecords(topic string, rawBatch []byte, outCh chan<- *kgo.Record) {
	var batch kmsg.RecordBatch
	if err := batch.ReadFrom(rawBatch); err != nil {
		return
	}
	rem := batch.Records
	for i := int32(0); i < batch.NumRecords && len(rem) > 0; i++ {
		recLen, n := binary.Varint(rem)
		if n <= 0 || recLen < 0 || n+int(recLen) > len(rem) {
			return
		}
		total := n + int(recLen)
		var rec kmsg.Record
		if err := rec.ReadFrom(rem[:total]); err != nil {
			return
		}
		rem = rem[total:]

		headers := make([]kgo.RecordHeader, 0, len(rec.Headers))
		for _, h := range rec.Headers {
			headers = append(headers, kgo.RecordHeader{Key: h.Key, Value: h.Value})
		}
		outCh <- &kgo.Record{
			Topic:   topic,
			Key:     rec.Key,
			Value:   rec.Value,
			Headers: headers,
		}
	}
}
