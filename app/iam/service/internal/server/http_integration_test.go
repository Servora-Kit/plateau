package server

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	userpb "github.com/Servora-Kit/plateau/api/gen/go/iam/user/v1"
	iamauthn "github.com/Servora-Kit/plateau/app/iam/service/internal/authn"
	"github.com/Servora-Kit/plateau/app/iam/service/internal/biz"
	"github.com/alexedwards/scs/v2"
)

func TestHTTPSessionLifecycle(t *testing.T) {
	f := newServerFixture(t)
	bad := f.request("POST", "/v1/iam/authn/login", `{"email":"alice@example.com","password":"incorrect"}`, nil)
	if bad.Code != 401 || len(bad.Result().Cookies()) != 0 || f.ent.IAMLoginSession.Query().CountX(t.Context()) != 0 {
		t.Fatalf("failed login created identity: %d %s", bad.Code, bad.Body)
	}
	cookie := f.login(t, testPassword, nil)
	if !cookie.Secure || !cookie.HttpOnly || cookie.Domain != "" || cookie.Path != "/" || cookie.SameSite != http.SameSiteLaxMode {
		t.Fatalf("unexpected cookie attributes: %+v", cookie)
	}
	ctx, err := f.manager.Load(t.Context(), cookie.Value)
	check(t, err)
	loginID := f.manager.GetString(ctx, iamauthn.LoginReferenceKey)
	login, err := f.logins.Find(t.Context(), loginID)
	check(t, err)
	deadline := f.manager.Deadline(ctx)
	other := f.login(t, testPassword, nil)
	for _, origin := range []string{"", "https://different.example"} {
		r := httptest.NewRequest("GET", f.web.URL+"/v1/iam/account/profile", nil)
		r.AddCookie(cookie)
		r.Header.Set("Origin", origin)
		w := httptest.NewRecorder()
		f.http.ServeHTTP(w, r)
		if w.Code != 200 {
			t.Fatalf("profile origin=%q: %d %s", origin, w.Code, w.Body)
		}
	}
	changed := f.request("POST", "/v1/iam/account/change-password", `{"currentPassword":"`+testPassword+`","newPassword":"new correct horse battery staple"}`, cookie)
	if changed.Code != 200 {
		t.Fatalf("change password: %d %s", changed.Code, changed.Body)
	}
	rotated := changed.Result().Cookies()[0]
	if rotated.Value == cookie.Value {
		t.Fatal("password change did not rotate SCS token")
	}
	newCtx, err := f.manager.Load(t.Context(), rotated.Value)
	check(t, err)
	if !f.manager.Deadline(newCtx).Equal(deadline) {
		t.Fatal("password change extended absolute deadline")
	}
	current, err := f.logins.Find(t.Context(), loginID)
	check(t, err)
	if !current.AuthTime.Equal(login.AuthTime) || current.RevokedAt != nil {
		t.Fatal("password change changed current authentication facts")
	}
	if w := f.request("GET", "/v1/iam/account/profile", "", other); w.Code != 401 {
		t.Fatalf("other login survived password change: %d", w.Code)
	}
	if w := f.request("GET", "/v1/iam/account/profile", "", rotated); w.Code != 200 {
		t.Fatalf("current login failed: %d", w.Code)
	}
	logout := f.request("POST", "/v1/iam/sessions/current:logout", "{}", rotated)
	if logout.Code != 200 {
		t.Fatalf("logout: %d %s", logout.Code, logout.Body)
	}
	cleared := logout.Result().Cookies()
	if len(cleared) != 1 || cleared[0].Value != "" || cleared[0].MaxAge >= 0 {
		t.Fatalf("logout cookie not cleared: %v", cleared)
	}
	// A previously loaded request may save again after logout; the reference stays revoked.
	f.manager.Put(newCtx, "late_request", true)
	lateToken, _, err := f.manager.Commit(newCtx)
	check(t, err)
	if w := f.request("GET", "/v1/iam/account/profile", "", &http.Cookie{Name: cookie.Name, Value: lateToken}); w.Code != 401 {
		t.Fatalf("late SCS save revived identity: %d", w.Code)
	}
	// Login itself also rotates an existing session token.
	another := f.login(t, "new correct horse battery staple", nil)
	renewed := f.login(t, "new correct horse battery staple", another)
	if renewed.Value == another.Value {
		t.Fatal("login reused pre-authentication token")
	}
}

type failingStore struct {
	scs.Store
	reads          int
	commit, remove bool
}

