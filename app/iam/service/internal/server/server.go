package server

import (
	iamauthn "github.com/Servora-Kit/plateau/app/iam/service/internal/authn"
	iamauthz "github.com/Servora-Kit/plateau/app/iam/service/internal/authz"
	"github.com/Servora-Kit/servora/core/registry"
	"github.com/Servora-Kit/servora/obs/metrics"
	"github.com/google/wire"
)

var ProviderSet = wire.NewSet(registry.NewRegistrar, metrics.New, iamauthn.NewSessionAuthenticator, iamauthz.NewOpenFGAAuthorizer, NewGRPCServer, NewHTTPServer)
