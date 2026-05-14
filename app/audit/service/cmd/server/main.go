package main

import (
	"context"
	"flag"
	"fmt"

	auditsvcpb "github.com/Servora-Kit/servora-platform/api/gen/go/servora/audit/service/v1"
	"github.com/Servora-Kit/servora-platform/app/audit/service/internal/data"
	auditcontractv1 "github.com/Servora-Kit/servora/api/gen/go/servora/extra/audit/v1"
	brokerv1 "github.com/Servora-Kit/servora/api/gen/go/servora/extra/broker/v1"
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

// localConfig is the wrapper struct used to extract this service's private
// AuditConsumerConfig from the merged kratos config via ScanConf. Once the
// servora BSR label that ships servora/conf/v1/annotations is available, this
// can move to a (section) annotation on AuditConsumerConfig + ScanSections.
type localConfig struct {
	AuditConsumer *auditsvcpb.AuditConsumerConfig `json:"audit_consumer"`
}

func main() {
	flag.Parse()

	err := bootstrap.BootstrapAndRun(flagconf, Name, Version, func(runtime *bootstrap.Runtime) (*kratos.App, func(), error) {
		bc := runtime.Bootstrap

		brokerCfg := &brokerv1.Broker{}
		auditCfg := &auditcontractv1.AuditContract{}
		if err := bootstrap.ScanSections(runtime, brokerCfg, auditCfg); err != nil {
			return nil, nil, fmt.Errorf("scan framework sections: %w", err)
		}

		lc, err := bootstrap.ScanConf[localConfig](runtime)
		if err != nil {
			return nil, nil, fmt.Errorf("scan audit_consumer config: %w", err)
		}
		consumerCfg := lc.AuditConsumer
		if consumerCfg == nil {
			consumerCfg = &auditsvcpb.AuditConsumerConfig{}
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
