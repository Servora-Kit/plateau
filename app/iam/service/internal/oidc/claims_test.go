package oidc

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"entgo.io/ent/dialect/sql"
	"github.com/Servora-Kit/plateau/app/iam/service/internal/biz"
	"github.com/Servora-Kit/plateau/app/iam/service/internal/data"
	entmodel "github.com/Servora-Kit/plateau/app/iam/service/internal/data/ent"
	_ "github.com/mattn/go-sqlite3"
	"github.com/zitadel/oidc/v3/pkg/oidc"
)

type stubOAuthRepo struct {
	completeErr error
	saveCodeErr error
}

func (s stubOAuthRepo) CompleteAuthorizationRequest(context.Context, string, string, string, time.Time) error {
	return s.completeErr
}

func (s stubOAuthRepo) SaveAuthorizationCode(context.Context, string, string, string, time.Time, time.Time) error {
	return s.saveCodeErr
}

func (s stubOAuthRepo) Issue(context.Context, biz.OAuthGrant, biz.OAuthTokenIssue) error {
	return nil
}

func (s stubOAuthRepo) RevokeSession(context.Context, string, time.Time) error { return nil }
func (s stubOAuthRepo) RevokeClient(context.Context, string, string, time.Time) error {
	return nil
}
func (s stubOAuthRepo) RevokeToken(context.Context, string, string, string, time.Time) error {
	return nil
}

func newSQLiteOIDCClient(t *testing.T) *entmodel.Client {
	t.Helper()
	driver, err := sql.Open("sqlite3", fmt.Sprintf("file:%s?mode=memory&cache=shared&_fk=1", t.Name()))
	if err != nil {
		t.Fatal(err)
	}
	client, cleanup, err := data.NewDBClient(driver)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(cleanup)
	return client
}

func isStorageNotFound(err error) bool {
	var target storageNotFoundError
	return errors.As(err, &target)
}

func TestPopulateUserinfoRejectsUnverifiedRequiredActionsDeletedOrInactive(t *testing.T) {
	client := newSQLiteOIDCClient(t)
	ctx := t.Context()
	now := time.Now().UTC()
	storage := &OIDCStorage{client: client, now: time.Now}

	if _, err := client.User.Create().
		SetID("u-valid").
		SetStatus(biz.UserStatusActive).
		SetName("Valid Person").
		SetEtag("etag-1").
		Save(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := client.LoginIdentifier.Create().
		SetID("li-valid").
		SetUserID("u-valid").
		SetType(biz.LoginIdentifierEmail).
		SetCanonicalValue("valid@example.com").
		SetDisplayValue("Valid@Example.com").
		SetVerifiedTime(now).
		Save(ctx); err != nil {
		t.Fatal(err)
	}

	info := &oidc.UserInfo{}
	if err := storage.populateUserinfo(ctx, info, "u-valid", "", []string{oidc.ScopeOpenID, oidc.ScopeEmail, oidc.ScopeProfile}); err != nil {
		t.Fatalf("populateUserinfo(valid) error = %v", err)
	}
	if info.Subject != "u-valid" || info.Email != "Valid@Example.com" || !bool(info.EmailVerified) || info.Name != "Valid Person" {
		t.Fatalf("populateUserinfo(valid) = %#v", info)
	}

	// Non-empty required_actions -> storageNotFoundError.
	if _, err := client.User.UpdateOneID("u-valid").SetRequiredActions([]string{"UPDATE_PASSWORD"}).Save(ctx); err != nil {
		t.Fatal(err)
	}
	if err := storage.populateUserinfo(ctx, &oidc.UserInfo{}, "u-valid", "", []string{oidc.ScopeOpenID, oidc.ScopeEmail}); !isStorageNotFound(err) {
		t.Fatalf("populateUserinfo(required_actions) = %v, want storageNotFoundError", err)
	}

	// Clear required_actions, clear verified_time -> storageNotFoundError even when ScopeEmail is omitted!
	if _, err := client.User.UpdateOneID("u-valid").SetRequiredActions([]string{}).Save(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := client.LoginIdentifier.UpdateOneID("li-valid").ClearVerifiedTime().Save(ctx); err != nil {
		t.Fatal(err)
	}
	if err := storage.populateUserinfo(ctx, &oidc.UserInfo{}, "u-valid", "", []string{oidc.ScopeOpenID}); !isStorageNotFound(err) {
		t.Fatalf("populateUserinfo(unverified email without email scope) = %v, want storageNotFoundError", err)
	}

	// Restore verified_time, soft-delete user -> storageNotFoundError.
	if _, err := client.LoginIdentifier.UpdateOneID("li-valid").SetVerifiedTime(now).Save(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := client.User.UpdateOneID("u-valid").SetDeleteTime(now).Save(ctx); err != nil {
		t.Fatal(err)
	}
	if err := storage.populateUserinfo(ctx, &oidc.UserInfo{}, "u-valid", "", []string{oidc.ScopeOpenID}); !isStorageNotFound(err) {
		t.Fatalf("populateUserinfo(soft-deleted) = %v, want storageNotFoundError", err)
	}
}

func TestOIDCStorageCompleteAuthRequestAndSaveAuthCodeDelegateToOAuthRepo(t *testing.T) {
	client := newSQLiteOIDCClient(t)
	storage := &OIDCStorage{
		client: client,
		tokens: stubOAuthRepo{completeErr: biz.ErrOAuthGrantInvalid, saveCodeErr: biz.ErrOAuthGrantInvalid},
		now:    time.Now,
	}

	if err := storage.CompleteAuthRequest(t.Context(), "req-1", "u-1", "login-1", time.Now()); !isStorageNotFound(err) {
		t.Fatalf("CompleteAuthRequest(ErrOAuthGrantInvalid) = %v, want storageNotFoundError", err)
	}
	if err := storage.SaveAuthCode(t.Context(), "req-1", "raw-code"); !isStorageNotFound(err) {
		t.Fatalf("SaveAuthCode(ErrOAuthGrantInvalid) = %v, want storageNotFoundError", err)
	}
}
