package session

import (
	"context"
	"fmt"
	"net/http"

	security "github.com/Servora-Kit/plateau/security"
)

// Resolver validates an opaque session credential and returns service-owned identity state.
type Resolver[T any] func(context.Context, string) (T, error)

// ActorMapper maps service-owned identity state to Plateau's shared Actor.
type ActorMapper[T any] func(T) (security.Actor, error)

// ContextExtender adds service-local trusted state without changing the shared Actor contract.
type ContextExtender[T any] func(context.Context, T) context.Context

// Authenticator owns one immutable session authentication profile.
type Authenticator[T any] struct {
	cookieName string
	resolve    Resolver[T]
	mapActor   ActorMapper[T]
	extend     ContextExtender[T]
}

// New constructs an opaque-session authenticator.
func New[T any](cookieName string, resolve Resolver[T], mapActor ActorMapper[T], extend ContextExtender[T]) (*Authenticator[T], error) {
	if err := (&http.Cookie{Name: cookieName, Value: "credential"}).Valid(); err != nil {
		return nil, fmt.Errorf("session authn: cookie name is invalid: %w", err)
	}
	if resolve == nil {
		return nil, fmt.Errorf("session authn: resolver is nil")
	}
	if mapActor == nil {
		return nil, fmt.Errorf("session authn: Actor mapper is nil")
	}
	return &Authenticator[T]{cookieName: cookieName, resolve: resolve, mapActor: mapActor, extend: extend}, nil
}

func (authenticator *Authenticator[T]) authenticate(ctx context.Context, credential string) (context.Context, error) {
	if ctx == nil {
		return nil, fmt.Errorf("session authn: context is nil")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if !validAuthenticator(authenticator) {
		return nil, fmt.Errorf("session authn: authenticator is invalid")
	}
	identity, err := authenticator.resolve(ctx, credential)
	if err != nil {
		return nil, err
	}
	actor, err := authenticator.mapActor(identity)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrActorMapping, err)
	}
	if !actor.Valid() || actor.Type == security.ActorTypeAnonymous {
		return nil, fmt.Errorf("%w: mapper returned invalid authenticated Actor", ErrActorMapping)
	}
	ctx = security.WithActor(ctx, actor)
	if authenticator.extend != nil {
		ctx = authenticator.extend(ctx, identity)
		if ctx == nil {
			return nil, fmt.Errorf("%w: extender returned nil context", ErrContextExtension)
		}
		stored, ok := security.ActorFrom(ctx)
		if !ok || stored != actor {
			return nil, fmt.Errorf("%w: extender changed shared Actor", ErrContextExtension)
		}
	}
	return ctx, nil
}

func validAuthenticator[T any](authenticator *Authenticator[T]) bool {
	return authenticator != nil && authenticator.cookieName != "" && authenticator.resolve != nil && authenticator.mapActor != nil
}
