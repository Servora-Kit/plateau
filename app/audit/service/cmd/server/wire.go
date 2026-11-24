//go:build wireinject
// +build wireinject

package main

import (
	"context"
	"log/slog"

	auditconfv1 "github.com/Servora-Kit/plateau/api/gen/go/audit/service/conf/v1"
	clickhousepb "github.com/Servora-Kit/plateau/api/gen/go/plateau/infra/clickhouse/v1"
	kafkapb "github.com/Servora-Kit/plateau/api/gen/go/plateau/infra/kafka/v1"
	auditpb "github.com/Servora-Kit/plateau/api/gen/go/plateau/security/audit/v1"
	"github.com/Servora-Kit/plateau/app/audit/service/internal/biz"
	"github.com/Servora-Kit/plateau/app/audit/service/internal/data"
	"github.com/Servora-Kit/plateau/app/audit/service/internal/server"
	"github.com/Servora-Kit/plateau/app/audit/service/internal/service"
	infrakafka "github.com/Servora-Kit/plateau/infra/kafka"
	"github.com/Servora-Kit/servora/core/bootstrap"

	"github.com/go-kratos/kratos/v3"
	"github.com/google/wire"
	"github.com/twmb/franz-go/pkg/kgo"
)

func newKafkaClient(cfg *kafkapb.Kafka, auditCfg *auditpb.AuditEmitter, l *slog.Logger) (*kgo.Client, error) {
	topic := data.DefaultTopic(auditCfg)
	group := data.DefaultConsumerGroup(cfg)
	loggerOpt := infrakafka.WithSlogLogger(l)
	if l != nil {
		loggerOpt = infrakafka.WithSlogLogger(l.With("scope", "audit/kafka"))
	}
	return infrakafka.NewClientOptional(context.Background(), cfg,
		loggerOpt,
		kgo.ConsumerGroup(group),
		kgo.ConsumeTopics(topic),
		kgo.DisableAutoCommit(),
	)
}

func wireApp(
	*bootstrap.Runtime,
	*kafkapb.Kafka,
	*clickhousepb.ClickHouse,
	*auditpb.AuditEmitter,
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
