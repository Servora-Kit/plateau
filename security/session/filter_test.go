package session

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/alexedwards/scs/v2"
	"github.com/alexedwards/scs/v2/memstore"
)

func TestNewFilterValidation(t *testing.T) {
	manager := scs.New()
	manager.Store = memstore.New()
	const configuredOrigin = "HTTPS://Admin.Example.com:8443///"

	if _, err := NewFilter(nil, FilterConfig{PublicOrigin: configuredOrigin, JSONPrefixes: []string{"/v1/admin/"}}); err == nil {
		t.Fatal("expected nil manager to fail")
	}
	for _, cfg := range []FilterConfig{
		{PublicOrigin: "", JSONPrefixes: []string{"/v1/admin/"}},
		{PublicOrigin: "   ", JSONPrefixes: []string{"/v1/admin/"}},
		{PublicOrigin: "ftp://admin.example.com", JSONPrefixes: []string{"/v1/admin/"}},
		{PublicOrigin: "https://admin.example.com/subpath", JSONPrefixes: []string{"/v1/admin/"}},
		{PublicOrigin: "https://admin.example.com?q=1", JSONPrefixes: []string{"/v1/admin/"}},
		{PublicOrigin: "https://admin.example.com#frag", JSONPrefixes: []string{"/v1/admin/"}},
		{PublicOrigin: "https://user:pass@admin.example.com", JSONPrefixes: []string{"/v1/admin/"}},
		{PublicOrigin: configuredOrigin},
		{PublicOrigin: configuredOrigin, JSONPrefixes: []string{"/v1/admin"}},
		{PublicOrigin: configuredOrigin, JSONPrefixes: []string{"v1/admin/"}},
		{PublicOrigin: configuredOrigin, SameOriginPaths: []string{"callback"}},
		{PublicOrigin: configuredOrigin, CrossOriginPaths: []string{"authorize"}},
		{PublicOrigin: configuredOrigin, SameOriginPaths: []string{"/dup"}, CrossOriginPaths: []string{"/dup"}},
		{PublicOrigin: configuredOrigin, JSONPrefixes: []string{"/v1/admin/"}, SameOriginPaths: []string{"/v1/admin/login"}},
		{PublicOrigin: configuredOrigin, JSONPrefixes: []string{"/v1/admin/"}, CrossOriginPaths: []string{"/v1/admin/sso"}},
	} {
		if _, err := NewFilter(manager, cfg); err == nil {
			t.Fatalf("NewFilter(%+v) succeeded, want error", cfg)
		}
	}
}

