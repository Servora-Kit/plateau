package server

import (
	iamauthn "github.com/Servora-Kit/plateau/app/iam/service/internal/authn"
	auditkafka "github.com/Servora-Kit/plateau/security/audit/kafka"
	"github.com/Servora-Kit/plateau/security/authz/openfga"
	"github.com/Servora-Kit/servora/core/registry"
	"github.com/Servora-Kit/servora/obs/metrics"
	"github.com/google/wire"
)

var ProviderSet = wire.NewSet(registry.NewRegistrar, metrics.New, iamauthn.NewSessionAuthenticator, iamauthn.NewServiceAuthenticator, auditkafka.NewDefault, openfga.NewDefault, NewGRPCServer, NewHTTPServer)
