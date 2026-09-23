// Package cap implements an embedded, Redis-backed Cap proof-of-work server.
//
// Its basic SHA-256 wire protocol and signed challenge semantics follow
// capjs-core 0.1.x. The package intentionally excludes instrumentation, RSW,
// format-2 challenges, Cap Standalone, and siteverify. Protocol v2 does not
// read challenge or verification records produced by the legacy stateful
// implementation.
package cap

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json/v2"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	pb "github.com/Servora-Kit/plateau/api/gen/go/plateau/security/cap/v1"
	khttp "github.com/go-kratos/kratos/v3/transport/http"
	"github.com/redis/go-redis/v9"
	"google.golang.org/protobuf/types/known/durationpb"
)

const (
	maxChallengeCount      = 1000
	maxChallengeSize       = 256
	maxChallengeDifficulty = 16
	minSigningSecretBytes  = 16

	challengeJWTHeader = `{"alg":"HS256","typ":"JWT"}`
)

var (
	challengeJWTHeaderEncoded = base64.RawURLEncoding.EncodeToString([]byte(challengeJWTHeader))

	issueVerificationScript = redis.NewScript(`
if redis.call("EXISTS", KEYS[1]) == 1 then
  return 0
end
if redis.call("EXISTS", KEYS[2]) == 1 then
  return -1
end
redis.call("PSETEX", KEYS[1], ARGV[1], "1")
redis.call("PSETEX", KEYS[2], ARGV[2], ARGV[3])
return 1
`)

	consumeScopedTokenScript = redis.NewScript(`
local scope = redis.call("GET", KEYS[1])
if not scope or scope ~= ARGV[1] then
  return 0
end
redis.call("DEL", KEYS[1])
return 1
`)
)

// ChallengeConfig overrides one challenge's server-controlled policy.
type ChallengeConfig struct {
	ChallengeCount      int
	ChallengeSize       int
	ChallengeDifficulty int
	ExpiresMs           int64
	Scope               string
}

// ChallengeResponse is returned by CreateChallenge.
type ChallengeResponse struct {
	Challenge ChallengeParams `json:"challenge"`
	Token     string          `json:"token"`
	Expires   int64           `json:"expires"`
}

// ChallengeParams are the public c/s/d proof-of-work parameters.
type ChallengeParams struct {
	C int `json:"c"`
	S int `json:"s"`
	D int `json:"d"`
}

// RedeemResponse is returned by RedeemChallenge.
type RedeemResponse struct {
	Success  bool   `json:"success"`
	Message  string `json:"message,omitempty"`
	Token    string `json:"token,omitempty"`
	Expires  int64  `json:"expires,omitempty"`
	Scope    string `json:"scope,omitempty"`
	IssuedAt int64  `json:"iat,omitempty"`
}

type challengeClaims struct {
	Nonce      string `json:"n"`
	Count      int    `json:"c"`
	Size       int    `json:"s"`
	Difficulty int    `json:"d"`
	Expires    int64  `json:"exp"`
	IssuedAt   int64  `json:"iat"`
	Scope      string `json:"sk,omitempty"`
}

// Cap is the embedded Cap server backed by Redis for one-time state.
type Cap struct {
	rdb             *redis.Client
	signingSecret   []byte
	keyPrefix       string
	challengeParams ChallengeParams
	challengeTTL    time.Duration
	tokenTTL        time.Duration
	defaultScope    string
	now             func() time.Time
}

// New creates a Cap instance from its generated protobuf configuration and
// shared Redis client. Configuration precedes runtime dependencies by project
// convention.
func New(config *pb.CAP, rdb *redis.Client) (*Cap, error) {
	if config == nil {
		return nil, fmt.Errorf("cap: config is nil")
	}
	if rdb == nil {
		return nil, fmt.Errorf("cap: Redis client is nil")
	}

	if err := config.Apply(); err != nil {
		return nil, fmt.Errorf("cap: invalid config: %w", err)
	}
	if len([]byte(config.GetSigningSecret())) < minSigningSecretBytes {
		return nil, fmt.Errorf("cap: signing_secret must be at least %d bytes", minSigningSecretBytes)
	}

	challengeTTL, err := validDuration("challenge_ttl", config.GetChallengeTtl())
	if err != nil {
		return nil, err
	}
	tokenTTL, err := validDuration("token_ttl", config.GetTokenTtl())
	if err != nil {
		return nil, err
	}
	params := ChallengeParams{
		C: int(config.GetChallengeCount()),
		S: int(config.GetChallengeSize()),
		D: int(config.GetChallengeDifficulty()),
	}
	if err := validateChallengeParams(params); err != nil {
		return nil, fmt.Errorf("cap: invalid config: %w", err)
	}

	prefix := config.GetRedisKeyPrefix()
	return &Cap{
		rdb:             rdb,
		signingSecret:   []byte(config.GetSigningSecret()),
		keyPrefix:       prefix,
		challengeParams: params,
		challengeTTL:    challengeTTL,
		tokenTTL:        tokenTTL,
		defaultScope:    config.GetDefaultScope(),
		now:             time.Now,
	}, nil
}

