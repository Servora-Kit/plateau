package biz

import (
	"fmt"
	"strings"

	"github.com/Servora-Kit/plateau/security"
	"github.com/google/uuid"
)

const (
	MinPasswordBytes = 8
	MaxPasswordBytes = 128
)

// NormalizeEmail returns the globally comparable value and the trimmed display value.
func NormalizeEmail(raw string) (canonical, display string, err error) {
	display = strings.TrimSpace(raw)
	if display == "" {
		return "", "", fmt.Errorf("email is empty")
	}
	return security.NormalizeEmail(display), display, nil
}

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
