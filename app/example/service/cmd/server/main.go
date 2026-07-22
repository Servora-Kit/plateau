package main

import (
	"flag"
	"fmt"

	"github.com/Servora-Kit/servora/core/bootstrap"

	"github.com/go-kratos/kratos/v3"
	"github.com/go-kratos/kratos/v3/registry"
	"github.com/go-kratos/kratos/v3/transport/grpc"
	"github.com/go-kratos/kratos/v3/transport/http"

	_ "go.uber.org/automaxprocs"
)

var (
	Name     = "example.service"
	Version  = "dev"
	flagconf string
)

func init() {
	flag.StringVar(&flagconf, "conf", "./configs/local", "config path, eg: -conf ./configs/local")
}

func newApp(rt *bootstrap.Runtime, reg registry.Registrar, gs *grpc.Server, hs *http.Server) *kratos.App {
	return rt.NewApp(
		kratos.Server(gs, hs),
		kratos.Registrar(reg),
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
	if err := bootstrap.Scan(rt); err != nil {
		return fmt.Errorf("scan bootstrap configs: %w", err)
	}

	return rt.Run(func() (*kratos.App, func(), error) {
		return wireApp(rt)
	})
}
