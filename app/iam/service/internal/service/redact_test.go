package service

import (
	"strings"
	"testing"

	accountpb "github.com/Servora-Kit/plateau/api/gen/go/plateau/iam/account/v1"
	authnpb "github.com/Servora-Kit/plateau/api/gen/go/plateau/iam/authn/v1"
	userpb "github.com/Servora-Kit/plateau/api/gen/go/plateau/iam/user/v1"
)

func TestIAMRequestRedactionDoesNotExposeSecrets(t *testing.T) {
	tests := []struct {
		name    string
		redact  func() string
		secrets []string
	}{
		{name: "submit password flow", redact: func() string {
			return (&authnpb.SubmitPasswordRequest{FlowId: "flow-1", Email: "person@example.com", Password: "login-password"}).Redact()
		}, secrets: []string{"login-password"}},
		{name: "complete update password flow", redact: func() string {
			return (&authnpb.CompleteUpdatePasswordRequest{FlowId: "flow-1", NewPassword: "new-password-secret"}).Redact()
		}, secrets: []string{"new-password-secret"}},
		{name: "registration password and CAP", redact: func() string {
			return (&accountpb.RegisterRequest{Email: "person@example.com", Password: "register-password", CapToken: "cap-secret"}).Redact()
		}, secrets: []string{"register-password", "cap-secret"}},
		{name: "email verification token", redact: func() string { return (&accountpb.VerifyEmailRequest{Token: "verification-secret"}).Redact() }, secrets: []string{"verification-secret"}},
		{name: "password change", redact: func() string {
			return (&accountpb.ChangePasswordRequest{CurrentPassword: "current-password", NewPassword: "new-password"}).Redact()
		}, secrets: []string{"current-password", "new-password"}},
		{name: "password reset token", redact: func() string {
			return (&accountpb.ConfirmPasswordResetRequest{Token: "reset-secret", NewPassword: "reset-password"}).Redact()
		}, secrets: []string{"reset-secret", "reset-password"}},
		{name: "account self-delete password", redact: func() string {
			return (&accountpb.DeleteAccountRequest{CurrentPassword: "delete-password-secret"}).Redact()
		}, secrets: []string{"delete-password-secret"}},
		{name: "admin create user password", redact: func() string {
			return (&userpb.CreateUserRequest{Password: "admin-create-secret"}).Redact()
		}, secrets: []string{"admin-create-secret"}},
		{name: "admin set user password", redact: func() string {
			return (&userpb.SetUserPasswordRequest{Name: "users/user-1", NewPassword: "admin-set-secret"}).Redact()
		}, secrets: []string{"admin-set-secret"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			redacted := test.redact()
			for _, secret := range test.secrets {
				if strings.Contains(redacted, secret) {
					t.Fatalf("redacted request exposes %q: %q", secret, redacted)
				}
			}
		})
	}
}
