package service

import (
	"strings"
	"testing"

	accountpb "github.com/Servora-Kit/plateau/api/gen/go/iam/account/v1"
	authnpb "github.com/Servora-Kit/plateau/api/gen/go/iam/authn/v1"
)

func TestIAMRequestRedactionDoesNotExposeSecrets(t *testing.T) {
	tests := []struct {
		name    string
		redact  func() string
		secrets []string
	}{
		{name: "login password", redact: func() string {
			return (&authnpb.LoginRequest{Email: "person@example.com", Password: "login-password"}).Redact()
		}, secrets: []string{"login-password"}},
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
