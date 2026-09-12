package authz

import (
	"testing"

	"github.com/Servora-Kit/plateau/security"
)

func TestSubjectMapsVerifiedActorTypes(t *testing.T) {
	got, err := subject(security.Actor{Type: security.ActorTypeHuman, ID: "iam-user-1"})
	if err != nil || got != "user:iam-user-1" {
		t.Fatalf("subject() = %q, %v", got, err)
	}
	got, err = subject(security.Actor{Type: security.ActorTypeService, ID: "iam-user-1"})
	if err != nil || got != "service:iam-user-1" {
		t.Fatalf("service subject=%q error=%v", got, err)
	}
	for _, actor := range []security.Actor{
		{Type: security.ActorTypeAnonymous},
		{Type: security.ActorTypeHuman},
	} {
		if got, err := subject(actor); err == nil || got != "" {
			t.Fatalf("subject(%+v) = %q, %v; want rejection", actor, got, err)
		}
	}
}
