//go:build wireinject
// +build wireinject

package main

import (
	"github.com/Servora-Kit/plateau/app/test/service/internal/server"
	"github.com/Servora-Kit/plateau/app/test/service/internal/service"
	"github.com/Servora-Kit/servora/core/bootstrap"

	"github.com/go-kratos/kratos/v3"
	"github.com/google/wire"
)

func wireApp(
	*bootstrap.Runtime,
) (*kratos.App, func(), error) {
	panic(wire.Build(
		bootstrap.ProviderSet,
		service.ProviderSet,
		server.ProviderSet,
		newApp,
	))
}
