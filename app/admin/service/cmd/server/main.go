package main

import (
	"context"
	"errors"
	"flag"
	"fmt"

	adminconfpb "github.com/Servora-Kit/plateau/api/gen/go/admin/conf/v1"
	openfgapb "github.com/Servora-Kit/plateau/api/gen/go/plateau/infra/openfga/v1"
	oidcpb "github.com/Servora-Kit/plateau/api/gen/go/plateau/security/oidc/v1"
	sessionpb "github.com/Servora-Kit/plateau/api/gen/go/plateau/security/session/v1"
	"github.com/Servora-Kit/plateau/app/admin/service/internal/data"
	"github.com/Servora-Kit/plateau/app/admin/service/internal/startup"
	"github.com/Servora-Kit/servora/core/bootstrap"
	"github.com/go-kratos/kratos/v3"
	"github.com/go-kratos/kratos/v3/registry"
	"github.com/go-kratos/kratos/v3/transport/http"

	_ "go.uber.org/automaxprocs"
)

var (
	Name     = "admin.service"
	Version  = "dev"
	flagconf string
)

func init() {
	flag.StringVar(&flagconf, "conf", "./configs", "config path, eg: -conf ./configs/local")
}

func newApp(rt *bootstrap.Runtime, reg registry.Registrar, hs *http.Server, initializer *startup.Initializer, _ *data.Data) *kratos.App {
	return rt.NewApp(
		kratos.Server(hs),
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

func run() error {
	rt, err := bootstrap.NewRuntime(flagconf, bootstrap.Name(Name), bootstrap.Version(Version))
	if err != nil {
		return err
	}
	adminCfg := &adminconfpb.Admin{}
	oidcClientCfg := &oidcpb.OIDCClient{}
	openFGACfg := &openfgapb.OpenFGA{}
	sessionCfg := &sessionpb.Session{}
	if err := bootstrap.Scan(rt, adminCfg, oidcClientCfg, openFGACfg, sessionCfg); err != nil {
		return errors.Join(fmt.Errorf("scan Admin configs: %w", err), rt.Close(context.Background()))
	}

	return rt.Run(func() (*kratos.App, func(), error) {
		return wireApp(rt, adminCfg, oidcClientCfg, openFGACfg, sessionCfg)
	})
}
