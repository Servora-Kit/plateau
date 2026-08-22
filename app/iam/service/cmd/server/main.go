package main

import (
	"flag"
	"fmt"

	iamconfpb "github.com/Servora-Kit/plateau/api/gen/go/iam/conf/v1"
	oidcconfpb "github.com/Servora-Kit/plateau/api/gen/go/iam/oidc/conf/v1"
	mailpb "github.com/Servora-Kit/plateau/api/gen/go/plateau/infra/mail/v1"
	openfgapb "github.com/Servora-Kit/plateau/api/gen/go/plateau/infra/openfga/v1"
	"github.com/Servora-Kit/plateau/app/iam/service/internal/data"
	"github.com/Servora-Kit/plateau/app/iam/service/internal/startup"
	redispb "github.com/Servora-Kit/servora/api/gen/go/servora/contrib/db/redis/v1"
	"github.com/Servora-Kit/servora/core/bootstrap"

	"github.com/go-kratos/kratos/v3"
	"github.com/go-kratos/kratos/v3/registry"
	"github.com/go-kratos/kratos/v3/transport/grpc"
	"github.com/go-kratos/kratos/v3/transport/http"

	_ "go.uber.org/automaxprocs"
)

var (
	Name     = "iam.service"
	Version  = "dev"
	flagconf string
)

func init() {
	flag.StringVar(&flagconf, "conf", "./configs", "config path, eg: -conf ./configs/local")
}

func newApp(rt *bootstrap.Runtime, reg registry.Registrar, gs *grpc.Server, hs *http.Server, initializer *startup.Initializer, _ *data.Data) *kratos.App {
	return rt.NewApp(
		kratos.Server(gs, hs),
		kratos.Registrar(reg),
		kratos.BeforeStart(initializer.Initialize),
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
	iamCfg := &iamconfpb.IAM{}
	oidcCfg := &oidcconfpb.OIDC{}
	redisCfg := &redispb.Redis{}
	mailCfg := &mailpb.Mail{}
	openFGACfg := &openfgapb.OpenFGA{}
	if err := bootstrap.Scan(rt, iamCfg, oidcCfg, redisCfg, mailCfg, openFGACfg); err != nil {
		return fmt.Errorf("scan IAM configs: %w", err)
	}

	return rt.Run(func() (*kratos.App, func(), error) {
		return wireApp(rt, iamCfg, oidcCfg, redisCfg, mailCfg, openFGACfg)
	})
}
