package data

import (
	"errors"
	"testing"
	"time"

	userpb "github.com/Servora-Kit/plateau/api/gen/go/iam/user/v1"
	"github.com/Servora-Kit/plateau/app/iam/service/internal/biz"
	"github.com/Servora-Kit/plateau/app/iam/service/internal/data/ent/authenticator"
	"github.com/Servora-Kit/plateau/app/iam/service/internal/data/ent/emailverificationtoken"
	"github.com/Servora-Kit/plateau/app/iam/service/internal/data/ent/iamloginsession"
	"github.com/Servora-Kit/plateau/app/iam/service/internal/data/ent/loginidentifier"
	"github.com/Servora-Kit/plateau/app/iam/service/internal/data/ent/oauthaccesstoken"
	"github.com/Servora-Kit/plateau/app/iam/service/internal/data/ent/oauthauthorizationcode"
	"github.com/Servora-Kit/plateau/app/iam/service/internal/data/ent/oauthrefreshtoken"
	"github.com/Servora-Kit/plateau/app/iam/service/internal/data/ent/oauthtokensession"
	"github.com/Servora-Kit/plateau/app/iam/service/internal/data/ent/oidcauthorizationrequest"
	"github.com/Servora-Kit/plateau/app/iam/service/internal/data/ent/passwordresettoken"
	"github.com/Servora-Kit/plateau/app/iam/service/internal/data/ent/user"
	entgomixin "github.com/Servora-Kit/plateau/infra/entgo/mixin"
	"github.com/Servora-Kit/plateau/security/password"
	crudpb "github.com/Servora-Kit/servora/api/gen/go/servora/crud/v1"
	corecrud "github.com/Servora-Kit/servora/core/crud"
)

