package server

import (
	"log/slog"

	khttp "github.com/go-kratos/kratos/v3/transport/http"

	auditpb "github.com/Servora-Kit/servora-platform/api/gen/go/audit/service/v1"
	"github.com/Servora-Kit/servora-platform/app/audit/service/internal/service"
	corev1 "github.com/Servora-Kit/servora/api/gen/go/servora/core/v1"
	"github.com/Servora-Kit/servora/obs/metrics"
	svrhttp "github.com/Servora-Kit/servora/transport/server/http"
	"github.com/Servora-Kit/servora/transport/server/middleware"
)

// NewHTTPServer creates the HTTP server for the audit service.
func NewHTTPServer(c *corev1.Server, obs *corev1.Observability, m *metrics.Metrics, l *slog.Logger, svc *service.AuditService) *khttp.Server {
	hlog := l.With("scope", "audit/server/http")

	ms := middleware.NewChainBuilder(hlog).
		WithTrace(obs.GetTrace()).
		WithMetrics(m).
		Build()

	opts := []svrhttp.ServerOption{
		svrhttp.WithLogger(hlog),
		svrhttp.WithMiddleware(ms...),
		svrhttp.WithMetrics(m),
		svrhttp.WithServices(func(s *khttp.Server) {
			auditpb.RegisterAuditQueryServiceHTTPServer(s, svc)
		}),
	}
	if c != nil && c.Http != nil {
		opts = append(opts, svrhttp.WithConfig(c.Http))
	}

	return svrhttp.NewServer(opts...)
}
