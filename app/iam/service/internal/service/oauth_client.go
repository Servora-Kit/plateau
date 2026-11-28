package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	iamclientv1 "github.com/Servora-Kit/plateau/api/gen/go/plateau/iam/client/v1"
	"github.com/Servora-Kit/plateau/app/iam/service/internal/biz"
	corecrud "github.com/Servora-Kit/plateau/crud"
	"github.com/Servora-Kit/plateau/security/audit"
	kerrors "github.com/go-kratos/kratos/v3/errors"
	"google.golang.org/protobuf/types/known/emptypb"
)

// OAuthClientService exposes administrator-only OAuth 2.0 / OIDC client management over gRPC.
type OAuthClientService struct {
	iamclientv1.UnimplementedOAuthClientServiceServer
	usecase      *biz.OAuthClientUsecase
	plan         *corecrud.ResourcePlan[*iamclientv1.OAuthClient]
	listPreparer *corecrud.ListPreparer
}

// NewOAuthClientService constructs the OAuthClient gRPC service adapter.
func NewOAuthClientService(usecase *biz.OAuthClientUsecase) (*OAuthClientService, error) {
	if usecase == nil {
		return nil, fmt.Errorf("oauth client service: dependency is missing")
	}
	listPreparer, err := corecrud.NewListPreparer()
	if err != nil {
		return nil, fmt.Errorf("create OAuthClient ListPreparer: %w", err)
	}
	return &OAuthClientService{
		usecase:      usecase,
		plan:         corecrud.MustBuildResourcePlan[*iamclientv1.OAuthClient](iamclientv1.OAuthClientCRUDDescriptor()),
		listPreparer: listPreparer,
	}, nil
}

func (s *OAuthClientService) RegisterOAuthClient(
	ctx context.Context,
	request *iamclientv1.RegisterOAuthClientRequest,
) (*iamclientv1.RegisterOAuthClientResponse, error) {
	if request.GetOauthClient() == nil {
		return nil, kerrors.New(400, "INVALID_ARGUMENT", "oauth_client is required")
	}
	prepared, err := s.plan.PrepareCreate(request.GetOauthClient())
	if err != nil {
		return nil, err
	}
	created, rawSecret, err := s.usecase.RegisterOAuthClient(ctx, request.GetOauthClientId(), prepared, request.GetInitialSecretName())
	if err != nil {
		return nil, oauthClientError(err)
	}
	filtered, err := s.plan.ToResponse(created)
	if err != nil {
		return nil, err
	}
	audit.SetResource(ctx, "iam.plateau.dev/OAuthClient", filtered.GetName())
	return &iamclientv1.RegisterOAuthClientResponse{
		Client:       filtered,
		ClientSecret: rawSecret,
	}, nil
}

func (s *OAuthClientService) GetOAuthClient(
	ctx context.Context,
	request *iamclientv1.GetOAuthClientRequest,
) (*iamclientv1.OAuthClient, error) {
	name, err := iamclientv1.ParseOAuthClientName(request.GetName())
	if err != nil {
		return nil, err
	}
	resource, err := s.usecase.GetOAuthClient(ctx, name)
	if err != nil {
		return nil, oauthClientError(err)
	}
	return s.plan.ToResponse(resource)
}

func (s *OAuthClientService) ListOAuthClients(
	ctx context.Context,
	request *iamclientv1.ListOAuthClientsRequest,
) (*iamclientv1.ListOAuthClientsResponse, error) {
	query, err := s.listPreparer.PrepareList(s.plan, corecrud.ListInput{
		Collection:   "oauthClients",
		PageSize:     request.GetPageSize(),
		PageToken:    request.GetPageToken(),
		Filter:       request.GetFilter(),
		OrderBy:      request.GetOrderBy(),
		IncludeTotal: request.GetIncludeTotal(),
	})
	if err != nil {
		return nil, err
	}
	result, err := s.usecase.ListOAuthClients(ctx, query)
	if err != nil {
		return nil, oauthClientError(err)
	}
	resources, err := s.plan.ToResponses(result.Items())
	if err != nil {
		return nil, err
	}
	var total *int64
	if value, ok := result.TotalSize(); ok {
		total = &value
	}
	return &iamclientv1.ListOAuthClientsResponse{
		Clients:       resources,
		NextPageToken: result.NextPageToken(),
		TotalSize:     total,
	}, nil
}

func (s *OAuthClientService) UpdateOAuthClient(
	ctx context.Context,
	request *iamclientv1.UpdateOAuthClientRequest,
) (*iamclientv1.OAuthClient, error) {
	if request.GetOauthClient() == nil {
		return nil, kerrors.New(400, "INVALID_ARGUMENT", "oauth_client is required")
	}
	name, err := iamclientv1.ParseOAuthClientName(request.GetOauthClient().GetName())
	if err != nil {
		return nil, err
	}
	prepared, err := s.plan.PrepareUpdate(
		request.GetOauthClient(),
		request.GetUpdateMask(),
		corecrud.UpdateOptions{Etag: request.GetOauthClient().GetEtag()},
	)
	if err != nil {
		return nil, err
	}
	resource, err := s.usecase.UpdateOAuthClient(ctx, name, prepared)
	if err != nil {
		return nil, oauthClientError(err)
	}
	return s.plan.ToResponse(resource)
}

