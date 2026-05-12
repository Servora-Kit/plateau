package main

import (
	"context"
	"flag"

	"github.com/Servora-Kit/servora-platform/app/audit/service/internal/data"
	"github.com/Servora-Kit/servora/core/bootstrap"

	"github.com/go-kratos/kratos/v2"
	"github.com/go-kratos/kratos/v2/log"
	"github.com/go-kratos/kratos/v2/registry"
	"github.com/go-kratos/kratos/v2/transport/grpc"
	"github.com/go-kratos/kratos/v2/transport/http"

	_ "go.uber.org/automaxprocs"
)

var (
	Name     = "audit.service"
	Version  = "dev"
	flagconf string
)

func init() {
	flag.StringVar(&flagconf, "conf", "./configs", "config path, eg: -conf config.yaml")
}

func newApp(identity bootstrap.SvcIdentity, l log.Logger, reg registry.Registrar, gs *grpc.Server, hs *http.Server, consumer *data.Consumer) *kratos.App {
	return kratos.New(
		kratos.ID(identity.ID),
		kratos.Name(identity.Name),
		kratos.Version(identity.Version),
		kratos.Metadata(identity.Metadata),
		kratos.Logger(l),
		kratos.Server(gs, hs),
		kratos.Registrar(reg),
		kratos.BeforeStart(func(ctx context.Context) error {
			return consumer.Start(ctx)
		}),
		kratos.AfterStop(func(ctx context.Context) error {
			return consumer.Stop(ctx)
		}),
	)
}

func main() {
	flag.Parse()

	err := bootstrap.BootstrapAndRun(flagconf, Name, Version, func(runtime *bootstrap.Runtime) (*kratos.App, func(), error) {
		bc := runtime.Bootstrap
		return wireApp(bc.Server, bc.Registry, bc.Data, bc.App, bc.Trace, bc.Metrics, runtime.Identity, runtime.Logger)
	})
	if err != nil {
		panic(err)
	}
}
