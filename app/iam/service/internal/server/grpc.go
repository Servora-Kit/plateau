package server

import (
	"log/slog"

	userpb "github.com/Servora-Kit/plateau/api/gen/go/iam/user/v1"
	"github.com/Servora-Kit/plateau/app/iam/service/internal/authn"
	"github.com/Servora-Kit/plateau/app/iam/service/internal/service"
	authnsecurity "github.com/Servora-Kit/plateau/security/authn"
	"github.com/Servora-Kit/plateau/security/authn/jwt"
	authzsecurity "github.com/Servora-Kit/plateau/security/authz"
	"github.com/Servora-Kit/plateau/security/authz/openfga"
	kgrpc "github.com/go-kratos/kratos/v3/transport/grpc"

	corepb "github.com/Servora-Kit/servora/api/gen/go/servora/core/v1"
	"github.com/Servora-Kit/servora/obs/metrics"
	svrgrpc "github.com/Servora-Kit/servora/transport/server/grpc"
	"github.com/Servora-Kit/servora/transport/server/middleware"
)

// NewGRPCServer creates the IAM gRPC server.
func NewGRPCServer(c *corepb.Server, obs *corepb.Observability, m *metrics.Metrics, serviceAuthn *jwt.Authenticator, authorizer *openfga.Authorizer, users *service.UserService, l *slog.Logger) *kgrpc.Server {
	log := l.With("scope", "iam/server/grpc")

	ms := middleware.NewChainBuilder(log).
		WithTrace(obs.GetTrace()).
		WithMetrics(m).
		Build()
	ms = append(ms,
		jwt.Server(serviceAuthn, authn.NewServiceClaims, authn.ServiceActor, authnsecurity.WithRulesFuncs(userpb.AuthnRules)),
		openfga.Server(authorizer, authzsecurity.WithRulesFuncs(userpb.AuthzRules)),
	)

	opts := []svrgrpc.ServerOption{
		svrgrpc.WithMiddleware(ms...),
		svrgrpc.WithServices(func(s *kgrpc.Server) {
			userpb.RegisterUserServiceServer(s, users)
		}),
	}
	if c != nil && c.Grpc != nil {
		opts = append(opts, svrgrpc.WithConfig(c.Grpc))
	}

	return svrgrpc.NewServer(opts...)
}