func (s *OAuthClientService) DisableOAuthClient(
	ctx context.Context,
	request *iamclientv1.DisableOAuthClientRequest,
) (*iamclientv1.OAuthClient, error) {
	name, err := iamclientv1.ParseOAuthClientName(request.GetName())
	if err != nil {
		return nil, err
	}
	resource, err := s.usecase.DisableOAuthClient(ctx, name, request.GetEtag())
	if err != nil {
		return nil, oauthClientError(err)
	}
	return s.plan.ToResponse(resource)
}

func (s *OAuthClientService) EnableOAuthClient(
	ctx context.Context,
	request *iamclientv1.EnableOAuthClientRequest,
) (*iamclientv1.OAuthClient, error) {
	name, err := iamclientv1.ParseOAuthClientName(request.GetName())
	if err != nil {
		return nil, err
	}
	resource, err := s.usecase.EnableOAuthClient(ctx, name, request.GetEtag())
	if err != nil {
		return nil, oauthClientError(err)
	}
	return s.plan.ToResponse(resource)
}

func (s *OAuthClientService) DeleteOAuthClient(
	ctx context.Context,
	request *iamclientv1.DeleteOAuthClientRequest,
) (*emptypb.Empty, error) {
	name, err := iamclientv1.ParseOAuthClientName(request.GetName())
	if err != nil {
		return nil, err
	}
	if err := s.usecase.DeleteOAuthClient(ctx, name, request.GetEtag()); err != nil {
		return nil, oauthClientError(err)
	}
	return &emptypb.Empty{}, nil
}

func (s *OAuthClientService) CreateOAuthClientSecret(
	ctx context.Context,
	request *iamclientv1.CreateOAuthClientSecretRequest,
) (*iamclientv1.CreateOAuthClientSecretResponse, error) {
	name, err := iamclientv1.ParseOAuthClientName(request.GetName())
	if err != nil {
		return nil, err
	}
	var expiresAt *time.Time
	if request.GetExpiresTime() != nil {
		if err := request.GetExpiresTime().CheckValid(); err != nil {
			return nil, oauthClientError(biz.ErrInvalidClientConfig)
		}
		exp := request.GetExpiresTime().AsTime().UTC()
		expiresAt = &exp
	}
	secret, plaintext, err := s.usecase.CreateOAuthClientSecret(ctx, name, request.GetSecretName(), expiresAt)
	if err != nil {
		return nil, oauthClientError(err)
	}
	return &iamclientv1.CreateOAuthClientSecretResponse{
		Secret:      secret,
		SecretValue: plaintext,
	}, nil
}

func (s *OAuthClientService) DeleteOAuthClientSecret(
	ctx context.Context,
	request *iamclientv1.DeleteOAuthClientSecretRequest,
) (*emptypb.Empty, error) {
	name, err := iamclientv1.ParseOAuthClientName(request.GetName())
	if err != nil {
		return nil, err
	}
	if err := s.usecase.DeleteOAuthClientSecret(ctx, name, request.GetSecretId()); err != nil {
		return nil, oauthClientError(err)
	}
	return &emptypb.Empty{}, nil
}

func oauthClientError(err error) error {
	switch {
	case errors.Is(err, biz.ErrNotFound):
		return iamclientv1.ErrorClientErrorReasonNotFound("OAuth client or secret not found").WithCause(err)
	case errors.Is(err, biz.ErrAlreadyExists):
		return iamclientv1.ErrorClientErrorReasonAlreadyExists("OAuth client already exists").WithCause(err)
	case errors.Is(err, biz.ErrStaticClientImmutable):
		return iamclientv1.ErrorClientErrorReasonStaticClientImmutable("static OAuth client is immutable").WithCause(err)
	case errors.Is(err, biz.ErrInvalidClientConfig):
		return iamclientv1.ErrorClientErrorReasonInvalidConfig("OAuth client configuration is invalid").WithCause(err)
	case errors.Is(err, biz.ErrClientInvalidState):
		return iamclientv1.ErrorClientErrorReasonInvalidState("OAuth client lifecycle state is invalid").WithCause(err)
	case errors.Is(err, biz.ErrClientEtagMismatch):
		return iamclientv1.ErrorClientErrorReasonEtagMismatch("OAuth client etag does not match").WithCause(err)
	case errors.Is(err, biz.ErrSecretLimitExceeded):
		return iamclientv1.ErrorClientErrorReasonSecretLimitExceeded("OAuth client secret limit exceeded").WithCause(err)
	case errors.Is(err, biz.ErrLastActiveSecret):
		return iamclientv1.ErrorClientErrorReasonLastActiveSecret("cannot delete the last active OAuth client secret").WithCause(err)
	}
	if _, ok := errors.AsType[*kerrors.Error](err); ok {
		return err
	}
	return kerrors.InternalServer("IAM OAuth client operation failed", "OAuth client operation failed").WithCause(err)
}
