package service

import (
	"context"
	"errors"
	"fmt"
	"strings"

	adminclientpb "github.com/Servora-Kit/plateau/api/gen/go/plateau/admin/client/v1"
	iamclientpb "github.com/Servora-Kit/plateau/api/gen/go/plateau/iam/client/v1"
	securityerrorspb "github.com/Servora-Kit/plateau/api/gen/go/plateau/security/errors/v1"
	"github.com/Servora-Kit/plateau/app/admin/service/internal/biz"
	"github.com/Servora-Kit/plateau/security/audit"
	kerrors "github.com/go-kratos/kratos/v3/errors"
	"google.golang.org/protobuf/types/known/emptypb"
)

const oauthClientResourceType = "iam.plateau.dev/OAuthClient"

var (
	_ adminclientpb.OAuthClientServiceServer     = (*OAuthClientService)(nil)
	_ adminclientpb.OAuthClientServiceHTTPServer = (*OAuthClientService)(nil)
)

// OAuthClientService implements plateau.admin.client.v1.OAuthClientServiceServer and HTTP handlers.
type OAuthClientService struct {
	adminclientpb.UnimplementedOAuthClientServiceServer
	uc *biz.OAuthClientUsecase
}

// NewOAuthClientService constructs OAuthClientService with canonical resource validation and domain error mapping.
func NewOAuthClientService(uc *biz.OAuthClientUsecase) (*OAuthClientService, error) {
	if uc == nil {
		return nil, fmt.Errorf("admin oauth client service: usecase is nil")
	}
	return &OAuthClientService{uc: uc}, nil
}

func (s *OAuthClientService) ListOAuthClients(ctx context.Context, req *adminclientpb.ListOAuthClientsRequest) (*adminclientpb.ListOAuthClientsResponse, error) {
	resp, err := s.uc.ListOAuthClients(ctx, req)
	if err != nil {
		return nil, oauthClientError(ctx, err)
	}
	return resp, nil
}

func (s *OAuthClientService) GetOAuthClient(ctx context.Context, req *adminclientpb.GetOAuthClientRequest) (*iamclientpb.OAuthClient, error) {
	name, err := iamclientpb.ParseOAuthClientName(req.GetName())
	if err != nil {
		return nil, err
	}
	client, err := s.uc.GetOAuthClient(ctx, name)
	if err != nil {
		return nil, oauthClientError(ctx, err)
	}
	return client, nil
}

func (s *OAuthClientService) CreateOAuthClient(ctx context.Context, req *adminclientpb.CreateOAuthClientRequest) (*adminclientpb.CreateOAuthClientResponse, error) {
	if req.GetOauthClient() == nil {
		return nil, kerrors.BadRequest("INVALID_ARGUMENT", "oauth_client is required")
	}
	if candidateID := strings.TrimSpace(req.GetOauthClientId()); candidateID != "" {
		if candidateName, err := iamclientpb.NewOAuthClientName(candidateID).Format(); err == nil {
			audit.SetResource(ctx, oauthClientResourceType, candidateName)
		}
	}
	resp, err := s.uc.CreateOAuthClient(ctx, req)
	if err != nil {
		return nil, oauthClientError(ctx, err)
	}
	if createdName := resp.GetClient().GetName(); createdName != "" {
		audit.SetResource(ctx, oauthClientResourceType, createdName)
	}
	return resp, nil
}

func (s *OAuthClientService) UpdateOAuthClient(ctx context.Context, req *adminclientpb.UpdateOAuthClientRequest) (*iamclientpb.OAuthClient, error) {
	if req.GetOauthClient() == nil {
		return nil, kerrors.BadRequest("INVALID_ARGUMENT", "oauth_client is required")
	}
	name, err := parseAndAuditOAuthClientName(ctx, req.GetOauthClient().GetName())
	if err != nil {
		return nil, err
	}
	updated, err := s.uc.UpdateOAuthClient(ctx, name, req.GetOauthClient(), req.GetUpdateMask())
	if err != nil {
		return nil, oauthClientError(ctx, err)
	}
	return updated, nil
}

func (s *OAuthClientService) DisableOAuthClient(ctx context.Context, req *adminclientpb.DisableOAuthClientRequest) (*iamclientpb.OAuthClient, error) {
	name, err := parseAndAuditOAuthClientName(ctx, req.GetName())
	if err != nil {
		return nil, err
	}
	disabled, err := s.uc.DisableOAuthClient(ctx, name, req.GetEtag())
	if err != nil {
		return nil, oauthClientError(ctx, err)
	}
	return disabled, nil
}

func (s *OAuthClientService) EnableOAuthClient(ctx context.Context, req *adminclientpb.EnableOAuthClientRequest) (*iamclientpb.OAuthClient, error) {
	name, err := parseAndAuditOAuthClientName(ctx, req.GetName())
	if err != nil {
		return nil, err
	}
	enabled, err := s.uc.EnableOAuthClient(ctx, name, req.GetEtag())
	if err != nil {
		return nil, oauthClientError(ctx, err)
	}
	return enabled, nil
}

