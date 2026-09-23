package clickhouse

import (
	"context"
	"strings"
	"testing"
	"time"

	ch "github.com/ClickHouse/clickhouse-go/v2"
	clickhousepb "github.com/Servora-Kit/plateau/api/gen/go/plateau/infra/clickhouse/v1"
	tlspb "github.com/Servora-Kit/servora/api/gen/go/servora/security/tls/v1"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/durationpb"
)

func TestNewConnOptionalAbsent(t *testing.T) {
	conn, err := NewConnOptional(context.Background(), nil)
	if err != nil {
		t.Fatalf("NewConnOptional(nil) error = %v", err)
	}
	if conn != nil {
		t.Fatal("NewConnOptional(nil) returned non-nil conn")
	}

	conn, err = NewConnOptional(context.Background(), &clickhousepb.ClickHouse{})
	if err != nil {
		t.Fatalf("NewConnOptional(empty) error = %v", err)
	}
	if conn != nil {
		t.Fatal("NewConnOptional(empty) returned non-nil conn")
	}
}

func TestClickHouseProtoDefaults(t *testing.T) {
	cfg := &clickhousepb.ClickHouse{Addrs: []string{"127.0.0.1:9000"}}
	if err := cfg.Apply(); err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	if got := cfg.GetDialTimeout().AsDuration(); got != 10*time.Second {
		t.Fatalf("DialTimeout = %s, want 10s", got)
	}
	if got := cfg.GetReadTimeout().AsDuration(); got != 30*time.Second {
		t.Fatalf("ReadTimeout = %s, want 30s", got)
	}
	if cfg.GetMaxOpenConns() != 10 {
		t.Fatalf("MaxOpenConns = %d, want 10", cfg.GetMaxOpenConns())
	}
	if cfg.GetMaxIdleConns() != 5 {
		t.Fatalf("MaxIdleConns = %d, want 5", cfg.GetMaxIdleConns())
	}
	if got := cfg.GetConnMaxLifetime().AsDuration(); got != 5*time.Minute {
		t.Fatalf("ConnMaxLifetime = %s, want 5m", got)
	}
	if cfg.GetCompression() != "none" {
		t.Fatalf("Compression = %q, want none", cfg.GetCompression())
	}
}

func TestNewConnOptionalReturnsPingError(t *testing.T) {
	cfg := &clickhousepb.ClickHouse{
		Addrs:       []string{"127.0.0.1:1"},
		DialTimeout: durationpb.New(time.Millisecond),
	}
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	conn, err := NewConnOptional(ctx, cfg)
	if err == nil {
		if conn != nil {
			_ = conn.Close()
		}
		t.Fatal("NewConnOptional() error = nil, want ping error")
	}
	if conn != nil {
		t.Fatal("NewConnOptional() returned conn with ping error")
	}
}

func TestNewConnOptionalRejectsInvalidTLS(t *testing.T) {
	cfg := &clickhousepb.ClickHouse{
		Addrs: []string{"127.0.0.1:9000"},
		Tls: &tlspb.TLS{
			Enable:   true,
			CertPath: "client.crt",
		},
	}

	conn, err := NewConnOptional(context.Background(), cfg)
	if err == nil {
		if conn != nil {
			_ = conn.Close()
		}
		t.Fatal("NewConnOptional() error = nil, want TLS validation error")
	}
	if conn != nil {
		t.Fatal("NewConnOptional() returned conn with TLS validation error")
	}
}

func TestNewConnOptionalDoesNotMutateConfigOnCompressionError(t *testing.T) {
	cfg := &clickhousepb.ClickHouse{
		Addrs:       []string{"127.0.0.1:9000"},
		Compression: proto.String("brotli"),
	}

	_, err := NewConnOptional(context.Background(), cfg)
	if err == nil {
		t.Fatal("NewConnOptional() error = nil, want compression configuration error")
	}
	if cfg.GetDialTimeout() != nil || cfg.GetReadTimeout() != nil || cfg.GetCompression() != "brotli" {
		t.Fatalf("config mutated after failed construction: %#v", cfg)
	}
}

func TestApplyCompression(t *testing.T) {
	opts := &ch.Options{}
	if err := applyCompression(opts, "zstd"); err != nil {
		t.Fatalf("applyCompression(zstd) error = %v", err)
	}
	if opts.Compression == nil || opts.Compression.Method != ch.CompressionZSTD {
		t.Fatalf("Compression = %#v, want zstd", opts.Compression)
	}

	opts = &ch.Options{}
	if err := applyCompression(opts, "lz4"); err != nil {
		t.Fatalf("applyCompression(lz4) error = %v", err)
	}
	if opts.Compression == nil || opts.Compression.Method != ch.CompressionLZ4 {
		t.Fatalf("Compression = %#v, want lz4", opts.Compression)
	}

	for _, raw := range []string{"", "none"} {
		opts = &ch.Options{}
		if err := applyCompression(opts, raw); err != nil {
			t.Fatalf("applyCompression(%q) error = %v", raw, err)
		}
		if opts.Compression != nil {
			t.Fatalf("Compression = %#v for %q, want nil", opts.Compression, raw)
		}
	}

	opts = &ch.Options{}
	err := applyCompression(opts, "brotli")
	if err == nil || !strings.Contains(err.Error(), "brotli") {
		t.Fatalf("applyCompression(brotli) error = %v, want value in configuration error", err)
	}
	if opts.Compression != nil {
		t.Fatalf("Compression = %#v after invalid value, want nil", opts.Compression)
	}
}
