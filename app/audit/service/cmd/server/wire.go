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
	clickhousepb "github.com/Servora-Kit/servora/api/gen/go/servora/infra/db/clickhouse/v1"
	kafkapb "github.com/Servora-Kit/servora/api/gen/go/servora/infra/kafka/v1"
	"github.com/Servora-Kit/servora/core/bootstrap"
	infrakafka "github.com/Servora-Kit/servora/infra/kafka"

	"github.com/go-kratos/kratos/v2"
	"github.com/google/wire"
	"github.com/twmb/franz-go/pkg/kgo"
)

func newKafkaClient(cfg *kafkapb.Kafka, auditCfg *auditcontractv1.AuditContract, l *slog.Logger) (*kgo.Client, error) {
	topic := data.DefaultTopic(auditCfg)
	group := data.DefaultConsumerGroup(cfg)
	return infrakafka.NewClientOptional(context.Background(), cfg, l,
		kgo.ConsumerGroup(group),
		kgo.ConsumeTopics(topic),
		kgo.DisableAutoCommit(),
	)
}

func wireApp(
	*bootstrap.Runtime,
	*kafkapb.Kafka,
	*clickhousepb.ClickHouse,
	*auditcontractv1.AuditContract,
	*auditconfv1.AuditConsumerConfig,
) (*kratos.App, func(), error) {
	panic(wire.Build(
		bootstrap.ProviderSet,
		newKafkaClient,
		data.ProviderSet,
		biz.ProviderSet,
		service.ProviderSet,
		server.ProviderSet,
		newApp,
	))
}
