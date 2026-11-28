package service

import (
	"strings"
	"testing"

	accountpb "github.com/Servora-Kit/plateau/api/gen/go/plateau/iam/account/v1"
	authnpb "github.com/Servora-Kit/plateau/api/gen/go/plateau/iam/authn/v1"
	iamclientv1 "github.com/Servora-Kit/plateau/api/gen/go/plateau/iam/client/v1"
	oidcconfv1 "github.com/Servora-Kit/plateau/api/gen/go/plateau/iam/oidc/conf/v1"
	userpb "github.com/Servora-Kit/plateau/api/gen/go/plateau/iam/user/v1"
	"github.com/Servora-Kit/plateau/security/audit"
	"google.golang.org/protobuf/encoding/protojson"
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
		{name: "register oauth client response secret", redact: func() string {
			return (&iamclientv1.RegisterOAuthClientResponse{
				Client:       &iamclientv1.OAuthClient{Name: "oauthClients/cli-1", ClientId: "cli-1"},
				ClientSecret: "pcs_top_secret_initial_value",
			}).Redact()
		}, secrets: []string{"pcs_top_secret_initial_value"}},
		{name: "create oauth client secret response value", redact: func() string {
			return (&iamclientv1.CreateOAuthClientSecretResponse{
				Secret:      &iamclientv1.OAuthClientSecret{SecretId: "sec-1", SecretPrefix: "pcs_abcd1234"},
				SecretValue: "pcs_abcd1234_rotated_plaintext_secret",
			}).Redact()
		}, secrets: []string{"pcs_abcd1234_rotated_plaintext_secret"}},
		{name: "static oidc config client secret", redact: func() string {
			return (&oidcconfv1.OAuthClient{
				ClientId:     new("admin"),
				ClientSecret: new("static-yaml-client-secret-32-bytes"),
			}).Redact()
		}, secrets: []string{"static-yaml-client-secret-32-bytes"}},
		{name: "audit redact clone strips oauth client secrets", redact: func() string {
			regClone := audit.RedactClone(&iamclientv1.RegisterOAuthClientResponse{
				ClientSecret: "pcs_audit_clone_initial_secret",
			})
			rotClone := audit.RedactClone(&iamclientv1.CreateOAuthClientSecretResponse{
				SecretValue: "pcs_audit_clone_rotated_secret",
			})
			return protojson.Format(regClone) + " " + protojson.Format(rotClone)
		}, secrets: []string{"pcs_audit_clone_initial_secret", "pcs_audit_clone_rotated_secret"}},
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
