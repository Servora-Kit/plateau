package session

import (
	"context"
	"fmt"

	userpb "github.com/Servora-Kit/plateau/api/gen/go/iam/user/v1"
	security "github.com/Servora-Kit/plateau/security"
	sessions "github.com/Servora-Kit/plateau/security/session"
	"github.com/alexedwards/scs/v2"
	"google.golang.org/protobuf/proto"
)

const userSessionKey = "plateau_session_user"

type userContextKey struct{}

// UserAuthenticator authenticates browser requests backed by a serialized iam.user.v1.User session snapshot.
type UserAuthenticator = Authenticator[*userpb.User]

// SaveUser validates user, marshals the snapshot, rotates the SCS session token, and stores it in the session.
func SaveUser(ctx context.Context, manager *scs.SessionManager, user *userpb.User) error {
	if ctx == nil {
		return fmt.Errorf("session authn: context is nil")
	}
	if manager == nil {
		return fmt.Errorf("session authn: manager is nil")
	}
	if !sessions.Loaded(ctx, manager) {
		return fmt.Errorf("session authn: HTTP session context is not loaded")
	}
	if user == nil || user.GetUserId() == "" {
		return fmt.Errorf("session authn: user_id is empty")
	}
	encoded, err := proto.Marshal(user)
	if err != nil {
		return fmt.Errorf("session authn: marshal user: %w", err)
	}
	if err := manager.RenewToken(ctx); err != nil {
		return fmt.Errorf("session authn: renew session token: %w", err)
	}
	manager.Put(ctx, userSessionKey, encoded)
	return nil
}

// NewUserAuthenticator constructs a Session Authenticator backed by Plateau's default User session snapshot.
func NewUserAuthenticator(manager *scs.SessionManager) (*UserAuthenticator, error) {
	return New(
		manager,
		func(ctx context.Context) (*userpb.User, error) {
			raw := manager.GetBytes(ctx, userSessionKey)
			if len(raw) == 0 {
				return nil, ErrMissingCredentials
			}
			user := new(userpb.User)
			if err := proto.Unmarshal(raw, user); err != nil {
				return nil, fmt.Errorf("%w: %w", ErrInvalidCredentials, err)
			}
			return user, nil
		},
		func(u *userpb.User) (security.Actor, error) {
			if u.GetUserId() == "" {
				return security.Actor{}, fmt.Errorf("user_id is empty")
			}
			return security.Actor{Type: security.ActorTypeHuman, ID: u.GetUserId()}, nil
		},
		func(ctx context.Context, u *userpb.User) context.Context {
			return context.WithValue(ctx, userContextKey{}, u)
		},
	)
}

// UserFrom extracts a defensive clone of the authenticated User snapshot from ctx.
func UserFrom(ctx context.Context) (*userpb.User, bool) {
	if ctx == nil {
		return nil, false
	}
	user, ok := ctx.Value(userContextKey{}).(*userpb.User)
	if !ok || user == nil {
		return nil, false
	}
	return proto.Clone(user).(*userpb.User), true
}
