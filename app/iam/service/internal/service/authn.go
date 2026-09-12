package service

import (
	"context"
	"errors"
	"fmt"

	authnpb "github.com/Servora-Kit/plateau/api/gen/go/iam/authn/v1"
	iamauthn "github.com/Servora-Kit/plateau/app/iam/service/internal/authn"
	"github.com/Servora-Kit/plateau/app/iam/service/internal/biz"
	sessions "github.com/Servora-Kit/plateau/security/session"
	"github.com/alexedwards/scs/v2"
	kerrors "github.com/go-kratos/kratos/v3/errors"
)

// AuthnService exposes credential Authentication.
type AuthnService struct {
	authnpb.UnimplementedAuthnServiceServer
	authentication *biz.AuthenticationUsecase
	manager        *scs.SessionManager
}

func NewAuthnService(authentication *biz.AuthenticationUsecase, manager *scs.SessionManager) (*AuthnService, error) {
	if authentication == nil || manager == nil {
		return nil, fmt.Errorf("authn service: usecase is nil")
	}
	return &AuthnService{authentication: authentication, manager: manager}, nil
}

func (s *AuthnService) Login(ctx context.Context, request *authnpb.LoginRequest) (*authnpb.LoginResponse, error) {
	if request == nil {
		return nil, authnpb.ErrorAuthnErrorReasonInvalidCredentials("credentials rejected")
	}
	if !sessions.Loaded(ctx, s.manager) {
		return nil, authnError(fmt.Errorf("HTTP session is not loaded"))
	}
	user, loginSession, err := s.authentication.Login(ctx, request.GetEmail(), request.GetPassword())
	if err != nil {
		return nil, authnError(err)
	}
	if err := s.manager.RenewToken(ctx); err != nil {
		return nil, authnError(err)
	}
	s.manager.Put(ctx, iamauthn.LoginReferenceKey, loginSession.ID)
	return &authnpb.LoginResponse{User: user}, nil
}

func authnError(err error) error {
	if errors.Is(err, biz.ErrInvalidCredentials) {
		return authnpb.ErrorAuthnErrorReasonInvalidCredentials("credentials rejected")
	}
	return kerrors.InternalServer("IAM authentication failed", "authentication failed").WithCause(err)
}
