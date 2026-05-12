package server

import (
	"github.com/Servora-Kit/servora/core/registry"
	"github.com/Servora-Kit/servora/obs/telemetry"
	"github.com/google/wire"
)

var ProviderSet = wire.NewSet(registry.NewRegistrar, telemetry.NewMetrics, NewGRPCServer, NewHTTPServer)
