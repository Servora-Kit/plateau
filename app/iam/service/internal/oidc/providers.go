package oidc

import "github.com/google/wire"

var ProviderSet = wire.NewSet(NewOIDCStorage, NewIAMProvider, NewOIDCInitializer)
