package data

import (
	"context"
	"fmt"

	"entgo.io/ent/dialect"
	entmodel "github.com/Servora-Kit/servora-platform/app/example/service/internal/data/ent"
	_ "github.com/Servora-Kit/servora-platform/app/example/service/internal/data/ent/runtime"
	corev1 "github.com/Servora-Kit/servora/api/gen/go/servora/core/v1"
	entdriver "github.com/Servora-Kit/servora/contrib/db/entgo"
	"github.com/google/wire"
	_ "github.com/jackc/pgx/v5/stdlib"
	_ "github.com/mattn/go-sqlite3"
)

// ProviderSet contains data-layer providers. NewUserRepo returns biz.UserRepo directly.
var ProviderSet = wire.NewSet(NewEntDriver, NewDBClient, NewData, NewUserRepo)

// Data owns shared persistence resources.
type Data struct {
	client *entmodel.Client
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

// NewData installs the shared client and returns its lifecycle cleanup.
func NewData(client *entmodel.Client) (*Data, func(), error) {
	if client == nil {
		return nil, nil, fmt.Errorf("Ent client is nil")
	}
	cleanup := func() { _ = client.Close() }
	return &Data{client: client}, cleanup, nil
}

// Ent returns the generated Ent client for the current repository operation.
func (data *Data) Ent(context.Context) *entmodel.Client {
	return data.client
}
