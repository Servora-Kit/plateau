package kafka

import (
	"context"
	"encoding/binary"
	"io"
	"log/slog"
	"net"
	"strconv"
	"strings"
	"testing"
	"time"

	kafkapb "github.com/Servora-Kit/plateau/api/gen/go/plateau/infra/kafka/v1"
	tlspb "github.com/Servora-Kit/servora/api/gen/go/servora/security/tls/v1"
	"github.com/twmb/franz-go/pkg/kgo"
	"github.com/twmb/franz-go/pkg/kmsg"
	"google.golang.org/protobuf/types/known/durationpb"
)

func TestBuildOptsClonesInputAndAppliesDefaults(t *testing.T) {
	cfg := &kafkapb.Kafka{
		Brokers:  []string{"127.0.0.1:9092"},
		ClientId: "plateau-test",
	}

	opts, err := BuildOpts(cfg, WithSlogLogger(testLogger()), kgo.ClientID("override"))
	if err != nil {
		t.Fatalf("BuildOpts() error = %v", err)
	}
	if len(opts) == 0 {
		t.Fatal("BuildOpts() returned no options")
	}
	if cfg.RetryMax != nil {
		t.Fatalf("BuildOpts mutated input RetryMax: %v", cfg.RetryMax)
	}
	if cfg.Compression != nil {
		t.Fatalf("BuildOpts mutated input Compression: %v", cfg.Compression)
	}

	cl, err := kgo.NewClient(opts...)
	if err != nil {
		t.Fatalf("kgo.NewClient() error = %v", err)
	}
	defer cl.Close()

	if got := cl.OptValue(kgo.ClientID); got != "override" {
		t.Fatalf("ClientID = %v, want override", got)
	}
	if got := cl.OptValue(kgo.RequestRetries); got != 3 && got != int64(3) {
		t.Fatalf("RequestRetries = %T(%v), want 3", got, got)
	}
	if got := cl.OptValue(kgo.UnknownTopicRetries); got != 3 && got != int64(3) {
		t.Fatalf("UnknownTopicRetries = %T(%v), want 3", got, got)
	}
	if got := cl.OptValue(kgo.MaxBufferedRecords); got != DefaultMaxBufferedRecords && got != int64(DefaultMaxBufferedRecords) {
		t.Fatalf("MaxBufferedRecords = %T(%v), want %d", got, got, DefaultMaxBufferedRecords)
	}
	if got := cl.OptValue(kgo.RecordDeliveryTimeout); got != DefaultRecordDeliveryTimeout {
		t.Fatalf("RecordDeliveryTimeout = %T(%v), want %s", got, got, DefaultRecordDeliveryTimeout)
	}

	longWriteCfg := &kafkapb.Kafka{
		Brokers:      []string{"127.0.0.1:9092"},
		WriteTimeout: durationpb.New(20 * time.Second),
	}
	longOpts, err := BuildOpts(longWriteCfg)
	if err != nil {
		t.Fatalf("BuildOpts(long write_timeout) error = %v", err)
	}
	longCl, err := kgo.NewClient(longOpts...)
	if err != nil {
		t.Fatalf("kgo.NewClient(long write_timeout) error = %v", err)
	}
	defer longCl.Close()
	if got := longCl.OptValue(kgo.RecordDeliveryTimeout); got != 20*time.Second {
		t.Fatalf("RecordDeliveryTimeout(write_timeout=20s) = %T(%v), want 20s", got, got)
	}
}

func TestBuildOptsValidationErrors(t *testing.T) {
	tests := []struct {
		name string
		cfg  *kafkapb.Kafka
		want string
	}{
		{
			name: "nil config",
			cfg:  nil,
			want: "must not be nil",
		},
		{
			name: "missing brokers",
			cfg:  &kafkapb.Kafka{},
			want: "brokers",
		},
		{
			name: "unsupported SASL",
			cfg: &kafkapb.Kafka{
				Brokers: []string{"127.0.0.1:9092"},
				Sasl:    &kafkapb.Kafka_SASL{Mechanism: "GSSAPI"},
			},
			want: "unsupported SASL mechanism",
		},
		{
			name: "unsupported compression",
			cfg: &kafkapb.Kafka{
				Brokers:     []string{"127.0.0.1:9092"},
				Compression: new("brotli"),
			},
			want: "unsupported compression",
		},
		{
			name: "unsupported required_acks",
			cfg: &kafkapb.Kafka{
				Brokers:      []string{"127.0.0.1:9092"},
				RequiredAcks: 2,
			},
			want: "unsupported required_acks",
		},
		{
			name: "invalid TLS CA path",
			cfg: &kafkapb.Kafka{
				Brokers: []string{"127.0.0.1:9092"},
				Tls: &tlspb.TLS{
					Enable: true,
					CaPath: "/nonexistent/ca.pem",
				},
			},
			want: "build TLS config",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := BuildOpts(tc.cfg)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("BuildOpts() error = %v, want containing %q", err, tc.want)
			}
		})
	}
}

