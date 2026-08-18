package cap

import (
	"testing"
)

// TestPRNG verifies the prng output matches the expected JS behavior.
// Reference values computed with the JS implementation:
//
//	prng("hello", 8)   → first 8 chars of the XORShift stream seeded by FNV1a("hello")
//	prng("test123", 4) → first 4 chars
func TestPRNG(t *testing.T) {
	tests := []struct {
		seed   string
		length int
		// expected computed manually by running the JS prng in Node.js
		// $ node -e "function fnv1a(s){let h=2166136261;for(let i=0;i<s.length;i++){h^=s.charCodeAt(i);h+=(h<<1)+(h<<4)+(h<<7)+(h<<8)+(h<<24);h>>>=0;}return h;}function prng(seed,length){let state=fnv1a(seed);function next(){state^=state<<13;state^=state>>>17;state^=state<<5;state>>>=0;return state;}let r='';while(r.length<length){r+=next().toString(16).padStart(8,'0');}return r.substring(0,length);}console.log(prng('hello',8),prng('test123',4));"
		want string
	}{
		{"hello", 8, "eb492c6e"},
		{"test123", 4, "7197"},
	}

	for _, tt := range tests {
		got := prng(tt.seed, tt.length)
		if got != tt.want {
			t.Errorf("prng(%q, %d) = %q, want %q", tt.seed, tt.length, got, tt.want)
		}
	}
}

// TestHashSHA256 verifies the SHA-256 helper.
func TestHashSHA256(t *testing.T) {
	// echo -n "hello" | sha256sum
	want := "2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824"
	got := hashSHA256("hello")
	if got != want {
		t.Errorf("hashSHA256(%q) = %q, want %q", "hello", got, want)
	}
}

// TestRandomHex verifies randomHex returns correctly-sized hex strings.
func TestRandomHex(t *testing.T) {
	for _, byteCount := range []int{8, 15, 25} {
		h, err := randomHex(byteCount)
		if err != nil {
			t.Fatalf("randomHex(%d) error: %v", byteCount, err)
		}
		if len(h) != byteCount*2 {
			t.Errorf("randomHex(%d) len = %d, want %d", byteCount, len(h), byteCount*2)
		}
		for _, c := range h {
			if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
				t.Errorf("randomHex(%d) produced non-hex char %q in %q", byteCount, c, h)
			}
		}
	}
}
