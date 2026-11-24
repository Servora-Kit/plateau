// Package schema defines the ClickHouse persistence models and DDL contracts
// for audit.service, organized one file per ClickHouse table.
package schema

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
)

const (
	// TableEvents is the ClickHouse table storing control-plane audit events.
	TableEvents = "events"
	// DefaultRetentionDays is the fallback table TTL when retention_days <= 0.
	DefaultRetentionDays int32 = 180
)

var (
	reIntervalFunc = regexp.MustCompile(`(?i)toIntervalDay\(\s*(\d+)\s*\)`)
	reIntervalSQL  = regexp.MustCompile(`(?i)INTERVAL\s+(\d+)\s+DAY`)
)

// EventRow maps 1:1 to the 19 columns of the ClickHouse events table via native ch struct tags.
type EventRow struct {
	EventID      string    `ch:"event_id"`
	EventType    string    `ch:"event_type"`
	Category     string    `ch:"category"`
	OccurredAt   time.Time `ch:"occurred_at"`
	IngestedAt   time.Time `ch:"ingested_at"`
	Service      string    `ch:"service"`
	Operation    string    `ch:"operation"`
	ActorType    string    `ch:"actor_type"`
	ActorID      string    `ch:"actor_id"`
	ResourceType string    `ch:"resource_type"`
	ResourceName string    `ch:"resource_name"`
	Success      bool      `ch:"success"`
	ErrorCode    string    `ch:"error_code"`
	ErrorMessage string    `ch:"error_message"`
	TraceID      string    `ch:"trace_id"`
	RequestID    string    `ch:"request_id"`
	ClientIP     string    `ch:"client_ip"`
	UserAgent    string    `ch:"user_agent"`
	DetailJSON   string    `ch:"detail_json"`
}

// EnsureEventsTable idempotently creates the events table and reconciles its TTL
// only when the configured retention_days differs from the active table metadata.
func EnsureEventsTable(ctx context.Context, conn driver.Conn, retentionDays int32) error {
	if conn == nil {
		return fmt.Errorf("audit schema: clickhouse connection is nil")
	}
	if retentionDays <= 0 {
		retentionDays = DefaultRetentionDays
	}

	ddlCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	createDDL := fmt.Sprintf(`
CREATE TABLE IF NOT EXISTS %s (
    event_id       String,
    event_type     LowCardinality(String),
    category       LowCardinality(String),
    occurred_at    DateTime64(6, 'UTC'),
    ingested_at    DateTime64(6, 'UTC') DEFAULT now64(6, 'UTC'),

    service        LowCardinality(String),
    operation      LowCardinality(String),

    actor_type     LowCardinality(String),
    actor_id       String,

    resource_type  LowCardinality(String),
    resource_name  String,

    success        Bool,
    error_code     LowCardinality(String),
    error_message  String CODEC(ZSTD(1)),

    trace_id       String,
    request_id     String,
    client_ip      String,
    user_agent     String CODEC(ZSTD(1)),

    detail_json    String CODEC(ZSTD(1)),

    INDEX idx_event_id event_id TYPE bloom_filter(0.01) GRANULARITY 1,
    INDEX idx_trace_id trace_id TYPE bloom_filter(0.01) GRANULARITY 1,
    INDEX idx_actor_id actor_id TYPE bloom_filter(0.01) GRANULARITY 1,
    INDEX idx_resource_name resource_name TYPE bloom_filter(0.01) GRANULARITY 1
) ENGINE = ReplacingMergeTree(ingested_at)
PARTITION BY toYYYYMM(occurred_at)
ORDER BY (occurred_at, event_id)
TTL toDateTime(occurred_at) + INTERVAL %d DAY
SETTINGS index_granularity = 8192
`, TableEvents, retentionDays)

	if err := conn.Exec(ddlCtx, createDDL); err != nil {
		return fmt.Errorf("create %s table: %w", TableEvents, err)
	}

	var engineFull string
	err := conn.QueryRow(
		ddlCtx,
		"SELECT engine_full FROM system.tables WHERE database = currentDatabase() AND name = ?",
		TableEvents,
	).Scan(&engineFull)
	if err != nil {
		return fmt.Errorf("inspect %s table metadata: %w", TableEvents, err)
	}

	if currentDays, ok := ExtractRetentionDays(engineFull); ok && currentDays == retentionDays {
		return nil
	}

	alterDDL := fmt.Sprintf(
		"ALTER TABLE %s MODIFY TTL toDateTime(occurred_at) + INTERVAL %d DAY",
		TableEvents,
		retentionDays,
	)
	if err := conn.Exec(ddlCtx, alterDDL); err != nil {
		return fmt.Errorf("modify %s table TTL: %w", TableEvents, err)
	}
	return nil
}

// ExtractRetentionDays parses the active TTL day count from ClickHouse system.tables.engine_full.
func ExtractRetentionDays(engineFull string) (int32, bool) {
	for _, re := range []*regexp.Regexp{reIntervalFunc, reIntervalSQL} {
		if m := re.FindStringSubmatch(engineFull); len(m) == 2 {
			if n, err := strconv.ParseInt(m[1], 10, 32); err == nil && n > 0 {
				return int32(n), true
			}
		}
	}
	return 0, false
}
