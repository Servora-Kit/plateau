package server

import (
	"log/slog"

	kgrpc "github.com/go-kratos/kratos/v3/transport/grpc"

	testv1 "github.com/Servora-Kit/plateau/api/gen/go/test/service/v1"
	"github.com/Servora-Kit/plateau/app/test/service/internal/service"
	corev1 "github.com/Servora-Kit/servora/api/gen/go/servora/core/v1"
	"github.com/Servora-Kit/servora/obs/metrics"
	svrgrpc "github.com/Servora-Kit/servora/transport/server/grpc"
	"github.com/Servora-Kit/servora/transport/server/middleware"
)

// NewGRPCServer creates the gRPC server for the test service.
func NewGRPCServer(c *corev1.Server, obs *corev1.Observability, m *metrics.Metrics, l *slog.Logger, svc *service.TestService) *kgrpc.Server {
	glog := l.With("scope", "server/grpc")

	ms := middleware.NewChainBuilder(glog).
		WithTrace(obs.GetTrace()).
		WithMetrics(m).
		Build()

	opts := []svrgrpc.ServerOption{
		svrgrpc.WithMiddleware(ms...),
		svrgrpc.WithServices(func(s *kgrpc.Server) {
			testv1.RegisterTestServiceServer(s, svc)
		}),
	}
	if c != nil && c.Grpc != nil {
		opts = append(opts, svrgrpc.WithConfig(c.Grpc))
	}

	return svrgrpc.NewServer(opts...)
}
