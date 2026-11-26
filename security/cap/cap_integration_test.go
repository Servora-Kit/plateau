package cap

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	redispb "github.com/Servora-Kit/plateau/api/gen/go/plateau/infra/redis/v1"
	capv1 "github.com/Servora-Kit/plateau/api/gen/go/plateau/security/cap/v1"
	infraredis "github.com/Servora-Kit/plateau/infra/redis"
	"github.com/alicebob/miniredis/v2"
	khttp "github.com/go-kratos/kratos/v3/transport/http"
	"google.golang.org/protobuf/types/known/durationpb"
)

func newTestCAP(t *testing.T) (*Cap, *Cap, *miniredis.Miniredis) {
	return newTestCAPWithConfig(t, testCAPConfig())
}

func newTestCAPWithConfig(t *testing.T, config *capv1.CAP) (*Cap, *Cap, *miniredis.Miniredis) {
	t.Helper()
	server := miniredis.RunT(t)
	client, cleanup, err := infraredis.New(&redispb.Redis{
		Addr:         server.Addr(),
		DialTimeout:  durationpb.New(time.Second),
		ReadTimeout:  durationpb.New(time.Second),
		WriteTimeout: durationpb.New(time.Second),
	})
	if err != nil {
		t.Fatalf("create Redis client: %v", err)
	}
	t.Cleanup(cleanup)
	first, err := New(config, client)
	if err != nil {
		t.Fatalf("New(first) error = %v", err)
	}
	second, err := New(config, client)
	if err != nil {
		t.Fatalf("New(second) error = %v", err)
	}
	return first, second, server
}

// solveChallenge follows cap-widget 0.1.57's basic {c,s,d} derivation.
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
	challenge, err := first.CreateChallenge(ctx, nil)
	if err != nil {
		t.Fatalf("CreateChallenge() error = %v", err)
	}
	solutions := solveChallenge(challenge.Token, challenge.Challenge)
	redeemed, err := second.RedeemChallenge(ctx, challenge.Token, solutions)
	if err != nil {
		t.Fatalf("RedeemChallenge() error = %v", err)
	}
	if !redeemed.Success || redeemed.Token == "" {
		t.Fatalf("RedeemChallenge() = %#v", redeemed)
	}
	replayed, err := first.RedeemChallenge(ctx, challenge.Token, solutions)
	if err != nil {
		t.Fatalf("replayed RedeemChallenge() error = %v", err)
	}
	if replayed.Success {
		t.Fatal("replayed challenge produced a second verification token")
	}
	valid, err := first.ValidateToken(ctx, redeemed.Token)
	if err != nil || !valid {
		t.Fatalf("ValidateToken() = %v, %v", valid, err)
	}
	valid, err = second.ValidateToken(ctx, redeemed.Token)
	if err != nil || valid {
		t.Fatalf("replayed ValidateToken() = %v, %v", valid, err)
	}
}

func TestInvalidSolutionDoesNotConsumeChallenge(t *testing.T) {
	captcha, _, _ := newTestCAP(t)
	challenge, err := captcha.CreateChallenge(t.Context(), nil)
	if err != nil {
		t.Fatalf("CreateChallenge() error = %v", err)
	}
	solutions := solveChallenge(challenge.Token, challenge.Challenge)
	wrong := append([]int(nil), solutions...)
	salt := prng(challenge.Token+"1", challenge.Challenge.S)
	target := prng(challenge.Token+"1d", challenge.Challenge.D)
	wrong[0] = 0
	for strings.HasPrefix(hashSHA256(salt+strconv.Itoa(wrong[0])), target) {
		wrong[0]++
	}
	failed, err := captcha.RedeemChallenge(t.Context(), challenge.Token, wrong)
	if err != nil {
		t.Fatalf("wrong RedeemChallenge() error = %v", err)
	}
	if failed.Success {
		t.Fatal("wrong solution succeeded")
	}
	retried, err := captcha.RedeemChallenge(t.Context(), challenge.Token, solutions)
	if err != nil || !retried.Success {
		t.Fatalf("retry RedeemChallenge() = %#v, %v", retried, err)
	}
}