func (s *failingStore) Find(token string) ([]byte, bool, error) {
	s.reads++
	return s.Store.Find(token)
}
func (s *failingStore) Commit(token string, data []byte, expiry time.Time) error {
	if s.commit {
		return errors.New("injected session save failure")
	}
	return s.Store.Commit(token, data, expiry)
}
func (s *failingStore) Delete(token string) error {
	if s.remove {
		return errors.New("injected session delete failure")
	}
	return s.Store.Delete(token)
}

func TestHTTPStoreFailureAfterBusinessCommit(t *testing.T) {
	for _, operation := range []string{"login", "password", "logout"} {
		t.Run(operation, func(t *testing.T) {
			f := newServerFixture(t)
			cookie := f.login(t, testPassword, nil)
			ctx, err := f.manager.Load(t.Context(), cookie.Value)
			check(t, err)
			loginID := f.manager.GetString(ctx, iamauthn.LoginReferenceKey)
			other := f.login(t, testPassword, nil)
			store := &failingStore{Store: f.manager.Store, commit: operation != "logout", remove: operation == "logout"}
			f.manager.Store = store
			var w *httptest.ResponseRecorder
			switch operation {
			case "login":
				w = f.request("POST", "/v1/iam/authn/login", `{"email":"alice@example.com","password":"`+testPassword+`"}`, nil)
			case "password":
				w = f.request("POST", "/v1/iam/account/change-password", `{"currentPassword":"`+testPassword+`","newPassword":"new correct horse battery staple"}`, cookie)
			case "logout":
				w = f.request("POST", "/v1/iam/sessions/current:logout", "{}", cookie)
			}
			if w.Code != 500 || (operation != "logout" && len(w.Result().Cookies()) != 0) || strings.Contains(w.Body.String(), `"user"`) || strings.HasSuffix(strings.TrimSpace(w.Body.String()), "{}") {
				t.Fatalf("store failure delivered success: %d %s cookie=%v", w.Code, w.Body, w.Result().Cookies())
			}
			for _, returned := range w.Result().Cookies() {
				if returned.Value != cookie.Value {
					t.Fatal("failed logout delivered a new session token")
				}
			}
			store.commit, store.remove = false, false
			if operation == "password" {
				f.login(t, "new correct horse battery staple", nil)
				if w := f.request("GET", "/v1/iam/account/profile", "", other); w.Code != 401 {
					t.Fatal("committed password revocation was rolled back")
				}
			}
			if operation == "logout" {
				login, err := f.logins.Find(t.Context(), loginID)
				check(t, err)
				if login.RevokedAt == nil {
					t.Fatal("committed logout was rolled back")
				}
				if w := f.request("GET", "/v1/iam/account/profile", "", cookie); w.Code != 401 {
					t.Fatal("failed cookie deletion restored login")
				}
			}
		})
	}
}

func TestHTTPResetAndDisableInvalidateAllLogins(t *testing.T) {
	for _, operation := range []string{"reset", "disable"} {
		t.Run(operation, func(t *testing.T) {
			f := newServerFixture(t)
			cookies := []*http.Cookie{f.login(t, testPassword, nil), f.login(t, testPassword, nil)}
			if operation == "reset" {
				check(t, f.resets.Create(t.Context(), f.user.GetUserId(), biz.HashOpaqueSecret("reset-proof"), time.Now().Add(time.Hour)))
				body := `{"token":"reset-proof","newPassword":"new correct horse battery staple"}`
				w := f.request("POST", "/v1/iam/account/password-reset/confirm", body, nil)
				if w.Code != 200 {
					t.Fatalf("reset: %d %s", w.Code, w.Body)
				}
				if replay := f.request("POST", "/v1/iam/account/password-reset/confirm", body, nil); replay.Code == 200 {
					t.Fatal("reset token was reused")
				}
				f.login(t, "new correct horse battery staple", nil)
			} else {
				_, err := f.users.UpdateStatus(t.Context(), f.user.GetUserId(), f.user.GetEtag(), userpb.UserStatus_USER_STATUS_DISABLED, time.Now())
				check(t, err)
			}
			for _, cookie := range cookies {
				if w := f.request("GET", "/v1/iam/account/profile", "", cookie); w.Code != 401 {
					t.Fatalf("%s left login active: %d", operation, w.Code)
				}
			}
		})
	}
}

