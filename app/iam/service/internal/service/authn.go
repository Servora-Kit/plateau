package service

import (
	"context"
	"errors"
	"fmt"

	authnpb "github.com/Servora-Kit/plateau/api/gen/go/plateau/iam/authn/v1"
	iamauthn "github.com/Servora-Kit/plateau/app/iam/service/internal/authn"
	"github.com/Servora-Kit/plateau/app/iam/service/internal/biz"
	"github.com/Servora-Kit/plateau/security"
	"github.com/Servora-Kit/plateau/security/audit"
	sessions "github.com/Servora-Kit/plateau/security/session"
	"github.com/alexedwards/scs/v2"
	kerrors "github.com/go-kratos/kratos/v3/errors"
)

// AuthnService exposes browser-bound authentication flows.
type AuthnService struct {
	authnpb.UnimplementedAuthnServiceServer
	authentication *biz.AuthenticationUsecase
	manager        *scs.SessionManager
}

func NewAuthnService(
	authentication *biz.AuthenticationUsecase,
	manager *scs.SessionManager,
) (*AuthnService, error) {
	if authentication == nil || manager == nil {
		return nil, fmt.Errorf("authn service: dependency is nil")
	}
	return &AuthnService{
		authentication: authentication,
		manager:        manager,
	}, nil
}

func (s *AuthnService) CreateAuthFlow(
	ctx context.Context,
	request *authnpb.CreateAuthFlowRequest,
) (*authnpb.CreateAuthFlowResponse, error) {
	if !sessions.Loaded(ctx, s.manager) {
		return nil, authnError(fmt.Errorf("HTTP session is not loaded"))
	}
	loginID := s.manager.GetString(ctx, iamauthn.LoginReferenceKey)
	ownerToken := s.manager.GetString(ctx, iamauthn.AuthFlowOwnerKey)
	flow, newOwnerToken, err := s.authentication.CreateAuthFlow(ctx, loginID, ownerToken, request.GetOidcRequestId())
	if err != nil {
		return nil, authnError(err)
	}
	if newOwnerToken != "" {
		s.manager.Put(ctx, iamauthn.AuthFlowOwnerKey, newOwnerToken)
	}
	return &authnpb.CreateAuthFlowResponse{Flow: flow}, nil
}

func (s *AuthnService) GetAuthFlow(
	ctx context.Context,
	request *authnpb.GetAuthFlowRequest,
) (*authnpb.GetAuthFlowResponse, error) {
	if !sessions.Loaded(ctx, s.manager) {
		return nil, authnError(fmt.Errorf("HTTP session is not loaded"))
	}
	ownerToken := s.manager.GetString(ctx, iamauthn.AuthFlowOwnerKey)
	loginID := s.manager.GetString(ctx, iamauthn.LoginReferenceKey)
	flow, err := s.authentication.GetAuthFlow(ctx, ownerToken, loginID, request.GetFlowId())
	if err != nil {
		return nil, authnError(err)
	}
	return &authnpb.GetAuthFlowResponse{Flow: flow}, nil
}

func (s *AuthnService) SubmitPassword(
	ctx context.Context,
	request *authnpb.SubmitPasswordRequest,
) (*authnpb.SubmitPasswordResponse, error) {
	if !sessions.Loaded(ctx, s.manager) {
		return nil, authnError(fmt.Errorf("HTTP session is not loaded"))
	}
	ownerToken := s.manager.GetString(ctx, iamauthn.AuthFlowOwnerKey)
	flow, loginSession, actorUserID, err := s.authentication.SubmitPassword(ctx, ownerToken, request.GetFlowId(), request.GetEmail(), request.GetPassword())
	enrichAuthnAudit(ctx, actorUserID)
	if err != nil {
		return nil, authnError(err)
	}
	if err := s.applyFlowSessionState(ctx, flow, loginSession); err != nil {
		return nil, authnError(err)
	}
	return &authnpb.SubmitPasswordResponse{Flow: flow}, nil
}

func (s *AuthnService) CompleteUpdatePassword(
	ctx context.Context,
	request *authnpb.CompleteUpdatePasswordRequest,
) (*authnpb.CompleteUpdatePasswordResponse, error) {
	if !sessions.Loaded(ctx, s.manager) {
		return nil, authnError(fmt.Errorf("HTTP session is not loaded"))
	}
	ownerToken := s.manager.GetString(ctx, iamauthn.AuthFlowOwnerKey)
	flow, loginSession, actorUserID, err := s.authentication.CompleteUpdatePassword(ctx, ownerToken, request.GetFlowId(), request.GetNewPassword())
	enrichAuthnAudit(ctx, actorUserID)
	if err != nil {
		return nil, authnError(err)
	}
	if err := s.applyFlowSessionState(ctx, flow, loginSession); err != nil {
		return nil, authnError(err)
	}
	return &authnpb.CompleteUpdatePasswordResponse{Flow: flow}, nil
}

