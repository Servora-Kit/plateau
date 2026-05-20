package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"

	auditconfv1 "github.com/Servora-Kit/servora-platform/api/gen/go/audit/service/conf/v1"
	"github.com/Servora-Kit/servora-platform/app/audit/service/internal/data"
	auditcontractv1 "github.com/Servora-Kit/servora/api/gen/go/servora/extra/audit/v1"
	brokerv1 "github.com/Servora-Kit/servora/api/gen/go/servora/extra/broker/v1"
	"github.com/Servora-Kit/servora/core/bootstrap"
	kratosv2 "github.com/Servora-Kit/servora/obs/logger/kratosv2"

	"github.com/go-kratos/kratos/v2"
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

func newApp(identity bootstrap.SvcIdentity, l *slog.Logger, reg registry.Registrar, gs *grpc.Server, hs *http.Server, consumer *data.Consumer) *kratos.App {
	return kratos.New(
		kratos.ID(identity.ID),
		kratos.Name(identity.Name),
		kratos.Version(identity.Version),
		kratos.Metadata(identity.Metadata),
		kratos.Logger(kratosv2.Wrap(l)),
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

		brokerCfg := &brokerv1.Broker{}
		auditCfg := &auditcontractv1.AuditContract{}
		consumerCfg := &auditconfv1.AuditConsumerConfig{}
		if err := bootstrap.ScanSections(runtime, brokerCfg, auditCfg, consumerCfg); err != nil {
			return nil, nil, fmt.Errorf("scan sections: %w", err)
		}

		return wireApp(
			bc.Server, bc.Registry, bc.App, bc.Trace, bc.Metrics,
			brokerCfg, auditCfg, consumerCfg,
			runtime.Identity, runtime.Logger,
		)
	})
	if err != nil {
		panic(err)
	}
}
