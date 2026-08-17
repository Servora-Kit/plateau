package openfga

import "errors"

var (
	ErrInvalidInput    = errors.New("openfga authz: invalid input")
	ErrUnauthenticated = errors.New("openfga authz: unauthenticated")
	ErrUnavailable     = errors.New("openfga authz: unavailable")
)
