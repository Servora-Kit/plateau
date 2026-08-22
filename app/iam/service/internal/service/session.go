package service

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	sessionpb "github.com/Servora-Kit/plateau/api/gen/go/iam/session/v1"
	iamauthn "github.com/Servora-Kit/plateau/app/iam/service/internal/authn"
	"github.com/Servora-Kit/plateau/app/iam/service/internal/biz"
	kerrors "github.com/go-kratos/kratos/v3/errors"
	"github.com/go-kratos/kratos/v3/transport"
	khttp "github.com/go-kratos/kratos/v3/transport/http"
)

// SessionService exposes IAM Login Session lifecycle commands.
type SessionService struct {
	sessionpb.UnimplementedSessionServiceServer
	sessions *biz.SessionUsecase
}

func NewSessionService(sessions *biz.SessionUsecase) (*SessionService, error) {
	if sessions == nil {
		return nil, fmt.Errorf("session service: usecase is nil")
	}
	return &SessionService{sessions: sessions}, nil
}

func (s *SessionService) Logout(ctx context.Context, _ *sessionpb.LogoutRequest) (*sessionpb.LogoutResponse, error) {
	_, loginSession, err := iamauthn.From(ctx)
	if err != nil {
		return nil, sessionpb.ErrorSessionErrorReasonRevoked("session is not active")
	}
	if err := s.sessions.Logout(ctx, loginSession.GetSessionId()); err != nil {
		return nil, sessionError(err)
	}
	clearSessionCookie(ctx)
	return &sessionpb.LogoutResponse{}, nil
}

func setSessionCookie(ctx context.Context, secret string) {
	if _, ok := transport.FromServerContext(ctx); !ok || secret == "" {
		return
	}
	khttp.SetCookie(ctx, sessionCookie(secret))
}

func clearSessionCookie(ctx context.Context) {
	if _, ok := transport.FromServerContext(ctx); !ok {
		return
	}
	khttp.SetCookie(ctx, expiredSessionCookie())
}

func sessionCookie(secret string) *http.Cookie {
	return &http.Cookie{
		Name: iamauthn.SessionCookieName, Value: secret, Path: "/", HttpOnly: true,
		Secure: true, SameSite: http.SameSiteLaxMode,
		MaxAge: int(biz.LoginSessionAbsoluteTTL.Seconds()),
	}
}

func expiredSessionCookie() *http.Cookie {
	return &http.Cookie{
		Name: iamauthn.SessionCookieName, Value: "", Path: "/", HttpOnly: true,
		Secure: true, SameSite: http.SameSiteLaxMode, MaxAge: -1,
	}
}

func sessionError(err error) error {
	if errors.Is(err, biz.ErrSessionRevoked) || errors.Is(err, biz.ErrUnauthenticated) {
		return sessionpb.ErrorSessionErrorReasonRevoked("session is not active")
	}
	return kerrors.InternalServer("IAM session operation failed", "session operation failed").WithCause(err)
}
