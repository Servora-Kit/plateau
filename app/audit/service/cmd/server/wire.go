//go:build wireinject
// +build wireinject

package main

import (
	auditconfv1 "github.com/Servora-Kit/plateau/api/gen/go/audit/conf/v1"
	clickhousepb "github.com/Servora-Kit/plateau/api/gen/go/plateau/infra/clickhouse/v1"
	kafkapb "github.com/Servora-Kit/plateau/api/gen/go/plateau/infra/kafka/v1"
	openfgapb "github.com/Servora-Kit/plateau/api/gen/go/plateau/infra/openfga/v1"
	jwtconfpb "github.com/Servora-Kit/plateau/api/gen/go/plateau/security/authn/jwt/v1"
	"github.com/Servora-Kit/plateau/app/audit/service/internal/biz"
	"github.com/Servora-Kit/plateau/app/audit/service/internal/data"
	"github.com/Servora-Kit/plateau/app/audit/service/internal/server"
	internalservice "github.com/Servora-Kit/plateau/app/audit/service/internal/service"
	"github.com/Servora-Kit/servora/core/bootstrap"
	"github.com/go-kratos/kratos/v3"
	"github.com/google/wire"
)

func wireApp(
	*bootstrap.Runtime,
	*auditconfv1.Audit,
	*clickhousepb.ClickHouse,
	*kafkapb.Kafka,
	*jwtconfpb.JwtAuthnConfig,
	*openfgapb.OpenFGA,
) (*kratos.App, func(), error) {
	panic(wire.Build(
		bootstrap.ProviderSet,
		data.ProviderSet,
		biz.ProviderSet,
		internalservice.ProviderSet,
		server.ProviderSet,
		newApp,
	))
}
