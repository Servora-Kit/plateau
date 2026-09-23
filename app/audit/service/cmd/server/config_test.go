package main

import (
	"testing"

	auditconfpb "github.com/Servora-Kit/plateau/api/gen/go/audit/service/conf/v1"
	clickhousepb "github.com/Servora-Kit/plateau/api/gen/go/plateau/infra/clickhouse/v1"
	auditcontractpb "github.com/Servora-Kit/servora/api/gen/go/servora/obs/audit/v1"
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
			contract, consumer, clickHouse := new(auditcontractpb.AuditContract), new(auditconfpb.AuditConsumerConfig), new(clickhousepb.ClickHouse)
			if err := bootstrap.Scan(&bootstrap.Runtime{Bootstrap: root, Config: loaded}, contract, consumer, clickHouse); err != nil {
				t.Fatal(err)
			}
			if !contract.GetEnabled() || contract.GetServiceName() != "audit" || contract.GetTopic() != "servora.audit.events" || consumer.GetConsumerBatchSize() != 100 || consumer.GetRetentionDays() != 90 || len(clickHouse.GetAddrs()) != 1 {
				t.Fatalf("Audit 配置段未完整加载: contract=%v consumer=%v clickhouse=%v", contract, consumer, clickHouse)
			}
		})
	}
}
