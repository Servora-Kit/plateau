//go:build wireinject
// +build wireinject

package main

import (
	"context"
	"log/slog"

	auditconfv1 "github.com/Servora-Kit/servora-platform/api/gen/go/audit/service/conf/v1"
	"github.com/Servora-Kit/servora-platform/app/audit/service/internal/biz"
	"github.com/Servora-Kit/servora-platform/app/audit/service/internal/data"
	"github.com/Servora-Kit/servora-platform/app/audit/service/internal/server"
	"github.com/Servora-Kit/servora-platform/app/audit/service/internal/service"
	auditcontractv1 "github.com/Servora-Kit/servora/api/gen/go/servora/extra/audit/v1"
	brokerv1 "github.com/Servora-Kit/servora/api/gen/go/servora/extra/broker/v1"
	"github.com/Servora-Kit/servora/core/bootstrap"
	"github.com/Servora-Kit/servora/infra/broker"
	brokerkafka "github.com/Servora-Kit/servora/infra/broker/kafka"

	"github.com/go-kratos/kratos/v2"
	"github.com/google/wire"
)

// newKafkaBroker wraps NewBrokerOptional with a background context for Wire injection.
func newKafkaBroker(cfg *brokerv1.Broker, l *slog.Logger) broker.Broker {
	return brokerkafka.NewBrokerOptional(context.Background(), cfg, l)
}

func wireApp(
	*bootstrap.Runtime,
	*brokerv1.Broker,
	*auditcontractv1.AuditContract,
	*auditconfv1.AuditConsumerConfig,
) (*kratos.App, func(), error) {
	panic(wire.Build(
		bootstrap.ProviderSet,
		newKafkaBroker,
		data.ProviderSet,
		biz.ProviderSet,
		service.ProviderSet,
		server.ProviderSet,
		newApp,
	))
}
