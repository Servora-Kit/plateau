// Package server wires the gRPC, internal HTTP, and Kafka inbound transports for audit.service.
package server

import (
	"github.com/Servora-Kit/plateau/security/authz/openfga"
	"github.com/Servora-Kit/servora/core/registry"
	"github.com/Servora-Kit/servora/obs/metrics"
	"github.com/google/wire"
)

// ProviderSet provides audit server-layer dependencies.
var ProviderSet = wire.NewSet(
	registry.NewRegistrar,
	metrics.New,
	NewServiceAuthenticator,
	openfga.NewDefault,
	NewGRPCServer,
	NewHTTPServer,
	NewKafkaServer,
)
