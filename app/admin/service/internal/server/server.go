// Package server wires Admin HTTP transport, session authentication, and OpenFGA authorization.
package server

import (
	adminauthn "github.com/Servora-Kit/plateau/app/admin/service/internal/authn"
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
	adminauthn.NewSessionAuthenticator,
	openfga.NewDefault,
	NewHTTPServer,
)
