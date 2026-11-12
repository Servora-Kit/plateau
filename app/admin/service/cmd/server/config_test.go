package main

import (
	"testing"
	"time"

	adminconfpb "github.com/Servora-Kit/plateau/api/gen/go/admin/conf/v1"
	openfgapb "github.com/Servora-Kit/plateau/api/gen/go/plateau/infra/openfga/v1"
	oidcpb "github.com/Servora-Kit/plateau/api/gen/go/plateau/security/oidc/v1"
	sessionpb "github.com/Servora-Kit/plateau/api/gen/go/plateau/security/session/v1"
	securityoidc "github.com/Servora-Kit/plateau/security/oidc"
	"github.com/Servora-Kit/plateau/security/session"
	"github.com/Servora-Kit/servora/core/bootstrap"
	"github.com/Servora-Kit/servora/core/bootstrap/config"
	"github.com/alexedwards/scs/v2/memstore"
)

func TestDevelopmentConfigScan(t *testing.T) {
	for _, environment := range []string{"local", "docker"} {
		t.Run(environment, func(t *testing.T) {
			t.Setenv("IAM_ADMIN_CLIENT_SECRET", "admin-secret-at-least-32-bytes!!")
			t.Setenv("ADMIN_OIDC_COOKIE_SECRET", "admin-cookie-secret-at-least-32-bytes")
			t.Setenv("IAM_BOOTSTRAP_USER_EMAIL", "seed@example.com")
			t.Setenv("FGA_STORE_ID", "01M2AY5F5K50N3M0Z9B911E84B")
			t.Setenv("FGA_MODEL_ID", "01M2AZ23CANE94JEQ5K56CS0P3")

			bc, loaded, err := config.LoadBootstrap("../../configs/"+environment, "admin.service", false)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = loaded.Close() })

			adminCfg := new(adminconfpb.Admin)
			oidcCfg := new(oidcpb.OIDCClient)
			openFGA := new(openfgapb.OpenFGA)
			httpSession := new(sessionpb.Session)
			if err := bootstrap.Scan(&bootstrap.Runtime{Bootstrap: bc, Config: loaded}, adminCfg, oidcCfg, openFGA, httpSession); err != nil {
				t.Fatal(err)
			}

			store := memstore.NewWithCleanupInterval(0)
			manager, err := session.New(httpSession, store)
			if err != nil {
				t.Fatal(err)
			}
			wantSecure := environment == "docker"
			if manager.Lifetime != 24*time.Hour || manager.IdleTimeout != 8*time.Hour || manager.Cookie.Secure != wantSecure || !manager.Cookie.HttpOnly {
				t.Fatalf("unexpected session manager config for %s: lifetime=%v idle=%v secure=%v", environment, manager.Lifetime, manager.IdleTimeout, manager.Cookie.Secure)
			}

			if _, err := securityoidc.NewRelyingParty(oidcCfg); err != nil {
				t.Fatalf("NewRelyingParty(%s) error = %v", environment, err)
			}
			if _, cleanup, err := securityoidc.NewServiceTokenMiddleware(oidcCfg); err != nil {
				t.Fatalf("NewServiceTokenMiddleware(%s) error = %v", environment, err)
			} else {
				cleanup()
			}

			if adminCfg.GetBootstrapAdminEmail() != "seed@example.com" {
				t.Fatalf("bootstrap_admin_email = %q, want seed@example.com", adminCfg.GetBootstrapAdminEmail())
			}
			if openFGA.GetStoreId() != "01M2AY5F5K50N3M0Z9B911E84B" || openFGA.GetApiUrl() == "" {
				t.Fatal("OpenFGA config did not load")
			}
		})
	}
}
