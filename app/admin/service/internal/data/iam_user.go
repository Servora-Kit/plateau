package data

import (
	"context"
	"fmt"

	userpb "github.com/Servora-Kit/plateau/api/gen/go/plateau/iam/user/v1"
	"github.com/Servora-Kit/plateau/app/admin/service/internal/biz"
	gogrpc "google.golang.org/grpc"
)

const iamServiceName = "iam.service"

type iamUserRepo struct {
	conn   *gogrpc.ClientConn
	client userpb.UserServiceClient
}

// NewIAMUserRepo dials iam.service via Servora's gRPC Dialer and returns biz.IAMUserRepo with connection cleanup.
func NewIAMUserRepo(data *Data) (biz.IAMUserRepo, func(), error) {
	if data == nil || data.grpcDialer == nil {
		return nil, nil, fmt.Errorf("admin iam user repo: dialer is nil")
	}
	conn, err := data.grpcDialer.Dial(context.Background(), iamServiceName)
	if err != nil {
		return nil, nil, fmt.Errorf("dial %s: %w", iamServiceName, err)
	}
	cleanup := func() { _ = conn.Close() }
	return &iamUserRepo{conn: conn, client: userpb.NewUserServiceClient(conn)}, cleanup, nil
}

func (r *iamUserRepo) ListUsers(ctx context.Context, req *userpb.ListUsersRequest) (*userpb.ListUsersResponse, error) {
	recoverConnTransientFailure(ctx, r.conn)
	return r.client.ListUsers(ctx, req)
}

func (r *iamUserRepo) GetUser(ctx context.Context, req *userpb.GetUserRequest) (*userpb.User, error) {
	recoverConnTransientFailure(ctx, r.conn)
	return r.client.GetUser(ctx, req)
}

func (r *iamUserRepo) CreateUser(ctx context.Context, req *userpb.CreateUserRequest) (*userpb.User, error) {
	recoverConnTransientFailure(ctx, r.conn)
	return r.client.CreateUser(ctx, req)
}

func (r *iamUserRepo) SendUserVerificationEmail(ctx context.Context, req *userpb.SendUserVerificationEmailRequest) error {
	recoverConnTransientFailure(ctx, r.conn)
	_, err := r.client.SendUserVerificationEmail(ctx, req)
	return err
}

func (r *iamUserRepo) UpdateUser(ctx context.Context, req *userpb.UpdateUserRequest) (*userpb.User, error) {
	recoverConnTransientFailure(ctx, r.conn)
	return r.client.UpdateUser(ctx, req)
}

func (r *iamUserRepo) SetUserPassword(ctx context.Context, req *userpb.SetUserPasswordRequest) error {
	recoverConnTransientFailure(ctx, r.conn)
	_, err := r.client.SetUserPassword(ctx, req)
	return err
}

func (r *iamUserRepo) RevokeUserSessions(ctx context.Context, req *userpb.RevokeUserSessionsRequest) error {
	recoverConnTransientFailure(ctx, r.conn)
	_, err := r.client.RevokeUserSessions(ctx, req)
	return err
}

func (r *iamUserRepo) DisableUser(ctx context.Context, req *userpb.DisableUserRequest) (*userpb.User, error) {
	recoverConnTransientFailure(ctx, r.conn)
	resp, err := r.client.DisableUser(ctx, req)
	if err != nil {
		return nil, err
	}
	return resp.GetUser(), nil
}

func (r *iamUserRepo) EnableUser(ctx context.Context, req *userpb.EnableUserRequest) (*userpb.User, error) {
	recoverConnTransientFailure(ctx, r.conn)
	resp, err := r.client.EnableUser(ctx, req)
	if err != nil {
		return nil, err
	}
	return resp.GetUser(), nil
}

func (r *iamUserRepo) DeleteUser(ctx context.Context, req *userpb.DeleteUserRequest) (*userpb.User, error) {
	recoverConnTransientFailure(ctx, r.conn)
	return r.client.DeleteUser(ctx, req)
}

func (r *iamUserRepo) UndeleteUser(ctx context.Context, req *userpb.UndeleteUserRequest) (*userpb.User, error) {
	recoverConnTransientFailure(ctx, r.conn)
	return r.client.UndeleteUser(ctx, req)
}

func (r *iamUserRepo) ExpungeUser(ctx context.Context, req *userpb.ExpungeUserRequest) error {
	recoverConnTransientFailure(ctx, r.conn)
	_, err := r.client.ExpungeUser(ctx, req)
	return err
}
