package service

import (
	"context"
	"errors"
	"testing"

	adminuserpb "github.com/Servora-Kit/plateau/api/gen/go/admin/user/v1"
	userpb "github.com/Servora-Kit/plateau/api/gen/go/iam/user/v1"
	securityerrorspb "github.com/Servora-Kit/plateau/api/gen/go/plateau/security/errors/v1"
	"github.com/Servora-Kit/plateau/app/admin/service/internal/biz"
	kerrors "github.com/go-kratos/kratos/v3/errors"
)

func TestUserErrorMatrixMapping(t *testing.T) {
	cases := []struct {
		name       string
		err        error
		wantCode   int32
		wantReason string
	}{
		{
			name:       "canceled maps to 499 CLIENT_CLOSED",
			err:        errors.Join(context.Canceled, errors.New("canceled")),
			wantCode:   499,
			wantReason: "CLIENT_CLOSED",
		},
		{
			name:       "deadline exceeded maps to 504 GATEWAY_TIMEOUT",
			err:        errors.Join(context.DeadlineExceeded, errors.New("timeout")),
			wantCode:   504,
			wantReason: "GATEWAY_TIMEOUT",
		},
		{
			name:       "unauthenticated maps to 401 SECURITY_ERROR_REASON_UNAUTHENTICATED",
			err:        biz.ErrUnauthenticated,
			wantCode:   401,
			wantReason: securityerrorspb.SecurityErrorReason_SECURITY_ERROR_REASON_UNAUTHENTICATED.String(),
		},
		{
			name:       "self action forbidden maps to 403 SELF_ACTION_FORBIDDEN",
			err:        biz.ErrSelfActionForbidden,
			wantCode:   403,
			wantReason: adminuserpb.AdminUserErrorReason_ADMIN_USER_ERROR_REASON_SELF_ACTION_FORBIDDEN.String(),
		},
		{
			name:       "target is admin maps to 403 TARGET_IS_ADMIN",
			err:        biz.ErrTargetIsAdmin,
			wantCode:   403,
			wantReason: adminuserpb.AdminUserErrorReason_ADMIN_USER_ERROR_REASON_TARGET_IS_ADMIN.String(),
		},
		{
			name:       "invalid update mask maps to 400 INVALID_ARGUMENT",
			err:        biz.ErrInvalidUpdateMask,
			wantCode:   400,
			wantReason: "INVALID_ARGUMENT",
		},
		{
			name:       "downstream IAM not found maps to 404 USER_NOT_FOUND",
			err:        userpb.ErrorUserErrorReasonNotFound("not found"),
			wantCode:   404,
			wantReason: adminuserpb.AdminUserErrorReason_ADMIN_USER_ERROR_REASON_USER_NOT_FOUND.String(),
		},
		{
			name:       "downstream IAM already exists maps to 409 USER_ALREADY_EXISTS",
			err:        userpb.ErrorUserErrorReasonAlreadyExists("exists"),
			wantCode:   409,
			wantReason: adminuserpb.AdminUserErrorReason_ADMIN_USER_ERROR_REASON_USER_ALREADY_EXISTS.String(),
		},
		{
			name:       "downstream IAM invalid state maps to 409 INVALID_STATE",
			err:        userpb.ErrorUserErrorReasonInvalidState("bad state"),
			wantCode:   409,
			wantReason: adminuserpb.AdminUserErrorReason_ADMIN_USER_ERROR_REASON_INVALID_STATE.String(),
		},
		{
			name:       "downstream IAM etag mismatch maps to 409 ETAG_MISMATCH",
			err:        userpb.ErrorUserErrorReasonEtagMismatch("stale etag"),
			wantCode:   409,
			wantReason: adminuserpb.AdminUserErrorReason_ADMIN_USER_ERROR_REASON_ETAG_MISMATCH.String(),
		},
		{
			name:       "downstream IAM verification email failed maps to 503 VERIFICATION_EMAIL_FAILED",
			err:        userpb.ErrorUserErrorReasonVerificationEmailFailed("smtp error"),
			wantCode:   503,
			wantReason: adminuserpb.AdminUserErrorReason_ADMIN_USER_ERROR_REASON_VERIFICATION_EMAIL_FAILED.String(),
		},
		{
			name:       "downstream 401/403 service credential failure isolates to 503 DOWNSTREAM_UNAVAILABLE",
			err:        securityerrorspb.ErrorSecurityErrorReasonPermissionDenied("service:admin denied"),
			wantCode:   503,
			wantReason: adminuserpb.AdminUserErrorReason_ADMIN_USER_ERROR_REASON_DOWNSTREAM_UNAVAILABLE.String(),
		},
		{
			name:       "unknown internal error maps to 503 DOWNSTREAM_UNAVAILABLE without leaking raw message",
			err:        errors.New("raw sql/secret internal failure"),
			wantCode:   503,
			wantReason: adminuserpb.AdminUserErrorReason_ADMIN_USER_ERROR_REASON_DOWNSTREAM_UNAVAILABLE.String(),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mapped := userError(context.Background(), tc.err)
			se := kerrors.FromError(mapped)
			if se == nil {
				t.Fatalf("userError(%v) returned non-Kratos error: %v", tc.err, mapped)
			}
			if se.Code != tc.wantCode || se.Reason != tc.wantReason {
				t.Fatalf("userError(%v) = (%d, %q), want (%d, %q)", tc.err, se.Code, se.Reason, tc.wantCode, tc.wantReason)
			}
			if se.Message == "raw sql/secret internal failure" {
				t.Fatal("userError leaked raw internal error message")
			}
		})
	}
}
