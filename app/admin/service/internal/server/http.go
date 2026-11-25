package server

import (
	"fmt"
	"log/slog"

	adminauditpb "github.com/Servora-Kit/plateau/api/gen/go/admin/audit/v1"
	adminauthpb "github.com/Servora-Kit/plateau/api/gen/go/admin/auth/v1"
	adminuserpb "github.com/Servora-Kit/plateau/api/gen/go/admin/user/v1"
	"github.com/Servora-Kit/plateau/app/admin/service/internal/service"
	"github.com/Servora-Kit/plateau/security"
	"github.com/Servora-Kit/plateau/security/audit"
	"github.com/Servora-Kit/plateau/security/authn/session"
	"github.com/Servora-Kit/plateau/security/authz/openfga"
	httpsession "github.com/Servora-Kit/plateau/security/session"
	corepb "github.com/Servora-Kit/servora/api/gen/go/servora/core/v1"
	"github.com/Servora-Kit/servora/obs/metrics"
	svrhttp "github.com/Servora-Kit/servora/transport/server/http"
	"github.com/Servora-Kit/servora/transport/server/http/health"
	"github.com/Servora-Kit/servora/transport/server/middleware"
	"github.com/alexedwards/scs/v2"
	khttp "github.com/go-kratos/kratos/v3/transport/http"
)

// NewHTTPServer creates the Admin HTTP server.
func NewHTTPServer(
	c *corepb.Server,
	obs *corepb.Observability,
	app *corepb.App,
	m *metrics.Metrics,
	manager *scs.SessionManager,
	sessionAuthn *session.UserAuthenticator,
	auditor audit.Auditor,
	authorizer *openfga.Authorizer,
	authSvc *service.AuthService,
	userSvc *service.UserService,
	auditSvc *service.AuditService,
	l *slog.Logger,
) (*khttp.Server, error) {
	browserFilter, err := httpsession.NewFilter(manager, httpsession.FilterConfig{
		PublicOrigin: app.GetExternalUrl(),
		JSONPrefixes: []string{"/v1/admin/"},
	})
	if err != nil {
		return nil, fmt.Errorf("admin http server: %w", err)
	}
	log := l.With("scope", "admin/server/http")

	ms := middleware.NewChainBuilder(log).
		WithTrace(obs.GetTrace()).
		WithMetrics(m).
		Build()
	ms = append(
		ms,
		session.Server(sessionAuthn, security.WithRulesFuncs(adminauthpb.AuthnRules, adminuserpb.AuthnRules, adminauditpb.AuthnRules)),
		audit.Server(auditor, security.WithRulesFuncs(adminauthpb.AuditRules, adminuserpb.AuditRules, adminauditpb.AuditRules)),
		openfga.Server(authorizer, security.WithRulesFuncs(adminauthpb.AuthzRules, adminuserpb.AuthzRules, adminauditpb.AuthzRules)),
	)

	opts := []svrhttp.ServerOption{
		svrhttp.WithFilter(browserFilter),
		svrhttp.WithMiddleware(ms...),
		svrhttp.WithMetrics(m),
		svrhttp.WithHealthCheck(health.NewHandler()),
		svrhttp.WithServices(func(s *khttp.Server) {
			authSvc.RegisterOIDCHTTP(s, auditor)
			adminauthpb.RegisterAuthServiceHTTPServer(s, authSvc)
			adminuserpb.RegisterUserServiceHTTPServer(s, userSvc)
			adminauditpb.RegisterAuditServiceHTTPServer(s, auditSvc)
		}),
	}
	if c.GetHttp() != nil {
		opts = append(opts, svrhttp.WithConfig(c.GetHttp()))
	}

	return svrhttp.NewServer(opts...), nil
}
