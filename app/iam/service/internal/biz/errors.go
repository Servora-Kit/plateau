package biz

import "errors"

var (
	// Persistence facts are owned by repository ports; data adapters may return them without creating public API errors.
	ErrNotFound      = errors.New("record not found")
	ErrAlreadyExists = errors.New("record already exists")
	ErrMutationMiss  = errors.New("conditional mutation matched no rows")
	ErrExpired       = errors.New("stored value expired")

	ErrUnauthenticated = errors.New("authentication required")
	ErrInvalidPassword = errors.New("invalid password")
)
