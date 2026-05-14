package data

import (
	"context"
	"fmt"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	auditsvcpb "github.com/Servora-Kit/servora-platform/api/gen/go/servora/audit/service/v1"
	pkgch "github.com/Servora-Kit/servora/infra/db/clickhouse"
	"github.com/Servora-Kit/servora/obs/logging"
)

// NewClickHouseClient opens a ClickHouse connection via pkg/db/clickhouse.
// Returns (nil, nil) when ClickHouse is not configured; returns an error when
// configured but connection failed — ensuring fail-fast for a core dependency.
func NewClickHouseClient(cfg *auditsvcpb.AuditConsumerConfig, l logger.Logger) (driver.Conn, error) {
	conn, err := pkgch.NewConnOptional(context.Background(), toClickHousePkgConfig(cfg.GetClickhouse()), l)
	if err != nil {
		return nil, fmt.Errorf("clickhouse client: %w", err)
	}
	return conn, nil
}

// toClickHousePkgConfig adapts the audit service's local ClickHouse proto into
// the generic infra/db/clickhouse.Config struct.
func toClickHousePkgConfig(c *auditsvcpb.ClickHouse) *pkgch.Config {
	if c == nil {
		return nil
	}
	return &pkgch.Config{
		Addrs:           c.GetAddrs(),
		Database:        c.GetDatabase(),
		Username:        c.GetUsername(),
		Password:        c.GetPassword(),
		DialTimeout:     c.GetDialTimeout().AsDuration(),
		ReadTimeout:     c.GetReadTimeout().AsDuration(),
		MaxOpenConns:    int(c.GetMaxOpenConns()),
		MaxIdleConns:    int(c.GetMaxIdleConns()),
		ConnMaxLifetime: c.GetConnMaxLifetime().AsDuration(),
		TLS:             c.GetTls(),
		TLSSkipVerify:   c.GetTlsSkipVerify(),
		Compress:        c.GetCompress(),
	}
}

// createAuditEventsTable executes the DDL to create the audit_events table idempotently.
func createAuditEventsTable(ctx context.Context, conn driver.Conn, retentionDays int32) error {
	if retentionDays <= 0 {
		retentionDays = 90
	}
	ddl := fmt.Sprintf(`
CREATE TABLE IF NOT EXISTS audit_events (
    event_id              String,
    event_type            LowCardinality(String),
    event_version         String,
    occurred_at           DateTime64(3, 'UTC'),

    service               LowCardinality(String),
    operation             String,

    actor_id              String,
    actor_type            LowCardinality(String),
    actor_display_name    String,

    target_type           LowCardinality(String),
    target_id             String,
    target_name           String,

    success               Bool,
    error_code            String,
    error_message         String,

    trace_id              String,
    request_id            String,

    detail                String
) ENGINE = MergeTree()
PARTITION BY toDate(occurred_at)
ORDER BY (service, event_type, occurred_at, event_id)
TTL occurred_at + INTERVAL %d DAY
SETTINGS index_granularity = 8192
`, retentionDays)

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	return conn.Exec(ctx, ddl)
}
