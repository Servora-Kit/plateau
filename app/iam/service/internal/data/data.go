package data

import (
	"database/sql/driver"
	"log/slog"

	"github.com/google/wire"
)

// ProviderSet provides all data layer dependencies.
// NewAuditRepo returns biz.AuditRepo directly — no wire.Bind needed.
var ProviderSet = wire.NewSet(
	NewData,
)

// Data holds shared data layer resources for the audit service.
type Data struct {
	log *slog.Logger
}

// NewData initialises the audit data layer: it runs the ClickHouse DDL
// (idempotent) and owns the connection lifecycle. Mirrors IAM's NewData pattern.
func NewData(conn driver.Conn, l *slog.Logger) (*Data, func(), error) {
	log := l.With("scope", "data/iam")

	cleanup := func() {}
	return &Data{log: log}, cleanup, nil
}
