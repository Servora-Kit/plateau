package server

import (
	"fmt"
	"log/slog"

	adminauthpb "github.com/Servora-Kit/plateau/api/gen/go/admin/auth/v1"
	adminuserpb "github.com/Servora-Kit/plateau/api/gen/go/admin/user/v1"
	adminauthn "github.com/Servora-Kit/plateau/app/admin/service/internal/authn"
	"github.com/Servora-Kit/plateau/app/admin/service/internal/service"
	authnsecurity "github.com/Servora-Kit/plateau/security/authn"
	"github.com/Servora-Kit/plateau/security/authn/session"
	authzsecurity "github.com/Servora-Kit/plateau/security/authz"
	"github.com/Servora-Kit/plateau/security/authz/openfga"
	httpsession "github.com/Servora-Kit/plateau/security/session"
	corepb "github.com/Servora-Kit/servora/api/gen/go/servora/core/v1"
	"github.com/Servora-Kit/servora/obs/metrics"
	svrhttp "github.com/Servora-Kit/servora/transport/server/http"
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
	sessionAuthn *adminauthn.SessionAuthenticator,
	authorizer *openfga.Authorizer,
	authSvc *service.AuthService,
	userSvc *service.UserService,
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
		session.Server(sessionAuthn, authnsecurity.WithRulesFuncs(adminauthpb.AuthnRules, adminuserpb.AuthnRules)),
		openfga.Server(authorizer, authzsecurity.WithRulesFuncs(adminauthpb.AuthzRules, adminuserpb.AuthzRules)),
	)

	opts := []svrhttp.ServerOption{
		svrhttp.WithFilter(browserFilter),
		svrhttp.WithMiddleware(ms...),
		svrhttp.WithMetrics(m),
		svrhttp.WithServices(func(s *khttp.Server) {
			authSvc.RegisterOIDCHTTP(s)
			adminauthpb.RegisterAuthServiceHTTPServer(s, authSvc)
			adminuserpb.RegisterUserServiceHTTPServer(s, userSvc)
		}),
	}
	if c.GetHttp() != nil {
		opts = append(opts, svrhttp.WithConfig(c.GetHttp()))
	}

	return svrhttp.NewServer(opts...), nil
}
