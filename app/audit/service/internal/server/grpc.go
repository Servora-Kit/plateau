package server

import (
	kgrpc "github.com/go-kratos/kratos/v2/transport/grpc"

	auditsvcpb "github.com/Servora-Kit/servora-platform/api/gen/go/audit/service/v1"
	"github.com/Servora-Kit/servora-platform/app/audit/service/internal/service"
	corev1 "github.com/Servora-Kit/servora/api/gen/go/servora/core/v1"
	"github.com/Servora-Kit/servora/obs/logging"
	"github.com/Servora-Kit/servora/obs/telemetry"
	svrgrpc "github.com/Servora-Kit/servora/transport/server/grpc"
	"github.com/Servora-Kit/servora/transport/server/middleware"
)

// NewGRPCServer creates the gRPC server for the audit service.
func NewGRPCServer(c *corev1.Server, trace *corev1.Trace, m *telemetry.Metrics, l logger.Logger, svc *service.AuditService) *kgrpc.Server {
	glog := logger.With(l, "audit/server/grpc")

	ms := middleware.NewChainBuilder(glog).
		WithTrace(trace).
		WithMetrics(m).
		Build()

	opts := []svrgrpc.ServerOption{
		svrgrpc.WithLogger(glog),
		svrgrpc.WithMiddleware(ms...),
		svrgrpc.WithServices(func(s *kgrpc.Server) {
			auditsvcpb.RegisterAuditQueryServiceServer(s, svc)
		}),
	}
	if c != nil && c.Grpc != nil {
		opts = append(opts, svrgrpc.WithConfig(c.Grpc))
	}

	return svrgrpc.NewServer(opts...)
}