func TestBuildOptsSupportsSASLAndCompressionVariants(t *testing.T) {
	for _, mech := range []string{"PLAIN", "SCRAM-SHA-256", "SCRAM-SHA-512"} {
		for _, comp := range []string{"none", "snappy", "gzip", "lz4", "zstd"} {
			cfg := &kafkapb.Kafka{
				Brokers:      []string{"127.0.0.1:9092"},
				Compression:  new(comp),
				RequiredAcks: -1,
				Sasl: &kafkapb.Kafka_SASL{
					Mechanism: mech,
					Username:  "user",
					Password:  "secret",
				},
			}
			if _, err := BuildOpts(cfg); err != nil {
				t.Fatalf("BuildOpts(%s, %s) error = %v", mech, comp, err)
			}
		}
	}
}

func TestNewClientOptionalDistinguishesAbsentFailureAndConnected(t *testing.T) {
	client, err := NewClientOptional(context.Background(), nil)
	if err != nil || client != nil {
		t.Fatalf("NewClientOptional(nil) = (%v, %v), want (nil, nil)", client, err)
	}
	client, err = NewClientOptional(context.Background(), &kafkapb.Kafka{})
	if err != nil || client != nil {
		t.Fatalf("NewClientOptional(empty) = (%v, %v), want (nil, nil)", client, err)
	}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	closedAddr := ln.Addr().String()
	_ = ln.Close()

	failCfg := &kafkapb.Kafka{
		Brokers:     []string{closedAddr},
		DialTimeout: durationpb.New(50 * time.Millisecond),
	}
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()
	client, err = NewClientOptional(ctx, failCfg)
	if err == nil || client != nil {
		if client != nil {
			client.Close()
		}
		t.Fatalf("NewClientOptional(closed broker) = (%v, %v), want (nil, err)", client, err)
	}

	brokerAddr := startStubKafkaPingServer(t)
	okCfg := &kafkapb.Kafka{
		Brokers:     []string{brokerAddr},
		ClientId:    "plateau-ping-ok",
		DialTimeout: durationpb.New(2 * time.Second),
	}
	client, err = NewClientOptional(context.Background(), okCfg)
	if err != nil {
		t.Fatalf("NewClientOptional(stub broker) error = %v", err)
	}
	if client == nil {
		t.Fatal("NewClientOptional(stub broker) returned nil client")
	}
	client.Close()
}

func TestNewWireProviderLifecycle(t *testing.T) {
	client, cleanup, err := New(nil, testLogger())
	if err != nil {
		t.Fatalf("New(nil) error = %v", err)
	}
	if client != nil {
		t.Fatalf("New(nil) client = %v, want nil", client)
	}
	if cleanup == nil {
		t.Fatal("New(nil) returned nil cleanup")
	}
	cleanup()

	closedLn, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen closed port: %v", err)
	}
	closedAddr := closedLn.Addr().String()
	_ = closedLn.Close()

	_, _, err = New(&kafkapb.Kafka{
		Brokers:     []string{closedAddr},
		DialTimeout: durationpb.New(150 * time.Millisecond),
	}, testLogger())
	if err == nil {
		t.Fatal("New(closed broker) succeeded, want Ping failure")
	}

	stubAddr := startStubKafkaPingServer(t)
	client, cleanup, err = New(&kafkapb.Kafka{
		Brokers:     []string{stubAddr},
		DialTimeout: durationpb.New(2 * time.Second),
	}, testLogger())
	if err != nil {
		t.Fatalf("New(stub broker) error = %v", err)
	}
	if client == nil || cleanup == nil {
		t.Fatalf("New(stub broker) client=%v cleanupNil=%v, want non-nil", client, cleanup == nil)
	}
	cleanup()
}

func TestNewCleanupFlushesBufferedRecords(t *testing.T) {
	brokerAddr := startStubKafkaProduceServer(t, 60*time.Millisecond)
	client, cleanup, err := New(&kafkapb.Kafka{
		Brokers:      []string{brokerAddr},
		ClientId:     "plateau-flush-test",
		DialTimeout:  durationpb.New(2 * time.Second),
		RequiredAcks: -1,
	}, testLogger())
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	producedErrCh := make(chan error, 1)
	client.TryProduce(context.Background(), &kgo.Record{
		Topic: "flush.test",
		Value: []byte("hello"),
	}, func(_ *kgo.Record, err error) {
		producedErrCh <- err
	})

	cleanup()

	select {
	case produceErr := <-producedErrCh:
		if produceErr != nil {
			t.Fatalf("buffered record failed during cleanup flush: %v", produceErr)
		}
	default:
		t.Fatal("cleanup returned before flushing buffered record")
	}
}

