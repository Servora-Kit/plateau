package biz

import (
	"testing"

	"github.com/google/uuid"
)

func TestNewUserIDReturnsDistinctUUIDv7(t *testing.T) {
	t.Parallel()

	first, err := NewUserID()
	if err != nil {
		t.Fatalf("first NewUserID() error = %v", err)
	}
	second, err := NewUserID()
	if err != nil {
		t.Fatalf("second NewUserID() error = %v", err)
	}
	if first == second {
		t.Fatal("NewUserID() reused an identifier")
	}
	parsed, err := uuid.Parse(first)
	if err != nil {
		t.Fatalf("parse User ID: %v", err)
	}
	if parsed.Version() != 7 {
		t.Fatalf("User ID version = %d, want 7", parsed.Version())
	}
}
