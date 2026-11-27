// Package kafka provides platform-level franz-go client construction from
// Plateau Kafka configuration.
package kafka

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	kafkapb "github.com/Servora-Kit/plateau/api/gen/go/plateau/infra/kafka/v1"
	svrtls "github.com/Servora-Kit/servora/security/tls"
	"github.com/twmb/franz-go/pkg/kgo"
	"github.com/twmb/franz-go/pkg/sasl/plain"
	"github.com/twmb/franz-go/pkg/sasl/scram"
	"github.com/twmb/franz-go/plugin/kotel"
	"google.golang.org/protobuf/proto"
)

const (
	// DefaultMaxBufferedRecords bounds async producer memory when Kafka is unavailable.
	DefaultMaxBufferedRecords = 4096
	// DefaultRecordDeliveryTimeout bounds total async delivery/retry time per record.
	DefaultRecordDeliveryTimeout = 10 * time.Second
	// DefaultCloseFlushTimeout bounds how long Wire cleanup waits for in-flight records to flush.
	DefaultCloseFlushTimeout = 5 * time.Second
)

// BuildOpts clones cfg, applies proto defaults/validation, maps Kafka config
// into franz-go options, and appends caller-supplied options.
func BuildOpts(cfg *kafkapb.Kafka, extra ...kgo.Opt) ([]kgo.Opt, error) {
	if cfg == nil {
		return nil, fmt.Errorf("kafka: config must not be nil")
	}
	config := proto.Clone(cfg).(*kafkapb.Kafka)
	if err := config.Apply(); err != nil {
		return nil, err
	}

	deliveryTimeout := DefaultRecordDeliveryTimeout
	if d := config.GetWriteTimeout(); d != nil && d.AsDuration() > deliveryTimeout {
		deliveryTimeout = d.AsDuration()
	}

	opts := []kgo.Opt{
		kgo.SeedBrokers(config.GetBrokers()...),
		kgo.MaxBufferedRecords(DefaultMaxBufferedRecords),
		kgo.RecordDeliveryTimeout(deliveryTimeout),
	}
	if config.GetClientId() != "" {
		opts = append(opts, kgo.ClientID(config.GetClientId()))
	}

	if d := config.GetDialTimeout(); d != nil && d.AsDuration() > 0 {
		opts = append(opts, kgo.DialTimeout(d.AsDuration()))
	}
	if d := config.GetReadTimeout(); d != nil && d.AsDuration() > 0 {
		opts = append(opts, kgo.RetryTimeout(d.AsDuration()))
	}
	if d := config.GetWriteTimeout(); d != nil && d.AsDuration() > 0 {
		opts = append(opts, kgo.ProduceRequestTimeout(d.AsDuration()))
	}
	if d := config.GetRetryBackoff(); d != nil && d.AsDuration() > 0 {
		backoff := d.AsDuration()
		opts = append(opts, kgo.RetryBackoffFn(func(int) time.Duration { return backoff }))
	}
	if config.GetRetryMax() > 0 {
		retries := int(config.GetRetryMax())
		opts = append(opts,
			kgo.RequestRetries(retries),
			kgo.RecordRetries(retries),
			kgo.UnknownTopicRetries(retries),
		)
	}
	if config.GetRequiredAcks() != 0 {
		acks, err := requiredAcks(config.GetRequiredAcks())
		if err != nil {
			return nil, err
		}
		opts = append(opts, kgo.RequiredAcks(acks))
	}
	if config.GetCompression() != "" {
		compression, err := compressionCodec(config.GetCompression())
		if err != nil {
			return nil, err
		}
		opts = append(opts, kgo.ProducerBatchCompression(compression))
	}
	if config.GetTls().GetEnable() {
		tlsCfg, err := svrtls.NewClientConfig(svrtls.ClientConfigOptions{
			CAPath:   config.GetTls().GetCaPath(),
			CertPath: config.GetTls().GetCertPath(),
			KeyPath:  config.GetTls().GetKeyPath(),
		})
		if err != nil {
			return nil, fmt.Errorf("kafka: build TLS config: %w", err)
		}
		opts = append(opts, kgo.DialTLSConfig(tlsCfg))
	}
	if sasl := config.GetSasl(); sasl != nil {
		saslOpt, err := buildSASL(sasl)
		if err != nil {
			return nil, err
		}
		opts = append(opts, saslOpt)
	}

	kotelSvc := kotel.NewKotel(kotel.WithTracer(kotel.NewTracer()), kotel.WithMeter(kotel.NewMeter()))
	opts = append(opts, kgo.WithHooks(kotelSvc.Hooks()...))
	opts = append(opts, extra...)
	return opts, nil
}

