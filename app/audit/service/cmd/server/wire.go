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
	clickhousepb "github.com/Servora-Kit/servora/api/gen/go/servora/contrib/db/clickhouse/v1"
	kafkapb "github.com/Servora-Kit/servora/api/gen/go/servora/contrib/kafka/v1"
	auditconfpb "github.com/Servora-Kit/servora/api/gen/go/servora/obs/audit/v1"
	contribkafka "github.com/Servora-Kit/servora/contrib/kafka"
	"github.com/Servora-Kit/servora/core/bootstrap"

	"github.com/go-kratos/kratos/v2"
	"github.com/google/wire"
	"github.com/twmb/franz-go/pkg/kgo"
)

func newKafkaClient(cfg *kafkapb.Kafka, auditCfg *auditconfpb.AuditContract, l *slog.Logger) (*kgo.Client, error) {
	topic := data.DefaultTopic(auditCfg)
	group := data.DefaultConsumerGroup(cfg)
	return contribkafka.NewClientOptional(context.Background(), cfg, l,
		kgo.ConsumerGroup(group),
		kgo.ConsumeTopics(topic),
		kgo.DisableAutoCommit(),
	)
}

func wireApp(
	*bootstrap.Runtime,
	*kafkapb.Kafka,
	*clickhousepb.ClickHouse,
	*auditconfpb.AuditContract,
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
