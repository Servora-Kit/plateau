package main

import (
	"context"
	"flag"
	"fmt"

	auditconfv1 "github.com/Servora-Kit/plateau/api/gen/go/audit/service/conf/v1"
	"github.com/Servora-Kit/plateau/app/audit/service/internal/data"
	clickhousepb "github.com/Servora-Kit/plateau/api/gen/go/plateau/infra/clickhouse/v1"
	kafkapb "github.com/Servora-Kit/servora/api/gen/go/servora/contrib/kafka/v1"
	auditconfpb "github.com/Servora-Kit/servora/api/gen/go/servora/obs/audit/v1"
	"github.com/Servora-Kit/servora/core/bootstrap"

	"github.com/go-kratos/kratos/v3"
	"github.com/go-kratos/kratos/v3/registry"
	"github.com/go-kratos/kratos/v3/transport/grpc"
	"github.com/go-kratos/kratos/v3/transport/http"

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

func newApp(rt *bootstrap.Runtime, reg registry.Registrar, gs *grpc.Server, hs *http.Server, consumer *data.Consumer) *kratos.App {
	return rt.NewApp(
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
	if err := run(); err != nil {
		panic(err)
	}
}

func run() (err error) {
	rt, err := bootstrap.NewRuntime(flagconf, bootstrap.Name(Name), bootstrap.Version(Version))
	if err != nil {
		return err
	}
	kafkaCfg := &kafkapb.Kafka{}
	clickHouseCfg := &clickhousepb.ClickHouse{}
	auditCfg := &auditconfpb.AuditContract{}
	consumerCfg := &auditconfv1.AuditConsumerConfig{}
	if err := bootstrap.Scan(rt, kafkaCfg, clickHouseCfg, auditCfg, consumerCfg); err != nil {
		return fmt.Errorf("scan bootstrap configs: %w", err)
	}

	return rt.Run(func() (*kratos.App, func(), error) {
		return wireApp(rt, kafkaCfg, clickHouseCfg, auditCfg, consumerCfg)
	})
}
