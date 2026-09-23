package cap

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"strings"
	"testing"
	"time"

	capv1 "github.com/Servora-Kit/plateau/api/gen/go/plateau/security/cap/v1"
	goredis "github.com/redis/go-redis/v9"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/durationpb"
)

const testSigningSecret = "0123456789abcdef0123456789abcdef"

func testCAPConfig() *capv1.CAP {
	return &capv1.CAP{
		SigningSecret:       proto.String(testSigningSecret),
		ChallengeCount:      proto.Int32(2),
		ChallengeSize:       proto.Int32(8),
		ChallengeDifficulty: proto.Int32(1),
	}
}

func disconnectedRedis(t *testing.T) *goredis.Client {
	t.Helper()
	client := goredis.NewClient(&goredis.Options{Addr: "127.0.0.1:1"})
	t.Cleanup(func() { _ = client.Close() })
	return client
}

func TestNewAppliesDefaultsAndValidatesConfiguration(t *testing.T) {
	client := disconnectedRedis(t)

	if _, err := New(nil, client); err == nil {
		t.Fatal("New(nil, client) error = nil")
	}
	if _, err := New(&capv1.CAP{SigningSecret: proto.String(testSigningSecret)}, nil); err == nil {
		t.Fatal("New(config, nil) error = nil")
	}
	if _, err := New(&capv1.CAP{SigningSecret: proto.String("too-short")}, client); err == nil {
		t.Fatal("New(short secret, client) error = nil")
	}
	if _, err := New(&capv1.CAP{
		SigningSecret:  proto.String(testSigningSecret),
		ChallengeCount: proto.Int32(maxChallengeCount + 1),
	}, client); err == nil {
		t.Fatal("New(out-of-range count, client) error = nil")
	}
	if _, err := New(&capv1.CAP{
		SigningSecret: proto.String(testSigningSecret),
		ChallengeTtl:  durationpb.New(-time.Second),
	}, client); err == nil {
		t.Fatal("New(negative TTL, client) error = nil")
	}

	config := &capv1.CAP{SigningSecret: proto.String(testSigningSecret)}
	captcha, err := New(config, client)
	if err != nil {
		t.Fatalf("New(default config) error = %v", err)
	}
	if captcha.challengeParams != (ChallengeParams{C: 50, S: 32, D: 4}) {
		t.Fatalf("challenge params = %#v", captcha.challengeParams)
	}
	if captcha.challengeTTL != 10*time.Minute || captcha.tokenTTL != 20*time.Minute {
		t.Fatalf("TTLs = %v/%v", captcha.challengeTTL, captcha.tokenTTL)
	}
	if captcha.keyPrefix != "cap:v2:" {
		t.Fatalf("key prefix = %q", captcha.keyPrefix)
	}
}

func TestChallengeJWTMatchesCapJSCoreVector(t *testing.T) {
	captcha, err := New(testCAPConfig(), disconnectedRedis(t))
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	captcha.now = func() time.Time { return time.UnixMilli(1_999_999_500_000) }
	claims := challengeClaims{
		Nonce:      "0123456789abcdef0123456789abcdef0123456789abcdef01",
		Count:      2,
		Size:       8,
		Difficulty: 1,
		Expires:    2_000_000_000_000,
		IssuedAt:   1_999_999_400_000,
		Scope:      "signup",
	}
	const wantToken = "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJuIjoiMDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWYwMTIzNDU2Nzg5YWJjZGVmMDEiLCJjIjoyLCJzIjo4LCJkIjoxLCJleHAiOjIwMDAwMDAwMDAwMDAsImlhdCI6MTk5OTk5OTQwMDAwMCwic2siOiJzaWdudXAifQ._aKqvA0KWtWfxlk8-9JpVP6YAH0cdmEWoKYc2OeF8OI"

	token, err := captcha.signChallenge(claims)
	if err != nil {
		t.Fatalf("signChallenge() error = %v", err)
	}
	if token != wantToken {
		t.Fatalf("token = %q, want capjs-core vector %q", token, wantToken)
	}
	parsed, signatureHex, err := captcha.verifyChallenge(token)
	if err != nil {
		t.Fatalf("verifyChallenge() error = %v", err)
	}
	if parsed != claims || signatureHex == "" {
		t.Fatalf("verifyChallenge() = %#v, %q", parsed, signatureHex)
	}

	if salt := prngFromHash(fnv1aResume(fnv1a(token), "1"), 8); salt != "684e1edb" {
		t.Fatalf("first salt = %q", salt)
	}
	firstHash := fnv1aResume(fnv1a(token), "1")
	if target := prngFromHash(fnv1aResume(firstHash, "d"), 1); target != "f" {
		t.Fatalf("first target = %q", target)
	}
	if !validSolutions(token, claims, []int{13, 5}) {
		t.Fatal("capjs-core solution vector rejected")
	}
}

