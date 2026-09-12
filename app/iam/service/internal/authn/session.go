package authn

import (
	"context"
	"errors"
	"fmt"

	"github.com/Servora-Kit/plateau/app/iam/service/internal/biz"
	"github.com/Servora-Kit/plateau/security"
	"github.com/Servora-Kit/plateau/security/authn/session"
	"github.com/alexedwards/scs/v2"
)

const LoginReferenceKey = "iam_login_id"

type SessionAuthenticator = session.Authenticator[*identity]

// NewSessionAuthenticator binds IAM session resolution and identity projection to the shared session backend.
func NewSessionAuthenticator(usecase *biz.SessionUsecase, manager *scs.SessionManager) (*SessionAuthenticator, error) {
	if usecase == nil {
		return nil, fmt.Errorf("IAM session authenticator: usecase is nil")
	}
	return session.New(
		manager,
		func(ctx context.Context) (*identity, error) {
			user, loginSession, err := usecase.Resolve(ctx, manager.GetString(ctx, LoginReferenceKey))
			if err != nil {
				return nil, resolutionError(err)
			}
			return &identity{user: user, session: loginSession}, nil
		},
		mapActor,
		withIdentity,
	)
}

func resolutionError(err error) error {
	switch {
	case errors.Is(err, biz.ErrUnauthenticated),
		errors.Is(err, biz.ErrSessionRevoked),
		errors.Is(err, biz.ErrUserDisabled),
		errors.Is(err, biz.ErrUserNotActive):
		return fmt.Errorf("%w: %w", session.ErrInvalidCredentials, err)
	default:
		return fmt.Errorf("%w: %w", session.ErrDependencyUnavailable, err)
	}
}

func mapActor(value *identity) (security.Actor, error) {
	if value == nil || value.user == nil || value.user.GetUserId() == "" || value.session == nil || value.session.ID == "" {
		return security.Actor{}, fmt.Errorf("IAM authenticated identity is incomplete")
	}
	return security.Actor{Type: security.ActorTypeHuman, ID: value.user.GetUserId()}, nil
}
