// Package data manages ClickHouse persistence, schema initialization, and OpenFGA client resources for audit.service.
package data

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	auditconfv1 "github.com/Servora-Kit/plateau/api/gen/go/audit/conf/v1"
	clickhousepb "github.com/Servora-Kit/plateau/api/gen/go/plateau/infra/clickhouse/v1"
	"github.com/Servora-Kit/plateau/app/audit/service/internal/data/schema"
	pkgch "github.com/Servora-Kit/plateau/infra/clickhouse"
	fgapkg "github.com/Servora-Kit/plateau/infra/openfga"
	"github.com/google/wire"
)

// ProviderSet provides audit data-layer dependencies.
var ProviderSet = wire.NewSet(
	NewClickHouseConn,
	NewData,
	NewEventRepo,
	fgapkg.New,
)

// Data holds shared persistence resources owned by audit.service.
type Data struct {
	clickhouse driver.Conn
	log        *slog.Logger
}

// NewClickHouseConn opens and validates a mandatory ClickHouse connection.
func NewClickHouseConn(cfg *clickhousepb.ClickHouse, l *slog.Logger) (driver.Conn, func(), error) {
	if l == nil {
		l = slog.Default()
	}
	log := l.With("scope", "audit/data/clickhouse")

	conn, err := pkgch.NewConnOptional(context.Background(), cfg)
	if err != nil {
		return nil, nil, fmt.Errorf("audit clickhouse connect: %w", err)
	}
	if conn == nil {
		return nil, nil, fmt.Errorf("audit clickhouse: configuration with non-empty addrs is required")
	}

	cleanup := func() {
		if closeErr := conn.Close(); closeErr != nil {
			log.Warn("failed to close ClickHouse connection", "err", closeErr)
		}
	}
	return conn, cleanup, nil
}

// NewData ensures the ClickHouse events table and reconciles configured TTL before serving traffic.
func NewData(conn driver.Conn, auditCfg *auditconfv1.Audit, l *slog.Logger) (*Data, error) {
	if conn == nil {
		return nil, fmt.Errorf("audit data: ClickHouse connection is nil")
	}
	if l == nil {
		l = slog.Default()
	}
	log := l.With("scope", "audit/data")

	retentionDays := schema.DefaultRetentionDays
	if auditCfg != nil && auditCfg.GetRetentionDays() > 0 {
		retentionDays = auditCfg.GetRetentionDays()
	}

	if err := schema.EnsureEventsTable(context.Background(), conn, retentionDays); err != nil {
		return nil, err
	}
	log.Info("audit events table ensured", "retention_days", retentionDays)

	return &Data{
		clickhouse: conn,
		log:        log,
	}, nil
}

// ClickHouse returns the validated ClickHouse connection.
func (d *Data) ClickHouse() driver.Conn {
	if d == nil {
		return nil
	}
	return d.clickhouse
}
