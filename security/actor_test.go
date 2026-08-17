package security

import (
	"context"
	"testing"
)

func TestActorValid(t *testing.T) {
	tests := []struct {
		name  string
		actor Actor
		valid bool
	}{
		{name: "human", actor: Actor{Type: ActorTypeHuman, ID: "user-1"}, valid: true},
		{name: "service", actor: Actor{Type: ActorTypeService, ID: "worker-1"}, valid: true},
		{name: "anonymous", actor: Actor{Type: ActorTypeAnonymous}, valid: true},
		{name: "human empty id", actor: Actor{Type: ActorTypeHuman}, valid: false},
		{name: "service empty id", actor: Actor{Type: ActorTypeService}, valid: false},
		{name: "anonymous id", actor: Actor{Type: ActorTypeAnonymous, ID: "unexpected"}, valid: false},
		{name: "unknown", actor: Actor{Type: ActorType("system"), ID: "system-1"}, valid: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := test.actor.Valid(); got != test.valid {
				t.Fatalf("Valid() = %v, want %v", got, test.valid)
			}
		})
	}
}

func TestActorContext(t *testing.T) {
	actor := Actor{Type: ActorTypeHuman, ID: "user-1"}
	ctx := WithActor(context.Background(), actor)
	got, ok := ActorFrom(ctx)
	if !ok || got != actor {
		t.Fatalf("ActorFrom() = (%+v, %v), want (%+v, true)", got, ok, actor)
	}

	if _, ok := ActorFrom(context.Background()); ok {
		t.Fatal("ActorFrom() accepted missing actor")
	}
	if _, ok := ActorFrom(nil); ok {
		t.Fatal("ActorFrom() accepted nil context")
	}
	if _, ok := ActorFrom(WithActor(context.Background(), Actor{Type: ActorTypeHuman})); ok {
		t.Fatal("ActorFrom() accepted invalid actor")
	}
}

func TestWithActorDoesNotValidateCarrier(t *testing.T) {
	invalid := Actor{Type: ActorTypeHuman}
	if _, ok := ActorFrom(WithActor(context.Background(), invalid)); ok {
		t.Fatal("invalid carrier unexpectedly became readable")
	}
}
