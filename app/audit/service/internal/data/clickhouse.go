package data

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	clickhousepb "github.com/Servora-Kit/plateau/api/gen/go/plateau/infra/clickhouse/v1"
	pkgch "github.com/Servora-Kit/plateau/infra/clickhouse"
)

// NewClickHouseClient opens a ClickHouse connection via infra/clickhouse.
// Returns (nil, nil) when ClickHouse is not configured; returns an error when
// configured but connection failed — ensuring fail-fast for a core dependency.
func NewClickHouseClient(cfg *clickhousepb.ClickHouse, l *slog.Logger) (driver.Conn, error) {
	conn, err := pkgch.NewConnOptional(context.Background(), cfg, l)
	if err != nil {
		return nil, fmt.Errorf("clickhouse client: %w", err)
	}
	return conn, nil
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
