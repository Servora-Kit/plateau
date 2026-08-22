package biz

import "github.com/google/wire"

var ProviderSet = wire.NewSet(NewSessionUsecase, NewAuthenticationUsecase, NewAccountUsecase, NewUserUsecase, NewAdminInitializer)
