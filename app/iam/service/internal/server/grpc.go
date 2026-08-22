package server

import (
	"log/slog"

	accountpb "github.com/Servora-Kit/plateau/api/gen/go/iam/account/v1"
	authnpb "github.com/Servora-Kit/plateau/api/gen/go/iam/authn/v1"
	sessionpb "github.com/Servora-Kit/plateau/api/gen/go/iam/session/v1"
	userpb "github.com/Servora-Kit/plateau/api/gen/go/iam/user/v1"
	"github.com/Servora-Kit/plateau/app/iam/service/internal/authn"
	"github.com/Servora-Kit/plateau/app/iam/service/internal/service"
	authnsecurity "github.com/Servora-Kit/plateau/security/authn"
	"github.com/Servora-Kit/plateau/security/authn/session"
	authzsecurity "github.com/Servora-Kit/plateau/security/authz"
	"github.com/Servora-Kit/plateau/security/authz/openfga"
	kgrpc "github.com/go-kratos/kratos/v3/transport/grpc"

	corepb "github.com/Servora-Kit/servora/api/gen/go/servora/core/v1"
	"github.com/Servora-Kit/servora/obs/metrics"
	svrgrpc "github.com/Servora-Kit/servora/transport/server/grpc"
	"github.com/Servora-Kit/servora/transport/server/middleware"
)

// NewGRPCServer creates the IAM gRPC server.
func NewGRPCServer(c *corepb.Server, obs *corepb.Observability, m *metrics.Metrics, sessionAuthn *authn.SessionAuthenticator, authorizer *openfga.Authorizer, authn *service.AuthnService, sessions *service.SessionService, account *service.AccountService, users *service.UserService, l *slog.Logger) *kgrpc.Server {
	log := l.With("scope", "iam/server/grpc")

	ms := middleware.NewChainBuilder(log).
		WithTrace(obs.GetTrace()).
		WithMetrics(m).
		Build()
	ms = append(ms,
		session.Server(sessionAuthn, authnsecurity.WithRulesFuncs(authnpb.AuthnRules, sessionpb.AuthnRules, accountpb.AuthnRules, userpb.AuthnRules)),
		openfga.Server(authorizer, authzsecurity.WithRulesFuncs(authnpb.AuthzRules, sessionpb.AuthzRules, accountpb.AuthzRules, userpb.AuthzRules)),
	)

	opts := []svrgrpc.ServerOption{
		svrgrpc.WithMiddleware(ms...),
		svrgrpc.WithServices(func(s *kgrpc.Server) {
			authnpb.RegisterAuthnServiceServer(s, authn)
			sessionpb.RegisterSessionServiceServer(s, sessions)
			userpb.RegisterUserServiceServer(s, users)
			accountpb.RegisterAccountServiceServer(s, account)
		}),
	}
	if c != nil && c.Grpc != nil {
		opts = append(opts, svrgrpc.WithConfig(c.Grpc))
	}

	return svrgrpc.NewServer(opts...)
}
