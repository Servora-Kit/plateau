package main

import (
	"context"
	"errors"
	"flag"
	"fmt"

	auditconfv1 "github.com/Servora-Kit/plateau/api/gen/go/plateau/audit/conf/v1"
	clickhousepb "github.com/Servora-Kit/plateau/api/gen/go/plateau/infra/clickhouse/v1"
	kafkapb "github.com/Servora-Kit/plateau/api/gen/go/plateau/infra/kafka/v1"
	openfgapb "github.com/Servora-Kit/plateau/api/gen/go/plateau/infra/openfga/v1"
	jwtconfpb "github.com/Servora-Kit/plateau/api/gen/go/plateau/security/authn/jwt/v1"
	"github.com/Servora-Kit/plateau/app/audit/service/internal/data"
	svrkafka "github.com/Servora-Kit/plateau/transport/server/kafka"
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
	flag.StringVar(&flagconf, "conf", "./configs", "config path, eg: -conf ./configs/local")
}

func newApp(
	rt *bootstrap.Runtime,
	reg registry.Registrar,
	gs *grpc.Server,
	hs *http.Server,
	ks *svrkafka.Server,
	_ *data.Data,
) *kratos.App {
	return rt.NewApp(
		kratos.Server(gs, hs, ks),
		kratos.Registrar(reg),
	)
}

func main() {
	flag.Parse()
	if err := run(); err != nil {
		panic(err)
	}
}

func run() error {
	rt, err := bootstrap.NewRuntime(flagconf, bootstrap.Name(Name), bootstrap.Version(Version))
	if err != nil {
		return err
	}

	auditCfg := &auditconfv1.Audit{}
	clickHouseCfg := &clickhousepb.ClickHouse{}
	kafkaCfg := &kafkapb.Kafka{}
	jwtAuthnCfg := &jwtconfpb.JwtAuthnConfig{}
	openFGACfg := &openfgapb.OpenFGA{}
	if err := bootstrap.Scan(rt, auditCfg, clickHouseCfg, kafkaCfg, jwtAuthnCfg, openFGACfg); err != nil {
		return errors.Join(fmt.Errorf("scan Audit configs: %w", err), rt.Close(context.Background()))
	}

	return rt.Run(func() (*kratos.App, func(), error) {
		return wireApp(rt, auditCfg, clickHouseCfg, kafkaCfg, jwtAuthnCfg, openFGACfg)
	})
}
