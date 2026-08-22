//go:build wireinject
// +build wireinject

package main

import (
	iamconfpb "github.com/Servora-Kit/plateau/api/gen/go/iam/conf/v1"
	oidcconfpb "github.com/Servora-Kit/plateau/api/gen/go/iam/oidc/conf/v1"
	mailpb "github.com/Servora-Kit/plateau/api/gen/go/plateau/infra/mail/v1"
	openfgapb "github.com/Servora-Kit/plateau/api/gen/go/plateau/infra/openfga/v1"
	"github.com/Servora-Kit/plateau/app/iam/service/internal/biz"
	"github.com/Servora-Kit/plateau/app/iam/service/internal/data"
	mailservice "github.com/Servora-Kit/plateau/app/iam/service/internal/mail"
	internaloidc "github.com/Servora-Kit/plateau/app/iam/service/internal/oidc"
	"github.com/Servora-Kit/plateau/app/iam/service/internal/server"
	internalservice "github.com/Servora-Kit/plateau/app/iam/service/internal/service"
	"github.com/Servora-Kit/plateau/app/iam/service/internal/startup"
	redispb "github.com/Servora-Kit/servora/api/gen/go/servora/contrib/db/redis/v1"
	"github.com/Servora-Kit/servora/core/bootstrap"
	"github.com/go-kratos/kratos/v3"
	"github.com/google/wire"
)

func wireApp(
	*bootstrap.Runtime,
	*iamconfpb.IAM,
	*oidcconfpb.OIDC,
	*redispb.Redis,
	*mailpb.Mail,
	*openfgapb.OpenFGA,
) (*kratos.App, func(), error) {
	panic(wire.Build(
		bootstrap.ProviderSet,
		data.ProviderSet,
		biz.ProviderSet,
		internaloidc.ProviderSet,
		startup.ProviderSet,
		mailservice.ProviderSet,
		internalservice.ProviderSet,
		server.ProviderSet,
		newApp,
	))
}
