package data

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	auditconfv1 "github.com/Servora-Kit/servora-platform/api/gen/go/audit/service/conf/v1"
	"github.com/google/wire"
)

// ProviderSet provides all data layer dependencies.
// NewAuditRepo returns biz.AuditRepo directly — no wire.Bind needed.
var ProviderSet = wire.NewSet(
	NewClickHouseClient,
	NewData,
	NewBatchWriter,
	NewConsumer,
	NewAuditRepo,
)

// Data holds shared data layer resources for the audit service.
type Data struct {
	clickhouse driver.Conn
	log        *slog.Logger
}

// NewData initialises the audit data layer: it runs the ClickHouse DDL
// (idempotent) and owns the connection lifecycle. Mirrors IAM's NewData pattern.
func NewData(conn driver.Conn, auditCfg *auditconfv1.AuditConsumerConfig, l *slog.Logger) (*Data, func(), error) {
	log := l.With("scope", "core/data/audit")

	cleanup := func() {
		log.Info("closing ClickHouse connection")
		if conn != nil {
			if err := conn.Close(); err != nil {
				log.Warn("failed to close ClickHouse connection", "err", err)
			}
		}
	}

	if conn != nil {
		retentionDays := int32(90)
		if auditCfg != nil && auditCfg.GetRetentionDays() > 0 {
			retentionDays = auditCfg.GetRetentionDays()
		}
		if err := createAuditEventsTable(context.Background(), conn, retentionDays); err != nil {
			cleanup()
			return nil, nil, fmt.Errorf("create audit_events table: %w", err)
		}
		log.Info("audit_events table ensured")
	}

	return &Data{clickhouse: conn, log: log}, cleanup, nil
}

// ClickHouse returns the ClickHouse connection. May be nil when ClickHouse is
// not configured; callers should nil-check before use.
func (d *Data) ClickHouse() driver.Conn {
	return d.clickhouse
}
