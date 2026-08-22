package biz

import (
	"fmt"
	"strings"

	"github.com/google/uuid"
	"golang.org/x/text/cases"
	"golang.org/x/text/unicode/norm"
)

var emailCaseFolder = cases.Fold()

// NormalizeEmail returns the globally comparable value and the trimmed display value.
func NormalizeEmail(raw string) (canonical, display string, err error) {
	display = strings.TrimSpace(raw)
	if display == "" {
		return "", "", fmt.Errorf("email is empty")
	}
	canonical = emailCaseFolder.String(norm.NFC.String(display))
	return canonical, display, nil
}

// NewUserID returns a time-ordered UUIDv7 identity that callers cannot supply or reuse.
func NewUserID() (string, error) {
	id, err := uuid.NewV7()
	if err != nil {
		return "", fmt.Errorf("generate User UUIDv7: %w", err)
	}
	return id.String(), nil
}
