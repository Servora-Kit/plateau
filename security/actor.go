// Package security provides shared Platform security value types.
package security

import "context"

// ActorType identifies an execution actor category.
type ActorType string

const (
	ActorTypeHuman     ActorType = "human"
	ActorTypeService   ActorType = "service"
	ActorTypeAnonymous ActorType = "anonymous"
)

// Actor identifies the stable execution identity for a request or business action.
type Actor struct {
	Type ActorType
	ID   string
}

// Valid reports whether the Actor satisfies Platform's identity invariant.
func (actor Actor) Valid() bool {
	switch actor.Type {
	case ActorTypeHuman, ActorTypeService:
		return actor.ID != ""
	case ActorTypeAnonymous:
		return actor.ID == ""
	default:
		return false
	}
}

type actorContextKey struct{}

// WithActor stores actor as an in-process context value. It does not authenticate
// credentials, authorize an operation, or load business facts.
func WithActor(ctx context.Context, actor Actor) context.Context {
	return context.WithValue(ctx, actorContextKey{}, actor)
}

// ActorFrom returns the valid Actor stored in ctx.
func ActorFrom(ctx context.Context) (Actor, bool) {
	if ctx == nil {
		return Actor{}, false
	}
	actor, ok := ctx.Value(actorContextKey{}).(Actor)
	if !ok || !actor.Valid() {
		return Actor{}, false
	}
	return actor, true
}
