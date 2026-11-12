// Package authn projects Admin local SCS browser sessions to Plateau Actors.
package authn

import (
	"context"
	"fmt"

	adminauthpb "github.com/Servora-Kit/plateau/api/gen/go/admin/auth/v1"
	"github.com/Servora-Kit/plateau/security"
	"github.com/Servora-Kit/plateau/security/authn/session"
	"github.com/alexedwards/scs/v2"
	"google.golang.org/protobuf/proto"
)

// SessionKey stores the serialized AdminPrincipal protobuf message inside Admin's local SCS session.
const SessionKey = "admin_session"

type principalContextKey struct{}

// SessionAuthenticator authenticates Admin HTTP requests from Admin's local PostgreSQL SCS store.
type SessionAuthenticator = session.Authenticator[*adminauthpb.AdminPrincipal]

// SavePrincipal rotates the SCS session token and stores the verified AdminPrincipal snapshot.
func SavePrincipal(ctx context.Context, manager *scs.SessionManager, principal *adminauthpb.AdminPrincipal) error {
	if manager == nil {
		return fmt.Errorf("admin authn: session manager is nil")
	}
	if principal.GetUserId() == "" {
		return fmt.Errorf("admin authn: principal user_id is empty")
	}
	encoded, err := proto.Marshal(principal)
	if err != nil {
		return fmt.Errorf("admin authn: marshal principal: %w", err)
	}
	if err := manager.RenewToken(ctx); err != nil {
		return fmt.Errorf("admin authn: renew session token: %w", err)
	}
	manager.Put(ctx, SessionKey, encoded)
	return nil
}

// PrincipalFrom extracts the authenticated AdminPrincipal snapshot from request context.
func PrincipalFrom(ctx context.Context) (*adminauthpb.AdminPrincipal, bool) {
	if ctx == nil {
		return nil, false
	}
	p, ok := ctx.Value(principalContextKey{}).(*adminauthpb.AdminPrincipal)
	if !ok || p.GetUserId() == "" {
		return nil, false
	}
	return p, true
}

// NewSessionAuthenticator binds Admin's local SCS session snapshot to Plateau's Session Authenticator.
func NewSessionAuthenticator(manager *scs.SessionManager) (*SessionAuthenticator, error) {
	return session.New(
		manager,
		func(ctx context.Context) (*adminauthpb.AdminPrincipal, error) {
			raw := manager.GetBytes(ctx, SessionKey)
			if len(raw) == 0 {
				return nil, session.ErrMissingCredentials
			}
			p := new(adminauthpb.AdminPrincipal)
			if err := proto.Unmarshal(raw, p); err != nil || p.GetUserId() == "" {
				return nil, fmt.Errorf("%w: invalid admin session snapshot", session.ErrInvalidCredentials)
			}
			return p, nil
		},
		func(p *adminauthpb.AdminPrincipal) (security.Actor, error) {
			if p.GetUserId() == "" {
				return security.Actor{}, fmt.Errorf("admin principal is incomplete")
			}
			return security.Actor{Type: security.ActorTypeHuman, ID: p.GetUserId()}, nil
		},
		func(ctx context.Context, p *adminauthpb.AdminPrincipal) context.Context {
			return context.WithValue(ctx, principalContextKey{}, p)
		},
	)
}
