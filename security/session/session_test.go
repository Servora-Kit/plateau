package session

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	sessionpb "github.com/Servora-Kit/plateau/api/gen/go/plateau/security/session/v1"
	"github.com/alexedwards/scs/v2"
	"github.com/alexedwards/scs/v2/memstore"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/durationpb"
)

func config() *sessionpb.Session {
	return &sessionpb.Session{Lifetime: durationpb.New(time.Hour), IdleTimeout: durationpb.New(10 * time.Minute), Cookie: &sessionpb.Cookie{Name: proto.String("__Host-test")}}
}
func newManager(t *testing.T) (*scs.SessionManager, *memstore.MemStore) {
	t.Helper()
	store := memstore.NewWithCleanupInterval(0)
	manager, err := New(config(), store)
	if err != nil {
		t.Fatal(err)
	}
	return manager, store
}

func TestConfigurationValidationAndIsolation(t *testing.T) {
	for _, change := range []func(*sessionpb.Session){
		func(c *sessionpb.Session) { c.Lifetime = durationpb.New(0) },
		func(c *sessionpb.Session) { c.IdleTimeout = durationpb.New(-time.Second) },
		func(c *sessionpb.Session) { c.Cookie.Name = proto.String("invalid name") },
		func(c *sessionpb.Session) { c.Cookie.Domain = "example.test" },
		func(c *sessionpb.Session) { c.Cookie.Path = proto.String("/account") },
		func(c *sessionpb.Session) { c.Cookie.Secure = proto.Bool(false) },
		func(c *sessionpb.Session) { c.Cookie.SameSite = proto.String("invalid") },
	} {
		c := config()
		change(c)
		if _, err := New(c, memstore.NewWithCleanupInterval(0)); err == nil {
			t.Fatalf("accepted config=%v", c)
		}
	}
	one, _ := newManager(t)
	two, _ := newManager(t)
	LoadAndSave(one)(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		if !Loaded(r.Context(), one) || Loaded(r.Context(), two) {
			t.Fatal("manager contexts not isolated")
		}
		one.Put(r.Context(), "identity", "user-1")
	})).ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))
}

func TestCookieHashRenewalAndAbsoluteDeadline(t *testing.T) {
	manager, store := newManager(t)
	var deadline time.Time
	w := httptest.NewRecorder()
	LoadAndSave(manager)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		manager.Put(r.Context(), "login", "reference")
		deadline = manager.Deadline(r.Context())
		w.WriteHeader(http.StatusNoContent)
	})).ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/login", nil))
	cookies := w.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("cookies=%v", cookies)
	}
	cookie := cookies[0]
	if !cookie.Secure || !cookie.HttpOnly || cookie.Path != "/" || cookie.Domain != "" || cookie.SameSite != http.SameSiteLaxMode || strings.Contains(cookie.Value, "reference") {
		t.Fatalf("cookie=%v", cookie)
	}
	if _, found, _ := store.Find(cookie.Value); found {
		t.Fatal("raw token in store")
	}
	entries, err := store.All()
	if err != nil || len(entries) != 1 {
		t.Fatalf("stored entries=%d error=%v", len(entries), err)
	}
	request := httptest.NewRequest(http.MethodGet, "/account", nil)
	request.AddCookie(cookie)
	w = httptest.NewRecorder()
	LoadAndSave(manager)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !manager.Deadline(r.Context()).Equal(deadline) || manager.GetString(r.Context(), "login") != "reference" {
			t.Fatal("read changed deadline or identity")
		}
		http.Error(w, "rejected", http.StatusUnauthorized)
	})).ServeHTTP(w, request)
	if w.Code != http.StatusUnauthorized || len(w.Result().Cookies()) != 1 {
		t.Fatalf("response=%v", w.Result())
	}
	for key, data := range entries {
		if err := store.Commit(key, data, time.Now().Add(-time.Second)); err != nil {
			t.Fatal(err)
		}
	}
	LoadAndSave(manager)(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		if manager.GetString(r.Context(), "login") != "" {
			t.Fatal("missing store entry restored")
		}
	})).ServeHTTP(httptest.NewRecorder(), request)
}

type failingStore struct{ scs.Store }

func (failingStore) Commit(string, []byte, time.Time) error {
	return errors.New("sensitive store error")
}

func TestStoreFailureAbortsSuccessResponse(t *testing.T) {
	manager, err := New(config(), failingStore{memstore.NewWithCleanupInterval(0)})
	if err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	continued := false
	LoadAndSave(manager)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		manager.Put(r.Context(), "login", "reference")
		w.WriteHeader(http.StatusOK)
		continued = true
		_, _ = w.Write([]byte("login succeeded"))
	})).ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/login", nil))
	if continued || w.Code != http.StatusInternalServerError || w.Header().Get("Set-Cookie") != "" || strings.Contains(w.Body.String(), "sensitive") || strings.Contains(w.Body.String(), "succeeded") {
		t.Fatalf("continued=%v response=%d %s", continued, w.Code, w.Body.String())
	}
}

func TestAnonymousRequestDoesNotCreateSession(t *testing.T) {
	manager, _ := newManager(t)
	w := httptest.NewRecorder()
	LoadAndSave(manager)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })).ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/", nil))
	if len(w.Result().Cookies()) != 0 {
		t.Fatal("anonymous request created cookie")
	}
}
