package password_test

import (
	"strings"
	"testing"

	"github.com/Servora-Kit/plateau/security/password"
	"github.com/alexedwards/argon2id"
)

func TestHashAndCompare(t *testing.T) {
	plaintext := "correct horse battery staple"
	hash, err := password.Hash(plaintext)
	if err != nil {
		t.Fatalf("Hash() error = %v", err)
	}
	if !strings.HasPrefix(hash, "$argon2id$v=19$m=65536,t=3,p=1$") {
		t.Fatalf("Hash() = %q, want fixed Argon2id PHC parameters", hash)
	}
	if strings.Contains(hash, plaintext) {
		t.Fatal("Hash() contains plaintext password")
	}

	match, needsRehash, err := password.Compare(plaintext, hash)
	if err != nil {
		t.Fatalf("Compare() error = %v", err)
	}
	if !match || needsRehash {
		t.Fatalf("Compare() = match:%t needsRehash:%t, want true,false", match, needsRehash)
	}

	match, needsRehash, err = password.Compare("wrong password", hash)
	if err != nil {
		t.Fatalf("Compare(wrong) error = %v", err)
	}
	if match || needsRehash {
		t.Fatalf("Compare(wrong) = match:%t needsRehash:%t, want false,false", match, needsRehash)
	}
}

func TestHashUsesIndependentSalt(t *testing.T) {
	first, err := password.Hash("same password")
	if err != nil {
		t.Fatalf("first Hash() error = %v", err)
	}
	second, err := password.Hash("same password")
	if err != nil {
		t.Fatalf("second Hash() error = %v", err)
	}
	if first == second {
		t.Fatal("Hash() reused salt")
	}
}

func TestCompareRejectsMalformedHash(t *testing.T) {
	match, needsRehash, err := password.Compare("password", "not-a-phc-hash")
	if err == nil {
		t.Fatal("Compare() error = nil, want malformed hash error")
	}
	if match || needsRehash {
		t.Fatalf("Compare() = match:%t needsRehash:%t after malformed hash", match, needsRehash)
	}
}

func TestCompareReportsParameterUpgrade(t *testing.T) {
	legacy, err := argon2id.CreateHash("password", &argon2id.Params{
		Memory:      32 * 1024,
		Iterations:  2,
		Parallelism: 1,
		SaltLength:  16,
		KeyLength:   32,
	})
	if err != nil {
		t.Fatalf("create legacy hash: %v", err)
	}

	match, needsRehash, err := password.Compare("password", legacy)
	if err != nil {
		t.Fatalf("Compare() error = %v", err)
	}
	if !match || !needsRehash {
		t.Fatalf("Compare() = match:%t needsRehash:%t, want true,true", match, needsRehash)
	}
}
