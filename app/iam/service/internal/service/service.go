package service

import "github.com/google/wire"

// ProviderSet provides all generated IAM service implementations.
var ProviderSet = wire.NewSet(NewAuthnService, NewSessionService, NewAccountService, NewUserService)
