package server

import (
	"context"
	"log/slog"

	auditeventpb "github.com/Servora-Kit/plateau/api/gen/go/plateau/audit/event/v1"
	jwtconfpb "github.com/Servora-Kit/plateau/api/gen/go/plateau/security/authn/jwt/v1"
	"github.com/Servora-Kit/plateau/app/audit/service/internal/service"
	"github.com/Servora-Kit/plateau/security"
	"github.com/Servora-Kit/plateau/security/authn/jwt"
	"github.com/Servora-Kit/plateau/security/authz/openfga"
	corepb "github.com/Servora-Kit/servora/api/gen/go/servora/core/v1"
	"github.com/Servora-Kit/servora/obs/metrics"
	svrgrpc "github.com/Servora-Kit/servora/transport/server/grpc"
	"github.com/Servora-Kit/servora/transport/server/middleware"
	kgrpc "github.com/go-kratos/kratos/v3/transport/grpc"
)

// NewServiceAuthenticator constructs the Resource Server JWT authenticator for audit.service.
func NewServiceAuthenticator(cfg *jwtconfpb.JwtAuthnConfig) (*jwt.Authenticator, func(), error) {
	return jwt.New(context.Background(), cfg)
}

// NewGRPCServer constructs the internal gRPC server protected by Service JWT AuthN and OpenFGA AuthZ.
func NewGRPCServer(
	c *corepb.Server,
	obs *corepb.Observability,
	m *metrics.Metrics,
	jwtAuthn *jwt.Authenticator,
	authorizer *openfga.Authorizer,
	events *service.EventService,
	l *slog.Logger,
) *kgrpc.Server {
	log := l.With("scope", "audit/server/grpc")

	ms := middleware.NewChainBuilder(log).
		WithTrace(obs.GetTrace()).
		WithMetrics(m).
		Build()
	ms = append(ms,
		jwt.ServerFor[jwt.ServiceClaims](jwtAuthn, security.WithRulesFuncs(auditeventpb.AuthnRules)),
		openfga.Server(authorizer, security.WithRulesFuncs(auditeventpb.AuthzRules)),
	)

	opts := []svrgrpc.ServerOption{
		svrgrpc.WithMiddleware(ms...),
		svrgrpc.WithServices(func(s *kgrpc.Server) {
			auditeventpb.RegisterEventServiceServer(s, events)
		}),
	}
	if c.GetGrpc() != nil {
		opts = append(opts, svrgrpc.WithConfig(c.GetGrpc()))
	}
	return svrgrpc.NewServer(opts...)
}
