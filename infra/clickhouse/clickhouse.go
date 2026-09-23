// Package clickhouse provides a platform-level ClickHouse connection helper.
//
// Usage:
//
//	conn, err := clickhouse.NewConnOptional(ctx, cfg)
//	if err != nil {
//	    // configured but failed to connect — fail-fast or degrade
//	}
//	if conn == nil {
//	    // not configured — handle gracefully
//	}
//	defer conn.Close()
package clickhouse

import (
	"context"
	"fmt"
	"strings"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	clickhousepb "github.com/Servora-Kit/plateau/api/gen/go/plateau/infra/clickhouse/v1"
	svrtls "github.com/Servora-Kit/servora/security/tls"
	"google.golang.org/protobuf/proto"
)

// NewConnOptional opens a ClickHouse connection from generated Plateau config.
//
// Return semantics:
//   - (nil, nil)  — ClickHouse is not configured (cfg nil or no addrs).
//   - (nil, err)  — configured but connection/ping failed; callers can fail-fast or degrade.
//   - (conn, nil) — connected successfully.
//
// The caller is responsible for closing the connection via conn.Close().
func NewConnOptional(ctx context.Context, cfg *clickhousepb.ClickHouse) (driver.Conn, error) {
	if cfg == nil || len(cfg.GetAddrs()) == 0 {
		return nil, nil
	}

	config := proto.Clone(cfg).(*clickhousepb.ClickHouse)
	if err := config.Apply(); err != nil {
		return nil, err
	}

	opts := &clickhouse.Options{
		Addr: config.GetAddrs(),
		Auth: clickhouse.Auth{
			Database: config.GetDatabase(),
			Username: config.GetUsername(),
			Password: config.GetPassword(),
		},
		DialTimeout:      config.GetDialTimeout().AsDuration(),
		ReadTimeout:      config.GetReadTimeout().AsDuration(),
		MaxOpenConns:     int(config.GetMaxOpenConns()),
		MaxIdleConns:     int(config.GetMaxIdleConns()),
		ConnMaxLifetime:  config.GetConnMaxLifetime().AsDuration(),
		ConnOpenStrategy: clickhouse.ConnOpenInOrder,
	}

	if config.GetTls().GetEnable() {
		tlsCfg, err := svrtls.NewClientConfig(svrtls.ClientConfigOptions{
			CAPath:   config.GetTls().GetCaPath(),
			CertPath: config.GetTls().GetCertPath(),
			KeyPath:  config.GetTls().GetKeyPath(),
		})
		if err != nil {
			return nil, fmt.Errorf("build ClickHouse TLS config: %w", err)
		}
		opts.TLS = tlsCfg
	}

	if err := applyCompression(opts, config.GetCompression()); err != nil {
		return nil, err
	}

	conn, err := clickhouse.Open(opts)
	if err != nil {
		return nil, fmt.Errorf("open ClickHouse: %w", err)
	}

	pingCtx, cancel := context.WithTimeout(ctx, config.GetDialTimeout().AsDuration())
	defer cancel()
	if err := conn.Ping(pingCtx); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("ping ClickHouse: %w", err)
	}

	return conn, nil
}

// applyCompression normalises the compression string and sets the appropriate
// compression option. Unknown values are rejected as configuration errors.
func applyCompression(opts *clickhouse.Options, raw string) error {
	v := strings.TrimSpace(strings.ToLower(raw))
	switch v {
	case "", "none":
		// no compression
	case "lz4":
		opts.Compression = &clickhouse.Compression{Method: clickhouse.CompressionLZ4}
	case "zstd":
		opts.Compression = &clickhouse.Compression{Method: clickhouse.CompressionZSTD}
	default:
		return fmt.Errorf("ClickHouse: unsupported compression %q", raw)
	}
	return nil
}
