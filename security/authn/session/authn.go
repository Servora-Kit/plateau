package session

import (
	"context"
	"fmt"

	security "github.com/Servora-Kit/plateau/security"
	sessions "github.com/Servora-Kit/plateau/security/session"
	"github.com/alexedwards/scs/v2"
)

// Resolver 从已装载上下文解析应用的当前身份。
type Resolver[T any] func(context.Context) (T, error)

// ActorMapper maps service-owned identity state to Plateau's shared Actor.
type ActorMapper[T any] func(T) (security.Actor, error)

// ContextExtender adds service-local trusted state without changing the shared Actor contract.
type ContextExtender[T any] func(context.Context, T) context.Context

// Authenticator owns one immutable session authentication profile.
type Authenticator[T any] struct {
	manager  *scs.SessionManager
	resolve  Resolver[T]
	mapActor ActorMapper[T]
	extend   ContextExtender[T]
}

// New 将应用身份解析器绑定到一个显式装载的会话管理器。
func New[T any](manager *scs.SessionManager, resolve Resolver[T], mapActor ActorMapper[T], extend ContextExtender[T]) (*Authenticator[T], error) {
	if manager == nil {
		return nil, fmt.Errorf("session authn: manager is nil")
	}
	if resolve == nil {
		return nil, fmt.Errorf("session authn: resolver is nil")
	}
	if mapActor == nil {
		return nil, fmt.Errorf("session authn: Actor mapper is nil")
	}
	return &Authenticator[T]{manager: manager, resolve: resolve, mapActor: mapActor, extend: extend}, nil
}

func (authenticator *Authenticator[T]) authenticate(ctx context.Context) (context.Context, error) {
	if ctx == nil {
		return nil, fmt.Errorf("session authn: context is nil")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if !validAuthenticator(authenticator) {
		return nil, fmt.Errorf("session authn: authenticator is invalid")
	}
	if !sessions.Loaded(ctx, authenticator.manager) {
		return nil, fmt.Errorf("session authn: HTTP session context is not loaded")
	}
	identity, err := authenticator.resolve(ctx)
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
	return authenticator != nil && authenticator.manager != nil && authenticator.resolve != nil && authenticator.mapActor != nil
}
