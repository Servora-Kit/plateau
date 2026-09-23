package main

import (
	"testing"
	"time"

	iampb "github.com/Servora-Kit/plateau/api/gen/go/iam/conf/v1"
	oidcpb "github.com/Servora-Kit/plateau/api/gen/go/iam/oidc/conf/v1"
	openfgapb "github.com/Servora-Kit/plateau/api/gen/go/plateau/infra/openfga/v1"
	sessionpb "github.com/Servora-Kit/plateau/api/gen/go/plateau/security/session/v1"
	"github.com/Servora-Kit/plateau/security/session"
	"github.com/Servora-Kit/servora/core/bootstrap"
	"github.com/Servora-Kit/servora/core/bootstrap/config"
	"github.com/alexedwards/scs/v2/memstore"
)

func TestDevelopmentConfigScan(t *testing.T) {
	for _, environment := range []string{"local", "docker"} {
		t.Run(environment, func(t *testing.T) {
			t.Setenv("IAM_TEST_WEB_CLIENT_SECRET", "test-web-secret-at-least-32-bytes")
			t.Setenv("IAM_ADMIN_CLIENT_SECRET", "admin-secret-at-least-32-bytes")
			t.Setenv("IAM_CAP_SIGNING_SECRET", "cap-signing-secret-at-least-32-bytes")
			t.Setenv("IAM_BOOTSTRAP_USER_EMAIL", "seed@example.com")
			t.Setenv("FGA_STORE_ID", "01M2AY5F5K50N3M0Z9B911E84B")
			t.Setenv("FGA_MODEL_ID", "01M2AZ23CANE94JEQ5K56CS0P3")
			bc, loaded, err := config.LoadBootstrap("../../configs/"+environment, "iam.service", false)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = loaded.Close() })
			iam, oidc, httpSession, openFGA := new(iampb.IAM), new(oidcpb.OIDC), new(sessionpb.Session), new(openfgapb.OpenFGA)
			if err := bootstrap.Scan(&bootstrap.Runtime{Bootstrap: bc, Config: loaded}, iam, oidc, httpSession, openFGA); err != nil {
				t.Fatal(err)
			}
			store := memstore.NewWithCleanupInterval(0)
			manager, err := session.New(httpSession, store)
			if err != nil {
				t.Fatal(err)
			}
			if manager.Lifetime != 30*24*time.Hour || manager.IdleTimeout != 7*24*time.Hour || !manager.Cookie.Secure || !manager.Cookie.HttpOnly {
				t.Fatal("session configuration/defaults did not load")
			}
			if oidc.GetServiceAccessTokenTtl().AsDuration() != 5*time.Minute || iam.GetBootstrapUserEmail() != "seed@example.com" || oidc.GetIssuer() != bc.GetApp().GetExternalUrl() {
				t.Fatal("IAM configuration/defaults disagree")
			}
			if openFGA.GetStoreId() != "01M2AY5F5K50N3M0Z9B911E84B" || openFGA.GetApiUrl() == "" {
				t.Fatal("OpenFGA 配置段未加载")
			}
		})
	}
}
