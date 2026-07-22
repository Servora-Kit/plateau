//go:build wireinject
// +build wireinject

package main

import (
	"github.com/Servora-Kit/servora-platform/app/example/service/internal/biz"
	"github.com/Servora-Kit/servora-platform/app/example/service/internal/data"
	"github.com/Servora-Kit/servora-platform/app/example/service/internal/server"
	"github.com/Servora-Kit/servora-platform/app/example/service/internal/service"
	"github.com/Servora-Kit/servora/core/bootstrap"

	"github.com/go-kratos/kratos/v3"
	"github.com/google/wire"
)

func wireApp(
	*bootstrap.Runtime,
) (*kratos.App, func(), error) {
	panic(wire.Build(
		bootstrap.ProviderSet,
		data.ProviderSet,
		biz.ProviderSet,
		service.ProviderSet,
		server.ProviderSet,
		newApp,
	))
}
