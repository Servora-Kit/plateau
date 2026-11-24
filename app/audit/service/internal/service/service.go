// Package service implements gRPC and inbound transport adapters for audit.service.
package service

import "github.com/google/wire"

// ProviderSet provides audit service-layer implementations.
var ProviderSet = wire.NewSet(NewEventService)
