package session

import "errors"

var (
	ErrMissingCredentials    = errors.New("session authn: credentials missing")
	ErrInvalidCredentials    = errors.New("session authn: credentials invalid")
	ErrDependencyUnavailable = errors.New("session authn: dependency unavailable")
	ErrActorMapping          = errors.New("session authn: actor mapping failed")
	ErrContextExtension      = errors.New("session authn: context extension failed")
)
