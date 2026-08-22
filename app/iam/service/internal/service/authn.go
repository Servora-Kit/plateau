package service

import (
	"context"
	"errors"
	"fmt"

	authnpb "github.com/Servora-Kit/plateau/api/gen/go/iam/authn/v1"
	"github.com/Servora-Kit/plateau/app/iam/service/internal/biz"
	kerrors "github.com/go-kratos/kratos/v3/errors"
)

// AuthnService exposes credential Authentication.
type AuthnService struct {
	authnpb.UnimplementedAuthnServiceServer
	authentication *biz.AuthenticationUsecase
}

func NewAuthnService(authentication *biz.AuthenticationUsecase) (*AuthnService, error) {
	if authentication == nil {
		return nil, fmt.Errorf("authn service: usecase is nil")
	}
	return &AuthnService{authentication: authentication}, nil
}

func (s *AuthnService) Login(ctx context.Context, request *authnpb.LoginRequest) (*authnpb.LoginResponse, error) {
	if request == nil {
		return nil, authnpb.ErrorAuthnErrorReasonInvalidCredentials("credentials rejected")
	}
	user, loginSession, secret, err := s.authentication.Login(ctx, request.GetEmail(), request.GetPassword())
	if err != nil {
		return nil, authnError(err)
	}
	setSessionCookie(ctx, secret)
	return &authnpb.LoginResponse{User: user, Session: loginSession}, nil
}

func authnError(err error) error {
	if errors.Is(err, biz.ErrInvalidCredentials) {
		return authnpb.ErrorAuthnErrorReasonInvalidCredentials("credentials rejected")
	}
	return kerrors.InternalServer("IAM authentication failed", "authentication failed").WithCause(err)
}