func TestUserVisibilityPaginationRequiredActionsAndCredentialSnapshotsSQLite(t *testing.T) {
	client := newTestEntClient(t)
	ctx := t.Context()
	d := &Data{ent: client}
	users, err := NewUserRepository(d)
	if err != nil {
		t.Fatal(err)
	}
	credentials, err := NewCredentialRepository(d)
	if err != nil {
		t.Fatal(err)
	}

	const deletedID = "01912345-6789-7abc-8def-000000000101"
	const activeID = "01912345-6789-7abc-8def-000000000102"
	deletedEmail := "deleted@example.com"
	activeEmail := "active@example.com"

	createdDeleted, err := users.Create(ctx, &userpb.User{
		UserId:          deletedID,
		Email:           &deletedEmail,
		RequiredActions: []userpb.UserRequiredAction{userpb.UserRequiredAction_USER_REQUIRED_ACTION_UPDATE_PASSWORD},
	}, "hash-1", deletedEmail)
	if err != nil {
		t.Fatalf("Create(deleted candidate): %v", err)
	}
	if len(createdDeleted.GetRequiredActions()) != 1 || createdDeleted.GetRequiredActions()[0] != userpb.UserRequiredAction_USER_REQUIRED_ACTION_UPDATE_PASSWORD {
		t.Fatalf("created required_actions = %v", createdDeleted.GetRequiredActions())
	}
	if _, err := users.Create(ctx, &userpb.User{
		UserId: activeID,
		Email:  &activeEmail,
	}, "hash-2", activeEmail); err != nil {
		t.Fatalf("Create(active): %v", err)
	}

	cred, err := credentials.FindActivePassword(ctx, deletedID)
	if err != nil {
		t.Fatalf("FindActivePassword: %v", err)
	}
	if cred.UserID != deletedID || cred.AuthenticatorID == "" || cred.PasswordAuthenticatorID == "" || cred.PasswordHash != "hash-1" {
		t.Fatalf("PasswordCredential 4-field snapshot = %#v", cred)
	}

	// Mark deletedID as soft-deleted with purge_time to verify non-locking visibility split & ListUsers scopes on SQLite.
	now := time.Now().UTC().Truncate(time.Second)
	purgeAt := now.Add(24 * time.Hour)
	if _, err := client.User.UpdateOneID(deletedID).SetDeleteTime(now).SetPurgeTime(purgeAt).Save(ctx); err != nil {
		t.Fatal(err)
	}

	if _, err := users.Get(ctx, deletedID); !errors.Is(err, biz.ErrNotFound) {
		t.Fatalf("Get(soft-deleted) = %v, want ErrNotFound", err)
	}
	if _, err := users.FindByEmail(ctx, deletedEmail); !errors.Is(err, biz.ErrNotFound) {
		t.Fatalf("FindByEmail(soft-deleted) = %v, want ErrNotFound", err)
	}
	adminProjected, err := users.GetUser(ctx, userpb.NewUserName(deletedID))
	if err != nil {
		t.Fatalf("GetUser(soft-deleted): %v", err)
	}
	if adminProjected.GetDeleteTime().AsTime() != now || adminProjected.GetPurgeTime().AsTime() != purgeAt {
		t.Fatalf("GetUser timestamps delete=%v purge=%v", adminProjected.GetDeleteTime(), adminProjected.GetPurgeTime())
	}
	byEmailIncl, err := users.FindByEmailIncludingDeleted(ctx, deletedEmail)
	if err != nil || byEmailIncl.GetUserId() != deletedID || byEmailIncl.GetDeleteTime() == nil {
		t.Fatalf("FindByEmailIncludingDeleted = %#v, err=%v", byEmailIncl, err)
	}

	plan := corecrud.MustBuildResourcePlan[*userpb.User](userpb.UserCRUDDescriptor())
	preparer, err := corecrud.NewListPreparer()
	if err != nil {
		t.Fatal(err)
	}
	qActive, err := preparer.PrepareList(plan, corecrud.ListInput{Collection: "users", PageSize: 1})
	if err != nil {
		t.Fatal(err)
	}
	activeList, err := users.ListUsers(ctx, qActive, false)
	if err != nil || len(activeList.Items()) != 1 || activeList.Items()[0].GetUserId() != activeID {
		t.Fatalf("ListUsers(showDeleted=false) = %#v, err=%v", activeList.Items(), err)
	}
	qAllPage1, err := preparer.PrepareList(plan, corecrud.ListInput{Collection: "users", PageSize: 1})
	if err != nil {
		t.Fatal(err)
	}
	allPage1, err := users.ListUsers(ctx, qAllPage1, true)
	if err != nil || len(allPage1.Items()) != 1 || allPage1.NextPageToken() == "" {
		t.Fatalf("ListUsers(showDeleted=true, page1) items=%d token=%q err=%v", len(allPage1.Items()), allPage1.NextPageToken(), err)
	}
	qCrossScope, err := preparer.PrepareList(plan, corecrud.ListInput{Collection: "users", PageSize: 1, PageToken: allPage1.NextPageToken()})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := users.ListUsers(ctx, qCrossScope, false); !crudpb.IsCrudErrorReasonInvalidPageToken(err) {
		t.Fatalf("cross-scope page token error = %v, want INVALID_PAGE_TOKEN", err)
	}
	qDeletedOnly, err := preparer.PrepareList(plan, corecrud.ListInput{Collection: "users", PageSize: 10, Filter: "delete_time != null"})
	if err != nil {
		t.Fatal(err)
	}
	deletedOnly, err := users.ListUsers(ctx, qDeletedOnly, true)
	if err != nil || len(deletedOnly.Items()) != 1 || deletedOnly.Items()[0].GetUserId() != deletedID {
		t.Fatalf("ListUsers(delete_time != null) = %#v, err=%v", deletedOnly.Items(), err)
	}
	qByEmail, err := preparer.PrepareList(plan, corecrud.ListInput{Collection: "users", PageSize: 10, Filter: `email = "ACTIVE@example.com"`})
	if err != nil {
		t.Fatal(err)
	}
	byEmail, err := users.ListUsers(ctx, qByEmail, false)
	if err != nil || len(byEmail.Items()) != 1 || byEmail.Items()[0].GetUserId() != activeID {
		t.Fatalf("ListUsers(email exact) = %#v, err=%v", byEmail.Items(), err)
	}
	qByEmailWild, err := preparer.PrepareList(plan, corecrud.ListInput{Collection: "users", PageSize: 10, Filter: `email = "deleted@*"`})
	if err != nil {
		t.Fatal(err)
	}
	byEmailDeleted, err := users.ListUsers(ctx, qByEmailWild, true)
	if err != nil || len(byEmailDeleted.Items()) != 1 || byEmailDeleted.Items()[0].GetUserId() != deletedID {
		t.Fatalf("ListUsers(email prefix, showDeleted=true) = %#v, err=%v", byEmailDeleted.Items(), err)
	}

	// Unknown persisted required_actions string must fail rather than emitting UNSPECIFIED.
	if _, err := client.User.UpdateOneID(activeID).SetRequiredActions([]string{"BOGUS_ACTION"}).Save(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := users.Get(ctx, activeID); err == nil {
		t.Fatal("expected error projecting unknown required_actions value")
	}
}

func TestUserSoftDeleteUndeleteAndExpungeLifecycle(t *testing.T) {
	client, _ := newPostgresTestClient(t)
	ctx := t.Context()
	d := &Data{ent: client}
	users, err := NewUserRepository(d)
	if err != nil {
		t.Fatal(err)
	}
	credentials, err := NewCredentialRepository(d)
	if err != nil {
		t.Fatal(err)
	}
	sessions, err := NewSessionRepository(d)
	if err != nil {
		t.Fatal(err)
	}
	verifyTokens, err := NewVerificationTokenRepo(d)
	if err != nil {
		t.Fatal(err)
	}
	resetTokens, err := NewPasswordResetTokenRepository(d)
	if err != nil {
		t.Fatal(err)
	}

	hash, err := password.Hash("correct horse battery staple")
	if err != nil {
		t.Fatal(err)
	}
	const targetID = "01912345-6789-7abc-8def-000000000001"
	const otherID = "01912345-6789-7abc-8def-000000000002"
	targetEmail := "target@example.com"
	otherEmail := "other@example.com"

	_, err = users.Create(ctx, &userpb.User{
		UserId:          targetID,
		Email:           &targetEmail,
		RequiredActions: []userpb.UserRequiredAction{userpb.UserRequiredAction_USER_REQUIRED_ACTION_UPDATE_PASSWORD},
	}, hash, targetEmail)
	if err != nil {
		t.Fatalf("Create target: %v", err)
	}
	other, err := users.Create(ctx, &userpb.User{UserId: otherID, Email: &otherEmail}, hash, otherEmail)
	if err != nil {
		t.Fatalf("Create other: %v", err)
	}

	now := time.Now().UTC().Truncate(time.Second)
	vHash := biz.HashOpaqueSecret("verify-target")
	if err := verifyTokens.Create(ctx, targetID, "", vHash, now.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if _, err := verifyTokens.ConsumeAndActivate(ctx, vHash, now); err != nil {
		t.Fatal(err)
	}
	activeTarget, err := users.Get(ctx, targetID)
	if err != nil {
		t.Fatal(err)
	}
	cred, err := credentials.FindActivePassword(ctx, targetID)
	if err != nil {
		t.Fatal(err)
	}
	// SessionRepo.Create must NOT create a login session while UPDATE_PASSWORD is present.
	gatedUser, gatedLogin, err := sessions.Create(ctx, *cred, activeTarget.GetEtag(), now)
	if err != nil || gatedLogin != nil || len(gatedUser.GetRequiredActions()) != 1 {
		t.Fatalf("SessionRepo.Create(gated) user=%#v login=%#v err=%v", gatedUser, gatedLogin, err)
	}

	// Clear UPDATE_PASSWORD via CompleteInitialPassword so we also get an active IAMLoginSession.
	nextHash, err := password.Hash("next horse battery staple")
	if err != nil {
		t.Fatal(err)
	}
	loginSession, err := credentials.CompleteInitialPassword(ctx, *cred, activeTarget.GetEtag(), nextHash, now)
	if err != nil || loginSession == nil {
		t.Fatalf("CompleteInitialPassword: session=%#v err=%v", loginSession, err)
	}
	activeTarget, err = users.Get(ctx, targetID)
	if err != nil || len(activeTarget.GetRequiredActions()) != 0 {
		t.Fatalf("after CompleteInitialPassword: user=%#v err=%v", activeTarget, err)
	}
	cred, err = credentials.FindActivePassword(ctx, targetID)
	if err != nil {
		t.Fatal(err)
	}

	targetIdentifier := client.LoginIdentifier.Query().Where(loginidentifier.UserIDEQ(targetID)).OnlyX(ctx)
	if _, err := client.EmailVerificationToken.Create().
		SetID("evt-unconsumed").
		SetUserID(targetID).
		SetLoginIdentifierID(targetIdentifier.ID).
		SetTokenHash("evt-unconsumed-hash").
		SetExpiresTime(now.Add(time.Hour)).
		Save(ctx); err != nil {
		t.Fatal(err)
	}
	if err := resetTokens.Create(ctx, targetID, "prt-unconsumed-hash", now.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if _, err := client.OAuthTokenSession.Create().
		SetID("ts-1").
		SetUserID(targetID).
		SetClientID("web-client").
		SetIamLoginSessionID(loginSession.ID).
		SetRefreshFamilyID("fam-1").
		SetScopes([]string{"openid"}).
		SetAmr([]string{"pwd"}).
		SetAuthTime(now).
		Save(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := client.OAuthAccessToken.Create().
		SetID("at-human-session").
		SetActorType(oauthaccesstoken.ActorTypeHuman).
		SetSubject(targetID).
		SetClientID("web-client").
		SetTokenSessionID("ts-1").
		SetScopes([]string{"openid"}).
		SetExpiresTime(now.Add(time.Hour)).
		Save(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := client.OAuthAccessToken.Create().
		SetID("at-human-direct").
		SetActorType(oauthaccesstoken.ActorTypeHuman).
		SetSubject(targetID).
		SetClientID("web-client").
		SetScopes([]string{"openid"}).
		SetExpiresTime(now.Add(time.Hour)).
		Save(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := client.OAuthAccessToken.Create().
		SetID("at-service-subject-target").
		SetActorType(oauthaccesstoken.ActorTypeService).
		SetSubject(targetID).
		SetClientID("web-client").
		SetScopes([]string{"openid"}).
		SetExpiresTime(now.Add(time.Hour)).
		Save(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := client.OAuthRefreshToken.Create().
		SetID("rt-1").
		SetTokenSessionID("ts-1").
		SetFamilyID("fam-1").
		SetTokenHash("rt-1-hash").
		SetExpiresTime(now.Add(time.Hour)).
		Save(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := client.OIDCAuthorizationRequest.Create().
		SetID("req-1").
		SetClientID("web-client").
		SetRedirectURI("https://app.example/cb").
		SetResponseType("code").
		SetScopes([]string{"openid"}).
		SetPkceChallenge("challenge").
		SetSubject(targetID).
		SetIamLoginSessionID(loginSession.ID).
		SetAuthTime(now).
		SetDone(true).
		SetExpiresTime(now.Add(time.Hour)).
		Save(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := client.OAuthAuthorizationCode.Create().
		SetID("code-1").
		SetCodeHash("code-1-hash").
		SetAuthorizationRequestID("req-1").
		SetClientID("web-client").
		SetSubject(targetID).
		SetRedirectURI("https://app.example/cb").
		SetScopes([]string{"openid"}).
		SetPkceChallenge("challenge").
		SetExpiresTime(now.Add(time.Hour)).
		Save(ctx); err != nil {
		t.Fatal(err)
	}
	// Pre-populate purge_time to confirm SoftDelete clears it.
	if _, err := client.User.UpdateOneID(targetID).SetPurgeTime(now.Add(24 * time.Hour)).Save(ctx); err != nil {
		t.Fatal(err)
	}

	// Expunge and Undelete must fail on a live user.
	if err := users.Expunge(ctx, targetID); !errors.Is(err, biz.ErrUserInvalidState) {
		t.Fatalf("Expunge live user = %v, want ErrUserInvalidState", err)
	}
	if _, err := users.Undelete(ctx, targetID, now); !errors.Is(err, biz.ErrAlreadyExists) {
		t.Fatalf("Undelete live user = %v, want ErrAlreadyExists", err)
	}

	// SoftDelete via self-service DeleteAccountProof.
	deletedAt := now.Add(time.Minute)
	softDeleted, err := users.SoftDelete(ctx, targetID, &biz.DeleteAccountProof{
		LoginID:      loginSession.ID,
		PasswordHash: cred.PasswordHash,
	}, deletedAt)
	if err != nil {
		t.Fatalf("SoftDelete(proof) error = %v", err)
	}
	if softDeleted.GetDeleteTime() == nil || softDeleted.GetPurgeTime() != nil {
		t.Fatalf("SoftDelete projection delete_time=%v purge_time=%v", softDeleted.GetDeleteTime(), softDeleted.GetPurgeTime())
	}

	// Visibility split: Get & FindByEmail hide soft-deleted; GetUser & FindByEmailIncludingDeleted expose it.
	if _, err := users.Get(ctx, targetID); !errors.Is(err, biz.ErrNotFound) {
		t.Fatalf("Get(soft-deleted) = %v, want ErrNotFound", err)
	}
	if _, err := users.FindByEmail(ctx, targetEmail); !errors.Is(err, biz.ErrNotFound) {
		t.Fatalf("FindByEmail(soft-deleted) = %v, want ErrNotFound", err)
	}
	if got, err := users.GetUser(ctx, userpb.NewUserName(targetID)); err != nil || got.GetDeleteTime() == nil {
		t.Fatalf("GetUser(soft-deleted) = %#v, err=%v", got, err)
	}
	if got, err := users.FindByEmailIncludingDeleted(ctx, targetEmail); err != nil || got.GetDeleteTime() == nil {
		t.Fatalf("FindByEmailIncludingDeleted(soft-deleted) = %#v, err=%v", got, err)
	}
	if _, err := users.SoftDelete(ctx, targetID, nil, deletedAt); !errors.Is(err, biz.ErrNotFound) {
		t.Fatalf("second SoftDelete = %v, want ErrNotFound", err)
	}

	// ListUsers show_deleted=false vs true + page token scope isolation.
	plan := corecrud.MustBuildResourcePlan[*userpb.User](userpb.UserCRUDDescriptor())
	preparer, err := corecrud.NewListPreparer()
	if err != nil {
		t.Fatal(err)
	}
	qActive, err := preparer.PrepareList(plan, corecrud.ListInput{Collection: "users", PageSize: 1})
	if err != nil {
		t.Fatal(err)
	}
	activeList, err := users.ListUsers(ctx, qActive, false)
	if err != nil || len(activeList.Items()) != 1 || activeList.Items()[0].GetUserId() != other.GetUserId() {
		t.Fatalf("ListUsers(showDeleted=false) items=%#v err=%v", activeList.Items(), err)
	}
	qAllPage1, err := preparer.PrepareList(plan, corecrud.ListInput{Collection: "users", PageSize: 1})
	if err != nil {
		t.Fatal(err)
	}
	allPage1, err := users.ListUsers(ctx, qAllPage1, true)
	if err != nil || len(allPage1.Items()) != 1 || allPage1.NextPageToken() == "" {
		t.Fatalf("ListUsers(showDeleted=true, page1) items=%d token=%q err=%v", len(allPage1.Items()), allPage1.NextPageToken(), err)
	}
	qCrossScope, err := preparer.PrepareList(plan, corecrud.ListInput{Collection: "users", PageSize: 1, PageToken: allPage1.NextPageToken()})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := users.ListUsers(ctx, qCrossScope, false); !crudpb.IsCrudErrorReasonInvalidPageToken(err) {
		t.Fatalf("cross-scope page token error = %v, want INVALID_PAGE_TOKEN", err)
	}
	qDeletedOnly, err := preparer.PrepareList(plan, corecrud.ListInput{Collection: "users", PageSize: 10, Filter: "delete_time != null"})
	if err != nil {
		t.Fatal(err)
	}
	deletedOnly, err := users.ListUsers(ctx, qDeletedOnly, true)
	if err != nil || len(deletedOnly.Items()) != 1 || deletedOnly.Items()[0].GetUserId() != targetID {
		t.Fatalf("ListUsers(delete_time != null) items=%#v err=%v", deletedOnly.Items(), err)
	}

	// Verify SoftDelete session/token/OIDC side effects and service token preservation.
	if got, err := sessions.Find(ctx, loginSession.ID); err != nil || got.RevokedAt == nil {
		t.Fatalf("login session after SoftDelete = %#v, err=%v", got, err)
	}
	if ts := client.OAuthTokenSession.GetX(ctx, "ts-1"); ts.RevokedTime == nil {
		t.Fatal("OAuthTokenSession not revoked on SoftDelete")
	}
	if at := client.OAuthAccessToken.GetX(ctx, "at-human-session"); at.RevokedTime == nil {
		t.Fatal("session human access token not revoked on SoftDelete")
	}
	if at := client.OAuthAccessToken.GetX(ctx, "at-human-direct"); at.RevokedTime == nil {
		t.Fatal("direct human access token not revoked on SoftDelete")
	}
	if svcAT := client.OAuthAccessToken.GetX(ctx, "at-service-subject-target"); svcAT.RevokedTime != nil {
		t.Fatal("service access token with subject=targetID was wrongly revoked on SoftDelete")
	}
	if rt := client.OAuthRefreshToken.GetX(ctx, "rt-1"); rt.RevokedTime == nil {
		t.Fatal("refresh token not revoked on SoftDelete")
	}
	if evt := client.EmailVerificationToken.GetX(ctx, "evt-unconsumed"); evt.ConsumedTime == nil {
		t.Fatal("unconsumed verification token not marked consumed on SoftDelete")
	}
	if prt := client.PasswordResetToken.Query().Where(passwordresettoken.TokenHashEQ("prt-unconsumed-hash")).OnlyX(ctx); prt.ConsumedTime == nil {
		t.Fatal("unconsumed reset token not marked consumed on SoftDelete")
	}
	if client.OIDCAuthorizationRequest.Query().Where(oidcauthorizationrequest.IDEQ("req-1")).CountX(ctx) != 0 {
		t.Fatal("OIDCAuthorizationRequest survived SoftDelete")
	}
	if client.OAuthAuthorizationCode.Query().Where(oauthauthorizationcode.IDEQ("code-1")).CountX(ctx) != 0 {
		t.Fatal("OAuthAuthorizationCode survived SoftDelete")
	}

	// Undelete restores visibility without resurrecting revoked sessions/tokens.
	undeleted, err := users.Undelete(ctx, targetID, deletedAt.Add(time.Minute))
	if err != nil {
		t.Fatalf("Undelete() error = %v", err)
	}
	if undeleted.GetDeleteTime() != nil || undeleted.GetEtag() == softDeleted.GetEtag() || undeleted.GetStatus() != userpb.UserStatus_USER_STATUS_ACTIVE {
		t.Fatalf("Undelete() user = %#v", undeleted)
	}
	if got, err := sessions.Find(ctx, loginSession.ID); err != nil || got.RevokedAt == nil {
		t.Fatal("Undelete resurrected revoked login session")
	}

	// Re-soft-delete via management (nil proof) and then Expunge across all 12 tables.
	if _, err := users.SoftDelete(ctx, targetID, nil, deletedAt.Add(2*time.Minute)); err != nil {
		t.Fatalf("SoftDelete(nil) error = %v", err)
	}
	if err := users.Expunge(ctx, targetID); err != nil {
		t.Fatalf("Expunge() error = %v", err)
	}
	if _, err := users.GetUser(ctx, userpb.NewUserName(targetID)); !errors.Is(err, biz.ErrNotFound) {
		t.Fatalf("GetUser(expunged) = %v, want ErrNotFound", err)
	}
	if _, err := users.FindByEmailIncludingDeleted(ctx, targetEmail); !errors.Is(err, biz.ErrNotFound) {
		t.Fatalf("FindByEmailIncludingDeleted(expunged) = %v, want ErrNotFound", err)
	}
	if client.User.Query().Where(user.IDEQ(targetID)).CountX(entgomixin.SkipSoftDelete(ctx)) != 0 {
		t.Fatal("users row survived Expunge")
	}
	if client.LoginIdentifier.Query().Where(loginidentifier.UserIDEQ(targetID)).CountX(ctx) != 0 ||
		client.Authenticator.Query().Where(authenticator.UserIDEQ(targetID)).CountX(ctx) != 0 ||
		client.PasswordAuthenticator.Query().CountX(ctx) != 1 || // only otherID's password authenticator remains
		client.EmailVerificationToken.Query().Where(emailverificationtoken.UserIDEQ(targetID)).CountX(ctx) != 0 ||
		client.PasswordResetToken.Query().Where(passwordresettoken.UserIDEQ(targetID)).CountX(ctx) != 0 ||
		client.IAMLoginSession.Query().Where(iamloginsession.UserIDEQ(targetID)).CountX(ctx) != 0 ||
		client.OAuthTokenSession.Query().Where(oauthtokensession.UserIDEQ(targetID)).CountX(ctx) != 0 ||
		client.OAuthRefreshToken.Query().Where(oauthrefreshtoken.IDEQ("rt-1")).CountX(ctx) != 0 ||
		client.OAuthAccessToken.Query().Where(oauthaccesstoken.ActorTypeEQ(oauthaccesstoken.ActorTypeHuman), oauthaccesstoken.SubjectEQ(targetID)).CountX(ctx) != 0 {
		t.Fatal("identity-owned rows survived Expunge")
	}
	// Service access token with subject=targetID and unrelated user must survive Expunge.
	if client.OAuthAccessToken.Query().Where(oauthaccesstoken.IDEQ("at-service-subject-target")).CountX(ctx) != 1 {
		t.Fatal("service access token with subject=targetID was deleted by Expunge")
	}
	if gotOther, err := users.Get(ctx, otherID); err != nil || gotOther.GetUserId() != otherID {
		t.Fatalf("unrelated user affected by Expunge: %v", err)
	}
}

func TestUserStatusTransitionsAndAdminPasswordOnDisabledUser(t *testing.T) {
	client, _ := newPostgresTestClient(t)
	ctx := t.Context()
	d := &Data{ent: client}
	users, err := NewUserRepository(d)
	if err != nil {
		t.Fatal(err)
	}
	credentials, err := NewCredentialRepository(d)
	if err != nil {
		t.Fatal(err)
	}
	sessions, err := NewSessionRepository(d)
	if err != nil {
		t.Fatal(err)
	}

	hash, err := password.Hash("initial horse battery staple")
	if err != nil {
		t.Fatal(err)
	}
	unverifiedEmail := "pending@example.com"
	pending, err := users.Create(ctx, &userpb.User{
		UserId:          "01912345-6789-7abc-8def-000000000010",
		Email:           &unverifiedEmail,
		RequiredActions: []userpb.UserRequiredAction{userpb.UserRequiredAction_USER_REQUIRED_ACTION_UPDATE_PASSWORD},
	}, hash, unverifiedEmail)
	if err != nil {
		t.Fatal(err)
	}

	now := time.Now().UTC()
	// EnableUser on non-disabled user -> ErrUserInvalidState.
	if _, err := users.UpdateStatus(ctx, pending.GetUserId(), pending.GetEtag(), userpb.UserStatus_USER_STATUS_ACTIVE, now); !errors.Is(err, biz.ErrUserInvalidState) {
		t.Fatalf("EnableUser on pending = %v, want ErrUserInvalidState", err)
	}
	// DisableUser on pending -> disabled; admin SetPasswordAndRevokeSessions works while disabled and preserves disabled authenticator state + required_actions.
	disabled, err := users.UpdateStatus(ctx, pending.GetUserId(), pending.GetEtag(), userpb.UserStatus_USER_STATUS_DISABLED, now)
	if err != nil || disabled.GetStatus() != userpb.UserStatus_USER_STATUS_DISABLED {
		t.Fatalf("DisableUser(pending) = %#v, err=%v", disabled, err)
	}
	adminHash, err := password.Hash("admin replacement horse battery staple")
	if err != nil {
		t.Fatal(err)
	}
	if err := credentials.SetPasswordAndRevokeSessions(ctx, pending.GetUserId(), adminHash, now.Add(time.Minute)); err != nil {
		t.Fatalf("SetPasswordAndRevokeSessions on disabled user = %v", err)
	}
	authRow := client.Authenticator.Query().Where(authenticator.UserIDEQ(pending.GetUserId())).OnlyX(ctx)
	if authRow.State != biz.AuthenticatorDisabled || authRow.RevokedTime == nil {
		t.Fatalf("SetPasswordAndRevokeSessions altered disabled authenticator state=%q revoked=%v", authRow.State, authRow.RevokedTime)
	}
	// EnableUser on unverified disabled user restores PENDING_EMAIL_VERIFICATION and clears authenticator revoked_time.
	restoredPending, err := users.UpdateStatus(ctx, pending.GetUserId(), disabled.GetEtag(), userpb.UserStatus_USER_STATUS_ACTIVE, now.Add(2*time.Minute))
	if err != nil || restoredPending.GetStatus() != userpb.UserStatus_USER_STATUS_PENDING_EMAIL_VERIFICATION {
		t.Fatalf("EnableUser(unverified) = %#v, err=%v", restoredPending, err)
	}
	if len(restoredPending.GetRequiredActions()) != 1 || restoredPending.GetRequiredActions()[0] != userpb.UserRequiredAction_USER_REQUIRED_ACTION_UPDATE_PASSWORD {
		t.Fatalf("required_actions lost across status/password updates: %v", restoredPending.GetRequiredActions())
	}
	authRow = client.Authenticator.Query().Where(authenticator.UserIDEQ(pending.GetUserId())).OnlyX(ctx)
	if authRow.State != biz.AuthenticatorActive || authRow.RevokedTime != nil {
		t.Fatalf("EnableUser did not restore authenticator: state=%q revoked=%v", authRow.State, authRow.RevokedTime)
	}

	// RevokeAll is idempotent when no sessions exist.
	if err := sessions.RevokeAll(ctx, pending.GetUserId(), now); err != nil {
		t.Fatalf("RevokeAll(empty) = %v", err)
	}

	// Unknown persisted required_actions string returns internal error rather than UNSPECIFIED.
	if _, err := client.User.UpdateOneID(pending.GetUserId()).SetRequiredActions([]string{"UNKNOWN_FUTURE_ACTION"}).Save(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := users.GetUser(ctx, userpb.NewUserName(pending.GetUserId())); err == nil {
		t.Fatal("expected error projecting unknown persisted required_actions value")
	}
}

func TestOAuthRepositoryRejectsUsersWithPendingRequiredActionsOrDeletedState(t *testing.T) {
	client, _ := newPostgresTestClient(t)
	ctx := t.Context()
	d := &Data{ent: client}
	creator, err := NewInitialUserCreator(d)
	if err != nil {
		t.Fatal(err)
	}
	users, err := NewUserRepository(d)
	if err != nil {
		t.Fatal(err)
	}
	oauth, err := NewOAuthRepository(d)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Second)
	if err := creator.CreateInitialUser(ctx, "u-oidc", "oidc@example.com", "oidc@example.com", "hash", now); err != nil {
		t.Fatal(err)
	}
	if _, err := client.IAMLoginSession.Create().SetID("login-oidc").SetUserID("u-oidc").SetCreateTime(now).Save(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := client.OIDCAuthorizationRequest.Create().
		SetID("auth-req-1").
		SetClientID("oidc-web").
		SetRedirectURI("https://app.example/cb").
		SetResponseType("code").
		SetScopes([]string{"openid"}).
		SetPkceChallenge("challenge").
		SetExpiresTime(now.Add(time.Hour)).
		Save(ctx); err != nil {
		t.Fatal(err)
	}

	// User has UPDATE_PASSWORD required action -> CompleteAuthorizationRequest must reject!
	if err := oauth.CompleteAuthorizationRequest(ctx, "auth-req-1", "u-oidc", "login-oidc", now); !errors.Is(err, biz.ErrOAuthGrantInvalid) {
		t.Fatalf("CompleteAuthorizationRequest with UPDATE_PASSWORD = %v, want ErrOAuthGrantInvalid", err)
	}

	// Clear required_actions -> CompleteAuthorizationRequest & SaveAuthorizationCode succeed.
	if _, err := client.User.UpdateOneID("u-oidc").SetRequiredActions([]string{}).Save(ctx); err != nil {
		t.Fatal(err)
	}
	if err := oauth.CompleteAuthorizationRequest(ctx, "auth-req-1", "u-oidc", "login-oidc", now); err != nil {
		t.Fatalf("CompleteAuthorizationRequest(clean) = %v", err)
	}
	if err := oauth.SaveAuthorizationCode(ctx, "auth-req-1", "code-id-1", "code-hash-1", now, now.Add(5*time.Minute)); err != nil {
		t.Fatalf("SaveAuthorizationCode(clean) = %v", err)
	}

	grant := biz.OAuthGrant{
		CodeID:    "code-id-1",
		Subject:   "u-oidc",
		ClientID:  "oidc-web",
		LoginID:   "login-oidc",
		Scopes:    []string{"openid"},
		Audiences: []string{"oidc-web"},
		AMR:       []string{"pwd"},
		AuthTime:  now,
	}
	// Soft-delete user before Issue -> Issue must reject!
	if _, err := users.SoftDelete(ctx, "u-oidc", nil, now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	if err := oauth.Issue(ctx, grant, biz.OAuthTokenIssue{
		AccessID:         "at-oidc-1",
		RefreshID:        "rt-oidc-1",
		RefreshHash:      "rt-hash-1",
		IssuedAt:         now.Add(time.Minute),
		AccessExpiresAt:  now.Add(15 * time.Minute),
		RefreshExpiresAt: now.Add(time.Hour),
	}); !errors.Is(err, biz.ErrOAuthGrantInvalid) {
		t.Fatalf("Issue on soft-deleted user = %v, want ErrOAuthGrantInvalid", err)
	}
}