func TestWithSlogLoggerExplicitOption(t *testing.T) {
	if WithSlogLogger(testLogger()) == nil {
		t.Fatal("WithSlogLogger() returned nil option")
	}
	if WithSlogLogger(nil) == nil {
		t.Fatal("WithSlogLogger(nil) returned nil option")
	}
}

func startStubKafkaPingServer(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen stub kafka: %v", err)
	}
	t.Cleanup(func() { _ = ln.Close() })

	host, portStr, err := net.SplitHostPort(ln.Addr().String())
	if err != nil {
		t.Fatalf("split host port: %v", err)
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		t.Fatalf("atoi port: %v", err)
	}

	go func() {
		for {
			conn, acceptErr := ln.Accept()
			if acceptErr != nil {
				return
			}
			go serveStubKafkaConn(conn, host, int32(port))
		}
	}()
	return ln.Addr().String()
}

func serveStubKafkaConn(conn net.Conn, host string, port int32) {
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

		var resp kmsg.Response
		switch kmsg.Key(apiKey) {
		case kmsg.ApiVersions:
			av := kmsg.NewPtrApiVersionsResponse()
			av.SetVersion(apiVersion)
			av.ApiKeys = []kmsg.ApiVersionsResponseApiKey{
				{ApiKey: int16(kmsg.ApiVersions), MinVersion: 0, MaxVersion: 3},
				{ApiKey: int16(kmsg.Metadata), MinVersion: 0, MaxVersion: 9},
			}
			resp = av
		case kmsg.Metadata:
			md := kmsg.NewPtrMetadataResponse()
			md.SetVersion(apiVersion)
			md.Brokers = []kmsg.MetadataResponseBroker{
				{NodeID: 0, Host: host, Port: port},
			}
			resp = md
		default:
			return
		}

		var out []byte
		out = binary.BigEndian.AppendUint32(out, corrID)
		if resp.IsFlexible() && resp.Key() != int16(kmsg.ApiVersions) {
			out = append(out, 0) // empty response header tag buffer
		}
		out = resp.AppendTo(out)

		var frame []byte
		frame = binary.BigEndian.AppendUint32(frame, uint32(len(out)))
		frame = append(frame, out...)
		if _, err := conn.Write(frame); err != nil {
			return
		}
	}
}

func startStubKafkaProduceServer(t *testing.T, produceDelay time.Duration) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen stub produce kafka: %v", err)
	}
	t.Cleanup(func() { _ = ln.Close() })

	host, portStr, err := net.SplitHostPort(ln.Addr().String())
	if err != nil {
		t.Fatalf("split host port: %v", err)
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		t.Fatalf("atoi port: %v", err)
	}

	go func() {
		for {
			conn, acceptErr := ln.Accept()
			if acceptErr != nil {
				return
			}
			go serveStubProduceConn(conn, host, int32(port), produceDelay)
		}
	}()
	return ln.Addr().String()
}

func serveStubProduceConn(conn net.Conn, host string, port int32, produceDelay time.Duration) {
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
				{ApiKey: int16(kmsg.Produce), MinVersion: 0, MaxVersion: 8},
				{ApiKey: int16(kmsg.InitProducerID), MinVersion: 0, MaxVersion: 1},
			}
			resp = av
		case kmsg.InitProducerID:
			ip := kmsg.NewPtrInitProducerIDResponse()
			ip.SetVersion(apiVersion)
			ip.ProducerID = 1
			ip.ProducerEpoch = 0
			resp = ip
		case kmsg.Metadata:
			req := kmsg.NewPtrMetadataRequest()
			req.SetVersion(apiVersion)
			_ = req.ReadFrom(body)
			md := kmsg.NewPtrMetadataResponse()
			md.SetVersion(apiVersion)
			md.Brokers = []kmsg.MetadataResponseBroker{
				{NodeID: 0, Host: host, Port: port},
			}
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
			if produceDelay > 0 {
				time.Sleep(produceDelay)
			}
			pr := kmsg.NewPtrProduceResponse()
			pr.SetVersion(apiVersion)
			for _, tp := range req.Topics {
				respTopic := kmsg.NewProduceResponseTopic()
				respTopic.Topic = tp.Topic
				for _, part := range tp.Partitions {
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

		var out []byte
		out = binary.BigEndian.AppendUint32(out, corrID)
		out = resp.AppendTo(out)
		var frame []byte
		frame = binary.BigEndian.AppendUint32(frame, uint32(len(out)))
		frame = append(frame, out...)
		if _, err := conn.Write(frame); err != nil {
			return
		}
	}
}

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}