func TestHTTPBrowserFilterScopeAndWriteGuards(t *testing.T) {
	f := newServerFixture(t)
	cookie := f.login(t, testPassword, nil)
	store := &failingStore{Store: f.manager.Store}
	f.manager.Store = store

	// 1. Real f.http rejects non-JSON and cross-origin writes on /v1/iam/* BEFORE touching session store or DB.
	for _, targetPath := range []string{
		"/v1/iam/authn/login",
		"/v1/iam/account/change-initial-password",
		"/v1/iam/account/delete",
		"/v1/iam/sessions/current:logout",
	} {
		store.reads = 0
		r := httptest.NewRequest(http.MethodPost, f.web.URL+targetPath, strings.NewReader("currentPassword="+testPassword))
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		r.AddCookie(cookie)
		w := httptest.NewRecorder()
		f.http.ServeHTTP(w, r)
		if w.Code != http.StatusUnsupportedMediaType || store.reads != 0 || len(w.Result().Cookies()) != 0 {
			t.Fatalf("%s form POST: status=%d reads=%d cookies=%v", targetPath, w.Code, store.reads, w.Result().Cookies())
		}

		r = httptest.NewRequest(http.MethodPost, f.web.URL+targetPath, strings.NewReader(`{"currentPassword":"`+testPassword+`","email":"alice@example.com","password":"`+testPassword+`"}`))
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("Origin", "https://evil.example")
		r.AddCookie(cookie)
		w = httptest.NewRecorder()
		f.http.ServeHTTP(w, r)
		if w.Code != http.StatusForbidden || store.reads != 0 || len(w.Result().Cookies()) != 0 {
			t.Fatalf("%s cross-origin POST: status=%d reads=%d cookies=%v", targetPath, w.Code, store.reads, w.Result().Cookies())
		}
	}
	if w := f.request("GET", "/v1/iam/account/profile", "", cookie); w.Code != http.StatusOK {
		t.Fatalf("rejected cross-origin writes mutated active session or account: %d %s", w.Code, w.Body)
	}

	// 2. /authorize/callback rejects cross-origin POST before session load, and rejects same-origin POST with 405.
	store.reads = 0
	r := httptest.NewRequest(http.MethodPost, f.web.URL+"/authorize/callback?id=req-1", strings.NewReader("id=req-1"))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.Header.Set("Origin", "https://evil.example")
	r.AddCookie(cookie)
	w := httptest.NewRecorder()
	f.http.ServeHTTP(w, r)
	if w.Code != http.StatusForbidden || store.reads != 0 {
		t.Fatalf("cross-origin callback POST: status=%d reads=%d", w.Code, store.reads)
	}

	r = httptest.NewRequest(http.MethodPost, f.web.URL+"/authorize/callback?id=req-1", strings.NewReader("id=req-1"))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.Header.Set("Origin", f.web.URL)
	r.AddCookie(cookie)
	w = httptest.NewRecorder()
	f.http.ServeHTTP(w, r)
	if w.Code != http.StatusMethodNotAllowed || w.Header().Get("Allow") != http.MethodGet || store.reads != 1 {
		t.Fatalf("same-origin callback POST: status=%d allow=%q reads=%d", w.Code, w.Header().Get("Allow"), store.reads)
	}

	// 3. Session routes load session on real f.http; non-session protocol/challenge routes never touch SCS store.
	for _, path := range []string{"/v1/iam/account/profile", "/authorize", "/authorize/callback", "/end_session"} {
		store.reads = 0
		req := httptest.NewRequest(http.MethodGet, f.web.URL+path, nil)
		req.AddCookie(cookie)
		rec := httptest.NewRecorder()
		f.http.ServeHTTP(rec, req)
		if store.reads != 1 {
			t.Fatalf("session route %s did not load session store: reads=%d status=%d", path, store.reads, rec.Code)
		}
	}
	for _, path := range []string{"/oauth/token", "/userinfo", "/oauth/introspect", "/revoke", "/.well-known/openid-configuration", "/keys", "/cap/challenge"} {
		store.reads = 0
		req := httptest.NewRequest(http.MethodGet, f.web.URL+path, nil)
		req.AddCookie(cookie)
		rec := httptest.NewRecorder()
		f.http.ServeHTTP(rec, req)
		if store.reads != 0 || len(rec.Result().Cookies()) != 0 {
			t.Fatalf("non-session route %s touched browser session: reads=%d cookies=%v", path, store.reads, rec.Result().Cookies())
		}
	}
	for _, path := range []string{"/v1/iam/users", "/v1/iam/users/" + f.user.GetUserId()} {
		if w := f.request("GET", path, "", cookie); w.Code != http.StatusNotFound {
			t.Fatalf("UserService HTTP still exposed: %s %d", path, w.Code)
		}
	}
}