func validDuration(name string, value *durationpb.Duration) (time.Duration, error) {
	if value == nil || !value.IsValid() {
		return 0, fmt.Errorf("cap: invalid config: %s is invalid", name)
	}
	duration := value.AsDuration()
	if duration <= 0 {
		return 0, fmt.Errorf("cap: invalid config: %s must be positive", name)
	}
	return duration, nil
}

func validateChallengeParams(params ChallengeParams) error {
	if params.C < 1 || params.C > maxChallengeCount {
		return fmt.Errorf("challenge_count must be in [1, %d]", maxChallengeCount)
	}
	if params.S < 1 || params.S > maxChallengeSize {
		return fmt.Errorf("challenge_size must be in [1, %d]", maxChallengeSize)
	}
	if params.D < 1 || params.D > maxChallengeDifficulty {
		return fmt.Errorf("challenge_difficulty must be in [1, %d]", maxChallengeDifficulty)
	}
	return nil
}

func fnv1a(value string) uint32 {
	return fnv1aResume(2166136261, value)
}

func fnv1aResume(state uint32, value string) uint32 {
	for i := range len(value) {
		state ^= uint32(value[i])
		state += (state << 1) + (state << 4) + (state << 7) + (state << 8) + (state << 24)
	}
	return state
}

func prng(seed string, length int) string {
	return prngFromHash(fnv1a(seed), length)
}

func prngFromHash(state uint32, length int) string {
	const digits = "0123456789abcdef"
	buffer := make([]byte, ((length+7)/8)*8)
	for offset := 0; offset < len(buffer); offset += 8 {
		state ^= state << 13
		state ^= state >> 17
		state ^= state << 5
		for digit := range 8 {
			buffer[offset+digit] = digits[state>>uint(28-digit*4)&0xf]
		}
	}
	return string(buffer[:length])
}

func randomHex(byteCount int) (string, error) {
	value := make([]byte, byteCount)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	return hex.EncodeToString(value), nil
}

func hashSHA256(value string) string {
	digest := sha256.Sum256([]byte(value))
	return hex.EncodeToString(digest[:])
}

// CreateChallenge creates a signed, stateless challenge token.
func (c *Cap) CreateChallenge(ctx context.Context, override *ChallengeConfig) (*ChallengeResponse, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	params := c.challengeParams
	ttl := c.challengeTTL
	scope := c.defaultScope
	if override != nil {
		if override.ChallengeCount > 0 {
			params.C = override.ChallengeCount
		}
		if override.ChallengeSize > 0 {
			params.S = override.ChallengeSize
		}
		if override.ChallengeDifficulty > 0 {
			params.D = override.ChallengeDifficulty
		}
		if override.ExpiresMs > 0 {
			ttl = time.Duration(override.ExpiresMs) * time.Millisecond
		}
		if override.Scope != "" {
			scope = override.Scope
		}
	}
	if err := validateChallengeParams(params); err != nil {
		return nil, fmt.Errorf("cap: create challenge: %w", err)
	}
	if ttl <= 0 {
		return nil, fmt.Errorf("cap: create challenge: expiration must be positive")
	}

	nonce, err := randomHex(25)
	if err != nil {
		return nil, fmt.Errorf("cap: generate challenge nonce: %w", err)
	}
	now := c.now().UnixMilli()
	expires := now + ttl.Milliseconds()
	claims := challengeClaims{
		Nonce:      nonce,
		Count:      params.C,
		Size:       params.S,
		Difficulty: params.D,
		Expires:    expires,
		IssuedAt:   now,
		Scope:      scope,
	}
	token, err := c.signChallenge(claims)
	if err != nil {
		return nil, fmt.Errorf("cap: sign challenge: %w", err)
	}

	return &ChallengeResponse{Challenge: params, Token: token, Expires: expires}, nil
}

