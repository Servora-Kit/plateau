package authn

import (
	"context"
	"errors"
	"testing"

	sessionpb "github.com/Servora-Kit/plateau/api/gen/go/iam/session/v1"
	userpb "github.com/Servora-Kit/plateau/api/gen/go/iam/user/v1"
	"github.com/Servora-Kit/plateau/app/iam/service/internal/biz"
	security "github.com/Servora-Kit/plateau/security"
	sessionauthn "github.com/Servora-Kit/plateau/security/authn/session"
)

func TestIAMSessionActorMappingAndContext(t *testing.T) {
	value := &identity{
		user:    &userpb.User{UserId: "user-1"},
		session: &sessionpb.Session{SessionId: "session-1"},
	}
	actor, err := mapActor(value)
	if err != nil || actor != (security.Actor{Type: security.ActorTypeHuman, ID: "user-1"}) {
		t.Fatalf("Actor = %+v, error = %v", actor, err)
	}
	ctx := withIdentity(context.Background(), value)
	user, loginSession, err := From(ctx)
	if err != nil || user != value.user || loginSession != value.session {
		t.Fatalf("identity context user=%p session=%p error=%v", user, loginSession, err)
	}
	if _, err := mapActor(&identity{}); err == nil {
		t.Fatal("mapActor accepted incomplete identity")
	}
}

func TestSessionResolutionErrorClassification(t *testing.T) {
	for _, err := range []error{biz.ErrUnauthenticated, biz.ErrSessionRevoked, biz.ErrUserDisabled, biz.ErrUserNotActive} {
		if mapped := resolutionError(err); !errors.Is(mapped, sessionauthn.ErrInvalidCredentials) || !errors.Is(mapped, err) {
			t.Fatalf("domain error %v mapped to %v", err, mapped)
		}
	}
	cause := errors.New("database unavailable")
	mapped := resolutionError(cause)
	if !errors.Is(mapped, sessionauthn.ErrDependencyUnavailable) || !errors.Is(mapped, cause) {
		t.Fatalf("dependency error mapped to %v", mapped)
	}
}
