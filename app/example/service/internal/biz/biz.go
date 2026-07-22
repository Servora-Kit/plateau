package biz

import "github.com/google/wire"


// ProviderSet contains business-layer providers.
var ProviderSet = wire.NewSet(NewUserUsecase)
