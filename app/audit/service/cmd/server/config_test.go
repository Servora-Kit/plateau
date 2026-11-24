package main

import (
	"testing"

	auditconfpb "github.com/Servora-Kit/plateau/api/gen/go/audit/service/conf/v1"
	clickhousepb "github.com/Servora-Kit/plateau/api/gen/go/plateau/infra/clickhouse/v1"
	auditpb "github.com/Servora-Kit/plateau/api/gen/go/plateau/security/audit/v1"
	"github.com/Servora-Kit/servora/core/bootstrap"
	"github.com/Servora-Kit/servora/core/bootstrap/config"
)

func TestDevelopmentConfigSectionKeys(t *testing.T) {
	for _, environment := range []string{"local", "docker"} {
		t.Run(environment, func(t *testing.T) {
			root, loaded, err := config.LoadBootstrap("../../configs/"+environment, "audit.service", false)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = loaded.Close() })
			emitter, consumer, clickHouse := new(auditpb.AuditEmitter), new(auditconfpb.AuditConsumerConfig), new(clickhousepb.ClickHouse)
			if err := bootstrap.Scan(&bootstrap.Runtime{Bootstrap: root, Config: loaded}, emitter, consumer, clickHouse); err != nil {
				t.Fatal(err)
			}
			if !emitter.GetEnabled() || emitter.GetBackend() != "noop" || emitter.GetTopic() != "plateau.audit.events" || consumer.GetConsumerBatchSize() != 100 || consumer.GetRetentionDays() != 90 || len(clickHouse.GetAddrs()) != 1 {
				t.Fatalf("Audit 配置段未完整加载: emitter=%v consumer=%v clickhouse=%v", emitter, consumer, clickHouse)
			}
		})
	}
}
