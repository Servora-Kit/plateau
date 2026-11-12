// Package biz implements Admin orchestration use cases and domain protection rules.
package biz

import "github.com/google/wire"

// ProviderSet provides Admin business-layer use cases and seed initializer.
var ProviderSet = wire.NewSet(NewAuthUsecase, NewUserUsecase, NewSeedInitializer)
