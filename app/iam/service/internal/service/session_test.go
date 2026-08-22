package service

import (
	"errors"
	"net/http"
	"testing"

	sessionpb "github.com/Servora-Kit/plateau/api/gen/go/iam/session/v1"
	"github.com/Servora-Kit/plateau/app/iam/service/internal/biz"
)

func TestSessionCookieUsesHostOnlySecurityContract(t *testing.T) {
	cookie := sessionCookie("opaque-secret")
	if cookie.Name != "__Host-iam_session" || cookie.Value != "opaque-secret" || cookie.Path != "/" || cookie.Domain != "" {
		t.Fatalf("session cookie identity = %#v", cookie)
	}
	if !cookie.HttpOnly || !cookie.Secure || cookie.SameSite != http.SameSiteLaxMode || cookie.MaxAge != int(biz.LoginSessionAbsoluteTTL.Seconds()) {
		t.Fatalf("session cookie security attributes = %#v", cookie)
	}

	expired := expiredSessionCookie()
	if expired.Name != cookie.Name || expired.Value != "" || expired.MaxAge >= 0 || !expired.HttpOnly || !expired.Secure || expired.Path != "/" || expired.Domain != "" || expired.SameSite != http.SameSiteLaxMode {
		t.Fatalf("expired session cookie = %#v", expired)
	}
}

func TestSessionErrorKeepsSessionFailuresOutOfAuthnContract(t *testing.T) {
	for _, err := range []error{biz.ErrUnauthenticated, biz.ErrSessionRevoked} {
		if mapped := sessionError(err); !sessionpb.IsSessionErrorReasonRevoked(mapped) {
			t.Fatalf("sessionError(%v) = %v", err, mapped)
		}
	}
	cause := errors.New("database unavailable")
	mapped := sessionError(cause)
	if sessionpb.IsSessionErrorReasonRevoked(mapped) || !errors.Is(mapped, cause) {
		t.Fatalf("dependency failure mapped to %v", mapped)
	}
}