func (s *OAuthClientService) DeleteOAuthClient(ctx context.Context, req *adminclientpb.DeleteOAuthClientRequest) (*emptypb.Empty, error) {
	name, err := parseAndAuditOAuthClientName(ctx, req.GetName())
	if err != nil {
		return nil, err
	}
	if err := s.uc.DeleteOAuthClient(ctx, name, req.GetEtag()); err != nil {
		return nil, oauthClientError(ctx, err)
	}
	return &emptypb.Empty{}, nil
}

func (s *OAuthClientService) CreateOAuthClientSecret(ctx context.Context, req *adminclientpb.CreateOAuthClientSecretRequest) (*adminclientpb.CreateOAuthClientSecretResponse, error) {
	name, err := parseAndAuditOAuthClientName(ctx, req.GetName())
	if err != nil {
		return nil, err
	}
	if req.GetExpiresTime() != nil {
		if tsErr := req.GetExpiresTime().CheckValid(); tsErr != nil {
			return nil, adminclientpb.ErrorAdminClientErrorReasonInvalidConfig("invalid expires_time").WithCause(tsErr)
		}
	}
	resp, err := s.uc.CreateOAuthClientSecret(ctx, name, req.GetSecretName(), req.GetExpiresTime())
	if err != nil {
		return nil, oauthClientError(ctx, err)
	}
	return resp, nil
}

func (s *OAuthClientService) DeleteOAuthClientSecret(ctx context.Context, req *adminclientpb.DeleteOAuthClientSecretRequest) (*emptypb.Empty, error) {
	name, err := parseAndAuditOAuthClientName(ctx, req.GetName())
	if err != nil {
		return nil, err
	}
	secretID := strings.TrimSpace(req.GetSecretId())
	if secretID == "" {
		return nil, kerrors.BadRequest("INVALID_ARGUMENT", "secret_id is required")
	}
	if err := s.uc.DeleteOAuthClientSecret(ctx, name, secretID); err != nil {
		return nil, oauthClientError(ctx, err)
	}
	return &emptypb.Empty{}, nil
}

func parseAndAuditOAuthClientName(ctx context.Context, raw string) (iamclientpb.OAuthClientName, error) {
	name, err := iamclientpb.ParseOAuthClientName(raw)
	if err != nil {
		return iamclientpb.OAuthClientName{}, err
	}
	audit.SetResource(ctx, oauthClientResourceType, "oauthClients/"+name.OauthClient)
	return name, nil
}

func oauthClientError(ctx context.Context, err error) error {
	ctxErr := ctx.Err()
	se := kerrors.FromError(err)

	switch {
	case errors.Is(ctxErr, context.Canceled):
		return kerrors.ClientClosed("CLIENT_CLOSED", "client closed request").WithCause(err)
	case errors.Is(ctxErr, context.DeadlineExceeded):
		return kerrors.GatewayTimeout("GATEWAY_TIMEOUT", "request deadline exceeded").WithCause(err)
	case errors.Is(err, biz.ErrUnauthenticated):
		return securityerrorspb.ErrorSecurityErrorReasonUnauthenticated("authentication required").WithCause(err)
	case iamclientpb.IsClientErrorReasonNotFound(err):
		return adminclientpb.ErrorAdminClientErrorReasonNotFound("oauth client or secret not found").WithCause(err)
	case iamclientpb.IsClientErrorReasonAlreadyExists(err):
		return adminclientpb.ErrorAdminClientErrorReasonAlreadyExists("oauth client already exists").WithCause(err)
	case iamclientpb.IsClientErrorReasonStaticClientImmutable(err):
		return adminclientpb.ErrorAdminClientErrorReasonStaticClientImmutable("static oauth client is managed by configuration and cannot be modified").WithCause(err)
	case iamclientpb.IsClientErrorReasonInvalidConfig(err):
		msg := "invalid oauth client configuration"
		if se != nil && strings.TrimSpace(se.Message) != "" {
			msg = se.Message
		}
		return adminclientpb.ErrorAdminClientErrorReasonInvalidConfig("%s", msg).WithCause(err)
	case iamclientpb.IsClientErrorReasonInvalidState(err):
		return adminclientpb.ErrorAdminClientErrorReasonInvalidState("oauth client state does not allow this operation").WithCause(err)
	case iamclientpb.IsClientErrorReasonEtagMismatch(err):
		return adminclientpb.ErrorAdminClientErrorReasonEtagMismatch("oauth client etag does not match").WithCause(err)
	case iamclientpb.IsClientErrorReasonSecretLimitExceeded(err):
		return adminclientpb.ErrorAdminClientErrorReasonSecretLimitExceeded("oauth client secret limit exceeded").WithCause(err)
	case iamclientpb.IsClientErrorReasonLastActiveSecret(err):
		return adminclientpb.ErrorAdminClientErrorReasonLastActiveSecret("cannot delete the last active secret of a confidential oauth client").WithCause(err)
	case se != nil && strings.HasPrefix(se.Reason, "CRUD_ERROR_REASON_") && se.Code == 400:
		msg := "invalid request parameters"
		if strings.TrimSpace(se.Message) != "" {
			msg = se.Message
		}
		return kerrors.BadRequest(se.Reason, msg).WithCause(err)
	case se != nil && se.Code == 400:
		return kerrors.BadRequest("INVALID_ARGUMENT", "invalid request argument").WithCause(err)
	default:
		return adminclientpb.ErrorAdminClientErrorReasonDownstreamUnavailable("downstream identity service is temporarily unavailable").WithCause(err)
	}
}
