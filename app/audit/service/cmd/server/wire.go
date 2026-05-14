//go:build wireinject
// +build wireinject

package main

import (
	"context"

	auditsvcpb "github.com/Servora-Kit/servora-platform/api/gen/go/servora/audit/service/v1"
	"github.com/Servora-Kit/servora-platform/app/audit/service/internal/biz"
	"github.com/Servora-Kit/servora-platform/app/audit/service/internal/data"
	"github.com/Servora-Kit/servora-platform/app/audit/service/internal/server"
	"github.com/Servora-Kit/servora-platform/app/audit/service/internal/service"
	corev1 "github.com/Servora-Kit/servora/api/gen/go/servora/core/v1"
	auditcontractv1 "github.com/Servora-Kit/servora/api/gen/go/servora/extra/audit/v1"
	brokerv1 "github.com/Servora-Kit/servora/api/gen/go/servora/extra/broker/v1"
	"github.com/Servora-Kit/servora/core/bootstrap"
	"github.com/Servora-Kit/servora/infra/broker"
	brokerkafka "github.com/Servora-Kit/servora/infra/broker/kafka"
	"github.com/Servora-Kit/servora/obs/logging"

	"github.com/go-kratos/kratos/v2"
	"github.com/go-kratos/kratos/v2/log"
	"github.com/google/wire"
)

// newKafkaBroker wraps NewBrokerOptional with a background context for Wire injection.
func newKafkaBroker(cfg *brokerv1.Broker, l logger.Logger) broker.Broker {
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
	*auditsvcpb.AuditConsumerConfig,
	bootstrap.SvcIdentity,
	log.Logger,
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
