package server

import (
	"log/slog"

	iamclientv1 "github.com/Servora-Kit/plateau/api/gen/go/plateau/iam/client/v1"
	userpb "github.com/Servora-Kit/plateau/api/gen/go/plateau/iam/user/v1"
	"github.com/Servora-Kit/plateau/app/iam/service/internal/service"
	"github.com/Servora-Kit/plateau/security"
	"github.com/Servora-Kit/plateau/security/audit"
	"github.com/Servora-Kit/plateau/security/authn/jwt"
	"github.com/Servora-Kit/plateau/security/authz/openfga"
	corepb "github.com/Servora-Kit/servora/api/gen/go/servora/core/v1"
	"github.com/Servora-Kit/servora/obs/metrics"
	svrgrpc "github.com/Servora-Kit/servora/transport/server/grpc"
	"github.com/Servora-Kit/servora/transport/server/middleware"
	kgrpc "github.com/go-kratos/kratos/v3/transport/grpc"
)

// NewGRPCServer creates the IAM gRPC server.
func NewGRPCServer(c *corepb.Server, obs *corepb.Observability, m *metrics.Metrics, jwtAuthn *jwt.Authenticator, auditor audit.Auditor, authorizer *openfga.Authorizer, users *service.UserService, oauthClients *service.OAuthClientService, l *slog.Logger) *kgrpc.Server {
	log := l.With("scope", "iam/server/grpc")

	ms := middleware.NewChainBuilder(log).
		WithTrace(obs.GetTrace()).
		WithMetrics(m).
		Build()
	ms = append(ms,
		jwt.ServerFor[jwt.ServiceClaims](jwtAuthn, security.WithRulesFuncs(userpb.AuthnRules, iamclientv1.AuthnRules)),
		audit.Server(auditor, security.WithRulesFuncs(userpb.AuditRules, iamclientv1.AuditRules)),
		openfga.Server(authorizer, security.WithRulesFuncs(userpb.AuthzRules, iamclientv1.AuthzRules)),
	)

	opts := []svrgrpc.ServerOption{
		svrgrpc.WithMiddleware(ms...),
		svrgrpc.WithServices(func(s *kgrpc.Server) {
			userpb.RegisterUserServiceServer(s, users)
			iamclientv1.RegisterOAuthClientServiceServer(s, oauthClients)
		}),
	}
	if c.GetGrpc() != nil {
		opts = append(opts, svrgrpc.WithConfig(c.GetGrpc()))
	}

	return svrgrpc.NewServer(opts...)
}
