package cap

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	redispb "github.com/Servora-Kit/servora/api/gen/go/servora/contrib/db/redis/v1"
	rediscontrib "github.com/Servora-Kit/servora/contrib/db/redis"
	"github.com/alicebob/miniredis/v2"
	khttp "github.com/go-kratos/kratos/v3/transport/http"
	"google.golang.org/protobuf/types/known/durationpb"
)

func newTestCAP(t *testing.T) (*Cap, *Cap, *miniredis.Miniredis) {
	t.Helper()

	server := miniredis.RunT(t)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	client, cleanup, err := rediscontrib.New(&redispb.Redis{
		Addr:         server.Addr(),
		DialTimeout:  durationpb.New(20 * time.Millisecond),
		ReadTimeout:  durationpb.New(20 * time.Millisecond),
		WriteTimeout: durationpb.New(20 * time.Millisecond),
	}, logger)
	if err != nil {
		t.Fatalf("create Redis client: %v", err)
	}
	t.Cleanup(cleanup)
	return New(client), New(client), server
}

func solveChallenge(token string, challenge ChallengeParams) []int {
	solutions := make([]int, challenge.C)
	for index := range solutions {
		indexText := strconv.Itoa(index + 1)
		salt := prng(token+indexText, challenge.S)
		target := prng(token+indexText+"d", challenge.D)
		for candidate := 0; ; candidate++ {
			if strings.HasPrefix(hashSHA256(salt+strconv.Itoa(candidate)), target) {
				solutions[index] = candidate
				break
			}
		}
	}
	return solutions
}

func TestCAPCrossInstanceOneTimeConsumption(t *testing.T) {
	first, second, _ := newTestCAP(t)
	ctx := t.Context()
	challenge, err := first.CreateChallenge(ctx, &ChallengeConfig{
		ChallengeCount:      2,
		ChallengeSize:       8,
		ChallengeDifficulty: 1,
	})
	if err != nil {
		t.Fatalf("CreateChallenge() error = %v", err)
	}

	redeemed, err := second.RedeemChallenge(ctx, challenge.Token, solveChallenge(challenge.Token, challenge.Challenge))
	if err != nil {
		t.Fatalf("RedeemChallenge() error = %v", err)
	}
	if !redeemed.Success || redeemed.Token == "" {
		t.Fatalf("RedeemChallenge() = %#v, want verification token", redeemed)
	}

	replayed, err := first.RedeemChallenge(ctx, challenge.Token, solveChallenge(challenge.Token, challenge.Challenge))
	if err != nil {
		t.Fatalf("replayed RedeemChallenge() error = %v", err)
	}
	if replayed.Success {
		t.Fatal("replayed challenge produced a second verification token")
	}

	valid, err := first.ValidateToken(ctx, redeemed.Token)
	if err != nil {
		t.Fatalf("ValidateToken() error = %v", err)
	}
	if !valid {
		t.Fatal("ValidateToken() = false, want first consumption success")
	}
	valid, err = second.ValidateToken(ctx, redeemed.Token)
	if err != nil {
		t.Fatalf("replayed ValidateToken() error = %v", err)
	}
	if valid {
		t.Fatal("ValidateToken() accepted a consumed verification token")
	}
}

func TestValidateTokenReportsRedisFailure(t *testing.T) {
	first, _, server := newTestCAP(t)
	server.Close()

	valid, err := first.ValidateToken(t.Context(), "identifier:verification-token")
	if err == nil {
		t.Fatal("ValidateToken() error = nil after Redis failure")
	}
	if valid {
		t.Fatal("ValidateToken() accepted token after Redis failure")
	}
}

func TestRegisterUsesUnversionedCAPPaths(t *testing.T) {
	captcha, _, _ := newTestCAP(t)
	server := khttp.NewServer()
	Register(server, captcha)

	request := httptest.NewRequest(http.MethodPost, "/cap/challenge", nil)
	response := httptest.NewRecorder()
	server.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("POST /cap/challenge status = %d, body = %s", response.Code, response.Body.String())
	}

	legacyRequest := httptest.NewRequest(http.MethodPost, "/v1/cap/challenge", nil)
	legacyResponse := httptest.NewRecorder()
	server.ServeHTTP(legacyResponse, legacyRequest)
	if legacyResponse.Code != http.StatusNotFound {
		t.Fatalf("POST /v1/cap/challenge status = %d, want 404", legacyResponse.Code)
	}
}