func (s *AuthnService) ResetAuthFlow(
	ctx context.Context,
	request *authnpb.ResetAuthFlowRequest,
) (*authnpb.ResetAuthFlowResponse, error) {
	if !sessions.Loaded(ctx, s.manager) {
		return nil, authnError(fmt.Errorf("HTTP session is not loaded"))
	}
	ownerToken := s.manager.GetString(ctx, iamauthn.AuthFlowOwnerKey)
	flow, actorUserID, err := s.authentication.ResetAuthFlow(ctx, ownerToken, request.GetFlowId())
	enrichAuthnAudit(ctx, actorUserID)
	if err != nil {
		return nil, authnError(err)
	}
	return &authnpb.ResetAuthFlowResponse{Flow: flow}, nil
}

func (s *AuthnService) CancelAuthFlow(
	ctx context.Context,
	request *authnpb.CancelAuthFlowRequest,
) (*authnpb.CancelAuthFlowResponse, error) {
	if !sessions.Loaded(ctx, s.manager) {
		return nil, authnError(fmt.Errorf("HTTP session is not loaded"))
	}
	ownerToken := s.manager.GetString(ctx, iamauthn.AuthFlowOwnerKey)
	redirectURI, actorUserID, err := s.authentication.CancelAuthFlow(ctx, ownerToken, request.GetFlowId())
	enrichAuthnAudit(ctx, actorUserID)
	if err != nil {
		return nil, authnError(err)
	}
	return &authnpb.CancelAuthFlowResponse{RedirectUri: redirectURI}, nil
}

func (s *AuthnService) applyFlowSessionState(
	ctx context.Context,
	flow *authnpb.AuthFlow,
	loginSession *biz.LoginSession,
) error {
	switch flow.GetState() {
	case authnpb.AuthFlowState_AUTH_FLOW_STATE_COMPLETED:
		if loginSession == nil || loginSession.ID == "" {
			return fmt.Errorf("completed auth flow is missing login session")
		}
		if err := s.manager.RenewToken(ctx); err != nil {
			return err
		}
		s.manager.Put(ctx, iamauthn.LoginReferenceKey, loginSession.ID)
		return nil
	case authnpb.AuthFlowState_AUTH_FLOW_STATE_REQUIRED_ACTION:
		if s.manager.Exists(ctx, iamauthn.LoginReferenceKey) {
			s.manager.Remove(ctx, iamauthn.LoginReferenceKey)
			if err := s.manager.RenewToken(ctx); err != nil {
				return err
			}
		}
		return nil
	default:
		return nil
	}
}

func enrichAuthnAudit(ctx context.Context, userID string) {
	if userID == "" {
		return
	}
	audit.SetActor(ctx, security.Actor{Type: security.ActorTypeHuman, ID: userID})
	audit.SetResource(ctx, "iam.plateau.dev/User", "users/"+userID)
}

func authnError(err error) error {
	switch {
	case errors.Is(err, biz.ErrInvalidCredentials):
		return authnpb.ErrorAuthnErrorReasonInvalidCredentials("credentials rejected").WithCause(err)
	case errors.Is(err, biz.ErrAuthFlowNotFound):
		return authnpb.ErrorAuthnErrorReasonFlowNotFound("authentication flow not found").WithCause(err)
	case errors.Is(err, biz.ErrAuthFlowExpired):
		return authnpb.ErrorAuthnErrorReasonFlowExpired("authentication flow expired").WithCause(err)
	case errors.Is(err, biz.ErrAuthFlowInvalidated):
		return authnpb.ErrorAuthnErrorReasonFlowInvalidated("authentication flow state changed").WithCause(err)
	case errors.Is(err, biz.ErrInvalidPassword):
		return authnpb.ErrorAuthnErrorReasonInvalidPassword("password does not meet requirements").WithCause(err)
	case errors.Is(err, biz.ErrOIDCRequestInvalid):
		return authnpb.ErrorAuthnErrorReasonOidcRequestInvalid("authorization request is invalid or expired").WithCause(err)
	default:
		return kerrors.InternalServer("IAM authentication failed", "authentication failed").WithCause(err)
	}
}
