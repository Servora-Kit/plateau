//go:build wireinject
// +build wireinject

package main

import (
	"context"

	auditconfv1 "github.com/Servora-Kit/servora-platform/api/gen/go/audit/service/conf/v1"
	"github.com/Servora-Kit/servora-platform/app/audit/service/internal/biz"
	"github.com/Servora-Kit/servora-platform/app/audit/service/internal/data"
	"github.com/Servora-Kit/servora-platform/app/audit/service/internal/server"
	"github.com/Servora-Kit/servora-platform/app/audit/service/internal/service"
	corev1 "github.com/Servora-Kit/servora/api/gen/go/servora/core/v1"
	auditcontractv1 "github.com/Servora-Kit/servora/api/gen/go/servora/extra/audit/v1"
	brokerv1 "github.com/Servora-Kit/servora/api/gen/go/servora/extra/broker/v1"
	"log/slog"

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
	*corev1.Server,
	*corev1.Registry,
	*corev1.App,
	*corev1.Trace,
	*corev1.Metrics,
	*brokerv1.Broker,
	*auditcontractv1.AuditContract,
	*auditconfv1.AuditConsumerConfig,
	bootstrap.SvcIdentity,
	*slog.Logger,
) (*kratos.App, func(), error) {
	panic(wire.Build(
		newKafkaBroker,
		data.ProviderSet,
		biz.ProviderSet,
		service.ProviderSet,
		server.ProviderSet,
		newApp,
	))
}
