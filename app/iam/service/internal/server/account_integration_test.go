package server

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/url"
	"testing"
	"time"
)

// Represents a proof already solved by the CAP widget. The real Redis consumer
// must accept it only once; CAP's separate protocol tests verify the proof itself.
func (f *serverFixture) capProof(t *testing.T) string {
	t.Helper()
	raw := make([]byte, 23)
	_, err := rand.Read(raw)
	check(t, err)
	id, secret := hex.EncodeToString(raw[:8]), hex.EncodeToString(raw[8:])
	digest := sha256.Sum256([]byte(secret))
	key := f.capPrefix + "token:" + id + ":" + hex.EncodeToString(digest[:])
	check(t, f.redis.Set(t.Context(), key, "", time.Minute).Err())
	t.Cleanup(func() { _ = f.redis.Del(t.Context(), key).Err() })
	return id + ":" + secret
}

func TestPublicAccountCAPAndOneTimeProofs(t *testing.T) {
	f := newServerFixture(t)
	body := func(proof string) string {
		return fmt.Sprintf(`{"email":"new@example.com","password":"correct horse battery staple","capToken":%q}`, proof)
	}
	if w := f.request("POST", "/v1/iam/account/register", body("invalid"), nil); w.Code == 200 {
		t.Fatal("invalid CAP proof accepted")
	}
	proof := f.capProof(t)
	w := f.request("POST", "/v1/iam/account/register", body(proof), nil)
	if w.Code != 200 || len(f.mailer.verification) != 1 || len(w.Result().Cookies()) != 0 {
		t.Fatalf("registration: %d %s", w.Code, w.Body)
	}
	if replay := f.request("POST", "/v1/iam/account/register", body(proof), nil); replay.Code == 200 {
		t.Fatal("CAP proof was reused")
	}
	if login := f.request("POST", "/v1/iam/authn/login", `{"email":"new@example.com","password":"correct horse battery staple"}`, nil); login.Code != 401 {
		t.Fatal("pending user could log in")
	}
	link, err := url.Parse(f.mailer.verification[0])
	check(t, err)
	verify := fmt.Sprintf(`{"token":%q}`, link.Fragment)
	if w := f.request("POST", "/v1/iam/account/verify-email", verify, nil); w.Code != 200 {
		t.Fatalf("verify: %d %s", w.Code, w.Body)
	}
	if w := f.request("POST", "/v1/iam/account/verify-email", verify, nil); w.Code == 200 {
		t.Fatal("email proof was reused")
	}
	if w := f.request("POST", "/v1/iam/authn/login", `{"email":"new@example.com","password":"correct horse battery staple"}`, nil); w.Code != 200 {
		t.Fatalf("verified user cannot log in: %d %s", w.Code, w.Body)
	}
	var responseBody string
	for _, email := range []string{"new@example.com", "unknown@example.com"} {
		request := fmt.Sprintf(`{"email":%q,"capToken":%q}`, email, f.capProof(t))
		w := f.request("POST", "/v1/iam/account/password-reset/request", request, nil)
		if w.Code != 200 {
			t.Fatalf("reset request: %d %s", w.Code, w.Body)
		}
		if responseBody != "" && responseBody != w.Body.String() {
			t.Fatal("password reset disclosed account existence")
		}
		responseBody = w.Body.String()
	}
	if len(f.mailer.reset) != 1 {
		t.Fatal("reset mail sent for unknown user")
	}
}
