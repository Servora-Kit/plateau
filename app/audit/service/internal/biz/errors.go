package biz

import "errors"

var (
	// ErrEventNotFound indicates the requested audit event does not exist.
	ErrEventNotFound = errors.New("audit event not found")
	// ErrInvalidArgument indicates an invalid query parameter or resource identifier.
	ErrInvalidArgument = errors.New("invalid audit argument")
	// ErrInvalidPageToken indicates a malformed, version-mismatched, or filter-mismatched page_token.
	ErrInvalidPageToken = errors.New("invalid audit page token")
	// ErrStorageUnavailable indicates the underlying ClickHouse store failed or timed out.
	ErrStorageUnavailable = errors.New("audit storage unavailable")
)
