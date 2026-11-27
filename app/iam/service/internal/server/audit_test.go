package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/Servora-Kit/plateau/security/audit"
)

func TestIAMAccountAndSessionAuditEnrichmentAndRedaction(t *testing.T) {
	f := newServerFixture(t)

	const (
		regPass      = "{{SECRET_0011regpass1234}}"
		resetNewPass = "{{SECRET_0012resetpass1234}}"
		changedPass  = "{{SECRET_0013changedpass12}}"
	)

	// 1. Register + VerifyEmail -> Register is resource category (anonymous), VerifyEmail sets human actor + user resource.
	regProof := f.capProof(t)
	if w := f.request("POST", "/v1/iam/account/register",
		fmt.Sprintf(`{"email":"auditee@example.com","password":%q,"capToken":%q}`, regPass, regProof), nil); w.Code != http.StatusOK {
		t.Fatalf("register status=%d body=%s", w.Code, w.Body)
	}
	verifyURL, err := url.Parse(f.mailer.verification[len(f.mailer.verification)-1])
	check(t, err)
	verifyToken := verifyURL.Fragment
	if w := f.request("POST", "/v1/iam/account/verify-email", fmt.Sprintf(`{"token":%q}`, verifyToken), nil); w.Code != http.StatusOK {
		t.Fatalf("verify-email status=%d body=%s", w.Code, w.Body)
	}

	// 2. Anti-enumeration RequestPasswordReset -> anonymous + empty resource; ConfirmPasswordReset -> human actor + user resource.
	resetProof := f.capProof(t)
	if w := f.request("POST", "/v1/iam/account/password-reset/request",
		fmt.Sprintf(`{"email":"auditee@example.com","capToken":%q}`, resetProof), nil); w.Code != http.StatusOK {
		t.Fatalf("password-reset/request status=%d body=%s", w.Code, w.Body)
	}
	resetURL, err := url.Parse(f.mailer.reset[len(f.mailer.reset)-1])
	check(t, err)
	resetToken := resetURL.Fragment
	if w := f.request("POST", "/v1/iam/account/password-reset/confirm",
		fmt.Sprintf(`{"token":%q,"newPassword":%q}`, resetToken, resetNewPass), nil); w.Code != http.StatusOK {
		t.Fatalf("password-reset/confirm status=%d body=%s", w.Code, w.Body)
	}
	// Failed ChangeInitialPassword stays anonymous (with email retained in redacted request data); successful completion enriches human actor & user resource.
	if w := f.request("POST", "/v1/iam/account/change-initial-password",
		fmt.Sprintf(`{"email":"auditee@example.com","currentPassword":%q,"newPassword":%q}`, resetNewPass, changedPass), nil); w.Code != http.StatusConflict {
		t.Fatalf("change-initial-password(no required action) status=%d, want 409; body=%s", w.Code, w.Body)
	}
	auditee, err := f.users.FindByEmail(t.Context(), "auditee@example.com")
	check(t, err)
	check(t, f.ent.User.UpdateOneID(auditee.GetUserId()).SetRequiredActions([]string{"UPDATE_PASSWORD"}).Exec(t.Context()))
	if w := f.request("POST", "/v1/iam/account/change-initial-password",
		fmt.Sprintf(`{"email":"auditee@example.com","currentPassword":%q,"newPassword":%q}`, resetNewPass, changedPass), nil); w.Code != http.StatusOK {
		t.Fatalf("change-initial-password(success) status=%d, want 200; body=%s", w.Code, w.Body)
	}
	// 3. Login + GetProfile (disabled -> 0 events) + UpdateProfile + ChangePassword + Logout + DeleteAccount.
	cookie := f.login(t, testPassword, nil)
	if w := f.request("GET", "/v1/iam/account/profile", "", cookie); w.Code != http.StatusOK {
		t.Fatalf("get profile status=%d", w.Code)
	}
	if w := f.request("PATCH", "/v1/iam/account/profile?updateMask=nickname", `{"nickname":"AliceAudited"}`, cookie); w.Code != http.StatusOK {
		t.Fatalf("update profile status=%d body=%s", w.Code, w.Body)
	}
	changeResp := f.request("POST", "/v1/iam/account/change-password",
		fmt.Sprintf(`{"currentPassword":%q,"newPassword":%q}`, testPassword, changedPass), cookie)
	if changeResp.Code != http.StatusOK {
		t.Fatalf("change-password status=%d body=%s", changeResp.Code, changeResp.Body)
	}
	for _, ck := range changeResp.Result().Cookies() {
		if ck.Name == f.manager.Cookie.Name && ck.Value != "" {
			cookie = ck
		}
	}
	if w := f.request("POST", "/v1/iam/sessions/current:logout", `{}`, cookie); w.Code != http.StatusOK {
		t.Fatalf("logout status=%d body=%s", w.Code, w.Body)
	}
	reloginCookie := f.login(t, changedPass, nil)
	if w := f.request("POST", "/v1/iam/account/delete", fmt.Sprintf(`{"currentPassword":%q}`, changedPass), reloginCookie); w.Code != http.StatusOK {
		t.Fatalf("delete account status=%d body=%s", w.Code, w.Body)
	}

	events := f.auditor.snapshot()
	if len(events) == 0 {
		t.Fatal("expected recorded IAM audit events")
	}

	byOp := make(map[string]int)
	for _, ev := range events {
		byOp[ev.Subject()]++
		if ev.Subject() == "/plateau.iam.account.v1.AccountService/GetProfile" {
			t.Fatalf("GetProfile is AUDIT_MODE_DISABLED and must not emit events: %+v", ev)
		}
		if ev.Subject() == "/plateau.iam.account.v1.AccountService/RequestPasswordReset" {
			if ev.Extensions()[audit.ExtActorType] != "anonymous" || ev.Extensions()[audit.ExtResourceName] != nil {
				t.Fatalf("RequestPasswordReset must remain anonymous with empty resource: %+v", ev.Extensions())
			}
		}
		if ev.Subject() == "/plateau.iam.account.v1.AccountService/ConfirmPasswordReset" {
			if ev.Extensions()[audit.ExtActorType] != "human" || ev.Extensions()[audit.ExtResourceName] == nil {
				t.Fatalf("ConfirmPasswordReset must enrich human actor and user resource: %+v", ev.Extensions())
			}
		}
		if ev.Subject() == "/plateau.iam.account.v1.AccountService/ChangeInitialPassword" {
			switch ev.Extensions()[audit.ExtOutcome] {
			case audit.OutcomeFailure:
				if ev.Extensions()[audit.ExtActorType] != "anonymous" || ev.Extensions()[audit.ExtResourceName] != nil || !strings.Contains(string(ev.Data()), "auditee@example.com") {
					t.Fatalf("failed ChangeInitialPassword must remain anonymous with email in data: ext=%+v data=%s", ev.Extensions(), string(ev.Data()))
				}
			case audit.OutcomeSuccess:
				if ev.Extensions()[audit.ExtActorType] != "human" || ev.Extensions()[audit.ExtActorID] != auditee.GetUserId() || ev.Extensions()[audit.ExtResourceName] != "users/"+auditee.GetUserId() {
					t.Fatalf("successful ChangeInitialPassword must enrich human actor and user resource: %+v", ev.Extensions())
				}
			}
		}
		if ev.Subject() == "/plateau.iam.session.v1.SessionService/Logout" ||
			ev.Subject() == "/plateau.iam.account.v1.AccountService/ChangePassword" ||
			ev.Subject() == "/plateau.iam.account.v1.AccountService/DeleteAccount" {
			if ev.Extensions()[audit.ExtResourceName] != "users/"+f.user.GetUserId() {
				t.Fatalf("%s resource = %v, want users/%s", ev.Subject(), ev.Extensions()[audit.ExtResourceName], f.user.GetUserId())
			}
		}

		raw, _ := json.Marshal(ev)
		for _, secret := range []string{regPass, resetNewPass, changedPass, testPassword, verifyToken, resetToken, regProof, resetProof} {
			if strings.Contains(string(raw), secret) {
				t.Fatalf("sensitive credential/token %q leaked in %s: %s", secret, ev.Subject(), string(raw))
			}
		}
	}

	for _, wantOp := range []string{
		"/plateau.iam.account.v1.AccountService/Register",
		"/plateau.iam.account.v1.AccountService/VerifyEmail",
		"/plateau.iam.account.v1.AccountService/RequestPasswordReset",
		"/plateau.iam.account.v1.AccountService/ConfirmPasswordReset",
		"/plateau.iam.account.v1.AccountService/ChangeInitialPassword",
		"/plateau.iam.authn.v1.AuthnService/Login",
		"/plateau.iam.account.v1.AccountService/UpdateProfile",
		"/plateau.iam.account.v1.AccountService/ChangePassword",
		"/plateau.iam.session.v1.SessionService/Logout",
		"/plateau.iam.account.v1.AccountService/DeleteAccount",
	} {
		if byOp[wantOp] == 0 {
			t.Fatalf("missing expected audit event for %s (got %v)", wantOp, byOp)
		}
	}
}
