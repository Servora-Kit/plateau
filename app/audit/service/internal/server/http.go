package server

import (
	khttp "github.com/go-kratos/kratos/v2/transport/http"

	auditsvcpb "github.com/Servora-Kit/servora-platform/api/gen/go/audit/service/v1"
	"github.com/Servora-Kit/servora-platform/app/audit/service/internal/service"
	corev1 "github.com/Servora-Kit/servora/api/gen/go/servora/core/v1"
	"github.com/Servora-Kit/servora/obs/logging"
	"github.com/Servora-Kit/servora/obs/telemetry"
	svrhttp "github.com/Servora-Kit/servora/transport/server/http"
	"github.com/Servora-Kit/servora/transport/server/middleware"
)

// NewHTTPServer creates the HTTP server for the audit service.
func NewHTTPServer(c *corev1.Server, trace *corev1.Trace, m *telemetry.Metrics, l logger.Logger, svc *service.AuditService) *khttp.Server {
	hlog := logger.With(l, "audit/server/http")

	ms := middleware.NewChainBuilder(hlog).
		WithTrace(trace).
		WithMetrics(m).
		Build()

	opts := []svrhttp.ServerOption{
		svrhttp.WithLogger(hlog),
		svrhttp.WithMiddleware(ms...),
		svrhttp.WithMetrics(m),
		svrhttp.WithServices(func(s *khttp.Server) {
			auditsvcpb.RegisterAuditHTTPServiceHTTPServer(s, svc)
		}),
	}
	if c != nil && c.Http != nil {
		opts = append(opts, svrhttp.WithConfig(c.Http))
	}

	return svrhttp.NewServer(opts...)
}