// NewClient constructs a franz-go client from Kafka config and validates
// connectivity via Ping. The caller owns the returned client's Close lifecycle.
func NewClient(ctx context.Context, cfg *kafkapb.Kafka, extra ...kgo.Opt) (*kgo.Client, error) {
	opts, err := BuildOpts(cfg, extra...)
	if err != nil {
		return nil, err
	}
	client, err := kgo.NewClient(opts...)
	if err != nil {
		return nil, fmt.Errorf("kafka: create client: %w", err)
	}

	pingCtx := ctx
	var cancel context.CancelFunc
	if d := cfg.GetDialTimeout(); d != nil && d.AsDuration() > 0 {
		pingCtx, cancel = context.WithTimeout(ctx, d.AsDuration())
		defer cancel()
	}
	if err := client.Ping(pingCtx); err != nil {
		client.Close()
		return nil, fmt.Errorf("kafka: ping: %w", err)
	}
	return client, nil
}

// NewClientOptional opens a franz-go client when Kafka is configured.
//
// Return semantics:
//   - (nil, nil)    — Kafka is not configured (cfg nil or no brokers).
//   - (nil, err)    — configured but validation, client construction, or Ping failed.
//   - (client, nil) — connected successfully; caller owns client.Close().
func NewClientOptional(ctx context.Context, cfg *kafkapb.Kafka, extra ...kgo.Opt) (*kgo.Client, error) {
	if cfg == nil || len(cfg.GetBrokers()) == 0 {
		return nil, nil
	}
	return NewClient(ctx, cfg, extra...)
}

// New opens an optional franz-go client for Wire dependency injection,
// validating connectivity via Ping whenever Kafka brokers are configured.
// When Kafka is unconfigured (cfg nil or empty brokers), it returns (nil, noop, nil).
// The returned cleanup flushes buffered produce records with a bounded timeout before closing.
func New(cfg *kafkapb.Kafka, l *slog.Logger) (*kgo.Client, func(), error) {
	client, err := NewClientOptional(context.Background(), cfg, WithSlogLogger(l))
	if err != nil {
		return nil, nil, err
	}
	if client == nil {
		return nil, func() {}, nil
	}
	cleanup := func() {
		flushCtx, cancel := context.WithTimeout(context.Background(), DefaultCloseFlushTimeout)
		defer cancel()
		if flushErr := client.Flush(flushCtx); flushErr != nil && l != nil {
			l.Warn("kafka: flush buffered records before close failed", "err", flushErr)
		}
		client.Close()
	}
	return client, cleanup, nil
}

func buildSASL(sasl *kafkapb.Kafka_SASL) (kgo.Opt, error) {
	switch strings.ToUpper(strings.TrimSpace(sasl.GetMechanism())) {
	case "PLAIN":
		return kgo.SASL(plain.Auth{
			User: sasl.GetUsername(),
			Pass: sasl.GetPassword(),
		}.AsMechanism()), nil
	case "SCRAM-SHA-256":
		return kgo.SASL(scram.Auth{
			User: sasl.GetUsername(),
			Pass: sasl.GetPassword(),
		}.AsSha256Mechanism()), nil
	case "SCRAM-SHA-512":
		return kgo.SASL(scram.Auth{
			User: sasl.GetUsername(),
			Pass: sasl.GetPassword(),
		}.AsSha512Mechanism()), nil
	default:
		return nil, fmt.Errorf("kafka: unsupported SASL mechanism %q", sasl.GetMechanism())
	}
}

func requiredAcks(v int32) (kgo.Acks, error) {
	switch v {
	case -1:
		return kgo.AllISRAcks(), nil
	case 0:
		return kgo.NoAck(), nil
	case 1:
		return kgo.LeaderAck(), nil
	default:
		return kgo.Acks{}, fmt.Errorf("kafka: unsupported required_acks %d", v)
	}
}

func compressionCodec(raw string) (kgo.CompressionCodec, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", "none":
		return kgo.NoCompression(), nil
	case "snappy":
		return kgo.SnappyCompression(), nil
	case "gzip":
		return kgo.GzipCompression(), nil
	case "lz4":
		return kgo.Lz4Compression(), nil
	case "zstd":
		return kgo.ZstdCompression(), nil
	default:
		return kgo.NoCompression(), fmt.Errorf("kafka: unsupported compression %q", raw)
	}
}

// WithSlogLogger adapts a slog logger to franz-go's native logger option.
// A nil logger produces a no-op option and does not bind a global logger.
func WithSlogLogger(l *slog.Logger) kgo.Opt {
	if l == nil {
		return kgo.WithHooks()
	}
	return kgo.WithLogger(slogAdapter{log: l})
}

type slogAdapter struct {
	log *slog.Logger
}

func (a slogAdapter) Level() kgo.LogLevel {
	if !a.log.Enabled(context.Background(), slog.LevelDebug) {
		return kgo.LogLevelInfo
	}
	return kgo.LogLevelDebug
}

func (a slogAdapter) Log(level kgo.LogLevel, msg string, keyvals ...any) {
	switch level {
	case kgo.LogLevelDebug:
		a.log.Debug(msg, keyvals...)
	case kgo.LogLevelInfo:
		a.log.Info(msg, keyvals...)
	case kgo.LogLevelWarn:
		a.log.Warn(msg, keyvals...)
	case kgo.LogLevelError:
		a.log.Error(msg, keyvals...)
	}
}
