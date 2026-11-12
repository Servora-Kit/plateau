package security

import "testing"

func TestNormalizeEmail(t *testing.T) {
	if got := NormalizeEmail("  J\u006f\u0308rg@Example.COM  "); got != "jörg@example.com" {
		t.Fatalf("NormalizeEmail(NFC) = %q, want jörg@example.com", got)
	}
	if got := NormalizeEmail("Straße@EXAMPLE.COM"); got != "strasse@example.com" {
		t.Fatalf("NormalizeEmail(case fold) = %q, want strasse@example.com", got)
	}
	if got := NormalizeEmail("   "); got != "" {
		t.Fatalf("NormalizeEmail(blank) = %q, want empty", got)
	}
}
