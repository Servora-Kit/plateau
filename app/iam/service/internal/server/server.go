package server

import (
	"github.com/Servora-Kit/servora/core/registry"
	"github.com/Servora-Kit/servora/obs/metrics"
	"github.com/google/wire"
)

var ProviderSet = wire.NewSet(registry.NewRegistrar, metrics.New, NewGRPCServer, NewHTTPServer)
