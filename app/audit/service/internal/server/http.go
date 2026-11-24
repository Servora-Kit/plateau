package server

import (
	"log/slog"

	corepb "github.com/Servora-Kit/servora/api/gen/go/servora/core/v1"
	"github.com/Servora-Kit/servora/obs/metrics"
	svrhttp "github.com/Servora-Kit/servora/transport/server/http"
	"github.com/Servora-Kit/servora/transport/server/http/health"
	"github.com/Servora-Kit/servora/transport/server/middleware"
	khttp "github.com/go-kratos/kratos/v3/transport/http"
)

// NewHTTPServer constructs the internal operations HTTP server (/healthz, /readyz, /metrics) on port 10020.
func NewHTTPServer(c *corepb.Server, obs *corepb.Observability, m *metrics.Metrics, l *slog.Logger) *khttp.Server {
	log := l.With("scope", "audit/server/http")

	ms := middleware.NewChainBuilder(log).
		WithTrace(obs.GetTrace()).
		WithMetrics(m).
		Build()

	opts := []svrhttp.ServerOption{
		svrhttp.WithMiddleware(ms...),
		svrhttp.WithMetrics(m),
		svrhttp.WithHealthCheck(health.NewHandler()),
	}
	if c.GetHttp() != nil {
		opts = append(opts, svrhttp.WithConfig(c.GetHttp()))
	}
	return svrhttp.NewServer(opts...)
}
