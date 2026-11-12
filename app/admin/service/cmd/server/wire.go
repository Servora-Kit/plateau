//go:build wireinject
// +build wireinject

package main

import (
	adminconfpb "github.com/Servora-Kit/plateau/api/gen/go/admin/conf/v1"
	openfgapb "github.com/Servora-Kit/plateau/api/gen/go/plateau/infra/openfga/v1"
	oidcpb "github.com/Servora-Kit/plateau/api/gen/go/plateau/security/oidc/v1"
	sessionpb "github.com/Servora-Kit/plateau/api/gen/go/plateau/security/session/v1"
	"github.com/Servora-Kit/plateau/app/admin/service/internal/biz"
	"github.com/Servora-Kit/plateau/app/admin/service/internal/data"
	"github.com/Servora-Kit/plateau/app/admin/service/internal/server"
	"github.com/Servora-Kit/plateau/app/admin/service/internal/service"
	"github.com/Servora-Kit/plateau/app/admin/service/internal/startup"
	"github.com/Servora-Kit/servora/core/bootstrap"
	"github.com/go-kratos/kratos/v3"
	"github.com/google/wire"
)

func wireApp(
	*bootstrap.Runtime,
	*adminconfpb.Admin,
	*oidcpb.OIDCClient,
	*openfgapb.OpenFGA,
	*sessionpb.Session,
) (*kratos.App, func(), error) {
	panic(wire.Build(
		bootstrap.ProviderSet,
		data.ProviderSet,
		biz.ProviderSet,
		startup.ProviderSet,
		service.ProviderSet,
		server.ProviderSet,
		newApp,
	))
}
