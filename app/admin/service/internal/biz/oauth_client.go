package biz

import (
	"context"
	"fmt"

	adminclientpb "github.com/Servora-Kit/plateau/api/gen/go/plateau/admin/client/v1"
	iamclientpb "github.com/Servora-Kit/plateau/api/gen/go/plateau/iam/client/v1"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/fieldmaskpb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// IAMOAuthClientRepo abstracts downstream IAM OAuthClientService gRPC operations.
type IAMOAuthClientRepo interface {
	RegisterOAuthClient(ctx context.Context, req *iamclientpb.RegisterOAuthClientRequest) (*iamclientpb.RegisterOAuthClientResponse, error)
	GetOAuthClient(ctx context.Context, req *iamclientpb.GetOAuthClientRequest) (*iamclientpb.OAuthClient, error)
	ListOAuthClients(ctx context.Context, req *iamclientpb.ListOAuthClientsRequest) (*iamclientpb.ListOAuthClientsResponse, error)
	UpdateOAuthClient(ctx context.Context, req *iamclientpb.UpdateOAuthClientRequest) (*iamclientpb.OAuthClient, error)
	DisableOAuthClient(ctx context.Context, req *iamclientpb.DisableOAuthClientRequest) (*iamclientpb.OAuthClient, error)
	EnableOAuthClient(ctx context.Context, req *iamclientpb.EnableOAuthClientRequest) (*iamclientpb.OAuthClient, error)
	DeleteOAuthClient(ctx context.Context, req *iamclientpb.DeleteOAuthClientRequest) error
	CreateOAuthClientSecret(ctx context.Context, req *iamclientpb.CreateOAuthClientSecretRequest) (*iamclientpb.CreateOAuthClientSecretResponse, error)
	DeleteOAuthClientSecret(ctx context.Context, req *iamclientpb.DeleteOAuthClientSecretRequest) error
}

// OAuthClientUsecase orchestrates IAM OAuth 2.0 / OIDC client lifecycle and secret rotation for platform administrators.
type OAuthClientUsecase struct {
	repo IAMOAuthClientRepo
}

// NewOAuthClientUsecase constructs OAuthClientUsecase backed by IAMOAuthClientRepo.
func NewOAuthClientUsecase(repo IAMOAuthClientRepo) (*OAuthClientUsecase, error) {
	if repo == nil {
		return nil, fmt.Errorf("admin oauth client usecase: repo is nil")
	}
	return &OAuthClientUsecase{repo: repo}, nil
}

func (uc *OAuthClientUsecase) ListOAuthClients(ctx context.Context, req *adminclientpb.ListOAuthClientsRequest) (*adminclientpb.ListOAuthClientsResponse, error) {
	if _, err := requireHumanActor(ctx); err != nil {
		return nil, err
	}
	resp, err := uc.repo.ListOAuthClients(ctx, &iamclientpb.ListOAuthClientsRequest{
		PageSize:     req.GetPageSize(),
		PageToken:    req.GetPageToken(),
		Filter:       req.GetFilter(),
		OrderBy:      req.GetOrderBy(),
		IncludeTotal: req.GetIncludeTotal(),
	})
	if err != nil {
		return nil, err
	}
	return &adminclientpb.ListOAuthClientsResponse{
		Clients:       resp.GetClients(),
		NextPageToken: resp.GetNextPageToken(),
		TotalSize:     resp.TotalSize,
	}, nil
}

func (uc *OAuthClientUsecase) GetOAuthClient(ctx context.Context, name iamclientpb.OAuthClientName) (*iamclientpb.OAuthClient, error) {
	formatted, err := prepareClientCall(ctx, name)
	if err != nil {
		return nil, err
	}
	return uc.repo.GetOAuthClient(ctx, &iamclientpb.GetOAuthClientRequest{Name: formatted})
}

func (uc *OAuthClientUsecase) CreateOAuthClient(ctx context.Context, req *adminclientpb.CreateOAuthClientRequest) (*adminclientpb.CreateOAuthClientResponse, error) {
	if _, err := requireHumanActor(ctx); err != nil {
		return nil, err
	}
	resp, err := uc.repo.RegisterOAuthClient(ctx, &iamclientpb.RegisterOAuthClientRequest{
		OauthClient:       req.GetOauthClient(),
		OauthClientId:     req.GetOauthClientId(),
		InitialSecretName: req.GetInitialSecretName(),
	})
	if err != nil {
		return nil, err
	}
	return &adminclientpb.CreateOAuthClientResponse{
		Client:       resp.GetClient(),
		ClientSecret: resp.GetClientSecret(),
	}, nil
}

func (uc *OAuthClientUsecase) UpdateOAuthClient(ctx context.Context, name iamclientpb.OAuthClientName, client *iamclientpb.OAuthClient, mask *fieldmaskpb.FieldMask) (*iamclientpb.OAuthClient, error) {
	formatted, err := prepareClientCall(ctx, name)
	if err != nil {
		return nil, err
	}
	sanitized := proto.CloneOf(client)
	if sanitized == nil {
		sanitized = &iamclientpb.OAuthClient{}
	}
	sanitized.Name = formatted

	return uc.repo.UpdateOAuthClient(ctx, &iamclientpb.UpdateOAuthClientRequest{
		OauthClient: sanitized,
		UpdateMask:  mask,
	})
}

func (uc *OAuthClientUsecase) DisableOAuthClient(ctx context.Context, name iamclientpb.OAuthClientName, etag string) (*iamclientpb.OAuthClient, error) {
	formatted, err := prepareClientCall(ctx, name)
	if err != nil {
		return nil, err
	}
	return uc.repo.DisableOAuthClient(ctx, &iamclientpb.DisableOAuthClientRequest{
		Name: formatted,
		Etag: etag,
	})
}

func (uc *OAuthClientUsecase) EnableOAuthClient(ctx context.Context, name iamclientpb.OAuthClientName, etag string) (*iamclientpb.OAuthClient, error) {
	formatted, err := prepareClientCall(ctx, name)
	if err != nil {
		return nil, err
	}
	return uc.repo.EnableOAuthClient(ctx, &iamclientpb.EnableOAuthClientRequest{
		Name: formatted,
		Etag: etag,
	})
}

func (uc *OAuthClientUsecase) DeleteOAuthClient(ctx context.Context, name iamclientpb.OAuthClientName, etag string) error {
	formatted, err := prepareClientCall(ctx, name)
	if err != nil {
		return err
	}
	return uc.repo.DeleteOAuthClient(ctx, &iamclientpb.DeleteOAuthClientRequest{
		Name: formatted,
		Etag: etag,
	})
}

func (uc *OAuthClientUsecase) CreateOAuthClientSecret(ctx context.Context, name iamclientpb.OAuthClientName, secretName string, expiresTime *timestamppb.Timestamp) (*adminclientpb.CreateOAuthClientSecretResponse, error) {
	formatted, err := prepareClientCall(ctx, name)
	if err != nil {
		return nil, err
	}
	resp, err := uc.repo.CreateOAuthClientSecret(ctx, &iamclientpb.CreateOAuthClientSecretRequest{
		Name:        formatted,
		SecretName:  secretName,
		ExpiresTime: expiresTime,
	})
	if err != nil {
		return nil, err
	}
	return &adminclientpb.CreateOAuthClientSecretResponse{
		Secret:      resp.GetSecret(),
		SecretValue: resp.GetSecretValue(),
	}, nil
}

func (uc *OAuthClientUsecase) DeleteOAuthClientSecret(ctx context.Context, name iamclientpb.OAuthClientName, secretID string) error {
	formatted, err := prepareClientCall(ctx, name)
	if err != nil {
		return err
	}
	return uc.repo.DeleteOAuthClientSecret(ctx, &iamclientpb.DeleteOAuthClientSecretRequest{
		Name:     formatted,
		SecretId: secretID,
	})
}

func prepareClientCall(ctx context.Context, name iamclientpb.OAuthClientName) (string, error) {
	if _, err := requireHumanActor(ctx); err != nil {
		return "", err
	}
	return name.Format()
}