func TestConcurrentRedeemIssuesOneToken(t *testing.T) {
	first, second, _ := newTestCAP(t)
	challenge, err := first.CreateChallenge(t.Context(), nil)
	if err != nil {
		t.Fatalf("CreateChallenge() error = %v", err)
	}
	solutions := solveChallenge(challenge.Token, challenge.Challenge)
	type result struct {
		success bool
		err     error
	}
	results := make(chan result, 8)
	for index := range 8 {
		captcha := first
		if index%2 == 1 {
			captcha = second
		}
		go func() {
			response, redeemErr := captcha.RedeemChallenge(t.Context(), challenge.Token, solutions)
			results <- result{success: response != nil && response.Success, err: redeemErr}
		}()
	}
	successes := 0
	for range 8 {
		result := <-results
		if result.err != nil {
			t.Fatalf("RedeemChallenge() error = %v", result.err)
		}
		if result.success {
			successes++
		}
	}
	if successes != 1 {
		t.Fatalf("successful redeems = %d, want 1", successes)
	}
}

func TestScopedChallengeAndVerification(t *testing.T) {
	captcha, _, _ := newTestCAP(t)
	challenge, err := captcha.CreateChallenge(t.Context(), &ChallengeConfig{Scope: "signup"})
	if err != nil {
		t.Fatalf("CreateChallenge() error = %v", err)
	}
	solutions := solveChallenge(challenge.Token, challenge.Challenge)
	wrongScope, err := captcha.RedeemChallengeWithScope(t.Context(), challenge.Token, solutions, "password-reset")
	if err != nil {
		t.Fatalf("wrong-scope redeem error = %v", err)
	}
	if wrongScope.Success {
		t.Fatal("wrong scope redeemed challenge")
	}
	redeemed, err := captcha.RedeemChallengeWithScope(t.Context(), challenge.Token, solutions, "signup")
	if err != nil || !redeemed.Success || redeemed.Scope != "signup" {
		t.Fatalf("scoped redeem = %#v, %v", redeemed, err)
	}
	valid, err := captcha.ValidateTokenWithScope(t.Context(), redeemed.Token, "password-reset")
	if err != nil || valid {
		t.Fatalf("wrong-scope validation = %v, %v", valid, err)
	}
	valid, err = captcha.ValidateTokenWithScope(t.Context(), redeemed.Token, "signup")
	if err != nil || !valid {
		t.Fatalf("correct-scope validation = %v, %v", valid, err)
	}
	valid, err = captcha.ValidateTokenWithScope(t.Context(), redeemed.Token, "signup")
	if err != nil || valid {
		t.Fatalf("replayed scoped validation = %v, %v", valid, err)
	}
}

func TestIssueScriptDoesNotPartiallyConsumeNonce(t *testing.T) {
	captcha, _, _ := newTestCAP(t)
	nonceKey := captcha.keyPrefix + "nonce:fixed"
	tokenKey := captcha.keyPrefix + "token:fixed"
	if err := captcha.rdb.Set(t.Context(), tokenKey, "existing", time.Minute).Err(); err != nil {
		t.Fatalf("seed token key: %v", err)
	}
	result, err := issueVerificationScript.Run(
		t.Context(), captcha.rdb, []string{nonceKey, tokenKey}, 60_000, 60_000, "signup",
	).Int64()
	if err != nil {
		t.Fatalf("issue script error = %v", err)
	}
	if result != -1 {
		t.Fatalf("issue script result = %d, want -1", result)
	}
	exists, err := captcha.rdb.Exists(t.Context(), nonceKey).Result()
	if err != nil || exists != 0 {
		t.Fatalf("nonce exists after token collision = %d, %v", exists, err)
	}
}

func TestRedisFailureClosesRedeemAndValidation(t *testing.T) {
	captcha, _, server := newTestCAP(t)
	challenge, err := captcha.CreateChallenge(t.Context(), nil)
	if err != nil {
		t.Fatalf("CreateChallenge() error = %v", err)
	}
	solutions := solveChallenge(challenge.Token, challenge.Challenge)
	server.Close()
	if response, redeemErr := captcha.RedeemChallenge(t.Context(), challenge.Token, solutions); redeemErr == nil || response != nil {
		t.Fatalf("RedeemChallenge() = %#v, %v after Redis failure", response, redeemErr)
	}
	validToken := "0000000000000000:000000000000000000000000000000"
	if valid, validateErr := captcha.ValidateToken(t.Context(), validToken); validateErr == nil || valid {
		t.Fatalf("ValidateToken() = %v, %v after Redis failure", valid, validateErr)
	}
}

