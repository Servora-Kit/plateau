package server

import (
	"fmt"
	"log/slog"

	accountpb "github.com/Servora-Kit/plateau/api/gen/go/iam/account/v1"
	authnpb "github.com/Servora-Kit/plateau/api/gen/go/iam/authn/v1"
	sessionpb "github.com/Servora-Kit/plateau/api/gen/go/iam/session/v1"
	iamauthn "github.com/Servora-Kit/plateau/app/iam/service/internal/authn"
	"github.com/Servora-Kit/plateau/app/iam/service/internal/oidc"
	"github.com/Servora-Kit/plateau/app/iam/service/internal/service"
	httpsession "github.com/Servora-Kit/plateau/security/session"
	"github.com/alexedwards/scs/v2"
	khttp "github.com/go-kratos/kratos/v3/transport/http"

	"github.com/Servora-Kit/plateau/security"
	"github.com/Servora-Kit/plateau/security/audit"
	"github.com/Servora-Kit/plateau/security/authn/session"
	"github.com/Servora-Kit/plateau/security/authz/openfga"
	"github.com/Servora-Kit/plateau/security/cap"
	corepb "github.com/Servora-Kit/servora/api/gen/go/servora/core/v1"
	"github.com/Servora-Kit/servora/obs/metrics"
	svrhttp "github.com/Servora-Kit/servora/transport/server/http"
	"github.com/Servora-Kit/servora/transport/server/http/health"
	"github.com/Servora-Kit/servora/transport/server/middleware"
)

// NewHTTPServer creates the IAM HTTP server.
func NewHTTPServer(c *corepb.Server, obs *corepb.Observability, app *corepb.App, m *metrics.Metrics, captcha *cap.Cap, oidcProvider *oidc.IAMProvider, manager *scs.SessionManager, sessionAuthn *iamauthn.SessionAuthenticator, auditor audit.Auditor, authorizer *openfga.Authorizer, authn *service.AuthnService, sessions *service.SessionService, account *service.AccountService, l *slog.Logger) (*khttp.Server, error) {
	browserFilter, err := httpsession.NewFilter(manager, httpsession.FilterConfig{
		PublicOrigin:     app.GetExternalUrl(),
		JSONPrefixes:     []string{"/v1/iam/"},
		SameOriginPaths:  []string{"/authorize/callback"},
		CrossOriginPaths: []string{"/authorize", "/end_session"},
	})
	if err != nil {
		return nil, fmt.Errorf("http server: %w", err)
	}
	log := l.With("scope", "iam/server/http")

	ms := middleware.NewChainBuilder(log).
		WithTrace(obs.GetTrace()).
		WithMetrics(m).
		Build()
	ms = append(ms,
		session.Server(sessionAuthn, security.WithRulesFuncs(authnpb.AuthnRules, sessionpb.AuthnRules, accountpb.AuthnRules)),
		audit.Server(auditor, security.WithRulesFuncs(authnpb.AuditRules, sessionpb.AuditRules, accountpb.AuditRules)),
		openfga.Server(authorizer, security.WithRulesFuncs(authnpb.AuthzRules, sessionpb.AuthzRules, accountpb.AuthzRules)),
	)

	opts := []svrhttp.ServerOption{
		svrhttp.WithFilter(browserFilter),
		svrhttp.WithMiddleware(ms...),
		svrhttp.WithMetrics(m),
		svrhttp.WithHealthCheck(health.NewHandler()),
		svrhttp.WithServices(func(s *khttp.Server) {
			cap.Register(s, captcha)
			oidc.RegisterHTTPServer(s, oidcProvider, auditor)
			authnpb.RegisterAuthnServiceHTTPServer(s, authn)
			sessionpb.RegisterSessionServiceHTTPServer(s, sessions)
			accountpb.RegisterAccountServiceHTTPServer(s, account)
		}),
	}
	if c.GetHttp() != nil {
		opts = append(opts, svrhttp.WithConfig(c.GetHttp()))
	}

	return svrhttp.NewServer(opts...), nil
}
