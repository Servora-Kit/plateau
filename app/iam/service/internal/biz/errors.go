package biz

import "errors"

var (
	// Persistence facts are owned by repository ports; data adapters may return them without creating public API errors.
	ErrNotFound      = errors.New("record not found")
	ErrAlreadyExists = errors.New("record already exists")
	ErrMutationMiss  = errors.New("conditional mutation matched no rows")
	ErrExpired       = errors.New("stored value expired")

	ErrUnauthenticated         = errors.New("authentication required")
	ErrInvalidPassword         = errors.New("invalid password")
	ErrAccountInvalidState     = errors.New("account state is invalid")
	ErrVerificationEmailFailed = errors.New("verification email delivery failed")
	ErrAuthFlowNotFound        = errors.New("authentication flow not found")
	ErrAuthFlowExpired         = errors.New("authentication flow expired")
	ErrAuthFlowInvalidated     = errors.New("authentication flow invalidated")
	ErrOIDCRequestInvalid      = errors.New("OIDC authorization request is invalid")

	ErrStaticClientImmutable = errors.New("static OAuth client is immutable")
	ErrInvalidClientConfig   = errors.New("invalid OAuth client configuration")
	ErrClientInvalidState    = errors.New("OAuth client lifecycle state is invalid")
	ErrClientEtagMismatch    = errors.New("OAuth client etag does not match")
	ErrSecretLimitExceeded   = errors.New("OAuth client secret limit exceeded")
	ErrLastActiveSecret      = errors.New("cannot delete the last active OAuth client secret")
)
