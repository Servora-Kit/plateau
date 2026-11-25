// Package server wires Admin HTTP transport, session authentication, and OpenFGA authorization.
package server

import (
	auditkafka "github.com/Servora-Kit/plateau/security/audit/kafka"
	"github.com/Servora-Kit/plateau/security/authn/session"
	"github.com/Servora-Kit/plateau/security/authz/openfga"
	"github.com/Servora-Kit/servora/core/registry"
	"github.com/Servora-Kit/servora/obs/metrics"
	"github.com/google/wire"
)

// ProviderSet provides Admin transport servers and security middleware providers.
var ProviderSet = wire.NewSet(
	registry.NewRegistrar,
	registry.NewDiscovery,
	metrics.New,
	session.NewUserAuthenticator,
	auditkafka.NewDefault,
	openfga.NewDefault,
	NewHTTPServer,
)
