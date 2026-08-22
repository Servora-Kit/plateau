// Package cap 实现了内嵌式 Cap 工作量证明（PoW）人机验证服务端。
// 算法移植自 Cap 官方 JS 实现（https://github.com/tiagozip/cap），
// 使用项目现有的 Redis 客户端替代文件系统存储。
package cap

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	nethttp "net/http"
	"strings"
	"time"

	khttp "github.com/go-kratos/kratos/v3/transport/http"

	goredis "github.com/redis/go-redis/v9"
)

const (
	challengeKeyPrefix = "cap:challenge:"
	tokenKeyPrefix     = "cap:token:"

	defaultChallengeCount      = 50
	defaultChallengeSize       = 32
	defaultChallengeDifficulty = 4
	defaultExpiresMs           = 600_000 // 10 minutes
	tokenTTL                   = 20 * time.Minute
)

// ChallengeConfig holds configuration for challenge creation.
type ChallengeConfig struct {
	ChallengeCount      int
	ChallengeSize       int
	ChallengeDifficulty int
	ExpiresMs           int64
}

// ChallengeResponse is returned by CreateChallenge.
type ChallengeResponse struct {
	Challenge ChallengeParams `json:"challenge"`
	Token     string          `json:"token"`
	Expires   int64           `json:"expires"`
}

// ChallengeParams are the c/s/d fields stored in Redis and returned to the client.
type ChallengeParams struct {
	C int `json:"c"`
	S int `json:"s"`
	D int `json:"d"`
}

// RedeemResponse is returned by RedeemChallenge.
type RedeemResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message,omitempty"`
	Token   string `json:"token,omitempty"`
	Expires int64  `json:"expires,omitempty"`
}

// challengeRecord is the JSON structure stored in Redis for a challenge.
type challengeRecord struct {
	Challenge ChallengeParams `json:"challenge"`
	Expires   int64           `json:"expires"`
}

// Cap is the embedded Cap CAPTCHA server backed by Redis.
type Cap struct {
	rdb *goredis.Client
}

// New creates a new Cap instance backed by the given Redis client.
func New(rdb *goredis.Client) *Cap {
	return &Cap{rdb: rdb}
}

// prng generates a deterministic hex string of the given length from a string seed.
// This is a direct port of the JS prng() function using FNV-1a + xorshift32.
func prng(seed string, length int) string {
	// FNV-1a 32-bit
	h := uint32(2166136261)
	for i := 0; i < len(seed); i++ {
		h ^= uint32(seed[i])
		h += (h << 1) + (h << 4) + (h << 7) + (h << 8) + (h << 24)
	}
	state := h

	next := func() uint32 {
		state ^= state << 13
		state ^= state >> 17
		state ^= state << 5
		return state
	}

	var result strings.Builder
	for result.Len() < length {
		rnd := next()
		result.WriteString(fmt.Sprintf("%08x", rnd))
	}
	return result.String()[:length]
}

