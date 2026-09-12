package service

import (
	"errors"
	"testing"

	sessionpb "github.com/Servora-Kit/plateau/api/gen/go/iam/session/v1"
	"github.com/Servora-Kit/plateau/app/iam/service/internal/biz"
)

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