func TestChallengeJWTRejectsTamperingAndUnsupportedClaims(t *testing.T) {
	captcha, err := New(testCAPConfig(), disconnectedRedis(t))
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	captcha.now = func() time.Time { return time.UnixMilli(1_000) }
	claims := challengeClaims{
		Nonce:      strings.Repeat("a", 50),
		Count:      2,
		Size:       8,
		Difficulty: 1,
		Expires:    10_000,
		IssuedAt:   500,
	}
	token, err := captcha.signChallenge(claims)
	if err != nil {
		t.Fatalf("signChallenge() error = %v", err)
	}

	tampered := token[:len(token)-1] + "A"
	if _, _, err := captcha.verifyChallenge(tampered); err == nil {
		t.Fatal("tampered token accepted")
	}
	other, err := New(&capv1.CAP{
		SigningSecret:       proto.String("fedcba9876543210fedcba9876543210"),
		ChallengeCount:      proto.Int32(2),
		ChallengeSize:       proto.Int32(8),
		ChallengeDifficulty: proto.Int32(1),
	}, disconnectedRedis(t))
	if err != nil {
		t.Fatalf("New(other) error = %v", err)
	}
	other.now = captcha.now
	if _, _, err := other.verifyChallenge(token); err == nil {
		t.Fatal("token accepted with another secret")
	}

	unsupported := signRawToken(t, challengeJWTHeader, `{"n":"`+strings.Repeat("a", 50)+`","c":2,"s":8,"d":1,"exp":10000,"iat":500,"f":2}`)
	if _, _, err := captcha.verifyChallenge(unsupported); err == nil {
		t.Fatal("format-2 token accepted")
	}
	wrongAlgorithm := signRawToken(t, `{"alg":"HS512","typ":"JWT"}`, `{"n":"`+strings.Repeat("a", 50)+`","c":2,"s":8,"d":1,"exp":10000,"iat":500}`)
	if _, _, err := captcha.verifyChallenge(wrongAlgorithm); err == nil {
		t.Fatal("non-HS256 header accepted")
	}

	captcha.now = func() time.Time { return time.UnixMilli(10_001) }
	if _, _, err := captcha.verifyChallenge(token); err == nil {
		t.Fatal("expired token accepted")
	}
}

func signRawToken(t *testing.T, header, payload string) string {
	t.Helper()
	headerPart := base64.RawURLEncoding.EncodeToString([]byte(header))
	payloadPart := base64.RawURLEncoding.EncodeToString([]byte(payload))
	input := headerPart + "." + payloadPart
	mac := hmac.New(sha256.New, []byte(testSigningSecret))
	_, _ = mac.Write([]byte(input))
	return input + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func TestPRNG(t *testing.T) {
	tests := []struct {
		seed   string
		length int
		want   string
	}{
		{"hello", 8, "eb492c6e"},
		{"test123", 4, "7197"},
	}
	for _, test := range tests {
		if got := prng(test.seed, test.length); got != test.want {
			t.Errorf("prng(%q, %d) = %q, want %q", test.seed, test.length, got, test.want)
		}
	}
}

func TestHashSHA256(t *testing.T) {
	const want = "2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824"
	if got := hashSHA256("hello"); got != want {
		t.Errorf("hashSHA256(hello) = %q, want %q", got, want)
	}
}

func TestRandomHex(t *testing.T) {
	for _, byteCount := range []int{8, 15, 25} {
		value, err := randomHex(byteCount)
		if err != nil {
			t.Fatalf("randomHex(%d) error = %v", byteCount, err)
		}
		if !isLowerHex(value, byteCount*2) {
			t.Errorf("randomHex(%d) = %q", byteCount, value)
		}
	}
}
