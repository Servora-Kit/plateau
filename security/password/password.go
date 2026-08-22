// Package password hashes and verifies Plateau passwords with fixed Argon2id parameters.
package password

import (
	"fmt"

	"github.com/alexedwards/argon2id"
)

var params = &argon2id.Params{
	Memory:      64 * 1024,
	Iterations:  3,
	Parallelism: 1,
	SaltLength:  16,
	KeyLength:   32,
}

// Hash returns an Argon2id PHC string with a fresh random salt.
func Hash(plaintext string) (string, error) {
	hash, err := argon2id.CreateHash(plaintext, params)
	if err != nil {
		return "", fmt.Errorf("hash password: %w", err)
	}
	return hash, nil
}

// Compare verifies plaintext against an Argon2id PHC string and reports whether
// the stored parameters should be upgraded after a successful match.
func Compare(plaintext, encodedHash string) (match, needsRehash bool, err error) {
	match, actual, err := argon2id.CheckHash(plaintext, encodedHash)
	if err != nil {
		return false, false, fmt.Errorf("compare password: %w", err)
	}
	return match, !equalParams(actual, params), nil
}

func equalParams(left, right *argon2id.Params) bool {
	return left != nil && right != nil &&
		left.Memory == right.Memory &&
		left.Iterations == right.Iterations &&
		left.Parallelism == right.Parallelism &&
		left.SaltLength == right.SaltLength &&
		left.KeyLength == right.KeyLength
}