func TestCapRejectsAmbiguousJSONBodies(t *testing.T) {
	captcha, _, _ := newTestCAP(t)
	server := khttp.NewServer()
	Register(server, captcha)

	tests := []struct {
		name string
		body []byte
	}{
		{name: "duplicate member", body: []byte(`{"token":"x","token":"y","solutions":[]}`)},
		{name: "multiple values", body: []byte(`{"token":"x","solutions":[]} {"token":"y"}`)},
		{name: "invalid utf8", body: []byte{'{', '"', 't', 'o', 'k', 'e', 'n', '"', ':', '"', 0xff, '"', ',', '"', 's', 'o', 'l', 'u', 't', 'i', 'o', 'n', 's', '"', ':', '[', ']', '}'}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPost, "/cap/redeem", bytes.NewReader(test.body))
			request.Header.Set("Content-Type", "application/json")
			recorder := httptest.NewRecorder()
			server.ServeHTTP(recorder, request)
			if recorder.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
			}
		})
	}
}

func TestCapWidgetV0157HTTPInterop(t *testing.T) {
	captcha, _, _ := newTestCAP(t)
	server := khttp.NewServer()
	Register(server, captcha)

	challengeRequest := httptest.NewRequest(http.MethodPost, "/cap/challenge", nil)
	challengeRecorder := httptest.NewRecorder()
	server.ServeHTTP(challengeRecorder, challengeRequest)
	if challengeRecorder.Code != http.StatusOK {
		t.Fatalf("challenge status = %d, body = %s", challengeRecorder.Code, challengeRecorder.Body.String())
	}
	var challenge ChallengeResponse
	if err := json.Unmarshal(challengeRecorder.Body.Bytes(), &challenge); err != nil {
		t.Fatalf("decode challenge: %v", err)
	}
	if challenge.Challenge != (ChallengeParams{C: 2, S: 8, D: 1}) || len(strings.Split(challenge.Token, ".")) != 3 {
		t.Fatalf("challenge response = %#v", challenge)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(challengeRecorder.Body.Bytes(), &raw); err != nil {
		t.Fatalf("decode raw challenge: %v", err)
	}
	for _, unsupported := range []string{"instrumentation", "format", "challenges"} {
		if _, exists := raw[unsupported]; exists {
			t.Fatalf("challenge unexpectedly contains %q", unsupported)
		}
	}

	solutions := solveChallenge(challenge.Token, challenge.Challenge)
	unsupportedBody, err := json.Marshal(map[string]any{
		"token": challenge.Token, "solutions": solutions, "instr": map[string]any{"value": true},
	})
	if err != nil {
		t.Fatalf("marshal unsupported redeem: %v", err)
	}
	unsupportedRequest := httptest.NewRequest(http.MethodPost, "/cap/redeem", bytes.NewReader(unsupportedBody))
	unsupportedRequest.Header.Set("Content-Type", "application/json")
	unsupportedRecorder := httptest.NewRecorder()
	server.ServeHTTP(unsupportedRecorder, unsupportedRequest)
	if unsupportedRecorder.Code != http.StatusBadRequest {
		t.Fatalf("unsupported redeem status = %d", unsupportedRecorder.Code)
	}

	redeemBody, err := json.Marshal(map[string]any{"token": challenge.Token, "solutions": solutions})
	if err != nil {
		t.Fatalf("marshal redeem: %v", err)
	}
	redeemRequest := httptest.NewRequest(http.MethodPost, "/cap/redeem", bytes.NewReader(redeemBody))
	redeemRequest.Header.Set("Content-Type", "application/json")
	redeemRecorder := httptest.NewRecorder()
	server.ServeHTTP(redeemRecorder, redeemRequest)
	if redeemRecorder.Code != http.StatusOK {
		t.Fatalf("redeem status = %d, body = %s", redeemRecorder.Code, redeemRecorder.Body.String())
	}
	var redeemed RedeemResponse
	if err := json.Unmarshal(redeemRecorder.Body.Bytes(), &redeemed); err != nil {
		t.Fatalf("decode redeem: %v", err)
	}
	if !redeemed.Success || redeemed.Token == "" || redeemed.Expires <= time.Now().UnixMilli() {
		t.Fatalf("redeem response = %#v", redeemed)
	}
	valid, err := captcha.ValidateToken(t.Context(), redeemed.Token)
	if err != nil || !valid {
		t.Fatalf("ValidateToken() = %v, %v", valid, err)
	}

	legacyRequest := httptest.NewRequest(http.MethodPost, "/v1/cap/challenge", nil)
	legacyRecorder := httptest.NewRecorder()
	server.ServeHTTP(legacyRecorder, legacyRequest)
	if legacyRecorder.Code != http.StatusNotFound {
		t.Fatalf("legacy route status = %d", legacyRecorder.Code)
	}
}
