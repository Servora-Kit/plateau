package main

import (
	"strings"
	"testing"
	"time"

	auditconfv1 "github.com/Servora-Kit/plateau/api/gen/go/audit/conf/v1"
	clickhousepb "github.com/Servora-Kit/plateau/api/gen/go/plateau/infra/clickhouse/v1"
	kafkapb "github.com/Servora-Kit/plateau/api/gen/go/plateau/infra/kafka/v1"
	openfgapb "github.com/Servora-Kit/plateau/api/gen/go/plateau/infra/openfga/v1"
	jwtconfpb "github.com/Servora-Kit/plateau/api/gen/go/plateau/security/authn/jwt/v1"
	"github.com/Servora-Kit/servora/core/bootstrap"
	"github.com/Servora-Kit/servora/core/bootstrap/config"
)

func TestDevelopmentConfigScan(t *testing.T) {
	for _, environment := range []string{"local", "docker"} {
		t.Run(environment, func(t *testing.T) {
			t.Setenv("FGA_STORE_ID", "01M2AY5F5K50N3M0Z9B911E84B")
			t.Setenv("FGA_MODEL_ID", "01M2AZ23CANE94JEQ5K56CS0P3")

			bc, loaded, err := config.LoadBootstrap("../../configs/"+environment, "audit.service", false)
			if err != nil {
				t.Fatalf("LoadBootstrap(%s) error = %v", environment, err)
			}
			t.Cleanup(func() { _ = loaded.Close() })

			if got := bc.GetServer().GetHttp().GetListen().GetAddr(); got != "0.0.0.0:10020" {
				t.Fatalf("http listen addr = %q, want 0.0.0.0:10020", got)
			}
			if got := bc.GetServer().GetGrpc().GetListen().GetAddr(); got != "0.0.0.0:10021" {
				t.Fatalf("grpc listen addr = %q, want 0.0.0.0:10021", got)
			}
			for _, tag := range bc.GetRegistry().GetConsul().GetTags() {
				if strings.Contains(strings.ToLower(tag), "traefik") {
					t.Fatalf("unexpected public traefik tag in %s bootstrap.yaml: %q", environment, tag)
				}
			}

			auditCfg := new(auditconfv1.Audit)
			clickHouseCfg := new(clickhousepb.ClickHouse)
			kafkaCfg := new(kafkapb.Kafka)
			jwtAuthnCfg := new(jwtconfpb.JwtAuthnConfig)
			openFGACfg := new(openfgapb.OpenFGA)
			if err := bootstrap.Scan(
				&bootstrap.Runtime{Bootstrap: bc, Config: loaded},
				auditCfg,
				clickHouseCfg,
				kafkaCfg,
				jwtAuthnCfg,
				openFGACfg,
			); err != nil {
				t.Fatalf("bootstrap.Scan(%s) error = %v", environment, err)
			}

			if auditCfg.GetTopic() != "plateau.audit.events" ||
				auditCfg.GetBatchSize() != 200 ||
				auditCfg.GetFlushInterval().AsDuration() != time.Second ||
				auditCfg.GetRetentionDays() != 180 {
				t.Fatalf("unexpected audit config (%s): %+v", environment, auditCfg)
			}
			if clickHouseCfg.GetDatabase() != "audit" || len(clickHouseCfg.GetAddrs()) != 1 {
				t.Fatalf("unexpected clickhouse config (%s): %+v", environment, clickHouseCfg)
			}
			if kafkaCfg.GetConsumerGroup() != "audit-consumer" || len(kafkaCfg.GetBrokers()) != 1 {
				t.Fatalf("unexpected kafka config (%s): %+v", environment, kafkaCfg)
			}
			if jwtAuthnCfg.GetAudience() != "audit" || jwtAuthnCfg.GetIssuer() == "" || jwtAuthnCfg.GetJwks().GetUri() == "" {
				t.Fatalf("unexpected jwt_authn_config (%s): %+v", environment, jwtAuthnCfg)
			}
			if environment == "docker" && !strings.HasPrefix(jwtAuthnCfg.GetJwks().GetUri(), "https://") {
				t.Fatalf("docker jwks.uri = %q, want https://", jwtAuthnCfg.GetJwks().GetUri())
			}
			if openFGACfg.GetStoreId() != "01M2AY5F5K50N3M0Z9B911E84B" || openFGACfg.GetApiUrl() == "" {
				t.Fatalf("unexpected open_fga config (%s): %+v", environment, openFGACfg)
			}
		})
	}
}
