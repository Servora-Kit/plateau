package server

import (
	"log/slog"

	accountpb "github.com/Servora-Kit/plateau/api/gen/go/iam/account/v1"
	authnpb "github.com/Servora-Kit/plateau/api/gen/go/iam/authn/v1"
	sessionpb "github.com/Servora-Kit/plateau/api/gen/go/iam/session/v1"
	userpb "github.com/Servora-Kit/plateau/api/gen/go/iam/user/v1"
	iamauthn "github.com/Servora-Kit/plateau/app/iam/service/internal/authn"
	"github.com/Servora-Kit/plateau/app/iam/service/internal/oidc"
	"github.com/Servora-Kit/plateau/app/iam/service/internal/service"
	khttp "github.com/go-kratos/kratos/v3/transport/http"

	authnsecurity "github.com/Servora-Kit/plateau/security/authn"
	"github.com/Servora-Kit/plateau/security/authn/session"
	authzsecurity "github.com/Servora-Kit/plateau/security/authz"
	"github.com/Servora-Kit/plateau/security/authz/openfga"
	"github.com/Servora-Kit/plateau/security/cap"
	corepb "github.com/Servora-Kit/servora/api/gen/go/servora/core/v1"
	"github.com/Servora-Kit/servora/obs/metrics"
	svrhttp "github.com/Servora-Kit/servora/transport/server/http"
	"github.com/Servora-Kit/servora/transport/server/middleware"
)

// NewHTTPServer creates the IAM HTTP server.
func NewHTTPServer(c *corepb.Server, obs *corepb.Observability, m *metrics.Metrics, captcha *cap.Cap, oidcProvider *oidc.IAMProvider, sessionAuthn *iamauthn.SessionAuthenticator, authorizer *openfga.Authorizer, authn *service.AuthnService, sessions *service.SessionService, account *service.AccountService, users *service.UserService, l *slog.Logger) *khttp.Server {
	log := l.With("scope", "iam/server/http")

	ms := middleware.NewChainBuilder(log).
		WithTrace(obs.GetTrace()).
		WithMetrics(m).
		Build()
	ms = append(ms,
		session.Server(sessionAuthn, authnsecurity.WithRulesFuncs(authnpb.AuthnRules, sessionpb.AuthnRules, accountpb.AuthnRules, userpb.AuthnRules)),
		openfga.Server(authorizer, authzsecurity.WithRulesFuncs(authnpb.AuthzRules, sessionpb.AuthzRules, accountpb.AuthzRules, userpb.AuthzRules)),
	)

	opts := []svrhttp.ServerOption{
		svrhttp.WithMiddleware(ms...),
		svrhttp.WithMetrics(m),
		svrhttp.WithServices(func(s *khttp.Server) {
			cap.Register(s, captcha)
			oidc.RegisterHTTPServer(s, oidcProvider)
			authnpb.RegisterAuthnServiceHTTPServer(s, authn)
			sessionpb.RegisterSessionServiceHTTPServer(s, sessions)
			accountpb.RegisterAccountServiceHTTPServer(s, account)
			userpb.RegisterUserServiceHTTPServer(s, users)
		}),
	}
	if c != nil && c.Http != nil {
		opts = append(opts, svrhttp.WithConfig(c.Http))
	}

	return svrhttp.NewServer(opts...)
}
