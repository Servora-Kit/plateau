package oidc

import (
	"time"

	"github.com/zitadel/oidc/v3/pkg/oidc"
	"github.com/zitadel/oidc/v3/pkg/op"
)

const (
	authorizationRequestTTL = 10 * time.Minute
	authorizationCodeTTL    = 5 * time.Minute
	accessTokenTTL          = 15 * time.Minute
	refreshTokenTTL         = 7 * 24 * time.Hour
	idTokenTTL              = 15 * time.Minute
	passwordAMR             = "pwd"
)

var supportedScopes = []string{
	oidc.ScopeOpenID,
	oidc.ScopeProfile,
	oidc.ScopeEmail,
	oidc.ScopeOfflineAccess,
}

// Compile-time checks for every ZITADEL protocol contract implemented by IAM.
var (
	_ op.Storage                   = (*OIDCStorage)(nil)
	_ op.AuthStorage               = (*OIDCStorage)(nil)
	_ op.OPStorage                 = (*OIDCStorage)(nil)
	_ op.CanSetUserinfoFromRequest = (*OIDCStorage)(nil)
	_ op.StorageNotFoundError      = storageNotFoundError{}
	_ op.Client                    = (*client)(nil)
	_ op.AuthRequest               = (*authorizationRequest)(nil)
	_ op.RefreshTokenRequest       = (*refreshTokenRequest)(nil)
	_ op.SigningKey                = (*signingKey)(nil)
	_ op.Key                       = (*publicKey)(nil)
	_ op.OpenIDProvider            = (*IAMProvider)(nil)
)

type storageNotFoundError struct{ cause error }

func (err storageNotFoundError) Error() string { return err.cause.Error() }

func (err storageNotFoundError) Unwrap() error { return err.cause }

func (storageNotFoundError) IsNotFound() {}
