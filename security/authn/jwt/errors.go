package jwt

import "errors"

var (
	ErrMissingCredentials   = errors.New("jwt authn: credentials missing")
	ErrMalformedCredentials = errors.New("jwt authn: malformed credentials")
	ErrInvalidToken         = errors.New("jwt authn: invalid token")
	ErrActorMapping         = errors.New("jwt authn: actor mapping failed")
)