// randomHex generates a cryptographically secure random hex string of the given byte count.
func randomHex(byteCount int) (string, error) {
	b := make([]byte, byteCount)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// hashSHA256 returns the hex SHA-256 digest of the input string.
func hashSHA256(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

// CreateChallenge generates a new PoW challenge, stores it in Redis, and returns it.
func (c *Cap) CreateChallenge(ctx context.Context, conf *ChallengeConfig) (*ChallengeResponse, error) {
	params := ChallengeParams{
		C: defaultChallengeCount,
		S: defaultChallengeSize,
		D: defaultChallengeDifficulty,
	}
	expiresMs := int64(defaultExpiresMs)

	if conf != nil {
		if conf.ChallengeCount > 0 {
			params.C = conf.ChallengeCount
		}
		if conf.ChallengeSize > 0 {
			params.S = conf.ChallengeSize
		}
		if conf.ChallengeDifficulty > 0 {
			params.D = conf.ChallengeDifficulty
		}
		if conf.ExpiresMs > 0 {
			expiresMs = conf.ExpiresMs
		}
	}

	// randomHex(25) => 50 hex chars, matching JS behavior
	token, err := randomHex(25)
	if err != nil {
		return nil, fmt.Errorf("cap: generate token: %w", err)
	}

	expires := time.Now().UnixMilli() + expiresMs

	record := challengeRecord{
		Challenge: params,
		Expires:   expires,
	}
	data, err := json.Marshal(record)
	if err != nil {
		return nil, fmt.Errorf("cap: marshal challenge: %w", err)
	}

	ttl := time.Duration(expiresMs) * time.Millisecond
	key := challengeKeyPrefix + token
	if err := c.rdb.Set(ctx, key, string(data), ttl).Err(); err != nil {
		return nil, fmt.Errorf("cap: store challenge: %w", err)
	}

	return &ChallengeResponse{
		Challenge: params,
		Token:     token,
		Expires:   expires,
	}, nil
}

// RedeemChallenge validates a PoW solution and, if valid, returns a one-time verification token.
func (c *Cap) RedeemChallenge(ctx context.Context, token string, solutions []int) (*RedeemResponse, error) {
	if token == "" || solutions == nil {
		return &RedeemResponse{Success: false, Message: "Invalid body"}, nil
	}

	key := challengeKeyPrefix + token
	raw, err := c.rdb.GetDel(ctx, key).Result()
	if errors.Is(err, goredis.Nil) {
		return &RedeemResponse{Success: false, Message: "Challenge invalid or expired"}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("cap: consume challenge: %w", err)
	}

	var record challengeRecord
	if err := json.Unmarshal([]byte(raw), &record); err != nil {
		return &RedeemResponse{Success: false, Message: "Challenge invalid or expired"}, nil
	}

	if record.Expires < time.Now().UnixMilli() {
		return &RedeemResponse{Success: false, Message: "Challenge invalid or expired"}, nil
	}

	if len(solutions) != record.Challenge.C {
		return &RedeemResponse{Success: false, Message: "Invalid solution count"}, nil
	}

	// Validate each solution: sha256(salt + solution) must start with target.
	for i := 0; i < record.Challenge.C; i++ {
		idx := i + 1 // 1-indexed, matching JS: i = i + 1
		salt := prng(fmt.Sprintf("%s%d", token, idx), record.Challenge.S)
		target := prng(fmt.Sprintf("%s%dd", token, idx), record.Challenge.D)
		digest := hashSHA256(fmt.Sprintf("%s%d", salt, solutions[i]))
		if !strings.HasPrefix(digest, target) {
			return &RedeemResponse{Success: false, Message: "Invalid solution"}, nil
		}
	}

	// Generate verification token: id:vertoken, store id:hash in Redis.
	vertoken, err := randomHex(15)
	if err != nil {
		return nil, fmt.Errorf("cap: generate vertoken: %w", err)
	}
	id, err := randomHex(8)
	if err != nil {
		return nil, fmt.Errorf("cap: generate id: %w", err)
	}

	hash := hashSHA256(vertoken)
	tokenKey := tokenKeyPrefix + id + ":" + hash

	expires := time.Now().UnixMilli() + int64(tokenTTL/time.Millisecond)
	// Store empty marker with TTL; the key itself encodes the validity.
	if err := c.rdb.Set(ctx, tokenKey, "1", tokenTTL).Err(); err != nil {
		return nil, fmt.Errorf("cap: store token: %w", err)
	}

	return &RedeemResponse{
		Success: true,
		Token:   id + ":" + vertoken,
		Expires: expires,
	}, nil
}

// ValidateToken consumes a one-time verification token.
// Returns true if the token is valid and has not been used before.
func (c *Cap) ValidateToken(ctx context.Context, token string) (bool, error) {
	if token == "" {
		return false, nil
	}

	parts := strings.SplitN(token, ":", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return false, nil
	}

	id, vertoken := parts[0], parts[1]
	hash := hashSHA256(vertoken)
	tokenKey := tokenKeyPrefix + id + ":" + hash

	// GetDel atomically reads and deletes the key (one-time use).
	val, err := c.rdb.GetDel(ctx, tokenKey).Result()
	if errors.Is(err, goredis.Nil) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("cap: consume verification token: %w", err)
	}
	if val == "" {
		return false, nil
	}

	return true, nil
}

// Cap 路由使用的 Kratos operation 常量，供外部白名单引用。
const (
	OperationCapChallenge = "/cap/challenge"
	OperationCapRedeem    = "/cap/redeem"
)

// Register 将 Cap 人机验证 HTTP 路由挂载到 Kratos HTTP 服务器，风格与其他服务注册一致。
//
// 注册路由（均无需认证，需将 OperationCapChallenge/OperationCapRedeem 加入白名单）：
//
//	POST /cap/challenge — 生成 PoW challenge，返回 {challenge, token, expires}
//	POST /cap/redeem   — 提交 PoW 解答，换取一次性验证 token
func Register(s *khttp.Server, c *Cap) {
	r := s.Route("/cap")

	// POST /cap/challenge — 客户端请求新的 PoW 挑战
	r.POST("/challenge", func(ctx khttp.Context) error {
		khttp.SetOperation(ctx, OperationCapChallenge)
		resp, err := c.CreateChallenge(ctx, nil)
		if err != nil {
			return ctx.JSON(nethttp.StatusInternalServerError, map[string]string{"error": "internal error"})
		}
		return ctx.JSON(nethttp.StatusOK, resp)
	})

	// POST /cap/redeem — 客户端提交 PoW 答案，服务端验证后返回验证 token
	r.POST("/redeem", func(ctx khttp.Context) error {
		khttp.SetOperation(ctx, OperationCapRedeem)
		var body struct {
			Token     string `json:"token"`
			Solutions []int  `json:"solutions"`
		}
		if err := json.NewDecoder(ctx.Request().Body).Decode(&body); err != nil {
			return ctx.JSON(nethttp.StatusBadRequest, map[string]string{"error": "invalid body"})
		}
		resp, err := c.RedeemChallenge(ctx, body.Token, body.Solutions)
		if err != nil {
			return ctx.JSON(nethttp.StatusInternalServerError, map[string]string{"error": "internal error"})
		}
		httpStatus := nethttp.StatusOK
		if !resp.Success {
			httpStatus = nethttp.StatusBadRequest
		}
		return ctx.JSON(httpStatus, resp)
	})
}
