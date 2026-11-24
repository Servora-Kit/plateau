// Package biz contains the domain models, CloudEvents projection rules,
// opaque pagination cursor validation, and repository ports for audit.service.
package biz

import "github.com/google/wire"

// ProviderSet provides audit domain usecases.
var ProviderSet = wire.NewSet(NewEventUsecase)
