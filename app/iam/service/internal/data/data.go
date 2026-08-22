package data

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"entgo.io/ent/dialect"
	"github.com/Servora-Kit/plateau/app/iam/service/internal/biz"
	entmodel "github.com/Servora-Kit/plateau/app/iam/service/internal/data/ent"
	_ "github.com/Servora-Kit/plateau/app/iam/service/internal/data/ent/runtime"
	capservice "github.com/Servora-Kit/plateau/security/cap"
	redispb "github.com/Servora-Kit/servora/api/gen/go/servora/contrib/db/redis/v1"
	corepb "github.com/Servora-Kit/servora/api/gen/go/servora/core/v1"
	entdriver "github.com/Servora-Kit/servora/contrib/db/entgo"
	rediscontrib "github.com/Servora-Kit/servora/contrib/db/redis"
	"github.com/google/wire"
	_ "github.com/jackc/pgx/v5/stdlib"
	fgaclient "github.com/openfga/go-sdk/client"
	goredis "github.com/redis/go-redis/v9"
)

// ProviderSet provides the IAM database driver, generated client, and data layer.
var ProviderSet = wire.NewSet(NewData, NewCAP, NewCAPVerifier, NewUserRepository, NewCredentialRepository, NewSessionRepository, NewTokenSessionRepository, NewVerificationTokenRepo, NewPasswordResetTokenRepository, NewInitialAdminCreator, NewAdminRelationWriter, NewEntDriver, NewDBClient, NewRedisClient, NewFGAClient)

// Data owns the IAM Ent client and repository-scoped persistence resources.
type Data struct {
	ent   *entmodel.Client
	redis *goredis.Client
	fga   *fgaclient.OpenFgaClient
	log   *slog.Logger
}

// NewData installs the generated database, Redis and official OpenFGA clients.
func NewData(client *entmodel.Client, redis *goredis.Client, openFGA *fgaclient.OpenFgaClient, l *slog.Logger) (*Data, error) {
	if client == nil {
		return nil, fmt.Errorf("Ent client is nil")
	}
	if redis == nil {
		return nil, fmt.Errorf("Redis client is nil")
	}
	if openFGA == nil {
		return nil, fmt.Errorf("OpenFGA client is nil")
	}
	if l == nil {
		l = slog.Default()
	}
	return &Data{ent: client, redis: redis, fga: openFGA, log: l.With("scope", "iam/data")}, nil
}

// InTx executes repository work atomically and rolls back on errors or panic.
func (data *Data) InTx(ctx context.Context, fn func(*entmodel.Tx) error) (err error) {
	if fn == nil {
		return fmt.Errorf("transaction function is nil")
	}
	tx, err := data.ent.Tx(ctx)
	if err != nil {
		return fmt.Errorf("begin IAM transaction: %w", err)
	}
	defer func() {
		if panicValue := recover(); panicValue != nil {
			_ = tx.Rollback()
			panic(panicValue)
		}
	}()
	if err := fn(tx); err != nil {
		return errors.Join(err, tx.Rollback())
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit IAM transaction: %w", err)
	}
	return nil
}

// NewCAP binds the shared CAP implementation to IAM's required Redis client.
func NewCAP(data *Data) *capservice.Cap {
	return capservice.New(data.redis)
}

// NewCAPVerifier exposes CAP token validation through the biz-owned capability port.
func NewCAPVerifier(captcha *capservice.Cap) biz.CAPVerifier {
	return captcha
}

// NewEntDriver resolves the configured SQL driver through Servora's Ent integration.
func NewEntDriver(config *corepb.Data) (dialect.Driver, error) {
	return entdriver.NewDriver(config)
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
func NewRedisClient(config *redispb.Redis, l *slog.Logger) (*goredis.Client, func(), error) {
	client, cleanup, err := rediscontrib.New(config, l)
	if err != nil {
		return nil, nil, err
	}
	return client, cleanup, nil
}
