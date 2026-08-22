package biz

import (
	"testing"

	"github.com/google/uuid"
)

func TestNormalizeEmail(t *testing.T) {
	t.Parallel()

	canonical, display, err := NormalizeEmail("  Jo\u0308RG@Example.COM \t")
	if err != nil {
		t.Fatalf("NormalizeEmail() error = %v", err)
	}
	if display != "Jo\u0308RG@Example.COM" {
		t.Fatalf("display = %q, want trimmed original representation", display)
	}
	if canonical != "jörg@example.com" {
		t.Fatalf("canonical = %q, want NFC case-folded value", canonical)
	}
}

func TestNormalizeEmailUsesUnicodeCaseFolding(t *testing.T) {
	t.Parallel()

	canonical, _, err := NormalizeEmail("Straße@EXAMPLE.COM")
	if err != nil {
		t.Fatalf("NormalizeEmail() error = %v", err)
	}
	if canonical != "strasse@example.com" {
		t.Fatalf("canonical = %q, want full Unicode case folding", canonical)
	}
}

func TestNormalizeEmailRejectsBlankInput(t *testing.T) {
	t.Parallel()

	if _, _, err := NormalizeEmail(" \t\n"); err == nil {
		t.Fatal("NormalizeEmail() error = nil, want blank email error")
	}
}

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
