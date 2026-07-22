package service

import "github.com/google/wire"

// ProviderSet contains application-layer providers.
var ProviderSet = wire.NewSet(NewUserService)
