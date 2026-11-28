package data

import (
	"context"
	"fmt"

	iamclientpb "github.com/Servora-Kit/plateau/api/gen/go/plateau/iam/client/v1"
	"github.com/Servora-Kit/plateau/app/admin/service/internal/biz"
	gogrpc "google.golang.org/grpc"
)

type iamOAuthClientRepo struct {
	conn   *gogrpc.ClientConn
	client iamclientpb.OAuthClientServiceClient
}

// NewIAMOAuthClientRepo dials iam.service via Servora's gRPC Dialer and returns biz.IAMOAuthClientRepo with connection cleanup.
func NewIAMOAuthClientRepo(data *Data) (biz.IAMOAuthClientRepo, func(), error) {
	if data == nil || data.grpcDialer == nil {
		return nil, nil, fmt.Errorf("admin iam oauth client repo: dialer is nil")
	}
	conn, err := data.grpcDialer.Dial(context.Background(), iamServiceName)
	if err != nil {
		return nil, nil, fmt.Errorf("dial %s: %w", iamServiceName, err)
	}
	cleanup := func() { _ = conn.Close() }
	return &iamOAuthClientRepo{conn: conn, client: iamclientpb.NewOAuthClientServiceClient(conn)}, cleanup, nil
}

func (r *iamOAuthClientRepo) RegisterOAuthClient(ctx context.Context, req *iamclientpb.RegisterOAuthClientRequest) (*iamclientpb.RegisterOAuthClientResponse, error) {
	recoverConnTransientFailure(ctx, r.conn)
	return r.client.RegisterOAuthClient(ctx, req)
}

func (r *iamOAuthClientRepo) GetOAuthClient(ctx context.Context, req *iamclientpb.GetOAuthClientRequest) (*iamclientpb.OAuthClient, error) {
	recoverConnTransientFailure(ctx, r.conn)
	return r.client.GetOAuthClient(ctx, req)
}

func (r *iamOAuthClientRepo) ListOAuthClients(ctx context.Context, req *iamclientpb.ListOAuthClientsRequest) (*iamclientpb.ListOAuthClientsResponse, error) {
	recoverConnTransientFailure(ctx, r.conn)
	return r.client.ListOAuthClients(ctx, req)
}

func (r *iamOAuthClientRepo) UpdateOAuthClient(ctx context.Context, req *iamclientpb.UpdateOAuthClientRequest) (*iamclientpb.OAuthClient, error) {
	recoverConnTransientFailure(ctx, r.conn)
	return r.client.UpdateOAuthClient(ctx, req)
}

func (r *iamOAuthClientRepo) DisableOAuthClient(ctx context.Context, req *iamclientpb.DisableOAuthClientRequest) (*iamclientpb.OAuthClient, error) {
	recoverConnTransientFailure(ctx, r.conn)
	return r.client.DisableOAuthClient(ctx, req)
}

func (r *iamOAuthClientRepo) EnableOAuthClient(ctx context.Context, req *iamclientpb.EnableOAuthClientRequest) (*iamclientpb.OAuthClient, error) {
	recoverConnTransientFailure(ctx, r.conn)
	return r.client.EnableOAuthClient(ctx, req)
}

func (r *iamOAuthClientRepo) DeleteOAuthClient(ctx context.Context, req *iamclientpb.DeleteOAuthClientRequest) error {
	recoverConnTransientFailure(ctx, r.conn)
	_, err := r.client.DeleteOAuthClient(ctx, req)
	return err
}

func (r *iamOAuthClientRepo) CreateOAuthClientSecret(ctx context.Context, req *iamclientpb.CreateOAuthClientSecretRequest) (*iamclientpb.CreateOAuthClientSecretResponse, error) {
	recoverConnTransientFailure(ctx, r.conn)
	return r.client.CreateOAuthClientSecret(ctx, req)
}

func (r *iamOAuthClientRepo) DeleteOAuthClientSecret(ctx context.Context, req *iamclientpb.DeleteOAuthClientSecretRequest) error {
	recoverConnTransientFailure(ctx, r.conn)
	_, err := r.client.DeleteOAuthClientSecret(ctx, req)
	return err
}
