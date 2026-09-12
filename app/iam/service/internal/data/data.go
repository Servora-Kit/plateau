package data

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"time"

	"entgo.io/ent/dialect"
	sessionpb "github.com/Servora-Kit/plateau/api/gen/go/plateau/security/session/v1"
	"github.com/Servora-Kit/plateau/app/iam/service/internal/biz"
	entmodel "github.com/Servora-Kit/plateau/app/iam/service/internal/data/ent"
	_ "github.com/Servora-Kit/plateau/app/iam/service/internal/data/ent/runtime"
	"github.com/Servora-Kit/plateau/security/cap"
	sessions "github.com/Servora-Kit/plateau/security/session"
	redispb "github.com/Servora-Kit/servora/api/gen/go/servora/contrib/db/redis/v1"
	corepb "github.com/Servora-Kit/servora/api/gen/go/servora/core/v1"
	entdriver "github.com/Servora-Kit/servora/contrib/db/entgo"
	rediscontrib "github.com/Servora-Kit/servora/contrib/db/redis"
	"github.com/alexedwards/scs/postgresstore"
	"github.com/alexedwards/scs/v2"
	"github.com/google/wire"
	_ "github.com/jackc/pgx/v5/stdlib"
	fgaclient "github.com/openfga/go-sdk/client"
	"github.com/redis/go-redis/v9"
)

// ProviderSet provides the IAM database driver, generated client, and data layer.
var ProviderSet = wire.NewSet(NewData, cap.New, NewCAPVerifier, NewUserRepository, NewCredentialRepository, NewSessionRepository, NewOAuthRepository, NewVerificationTokenRepo, NewPasswordResetTokenRepository, NewInitialUserCreator, NewSQLDB, NewEntDriver, NewDBClient, NewHTTPSessionManager, NewRedisClient, NewFGAClient)

// Data owns the IAM Ent client and repository-scoped persistence resources.
type Data struct {
	ent   *entmodel.Client
	redis *redis.Client
	fga   *fgaclient.OpenFgaClient
	log   *slog.Logger
}

// NewData installs the generated database, Redis and official OpenFGA clients.
func NewData(client *entmodel.Client, redis *redis.Client, openFGA *fgaclient.OpenFgaClient, l *slog.Logger) (*Data, error) {
	if client == nil {
		return nil, fmt.Errorf("ent client is nil")
	}
	if redis == nil {
		return nil, fmt.Errorf("redis client is nil")
	}
	if openFGA == nil {
		return nil, fmt.Errorf("OpenFGA client is nil")
	}
	if l == nil {
		l = slog.Default()
	}
	return &Data{ent: client, redis: redis, fga: openFGA, log: l.With("scope", "iam/data")}, nil
}

// NewCAPVerifier exposes CAP token validation through the biz-owned capability port.
func NewCAPVerifier(captcha *cap.Cap) biz.CAPVerifier {
	return captcha
}

// NewEntDriver resolves the configured SQL driver through Servora's Ent integration.
func NewEntDriver(config *corepb.Data, db *sql.DB) (dialect.Driver, error) {
	return entdriver.NewDriver(config, entdriver.WithDB(db))
}

// NewDBClient creates all IAM tables through Ent's idempotent schema migration.
func NewDBClient(driver dialect.Driver) (*entmodel.Client, func(), error) {
	if driver == nil {
		return nil, nil, fmt.Errorf("database driver is nil")
	}
	client := entmodel.NewClient(entmodel.Driver(driver))
	if err := client.Schema.Create(context.Background()); err != nil {
		_ = client.Close()
		return nil, nil, fmt.Errorf("create IAM schema: %w", err)
	}
	cleanup := func() { _ = client.Close() }
	return client, cleanup, nil
}

// NewRedisClient creates and verifies the shared Redis client.
func NewRedisClient(config *redispb.Redis) (*redis.Client, func(), error) {
	client, cleanup, err := rediscontrib.New(config)
	if err != nil {
		return nil, nil, err
	}
	return client, cleanup, nil
}

// NewSQLDB 拥有 Ent 和 SCS 共用的 pgx 连接池。
func NewSQLDB(config *corepb.Data) (*sql.DB, func(), error) {
	if config == nil || config.Database == nil || config.Database.GetSource() == "" {
		return nil, nil, fmt.Errorf("IAM PostgreSQL configuration is required")
	}
	switch config.Database.GetDriver() {
	case "postgres", "postgresql", "pgx":
	default:
		return nil, nil, fmt.Errorf("IAM requires PostgreSQL")
	}
	db, err := sql.Open("pgx", config.Database.GetSource())
	if err != nil {
		return nil, nil, fmt.Errorf("open IAM database: %w", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, nil, fmt.Errorf("connect IAM database: %w", err)
	}
	return db, func() { _ = db.Close() }, nil
}

// NewHTTPSessionManager 依赖已初始化的 Ent Schema，再启用 Store 清理流程。
func NewHTTPSessionManager(config *sessionpb.Session, db *sql.DB, client *entmodel.Client) (*scs.SessionManager, func(), error) {
	if db == nil || client == nil {
		return nil, nil, fmt.Errorf("IAM session database is nil")
	}
	store := postgresstore.New(db)
	manager, err := sessions.New(config, store)
	if err != nil {
		store.StopCleanup()
		return nil, nil, err
	}
	return manager, store.StopCleanup, nil
}