func (c *Cap) signChallenge(claims challengeClaims) (string, error) {
	payload, err := json.Marshal(claims)
	if err != nil {
		return "", err
	}
	body := base64.RawURLEncoding.EncodeToString(payload)
	signingInput := challengeJWTHeaderEncoded + "." + body
	mac := hmac.New(sha256.New, c.signingSecret)
	_, _ = mac.Write([]byte(signingInput))
	return signingInput + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil)), nil
}

func (c *Cap) verifyChallenge(token string) (challengeClaims, string, error) {
	var claims challengeClaims
	parts := strings.Split(token, ".")
	if len(parts) != 3 || parts[0] != challengeJWTHeaderEncoded || parts[1] == "" || parts[2] == "" {
		return claims, "", errors.New("invalid token")
	}

	signature, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return claims, "", errors.New("invalid token")
	}
	mac := hmac.New(sha256.New, c.signingSecret)
	_, _ = mac.Write([]byte(parts[0] + "." + parts[1]))
	if !hmac.Equal(signature, mac.Sum(nil)) {
		return claims, "", errors.New("invalid token")
	}

	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return claims, "", errors.New("invalid token")
	}
	if err := json.Unmarshal(payload, &claims, json.RejectUnknownMembers(true)); err != nil {
		return claims, "", errors.New("invalid token")
	}
	if !isLowerHex(claims.Nonce, 50) || claims.IssuedAt <= 0 || claims.Expires <= claims.IssuedAt {
		return claims, "", errors.New("invalid token")
	}
	if err := validateChallengeParams(ChallengeParams{C: claims.Count, S: claims.Size, D: claims.Difficulty}); err != nil {
		return claims, "", errors.New("invalid token")
	}
	if claims.Expires < c.now().UnixMilli() {
		return claims, "", errors.New("expired token")
	}
	return claims, hex.EncodeToString(signature), nil
}

// RedeemChallenge validates a proof and consumes the configured handler scope.
func (c *Cap) RedeemChallenge(ctx context.Context, token string, solutions []int) (*RedeemResponse, error) {
	return c.RedeemChallengeWithScope(ctx, token, solutions, c.defaultScope)
}

// RedeemChallengeWithScope validates a proof against a server-controlled scope.
func (c *Cap) RedeemChallengeWithScope(ctx context.Context, token string, solutions []int, expectedScope string) (*RedeemResponse, error) {
	if token == "" || solutions == nil {
		return failedRedeem("Invalid body"), nil
	}

	claims, signatureHex, err := c.verifyChallenge(token)
	if err != nil {
		return failedRedeem("Challenge invalid or expired"), nil
	}
	if expectedScope != "" && claims.Scope != expectedScope {
		return failedRedeem("Challenge invalid or expired"), nil
	}
	if len(solutions) != claims.Count {
		return failedRedeem("Invalid solution"), nil
	}
	if !validSolutions(token, claims, solutions) {
		return failedRedeem("Invalid solution"), nil
	}

	remainingTTL := time.Duration(claims.Expires-c.now().UnixMilli()) * time.Millisecond
	if remainingTTL <= 0 {
		return failedRedeem("Challenge invalid or expired"), nil
	}

	for range 3 {
		verificationSecret, randomErr := randomHex(15)
		if randomErr != nil {
			return nil, fmt.Errorf("cap: generate verification token: %w", randomErr)
		}
		id, randomErr := randomHex(8)
		if randomErr != nil {
			return nil, fmt.Errorf("cap: generate verification id: %w", randomErr)
		}
		tokenKey := c.keyPrefix + "token:" + id + ":" + hashSHA256(verificationSecret)
		nonceKey := c.keyPrefix + "nonce:" + signatureHex
		result, scriptErr := issueVerificationScript.Run(
			ctx,
			c.rdb,
			[]string{nonceKey, tokenKey},
			remainingTTL.Milliseconds(),
			c.tokenTTL.Milliseconds(),
			claims.Scope,
		).Int64()
		if scriptErr != nil {
			return nil, fmt.Errorf("cap: issue verification token: %w", scriptErr)
		}
		switch result {
		case 0:
			return failedRedeem("Challenge invalid or expired"), nil
		case -1:
			continue
		case 1:
			now := c.now().UnixMilli()
			return &RedeemResponse{
				Success:  true,
				Token:    id + ":" + verificationSecret,
				Expires:  now + c.tokenTTL.Milliseconds(),
				Scope:    claims.Scope,
				IssuedAt: claims.IssuedAt,
			}, nil
		default:
			return nil, fmt.Errorf("cap: issue verification token: unexpected Redis result %d", result)
		}
	}
	return nil, fmt.Errorf("cap: issue verification token: repeated token collision")
}

