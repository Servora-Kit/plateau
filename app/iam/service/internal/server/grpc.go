package server

import (
	"log/slog"

	kgrpc "github.com/go-kratos/kratos/v3/transport/grpc"

	corev1 "github.com/Servora-Kit/servora/api/gen/go/servora/core/v1"
	"github.com/Servora-Kit/servora/obs/metrics"
	svrgrpc "github.com/Servora-Kit/servora/transport/server/grpc"
	"github.com/Servora-Kit/servora/transport/server/middleware"
)

// NewGRPCServer creates the gRPC server for the audit service.
func NewGRPCServer(c *corev1.Server, obs *corev1.Observability, m *metrics.Metrics, l *slog.Logger) *kgrpc.Server {
	glog := l.With("scope", "audit/server/grpc")

	ms := middleware.NewChainBuilder(glog).
		WithTrace(obs.GetTrace()).
		WithMetrics(m).
		Build()

	opts := []svrgrpc.ServerOption{
		svrgrpc.WithMiddleware(ms...),
		svrgrpc.WithServices(func(s *kgrpc.Server) {
		}),
	}
	if c != nil && c.Grpc != nil {
		opts = append(opts, svrgrpc.WithConfig(c.Grpc))
	}

	return svrgrpc.NewServer(opts...)
}
