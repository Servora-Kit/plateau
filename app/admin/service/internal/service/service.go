// Package service adapts generated Protobuf HTTP interfaces to Admin use cases.
package service

import "github.com/google/wire"

// ProviderSet provides Admin transport-facing service adapters.
var ProviderSet = wire.NewSet(NewAuthService, NewUserService)