func TestNewFilterThreeTierRoutingAndOriginProtection(t *testing.T) {
	manager := scs.New()
	manager.Store = memstore.New()
	const rawOrigin = "HTTPS://IAM.Example.com:8443///"
	const normalizedOrigin = "https://iam.example.com:8443"

	filter, err := NewFilter(manager, FilterConfig{
		PublicOrigin:     rawOrigin,
		JSONPrefixes:     []string{"/v1/iam/"},
		SameOriginPaths:  []string{"/authorize/callback"},
		CrossOriginPaths: []string{"/authorize", "/end_session"},
	})
	if err != nil {
		t.Fatalf("NewFilter: %v", err)
	}

	called := 0
	h := filter(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called++
		wantLoaded := strings.HasPrefix(r.URL.Path, "/v1/iam/") ||
			r.URL.Path == "/authorize/callback" ||
			r.URL.Path == "/authorize" ||
			r.URL.Path == "/end_session"
		if Loaded(r.Context(), manager) != wantLoaded {
			t.Fatalf("path %s Loaded=%v want=%v", r.URL.Path, Loaded(r.Context(), manager), wantLoaded)
		}
		if wantLoaded && r.Method == http.MethodPost {
			manager.Put(r.Context(), "uid", "u-1")
		}
		w.WriteHeader(http.StatusOK)
	}))

	// 1. JSONPrefixes: safe methods bypass Content-Type and Origin checks while loading session.
	for _, method := range []string{http.MethodGet, http.MethodHead, http.MethodOptions} {
		before := called
		r := httptest.NewRequest(method, normalizedOrigin+"/v1/iam/account/profile", nil)
		r.Header.Set("Origin", "https://other.example")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != http.StatusOK || called != before+1 {
			t.Fatalf("JSON safe %s status=%d called=%d", method, w.Code, called-before)
		}
	}

	// 2. JSONPrefixes: non-JSON Content-Type on unsafe methods -> 415 with zero session side effects.
	for _, method := range []string{http.MethodPost, http.MethodPatch, http.MethodDelete} {
		for _, ct := range []string{"", "text/plain", "application/x-www-form-urlencoded", "multipart/form-data; boundary=abc"} {
			before := called
			r := httptest.NewRequest(method, normalizedOrigin+"/v1/iam/account/delete", strings.NewReader("{}"))
			if ct != "" {
				r.Header.Set("Content-Type", ct)
			}
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			if w.Code != http.StatusUnsupportedMediaType || called != before || len(w.Result().Cookies()) != 0 {
				t.Fatalf("JSON %s ct=%q status=%d called=%d cookies=%v", method, ct, w.Code, called-before, w.Result().Cookies())
			}
		}
	}

	// 3. JSONPrefixes: cross-site Sec-Fetch-Site or mismatched/duplicate Origin -> 403 before session load.
	for _, fetchSite := range []string{"cross-site", "same-site"} {
		before := called
		r := httptest.NewRequest(http.MethodPost, normalizedOrigin+"/v1/iam/account/delete", strings.NewReader("{}"))
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("Sec-Fetch-Site", fetchSite)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != http.StatusForbidden || called != before || len(w.Result().Cookies()) != 0 {
			t.Fatalf("JSON Sec-Fetch-Site=%q status=%d", fetchSite, w.Code)
		}
	}
	for _, origins := range [][]string{
		{"https://evil.example"},
		{"null"},
		{"http://iam.example.com:8443"},
		{"https://iam.example.com"},
		{normalizedOrigin, "https://evil.example"},
	} {
		before := called
		r := httptest.NewRequest(http.MethodPost, normalizedOrigin+"/v1/iam/account/delete", strings.NewReader("{}"))
		r.Header.Set("Content-Type", "application/json; charset=utf-8")
		for _, o := range origins {
			r.Header.Add("Origin", o)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != http.StatusForbidden || called != before || len(w.Result().Cookies()) != 0 {
			t.Fatalf("JSON Origin=%v status=%d", origins, w.Code)
		}
	}

	// 4. JSONPrefixes: valid JSON + same-origin / omitted Origin -> allowed and persists session cookie.
	var cookie *http.Cookie
	for _, origin := range []string{"", normalizedOrigin} {
		before := called
		r := httptest.NewRequest(http.MethodPost, normalizedOrigin+"/v1/iam/authn/login", strings.NewReader("{}"))
		r.Header.Set("Content-Type", "application/json; charset=utf-8")
		r.Header.Set("Sec-Fetch-Site", "same-origin")
		if origin != "" {
			r.Header.Set("Origin", origin)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != http.StatusOK || called != before+1 || len(w.Result().Cookies()) == 0 {
			t.Fatalf("JSON valid origin=%q status=%d cookies=%v", origin, w.Code, w.Result().Cookies())
		}
		cookie = w.Result().Cookies()[0]
	}

	// 5. SameOriginPaths: form POST allowed when same-origin, blocked (403) when cross-origin before touching session.
	before := called
	r := httptest.NewRequest(http.MethodPost, normalizedOrigin+"/authorize/callback", strings.NewReader("id=1"))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.Header.Set("Origin", "https://evil.example")
	r.AddCookie(cookie)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != http.StatusForbidden || called != before || len(w.Result().Cookies()) != 0 {
		t.Fatalf("SameOriginPaths cross-origin POST status=%d called=%d", w.Code, called-before)
	}

	before = called
	r = httptest.NewRequest(http.MethodPost, normalizedOrigin+"/authorize/callback", strings.NewReader("id=1"))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.Header.Set("Origin", normalizedOrigin)
	r.AddCookie(cookie)
	w = httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != http.StatusOK || called != before+1 {
		t.Fatalf("SameOriginPaths same-origin POST status=%d called=%d", w.Code, called-before)
	}

	// 6. CrossOriginPaths: RP cross-origin form POST (e.g. OIDC /end_session) loads session without 403/415.
	before = called
	r = httptest.NewRequest(http.MethodPost, normalizedOrigin+"/end_session", strings.NewReader("id_token_hint=tok"))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.Header.Set("Origin", "https://rp.example.com")
	r.Header.Set("Sec-Fetch-Site", "cross-site")
	r.AddCookie(cookie)
	w = httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != http.StatusOK || called != before+1 {
		t.Fatalf("CrossOriginPaths POST status=%d called=%d", w.Code, called-before)
	}

	// 7. Unmatched routes bypass session completely and never renew cookies.
	before = called
	r = httptest.NewRequest(http.MethodGet, normalizedOrigin+"/healthz", nil)
	r.AddCookie(cookie)
	w = httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != http.StatusOK || called != before+1 || len(w.Result().Cookies()) != 0 {
		t.Fatalf("unmatched route status=%d cookies=%v", w.Code, w.Result().Cookies())
	}
}
