package biz

import (
	"fmt"

	"github.com/google/uuid"
)

const (
	MinPasswordBytes = 8
	MaxPasswordBytes = 128
)

// NewUserID returns a time-ordered UUIDv7 identity that callers cannot supply or reuse.
func NewUserID() (string, error) {
	id, err := uuid.NewV7()
	if err != nil {
		return "", fmt.Errorf("generate User UUIDv7: %w", err)
	}
	return id.String(), nil
}

// ValidatePassword enforces the shared 8..128 byte password policy before hashing.
func ValidatePassword(plaintext string) error {
	if len(plaintext) < MinPasswordBytes || len(plaintext) > MaxPasswordBytes {
		return ErrInvalidPassword
	}
	return nil
}
