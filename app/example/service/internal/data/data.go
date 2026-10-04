package data

import (
	"context"
	"fmt"
	"log/slog"

	"entgo.io/ent/dialect"
	entmodel "github.com/Servora-Kit/plateau/app/example/service/internal/data/ent"
	_ "github.com/Servora-Kit/plateau/app/example/service/internal/data/ent/runtime"
	corev1 "github.com/Servora-Kit/servora/api/gen/go/servora/core/v1"
	entdriver "github.com/Servora-Kit/servora/contrib/db/entgo"
	"github.com/Servora-Kit/servora/obs/metrics"
	grpcclient "github.com/Servora-Kit/servora/transport/client/grpc"
	clientmw "github.com/Servora-Kit/servora/transport/client/middleware"
	"github.com/go-kratos/kratos/v3/registry"
	"github.com/google/wire"
	_ "github.com/jackc/pgx/v5/stdlib"
	_ "github.com/mattn/go-sqlite3"
)

// ProviderSet contains data-layer providers. NewUserRepo returns biz.UserRepo directly.
var ProviderSet = wire.NewSet(NewDialer, NewEntDriver, NewDBClient, NewData, NewUserRepo, NewExampleRepo)

// Data owns shared persistence resources.
type Data struct {
	grpcDialer *grpcclient.Dialer
	entClient  *entmodel.Client
}

// NewData installs the shared client and returns its lifecycle cleanup.
func NewData(dialer *grpcclient.Dialer, client *entmodel.Client) (*Data, func(), error) {
	if client == nil {
		return nil, nil, fmt.Errorf("Ent client is nil")
	}
	cleanup := func() { _ = client.Close() }
	return &Data{grpcDialer: dialer, entClient: client}, cleanup, nil
}

func NewDialer(data *corev1.Data, obs *corev1.Observability, mtc *metrics.Metrics, discovery registry.Discovery, l *slog.Logger) *grpcclient.Dialer {
	mw := clientmw.NewChainBuilder(l).
		WithTrace(obs.GetTrace()).
		WithMetrics(mtc).
		Build()
	return grpcclient.NewDialer(
		grpcclient.WithData(data),
		grpcclient.WithDiscovery(discovery),
		grpcclient.WithLogger(l),
		grpcclient.WithMiddleware(mw...),
	)
}

// NewEntDriver resolves the configured SQL driver through Servora's Ent integration.
func NewEntDriver(config *corev1.Data) (dialect.Driver, error) {
	return entdriver.NewDriver(config)
}

// NewDBClient creates and migrates the generated Ent client.
func NewDBClient(driver dialect.Driver) (*entmodel.Client, error) {
	client := entmodel.NewClient(entmodel.Driver(driver))
	if err := client.Schema.Create(context.Background()); err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("create example User schema: %w", err)
	}
	return client, nil
}

// Ent returns the generated Ent client for the current repository operation.
func (data *Data) Ent(context.Context) *entmodel.Client {
	return data.entClient
}
