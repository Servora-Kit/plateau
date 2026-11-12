package data

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	userpb "github.com/Servora-Kit/plateau/api/gen/go/iam/user/v1"
	"github.com/Servora-Kit/plateau/app/admin/service/internal/biz"
	securityoidc "github.com/Servora-Kit/plateau/security/oidc"
	kerrors "github.com/go-kratos/kratos/v3/errors"
	gogrpc "google.golang.org/grpc"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
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
	if r.conn.GetState() == connectivity.TransientFailure {
		r.conn.ResetConnectBackoff()
		waitCtx, cancel := context.WithTimeout(ctx, 250*time.Millisecond)
		for r.conn.GetState() != connectivity.Ready && waitCtx.Err() == nil {
			if !r.conn.WaitForStateChange(waitCtx, r.conn.GetState()) {
				break
			}
		}
		cancel()
	}
	resp, err := r.client.ListUsers(ctx, req)
	if err != nil {
		return nil, translateIAMError(ctx, err)
	}
	return resp, nil
}

func (r *iamUserRepo) GetUser(ctx context.Context, req *userpb.GetUserRequest) (*userpb.User, error) {
	resp, err := r.client.GetUser(ctx, req)
	if err != nil {
		return nil, translateIAMError(ctx, err)
	}
	return resp, nil
}

func (r *iamUserRepo) CreateUser(ctx context.Context, req *userpb.CreateUserRequest) (*userpb.User, error) {
	resp, err := r.client.CreateUser(ctx, req)
	if err != nil {
		return nil, translateIAMError(ctx, err)
	}
	return resp, nil
}

func (r *iamUserRepo) SendUserVerificationEmail(ctx context.Context, req *userpb.SendUserVerificationEmailRequest) error {
	if _, err := r.client.SendUserVerificationEmail(ctx, req); err != nil {
		return translateIAMError(ctx, err)
	}
	return nil
}

func (r *iamUserRepo) UpdateUser(ctx context.Context, req *userpb.UpdateUserRequest) (*userpb.User, error) {
	resp, err := r.client.UpdateUser(ctx, req)
	if err != nil {
		return nil, translateIAMError(ctx, err)
	}
	return resp, nil
}

func (r *iamUserRepo) SetUserPassword(ctx context.Context, req *userpb.SetUserPasswordRequest) error {
	if _, err := r.client.SetUserPassword(ctx, req); err != nil {
		return translateIAMError(ctx, err)
	}
	return nil
}

func (r *iamUserRepo) RevokeUserSessions(ctx context.Context, req *userpb.RevokeUserSessionsRequest) error {
	if _, err := r.client.RevokeUserSessions(ctx, req); err != nil {
		return translateIAMError(ctx, err)
	}
	return nil
}

func (r *iamUserRepo) DisableUser(ctx context.Context, req *userpb.DisableUserRequest) (*userpb.User, error) {
	resp, err := r.client.DisableUser(ctx, req)
	if err != nil {
		return nil, translateIAMError(ctx, err)
	}
	return resp.GetUser(), nil
}

func (r *iamUserRepo) EnableUser(ctx context.Context, req *userpb.EnableUserRequest) (*userpb.User, error) {
	resp, err := r.client.EnableUser(ctx, req)
	if err != nil {
		return nil, translateIAMError(ctx, err)
	}
	return resp.GetUser(), nil
}

func (r *iamUserRepo) DeleteUser(ctx context.Context, req *userpb.DeleteUserRequest) (*userpb.User, error) {
	resp, err := r.client.DeleteUser(ctx, req)
	if err != nil {
		return nil, translateIAMError(ctx, err)
	}
	return resp, nil
}

func (r *iamUserRepo) UndeleteUser(ctx context.Context, req *userpb.UndeleteUserRequest) (*userpb.User, error) {
	resp, err := r.client.UndeleteUser(ctx, req)
	if err != nil {
		return nil, translateIAMError(ctx, err)
	}
	return resp, nil
}

func (r *iamUserRepo) ExpungeUser(ctx context.Context, req *userpb.ExpungeUserRequest) error {
	if _, err := r.client.ExpungeUser(ctx, req); err != nil {
		return translateIAMError(ctx, err)
	}
	return nil
}

func translateIAMError(ctx context.Context, err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(ctx.Err(), context.Canceled) {
		return errors.Join(context.Canceled, err)
	}
	grpcCode := status.Code(err)
	if errors.Is(ctx.Err(), context.DeadlineExceeded) || errors.Is(err, context.DeadlineExceeded) || grpcCode == codes.DeadlineExceeded {
		return errors.Join(context.DeadlineExceeded, err)
	}
	if errors.Is(err, context.Canceled) {
		return errors.Join(context.Canceled, err)
	}

	switch {
	case userpb.IsUserErrorReasonNotFound(err):
		return errors.Join(biz.ErrUserNotFound, err)
	case userpb.IsUserErrorReasonAlreadyExists(err):
		return errors.Join(biz.ErrUserAlreadyExists, err)
	case userpb.IsUserErrorReasonInvalidState(err):
		return errors.Join(biz.ErrUserInvalidState, err)
	case userpb.IsUserErrorReasonEtagMismatch(err):
		return errors.Join(biz.ErrUserEtagMismatch, err)
	case userpb.IsUserErrorReasonVerificationEmailFailed(err):
		return errors.Join(biz.ErrVerificationEmailFailed, err)
	}

	if se := kerrors.FromError(err); se != nil {
		if strings.HasPrefix(se.Reason, "CRUD_ERROR_REASON_") {
			return kerrors.BadRequest(se.Reason, "invalid request parameters").WithCause(err)
		}
		if se.Code == 400 || grpcCode == codes.InvalidArgument {
			return kerrors.BadRequest("INVALID_ARGUMENT", "invalid request argument").WithCause(err)
		}
	}

	if errors.Is(err, securityoidc.ErrTokenSourceUnavailable) {
		return errors.Join(biz.ErrDownstreamUnavailable, err)
	}
	return errors.Join(biz.ErrDownstreamUnavailable, err)
}