func validSolutions(token string, claims challengeClaims, solutions []int) bool {
	tokenHash := fnv1a(token)
	for index, solution := range solutions {
		if solution < 0 {
			return false
		}
		indexText := strconv.Itoa(index + 1)
		saltHash := fnv1aResume(tokenHash, indexText)
		targetHash := fnv1aResume(saltHash, "d")
		salt := prngFromHash(saltHash, claims.Size)
		target := prngFromHash(targetHash, claims.Difficulty)
		digest := sha256.Sum256([]byte(salt + strconv.Itoa(solution)))
		if !matchesHexPrefix(digest, target) {
			return false
		}
	}
	return true
}

func matchesHexPrefix(digest [sha256.Size]byte, target string) bool {
	fullBytes := len(target) / 2
	for index := range fullBytes {
		want, err := strconv.ParseUint(target[index*2:index*2+2], 16, 8)
		if err != nil || digest[index] != byte(want) {
			return false
		}
	}
	if len(target)%2 == 1 {
		want, err := strconv.ParseUint(target[len(target)-1:], 16, 4)
		if err != nil || digest[fullBytes]>>4 != byte(want) {
			return false
		}
	}
	return true
}

func failedRedeem(message string) *RedeemResponse {
	return &RedeemResponse{Success: false, Message: message}
}

// ValidateToken atomically consumes any valid one-time verification token.
func (c *Cap) ValidateToken(ctx context.Context, token string) (bool, error) {
	key, ok := c.verificationKey(token)
	if !ok {
		return false, nil
	}
	_, err := c.rdb.GetDel(ctx, key).Result()
	if errors.Is(err, redis.Nil) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("cap: consume verification token: %w", err)
	}
	return true, nil
}

// ValidateTokenWithScope atomically consumes a token only when its server-side
// scope matches expectedScope. A mismatch leaves the token available.
func (c *Cap) ValidateTokenWithScope(ctx context.Context, token, expectedScope string) (bool, error) {
	key, ok := c.verificationKey(token)
	if !ok {
		return false, nil
	}
	result, err := consumeScopedTokenScript.Run(ctx, c.rdb, []string{key}, expectedScope).Int64()
	if err != nil {
		return false, fmt.Errorf("cap: consume scoped verification token: %w", err)
	}
	return result == 1, nil
}

func (c *Cap) verificationKey(token string) (string, bool) {
	id, secret, ok := strings.Cut(token, ":")
	if !ok || strings.Contains(secret, ":") || !isLowerHex(id, 16) || !isLowerHex(secret, 30) {
		return "", false
	}
	return c.keyPrefix + "token:" + id + ":" + hashSHA256(secret), true
}

func isLowerHex(value string, length int) bool {
	if len(value) != length {
		return false
	}
	for index := range len(value) {
		if (value[index] < '0' || value[index] > '9') && (value[index] < 'a' || value[index] > 'f') {
			return false
		}
	}
	return true
}

// Cap route operation names are exported for authentication allowlists.
const (
	OperationCapChallenge = "/cap/challenge"
	OperationCapRedeem    = "/cap/redeem"
)

// Register mounts the embedded Cap routes on a Kratos HTTP server.
func Register(server *khttp.Server, captcha *Cap) {
	route := server.Route("/cap")
	route.POST("/challenge", func(ctx khttp.Context) error {
		khttp.SetOperation(ctx, OperationCapChallenge)
		response, err := captcha.CreateChallenge(ctx, nil)
		if err != nil {
			return ctx.JSON(http.StatusInternalServerError, map[string]string{"error": "internal error"})
		}
		return ctx.JSON(http.StatusOK, response)
	})
	route.POST("/redeem", func(ctx khttp.Context) error {
		khttp.SetOperation(ctx, OperationCapRedeem)
		var body struct {
			Token     string `json:"token"`
			Solutions []int  `json:"solutions"`
		}
		if err := json.UnmarshalRead(ctx.Request().Body, &body, json.RejectUnknownMembers(true)); err != nil {
			return ctx.JSON(http.StatusBadRequest, map[string]string{"error": "invalid body"})
		}
		response, err := captcha.RedeemChallenge(ctx, body.Token, body.Solutions)
		if err != nil {
			return ctx.JSON(http.StatusInternalServerError, map[string]string{"error": "internal error"})
		}
		status := http.StatusOK
		if !response.Success {
			status = http.StatusBadRequest
		}
		return ctx.JSON(status, response)
	})
}
