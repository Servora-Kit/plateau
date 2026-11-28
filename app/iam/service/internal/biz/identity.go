package biz

import (
	"fmt"
	"time"

	"github.com/google/uuid"
)

const (
	MinPasswordBytes = 8
	MaxPasswordBytes = 128
)

// NewID returns a time-ordered UUIDv7 identifier.
func NewID() (string, error) {
	id, err := uuid.NewV7()
	if err != nil {
		return "", fmt.Errorf("generate UUIDv7: %w", err)
	}
	return id.String(), nil
}

// NewUserID returns a time-ordered UUIDv7 identity that callers cannot supply or reuse.
func NewUserID() (string, error) {
	return NewID()
}

// ValidatePassword enforces the shared 8..128 byte password policy before hashing.
func ValidatePassword(plaintext string) error {
	if len(plaintext) < MinPasswordBytes || len(plaintext) > MaxPasswordBytes {
		return ErrInvalidPassword
	}
	return nil
}

// NormalizeTimestamp truncates timestamps to UTC microsecond precision to match PostgreSQL timestamptz storage.
func NormalizeTimestamp(value time.Time) time.Time {
	return value.UTC().Truncate(time.Microsecond)
}
