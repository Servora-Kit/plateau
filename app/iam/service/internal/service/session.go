package service

import (
	"context"
	"errors"
	"fmt"

	sessionpb "github.com/Servora-Kit/plateau/api/gen/go/iam/session/v1"
	iamauthn "github.com/Servora-Kit/plateau/app/iam/service/internal/authn"
	"github.com/Servora-Kit/plateau/app/iam/service/internal/biz"
	sessions "github.com/Servora-Kit/plateau/security/session"
	"github.com/alexedwards/scs/v2"
	kerrors "github.com/go-kratos/kratos/v3/errors"
)

type SessionService struct {
	sessionpb.UnimplementedSessionServiceServer
	sessions *biz.SessionUsecase
	manager  *scs.SessionManager
}

func NewSessionService(sessions *biz.SessionUsecase, manager *scs.SessionManager) (*SessionService, error) {
	if sessions == nil || manager == nil {
		return nil, fmt.Errorf("session service: dependency is nil")
	}
	return &SessionService{sessions: sessions, manager: manager}, nil
}

func (s *SessionService) Logout(ctx context.Context, _ *sessionpb.LogoutRequest) (*sessionpb.LogoutResponse, error) {
	if !sessions.Loaded(ctx, s.manager) {
		return nil, sessionError(fmt.Errorf("HTTP session is not loaded"))
	}
	_, login, err := iamauthn.From(ctx)
	if err != nil {
		return nil, sessionpb.ErrorSessionErrorReasonRevoked("session is not active")
	}
	if err := s.sessions.Logout(ctx, login.ID); err != nil {
		return nil, sessionError(err)
	}
	if err := s.manager.Destroy(ctx); err != nil {
		return nil, sessionError(err)
	}
	return &sessionpb.LogoutResponse{}, nil
}

func sessionError(err error) error {
	if errors.Is(err, biz.ErrSessionRevoked) || errors.Is(err, biz.ErrUnauthenticated) {
		return sessionpb.ErrorSessionErrorReasonRevoked("session is not active")
	}
	return kerrors.InternalServer("IAM session operation failed", "session operation failed").WithCause(err)
}
