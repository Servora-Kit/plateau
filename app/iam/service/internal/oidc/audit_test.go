package oidc

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"

	"github.com/Servora-Kit/plateau/security/audit"
	httpsession "github.com/Servora-Kit/plateau/security/session"
	cloudevents "github.com/cloudevents/sdk-go/v2"
	khttp "github.com/go-kratos/kratos/v3/transport/http"
)

type captureAuditor struct {
	mu     sync.Mutex
	events []cloudevents.Event
}

func (c *captureAuditor) Emit(_ context.Context, ev cloudevents.Event) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.events = append(c.events, ev.Clone())
	return nil
}

func (c *captureAuditor) takeAll() []cloudevents.Event {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]cloudevents.Event, len(c.events))
	for i, ev := range c.events {
		out[i] = ev.Clone()
	}
	c.events = nil
	return out
}

func TestOIDCNonIntrusiveHTTPHandlerAudit(t *testing.T) {
	fixture := newProviderFixture(t)
	auditor := &captureAuditor{}

	srv := khttp.NewServer(khttp.Filter(func(next http.Handler) http.Handler {
		return httpsession.LoadAndSave(fixture.manager)(next)
	}))
	RegisterHTTPServer(srv, fixture.provider, auditor)

	doReq := func(method, target, body string, headers map[string]string, cookies ...*http.Cookie) *httptest.ResponseRecorder {
		var r *http.Request
		if body != "" {
			r = httptest.NewRequest(method, target, strings.NewReader(body))
		} else {
			r = httptest.NewRequest(method, target, nil)
		}
		r.RemoteAddr = "198.51.100.42:54321"
		r.Header.Set("User-Agent", "OIDCAuditTest/1.0")
		for k, v := range headers {
			r.Header.Set(k, v)
		}
		for _, ck := range cookies {
			r.AddCookie(ck)
		}
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, r)
		return w
	}

	// 1. Discovery and JWKS must not emit audit noise.
	if w := doReq(http.MethodGet, testIssuer+"/.well-known/openid-configuration", "", nil); w.Code != http.StatusOK {
		t.Fatalf("discovery status = %d", w.Code)
	}
	if w := doReq(http.MethodGet, testIssuer+"/keys", "", nil); w.Code != http.StatusOK {
		t.Fatalf("keys status = %d", w.Code)
	}
	if got := auditor.takeAll(); len(got) != 0 {
		t.Fatalf("expected 0 audit events for discovery/keys, got %d", len(got))
	}

	// Prepare PKCE authorization request.
	verifier := "pkce-secret-verifier-must-never-leak-0123456789abcdef"
	digest := sha256.Sum256([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(digest[:])
	authValues := url.Values{
		"client_id":             {testClientID},
		"redirect_uri":          {testRedirectURI},
		"response_type":         {"code"},
		"scope":                 {"openid profile email"},
		"state":                 {"state-audit-1"},
		"nonce":                 {"nonce-audit-1"},
		"code_challenge":        {challenge},
		"code_challenge_method": {"S256"},
	}
	authResp := doReq(http.MethodGet, testIssuer+"/authorize?"+authValues.Encode(), "", nil)
	if authResp.Code != http.StatusFound {
		t.Fatalf("authorize status = %d body = %s", authResp.Code, authResp.Body.String())
	}
	cbLoc, err := url.Parse(authResp.Header().Get("Location"))
	if err != nil {
		t.Fatal(err)
	}
	reqID := cbLoc.Query().Get("id")
	if reqID == "" {
		t.Fatalf("missing auth request id in location %q", authResp.Header().Get("Location"))
	}
	_ = auditor.takeAll()

	// 2. Unauthenticated /authorize/callback -> 302 /login must be skipped (zero noise).
	unauthCB := doReq(http.MethodGet, testIssuer+"/authorize/callback?id="+url.QueryEscape(reqID), "", nil)
	if unauthCB.Code != http.StatusFound || !strings.HasPrefix(unauthCB.Header().Get("Location"), "/login?") {
		t.Fatalf("unauth callback status = %d loc = %q", unauthCB.Code, unauthCB.Header().Get("Location"))
	}
	if got := auditor.takeAll(); len(got) != 0 {
		t.Fatalf("expected unauthenticated 302 /login redirect to be skipped, got %d events", len(got))
	}

	// 3. Invalid method and missing request id on /authorize/callback must emit failure events.
	if w := doReq(http.MethodPost, testIssuer+"/authorize/callback", "", nil); w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("POST callback status = %d, want 405", w.Code)
	}
	if w := doReq(http.MethodGet, testIssuer+"/authorize/callback", "", nil); w.Code != http.StatusBadRequest {
		t.Fatalf("missing id callback status = %d, want 400", w.Code)
	}
	failCBEvents := auditor.takeAll()
	if len(failCBEvents) != 2 {
		t.Fatalf("expected 2 failure events for invalid callback calls, got %d", len(failCBEvents))
	}
	for _, ev := range failCBEvents {
		if ev.Type() != "plateau.iam.oidc.authorize.v1" || ev.Extensions()[audit.ExtOutcome] != audit.OutcomeFailure || ev.Extensions()[audit.ExtActorType] != "anonymous" {
			t.Fatalf("unexpected failed callback event: %+v", ev)
		}
	}

	// 4. Authenticated /authorize/callback -> emits success with human actor, user resource, client_id & scopes, and zero code/cookie leakage.
	sessionCookie := &http.Cookie{Name: fixture.manager.Cookie.Name, Value: fixture.sessionSecret}
	okCB := doReq(http.MethodGet, testIssuer+"/authorize/callback?id="+url.QueryEscape(reqID), "", nil, sessionCookie)
	if okCB.Code != http.StatusFound {
		t.Fatalf("authenticated callback status = %d body = %s", okCB.Code, okCB.Body.String())
	}
	rpLoc, err := url.Parse(okCB.Header().Get("Location"))
	if err != nil {
		t.Fatal(err)
	}
	authCode := rpLoc.Query().Get("code")
	if authCode == "" {
		t.Fatalf("expected authorization code in redirect %q", okCB.Header().Get("Location"))
	}

	cbEvents := auditor.takeAll()
	if len(cbEvents) != 1 {
		t.Fatalf("expected 1 authorize callback event, got %d", len(cbEvents))
	}
	cbEv := cbEvents[0]
	if cbEv.Type() != "plateau.iam.oidc.authorize.v1" || cbEv.Subject() != "GET /authorize/callback" {
		t.Fatalf("unexpected authorize event type/subject: type=%q subject=%q", cbEv.Type(), cbEv.Subject())
	}
	if cbEv.Extensions()[audit.ExtOutcome] != audit.OutcomeSuccess ||
		cbEv.Extensions()[audit.ExtActorType] != "human" ||
		cbEv.Extensions()[audit.ExtActorID] != fixture.userID ||
		cbEv.Extensions()[audit.ExtResourceType] != "iam.plateau.dev/User" ||
		cbEv.Extensions()[audit.ExtResourceName] != "users/"+fixture.userID {
		t.Fatalf("unexpected authorize event extensions: %+v", cbEv.Extensions())
	}
	var cbDetail map[string]any
	if err := json.Unmarshal(cbEv.Data(), &cbDetail); err != nil {
		t.Fatalf("unmarshal authorize detail: %v", err)
	}
	if cbDetail["client_id"] != testClientID {
		t.Fatalf("authorize detail client_id = %v, want %q", cbDetail["client_id"], testClientID)
	}
	rawCBJSON, _ := json.Marshal(cbEv)
	for _, secret := range []string{authCode, fixture.sessionSecret, verifier} {
		if strings.Contains(string(rawCBJSON), secret) {
			t.Fatalf("sensitive value %q leaked into authorize callback event: %s", secret, string(rawCBJSON))
		}
	}

	// 5. Successful POST /oauth/token must emit 0 audit events (WithHTTPOnlyErrors).
	basicHeader := "Basic " + base64.StdEncoding.EncodeToString([]byte(url.QueryEscape(testClientID)+":"+url.QueryEscape(testClientSecret)))
	tokenForm := url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {authCode},
		"redirect_uri":  {testRedirectURI},
		"code_verifier": {verifier},
	}
	tokenResp := doReq(http.MethodPost, testIssuer+"/oauth/token", tokenForm.Encode(), map[string]string{
		"Content-Type":  "application/x-www-form-urlencoded",
		"Authorization": basicHeader,
	})
	tokens := decodeTokenResponse(t, tokenResp)
	if got := auditor.takeAll(); len(got) != 0 {
		t.Fatalf("expected 0 audit events for successful /oauth/token, got %d", len(got))
	}

	// 6. Failed POST /oauth/token (BasicAuth + PostForm) must record ONLY grant_type & client_id, never secrets/codes/tokens.
	const (
		wrongBasicSecret = "{{SECRET_0001badsecret}}"
		leakedCode       = "{{SECRET_0002leakedcode}}"
		leakedRefresh    = "{{SECRET_0003leakedrefresh}}"
		leakedVerifier   = "{{SECRET_0004leakedverifier}}"
		formSecret       = "{{SECRET_0005formsecret}}"
	)
	badBasicHeader := "Basic " + base64.StdEncoding.EncodeToString([]byte(url.QueryEscape(testClientID)+":"+url.QueryEscape(wrongBasicSecret)))
	badTokenBody := url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {leakedCode},
		"refresh_token": {leakedRefresh},
		"code_verifier": {leakedVerifier},
		"redirect_uri":  {testRedirectURI},
	}
	badBasicResp := doReq(http.MethodPost, testIssuer+"/oauth/token", badTokenBody.Encode(), map[string]string{
		"Content-Type":  "application/x-www-form-urlencoded",
		"Authorization": badBasicHeader,
	})
	if badBasicResp.Code < http.StatusBadRequest {
		t.Fatalf("expected failed token status >= 400, got %d", badBasicResp.Code)
	}

	badFormBody := url.Values{
		"grant_type":    {"refresh_token"},
		"client_id":     {"form-client-app"},
		"client_secret": {formSecret},
		"refresh_token": {leakedRefresh},
	}
	badFormResp := doReq(http.MethodPost, testIssuer+"/oauth/token", badFormBody.Encode(), map[string]string{
		"Content-Type": "application/x-www-form-urlencoded",
	})
	if badFormResp.Code < http.StatusBadRequest {
		t.Fatalf("expected failed form token status >= 400, got %d", badFormResp.Code)
	}

	tokenFailEvents := auditor.takeAll()
	if len(tokenFailEvents) != 2 {
		t.Fatalf("expected 2 failed /oauth/token audit events, got %d", len(tokenFailEvents))
	}
	for i, wantClient := range []struct {
		clientID  string
		grantType string
	}{
		{clientID: testClientID, grantType: "authorization_code"},
		{clientID: "form-client-app", grantType: "refresh_token"},
	} {
		ev := tokenFailEvents[i]
		if ev.Type() != "plateau.iam.oidc.token.v1" || ev.Subject() != "POST /oauth/token" || ev.Extensions()[audit.ExtOutcome] != audit.OutcomeFailure {
			t.Fatalf("token failure event[%d] unexpected metadata: %+v", i, ev)
		}
		var detail map[string]any
		if err := json.Unmarshal(ev.Data(), &detail); err != nil {
			t.Fatalf("token failure event[%d] invalid JSON data: %v", i, err)
		}
		if len(detail) != 2 || detail["client_id"] != wantClient.clientID || detail["grant_type"] != wantClient.grantType {
			t.Fatalf("token failure event[%d] detail = %+v, want only client_id=%q grant_type=%q", i, detail, wantClient.clientID, wantClient.grantType)
		}
		serialized, _ := json.Marshal(ev)
		for _, forbidden := range []string{wrongBasicSecret, leakedCode, leakedRefresh, leakedVerifier, formSecret, testClientSecret} {
			if strings.Contains(string(serialized), forbidden) {
				t.Fatalf("sensitive token parameter %q leaked into event[%d]: %s", forbidden, i, string(serialized))
			}
		}
	}

	// 7. RP-initiated /end_session emits plateau.iam.oidc.end_session.v1 with human actor, user resource, client_id, and zero raw id_token_hint leakage.
	endResp := doReq(http.MethodGet, testIssuer+"/end_session?id_token_hint="+url.QueryEscape(tokens.IDToken), "", nil, sessionCookie)
	if endResp.Code != http.StatusFound {
		t.Fatalf("end_session status = %d body = %s", endResp.Code, endResp.Body.String())
	}
	endEvents := auditor.takeAll()
	if len(endEvents) != 1 {
		t.Fatalf("expected 1 end_session audit event, got %d", len(endEvents))
	}
	endEv := endEvents[0]
	if endEv.Type() != "plateau.iam.oidc.end_session.v1" ||
		endEv.Subject() != "GET /end_session" ||
		endEv.Extensions()[audit.ExtOutcome] != audit.OutcomeSuccess ||
		endEv.Extensions()[audit.ExtActorType] != "human" ||
		endEv.Extensions()[audit.ExtActorID] != fixture.userID ||
		endEv.Extensions()[audit.ExtResourceName] != "users/"+fixture.userID {
		t.Fatalf("unexpected end_session event: %+v", endEv)
	}
	var endDetail map[string]any
	if err := json.Unmarshal(endEv.Data(), &endDetail); err != nil || endDetail["client_id"] != testClientID {
		t.Fatalf("unexpected end_session detail: err=%v detail=%+v", err, endDetail)
	}
	rawEndJSON, _ := json.Marshal(endEv)
	if strings.Contains(string(rawEndJSON), tokens.IDToken) {
		t.Fatalf("raw id_token_hint leaked into end_session audit event!")
	}

	// 8. Authenticated /authorize/callback with unknown/expired request_id attributes failure to verified human user.
	unknownReqCB := doReq(http.MethodGet, testIssuer+"/authorize/callback?id=expired-or-unknown-req", "", nil, sessionCookie)
	if unknownReqCB.Code != http.StatusBadRequest {
		t.Fatalf("unknown request_id callback status = %d, want 400", unknownReqCB.Code)
	}
	unknownReqEvents := auditor.takeAll()
	if len(unknownReqEvents) != 1 {
		t.Fatalf("expected 1 failure event for authenticated invalid request_id, got %d", len(unknownReqEvents))
	}
	if unknownReqEvents[0].Extensions()[audit.ExtOutcome] != audit.OutcomeFailure ||
		unknownReqEvents[0].Extensions()[audit.ExtActorType] != "human" ||
		unknownReqEvents[0].Extensions()[audit.ExtActorID] != fixture.userID ||
		unknownReqEvents[0].Extensions()[audit.ExtResourceName] != "users/"+fixture.userID {
		t.Fatalf("expected verified human attribution on CompleteAuthRequest failure, got %+v", unknownReqEvents[0].Extensions())
	}

	// 9. POST /oauth/token enforces 64 KiB body cap (413) and truncates oversized detail fields.
	oversizedBody := "grant_type=client_credentials&client_id=admin&pad=" + strings.Repeat("x", 70<<10)
	tooLargeResp := doReq(http.MethodPost, testIssuer+"/oauth/token", oversizedBody, map[string]string{
		"Content-Type": "application/x-www-form-urlencoded",
	})
	if tooLargeResp.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("oversized /oauth/token status = %d, want 413", tooLargeResp.Code)
	}
	longForm := url.Values{
		"client_id":  {strings.Repeat("c", 600)},
		"grant_type": {strings.Repeat("g", 600)},
	}
	longResp := doReq(http.MethodPost, testIssuer+"/oauth/token", longForm.Encode(), map[string]string{
		"Content-Type": "application/x-www-form-urlencoded",
	})
	if longResp.Code < http.StatusBadRequest {
		t.Fatalf("long form /oauth/token status = %d, want >= 400", longResp.Code)
	}
	guardEvents := auditor.takeAll()
	if len(guardEvents) != 2 {
		t.Fatalf("expected 2 failure events for 413 and overlong POST /oauth/token, got %d", len(guardEvents))
	}
	if guardEvents[0].Extensions()[audit.ExtErrorCode] != "REQUEST_ENTITY_TOO_LARGE" {
		t.Fatalf("413 event errorcode = %v, want REQUEST_ENTITY_TOO_LARGE", guardEvents[0].Extensions()[audit.ExtErrorCode])
	}
	var longDetail map[string]any
	if err := json.Unmarshal(guardEvents[1].Data(), &longDetail); err != nil {
		t.Fatalf("unmarshal overlong /oauth/token detail: %v", err)
	}
	gotCID, _ := longDetail["client_id"].(string)
	gotGT, _ := longDetail["grant_type"].(string)
	if len(gotCID) != maxAuditDetailFieldBytes || len(gotGT) != maxAuditDetailFieldBytes {
		t.Fatalf("expected truncated detail lengths (%d, %d) == %d", len(gotCID), len(gotGT), maxAuditDetailFieldBytes)
	}

	// 10. OAuth redirect error helper marks 302 ?error=... callback redirects as OAUTH_* failures.
	if got := redirectOAuthErrorCode("https://admin.local/callback?error=server_error&error_description=db+down"); got != "OAUTH_SERVER_ERROR" {
		t.Fatalf("redirectOAuthErrorCode(error) = %q, want OAUTH_SERVER_ERROR", got)
	}
	if got := redirectOAuthErrorCode("https://admin.local/callback?code=valid-code&state=s1"); got != "" {
		t.Fatalf("redirectOAuthErrorCode(success code) = %q, want empty", got)
	}
}
