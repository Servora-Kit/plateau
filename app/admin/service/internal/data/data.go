// Package data implements Admin persistence, OpenFGA seed storage, and IAM gRPC clients.
package data

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"time"

	"entgo.io/ent/dialect"
	oidcpb "github.com/Servora-Kit/plateau/api/gen/go/plateau/security/oidc/v1"
	sessionpb "github.com/Servora-Kit/plateau/api/gen/go/plateau/security/session/v1"
	entmodel "github.com/Servora-Kit/plateau/app/admin/service/internal/data/ent"
	_ "github.com/Servora-Kit/plateau/app/admin/service/internal/data/ent/runtime"
	fgapkg "github.com/Servora-Kit/plateau/infra/openfga"
	securityoidc "github.com/Servora-Kit/plateau/security/oidc"
	sessions "github.com/Servora-Kit/plateau/security/session"
	corepb "github.com/Servora-Kit/servora/api/gen/go/servora/core/v1"
	entdriver "github.com/Servora-Kit/servora/contrib/db/entgo"
	"github.com/Servora-Kit/servora/obs/metrics"
	grpcclient "github.com/Servora-Kit/servora/transport/client/grpc"
	clientmw "github.com/Servora-Kit/servora/transport/client/middleware"
	"github.com/alexedwards/scs/v2"
	"github.com/go-kratos/kratos/v3/registry"
	"github.com/google/wire"
	_ "github.com/jackc/pgx/v5/stdlib"
	fgaclient "github.com/openfga/go-sdk/client"
)

// ProviderSet provides the Admin database, SCS session store, OpenFGA client, and IAM gRPC client.
var ProviderSet = wire.NewSet(
	NewSQLDB,
	NewEntDriver,
	NewDBClient,
	NewHTTPSessionManager,
	NewDialer,
	NewData,
	NewIAMUserRepo,
	NewAdminSeedRepo,
	fgapkg.New,
)

// Data owns shared Admin persistence and downstream client resources.
type Data struct {
	grpcDialer *grpcclient.Dialer
	ent        *entmodel.Client
	fga        *fgaclient.OpenFgaClient
}

// NewData validates and bundles shared data-layer dependencies.
func NewData(dialer *grpcclient.Dialer, client *entmodel.Client, fga *fgaclient.OpenFgaClient) (*Data, error) {
	if dialer == nil {
		return nil, fmt.Errorf("admin data: grpc dialer is nil")
	}
	if client == nil {
		return nil, fmt.Errorf("admin data: ent client is nil")
	}
	if fga == nil {
		return nil, fmt.Errorf("admin data: OpenFGA client is nil")
	}
	return &Data{grpcDialer: dialer, ent: client, fga: fga}, nil
}

// NewSQLDB opens and pings the PostgreSQL pool shared by Ent schema migration and SCS.
func NewSQLDB(config *corepb.Data) (*sql.DB, func(), error) {
	if config.GetDatabase().GetSource() == "" {
		return nil, nil, fmt.Errorf("admin PostgreSQL configuration is required")
	}
	switch config.GetDatabase().GetDriver() {
	case "postgres", "postgresql", "pgx":
	default:
		return nil, nil, fmt.Errorf("admin requires PostgreSQL")
	}
	db, err := sql.Open("pgx", config.GetDatabase().GetSource())
	if err != nil {
		return nil, nil, fmt.Errorf("open admin database: %w", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, nil, fmt.Errorf("connect admin database: %w", err)
	}
	return db, func() { _ = db.Close() }, nil
}

// NewEntDriver resolves the configured SQL driver through Servora's Ent integration.
func NewEntDriver(config *corepb.Data, db *sql.DB) (dialect.Driver, error) {
	return entdriver.NewDriver(config, entdriver.WithDB(db))
}

// NewDBClient runs idempotent Ent schema migration for Admin's local tables.
func NewDBClient(driver dialect.Driver) (*entmodel.Client, func(), error) {
	if driver == nil {
		return nil, nil, fmt.Errorf("admin database driver is nil")
	}
	client := entmodel.NewClient(entmodel.Driver(driver))
	if err := client.Schema.Create(context.Background()); err != nil {
		_ = client.Close()
		return nil, nil, fmt.Errorf("create admin schema: %w", err)
	}
	return client, func() { _ = client.Close() }, nil
}

// NewHTTPSessionManager depends on the migrated Ent client so sessions table exists before SCS cleanup runs.
func NewHTTPSessionManager(config *sessionpb.Session, db *sql.DB, client *entmodel.Client) (*scs.SessionManager, func(), error) {
	if client == nil {
		return nil, nil, fmt.Errorf("admin session schema client is nil")
	}
	return sessions.NewPostgres(config, db)
}

// NewDialer constructs Servora's gRPC Dialer with tracing, metrics, logging, and OAuth2 client_credentials middleware.
func NewDialer(
	dataCfg *corepb.Data,
	obs *corepb.Observability,
	mtc *metrics.Metrics,
	oidcCfg *oidcpb.OIDCClient,
	discovery registry.Discovery,
	l *slog.Logger,
) (*grpcclient.Dialer, func(), error) {
	tokenMW, cleanup, err := securityoidc.NewServiceTokenMiddleware(oidcCfg)
	if err != nil {
		return nil, nil, fmt.Errorf("build service token middleware: %w", err)
	}
	mw := clientmw.NewChainBuilder(l).
		WithTrace(obs.GetTrace()).
		WithMetrics(mtc).
		WithoutCircuitBreaker().
		Build()
	mw = append(mw, tokenMW)

	dialer := grpcclient.NewDialer(
		grpcclient.WithData(dataCfg),
		grpcclient.WithDiscovery(discovery),
		grpcclient.WithLogger(l),
		grpcclient.WithMiddleware(mw...),
	)
	return dialer, cleanup, nil
}
