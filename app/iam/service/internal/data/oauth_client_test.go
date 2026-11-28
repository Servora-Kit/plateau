package data

import (
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	iamclientv1 "github.com/Servora-Kit/plateau/api/gen/go/plateau/iam/client/v1"
	"github.com/Servora-Kit/plateau/app/iam/service/internal/biz"
	entmodel "github.com/Servora-Kit/plateau/app/iam/service/internal/data/ent"
	"github.com/Servora-Kit/plateau/app/iam/service/internal/data/ent/authflow"
	"github.com/Servora-Kit/plateau/app/iam/service/internal/data/ent/oauthaccesstoken"
	"github.com/Servora-Kit/plateau/app/iam/service/internal/data/ent/oauthclientsecret"
	"github.com/Servora-Kit/plateau/app/iam/service/internal/data/ent/oauthconsent"
	"github.com/Servora-Kit/plateau/app/iam/service/internal/data/ent/oidcauthorizationrequest"
	corecrud "github.com/Servora-Kit/plateau/crud"
	"google.golang.org/protobuf/types/known/fieldmaskpb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func TestOAuthClientRepositoryCRUDListFilterAndSecretLifecycle(t *testing.T) {
	client, _ := newPostgresTestClient(t)
	ctx := t.Context()
	d := &Data{ent: client}
	repo, err := NewOAuthClientRepository(d)
	if err != nil {
		t.Fatal(err)
	}

	baseTime := time.Date(2026, 11, 28, 8, 0, 0, 0, time.UTC)

	// Seed one static client directly in DB.
	if _, err := client.OAuthClient.Create().
		SetID("static-admin").
		SetSource(biz.ClientSourceStatic).
		SetStatus(biz.ClientStatusActive).
		SetDisplayName("Static Admin").
		SetApplicationType(biz.AppTypeWeb).
		SetTokenEndpointAuthMethod(biz.AuthMethodBasic).
		SetAllowedGrantTypes([]string{biz.GrantTypeClientCredentials}).
		SetAudiences([]string{"iam"}).
		SetTrusted(true).
		SetEtag("etag-static-1").
		SetCreateTime(baseTime).
		SetUpdateTime(baseTime).
		Save(ctx); err != nil {
		t.Fatal(err)
	}

	// Create confidential dynamic web client with initial secret.
	plain1, prefix1, hash1, err := biz.NewClientSecret()
	if err != nil {
		t.Fatal(err)
	}
	webDesc := "Partner portal"
	webLogo := "https://partner.example.com/logo.png"
	createdWeb, err := repo.Create(ctx, &iamclientv1.OAuthClient{
		ClientId:                "partner-web",
		DisplayName:             new("Partner Web"),
		Description:             &webDesc,
		LogoUri:                 &webLogo,
		ApplicationType:         iamclientv1.ApplicationType_APPLICATION_TYPE_WEB.Enum(),
		TokenEndpointAuthMethod: iamclientv1.TokenEndpointAuthMethod_TOKEN_ENDPOINT_AUTH_METHOD_CLIENT_SECRET_BASIC.Enum(),
		RedirectUris:            []string{"https://partner.example.com/cb"},
		PostLogoutRedirectUris:  []string{"https://partner.example.com/logout"},
		AllowedGrantTypes:       []string{biz.GrantTypeAuthorizationCode, biz.GrantTypeRefreshToken},
		AllowedResponseTypes:    []string{biz.ResponseTypeCode},
		AllowedScopes:           []string{"openid", "profile", "email"},
		Trusted:                 new(false),
	}, &iamclientv1.OAuthClientSecret{
		SecretId:     "sec-1",
		Name:         "Primary Key",
		SecretPrefix: prefix1,
	}, hash1, baseTime.Add(time.Minute))
	if err != nil {
		t.Fatalf("Create(partner-web): %v", err)
	}
	if createdWeb.GetName() != "oauthClients/partner-web" ||
		createdWeb.GetSource() != iamclientv1.ClientSource_CLIENT_SOURCE_DYNAMIC ||
		createdWeb.GetStatus() != iamclientv1.ClientStatus_CLIENT_STATUS_ACTIVE ||
		len(createdWeb.GetSecrets()) != 1 ||
		createdWeb.GetSecrets()[0].GetSecretPrefix() != prefix1 {
		t.Fatalf("createdWeb = %+v", createdWeb)
	}
	if strings.Contains(createdWeb.String(), plain1) || strings.Contains(createdWeb.String(), hash1) {
		t.Fatalf("OAuthClient DTO leaked secret plaintext or hash: %s", createdWeb.String())
	}

	// Create public SPA client without secrets.
	createdSPA, err := repo.Create(ctx, &iamclientv1.OAuthClient{
		ClientId:                "partner-spa",
		DisplayName:             new("Partner SPA"),
		ApplicationType:         iamclientv1.ApplicationType_APPLICATION_TYPE_USER_AGENT.Enum(),
		TokenEndpointAuthMethod: iamclientv1.TokenEndpointAuthMethod_TOKEN_ENDPOINT_AUTH_METHOD_NONE.Enum(),
		RedirectUris:            []string{"http://localhost:5173/cb"},
		AllowedGrantTypes:       []string{biz.GrantTypeAuthorizationCode},
		AllowedResponseTypes:    []string{biz.ResponseTypeCode},
		AllowedScopes:           []string{"openid"},
		Trusted:                 new(false),
	}, nil, "", baseTime.Add(2*time.Minute))
	if err != nil {
		t.Fatalf("Create(partner-spa): %v", err)
	}
	if len(createdSPA.GetSecrets()) != 0 {
		t.Fatalf("public SPA unexpectedly created secrets: %+v", createdSPA.GetSecrets())
	}

	// List pagination, enum filters, and ordering.
	plan := corecrud.MustBuildResourcePlan[*iamclientv1.OAuthClient](iamclientv1.OAuthClientCRUDDescriptor())
	preparer, err := corecrud.NewListPreparer()
	if err != nil {
		t.Fatal(err)
	}
	qPage1, err := preparer.PrepareList(plan, corecrud.ListInput{
		Collection:   "oauthClients",
		PageSize:     2,
		IncludeTotal: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	resPage1, err := repo.List(ctx, qPage1)
	if err != nil {
		t.Fatalf("List(page1): %v", err)
	}
	total, hasTotal := resPage1.TotalSize()
	if !hasTotal || total != 3 || len(resPage1.Items()) != 2 || resPage1.NextPageToken() == "" {
		t.Fatalf("page1 total=%d items=%d token=%q", total, len(resPage1.Items()), resPage1.NextPageToken())
	}
	qPage2, err := preparer.PrepareList(plan, corecrud.ListInput{
		Collection: "oauthClients",
		PageSize:   2,
		PageToken:  resPage1.NextPageToken(),
	})
	if err != nil {
		t.Fatal(err)
	}
	resPage2, err := repo.List(ctx, qPage2)
	if err != nil || len(resPage2.Items()) != 1 || resPage2.Items()[0].GetClientId() != "static-admin" {
		t.Fatalf("page2 items=%+v err=%v", resPage2.Items(), err)
	}

	qFilter, err := preparer.PrepareList(plan, corecrud.ListInput{
		Collection: "oauthClients",
		Filter:     `source = CLIENT_SOURCE_DYNAMIC AND application_type = APPLICATION_TYPE_USER_AGENT AND token_endpoint_auth_method = TOKEN_ENDPOINT_AUTH_METHOD_NONE`,
	})
	if err != nil {
		t.Fatal(err)
	}
	resFilter, err := repo.List(ctx, qFilter)
	if err != nil || len(resFilter.Items()) != 1 || resFilter.Items()[0].GetClientId() != "partner-spa" {
		t.Fatalf("filtered items=%+v err=%v", resFilter.Items(), err)
	}

	// Update mutable fields and clear description & logo_uri.
	updatedWeb, err := repo.Update(ctx, "partner-web", &iamclientv1.OAuthClient{
		DisplayName: new("Partner Web v2"),
	}, &fieldmaskpb.FieldMask{Paths: []string{
		iamclientv1.OAuthClientFields.DisplayName.String(),
		iamclientv1.OAuthClientFields.Description.String(),
		iamclientv1.OAuthClientFields.LogoUri.String(),
	}}, createdWeb.GetEtag(), baseTime.Add(3*time.Minute))
	if err != nil {
		t.Fatalf("Update(partner-web): %v", err)
	}
	if updatedWeb.GetDisplayName() != "Partner Web v2" || updatedWeb.Description != nil || updatedWeb.LogoUri != nil {
		t.Fatalf("updatedWeb clear failed: %+v", updatedWeb)
	}

	// Secret rotation cap (max 5) & last-active-secret guard (expired secret does not count as active!).
	for i := 2; i <= biz.MaxClientSecrets; i++ {
		_, pref, h, _ := biz.NewClientSecret()
		var exp *timestamppb.Timestamp
		if i == biz.MaxClientSecrets {
			exp = timestamppb.New(baseTime.Add(-time.Hour))
		}
		if _, err := repo.CreateSecret(ctx, "partner-web", &iamclientv1.OAuthClientSecret{
			SecretId:     "sec-" + string(rune('0'+i)),
			Name:         "Key",
			SecretPrefix: pref,
			ExpiresTime:  exp,
		}, h, baseTime.Add(4*time.Minute)); err != nil {
			t.Fatalf("CreateSecret #%d: %v", i, err)
		}
	}
	if _, err := repo.CreateSecret(ctx, "partner-web", &iamclientv1.OAuthClientSecret{
		SecretId: "sec-6", Name: "Overflow", SecretPrefix: "pcs_overflow",
	}, "hash-6", baseTime.Add(4*time.Minute)); !errors.Is(err, biz.ErrSecretLimitExceeded) {
		t.Fatalf("CreateSecret #6 = %v, want ErrSecretLimitExceeded", err)
	}

	// Delete sec-2, sec-3, sec-4 -> leaves sec-1 (active) and sec-5 (expired at baseTime-1h).
	for _, id := range []string{"sec-2", "sec-3", "sec-4"} {
		if err := repo.DeleteSecret(ctx, "partner-web", id, baseTime.Add(5*time.Minute)); err != nil {
			t.Fatalf("DeleteSecret(%s): %v", id, err)
		}
	}
	// Deleting sec-1 when only expired sec-5 remains MUST fail with ErrLastActiveSecret!
	if err := repo.DeleteSecret(ctx, "partner-web", "sec-1", baseTime.Add(5*time.Minute)); !errors.Is(err, biz.ErrLastActiveSecret) {
		t.Fatalf("DeleteSecret(last active while expired exists) = %v, want ErrLastActiveSecret", err)
	}
	// Deleting expired sec-5 while active sec-1 remains MUST succeed!
	if err := repo.DeleteSecret(ctx, "partner-web", "sec-5", baseTime.Add(5*time.Minute)); err != nil {
		t.Fatalf("DeleteSecret(expired secret) = %v", err)
	}
	// If all 2 secrets expire later, deleting one already-expired secret is allowed to unblock rotation, while deleting the final secret is blocked!
	if _, err := repo.CreateSecret(ctx, "partner-web", &iamclientv1.OAuthClientSecret{
		SecretId: "sec-exp-a", Name: "Exp A", SecretPrefix: "pcs_exp_aaaa", ExpiresTime: timestamppb.New(baseTime.Add(6 * time.Minute)),
	}, "hash-exp-a", baseTime.Add(5*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if _, err := client.OAuthClientSecret.UpdateOneID("sec-1").SetExpiresTime(baseTime.Add(6 * time.Minute)).Save(ctx); err != nil {
		t.Fatal(err)
	}
	if err := repo.DeleteSecret(ctx, "partner-web", "sec-1", baseTime.Add(10*time.Minute)); err != nil {
		t.Fatalf("DeleteSecret(when all secrets expired, >1 total) = %v, want nil", err)
	}
	if err := repo.DeleteSecret(ctx, "partner-web", "sec-exp-a", baseTime.Add(10*time.Minute)); !errors.Is(err, biz.ErrLastActiveSecret) {
		t.Fatalf("DeleteSecret(final secret) = %v, want ErrLastActiveSecret", err)
	}
}

func TestOAuthClientDisableDeleteAndStaticPurgeCascadeRevocation(t *testing.T) {
	client, _ := newPostgresTestClient(t)
	ctx := t.Context()
	d := &Data{ent: client}
	clientRepo, err := NewOAuthClientRepository(d)
	if err != nil {
		t.Fatal(err)
	}
	oauthRepo, err := NewOAuthRepository(d)
	if err != nil {
		t.Fatal(err)
	}
	creator, err := NewInitialUserCreator(d)
	if err != nil {
		t.Fatal(err)
	}

	now := time.Date(2026, 11, 28, 9, 0, 0, 0, time.UTC)
	const userID = "01912345-6789-7abc-8def-000000000777"
	if err := creator.CreateInitialUser(ctx, userID, "user@example.com", "user@example.com", "hash", now); err != nil {
		t.Fatal(err)
	}
	if _, err := client.User.UpdateOneID(userID).SetRequiredActions([]string{}).Save(ctx); err != nil {
		t.Fatal(err)
	}
	login, err := client.LoginSession.Create().SetID("login-cascade").SetUserID(userID).SetAmr([]string{"pwd"}).SetCreateTime(now).Save(ctx)
	if err != nil {
		t.Fatal(err)
	}

	_, pref, secHash, _ := biz.NewClientSecret()
	dynClient, err := clientRepo.Create(ctx, &iamclientv1.OAuthClient{
		ClientId:                "cascade-app",
		DisplayName:             new("Cascade App"),
		ApplicationType:         iamclientv1.ApplicationType_APPLICATION_TYPE_WEB.Enum(),
		TokenEndpointAuthMethod: iamclientv1.TokenEndpointAuthMethod_TOKEN_ENDPOINT_AUTH_METHOD_CLIENT_SECRET_BASIC.Enum(),
		RedirectUris:            []string{"https://cascade.example.com/cb"},
		AllowedGrantTypes:       []string{biz.GrantTypeAuthorizationCode, biz.GrantTypeRefreshToken, biz.GrantTypeClientCredentials},
		AllowedResponseTypes:    []string{biz.ResponseTypeCode},
		AllowedScopes:           []string{"openid"},
		Audiences:               []string{"iam"},
	}, &iamclientv1.OAuthClientSecret{
		SecretId: "sec-cascade", Name: "Default", SecretPrefix: pref,
	}, secHash, now)
	if err != nil {
		t.Fatal(err)
	}

	seedClientArtifacts := func(suffix string, cid string) {
		t.Helper()
		reqID := "pending-req-" + suffix
		if _, err := client.OIDCAuthorizationRequest.Create().
			SetID(reqID).
			SetClientID(cid).
			SetRedirectURI("https://cascade.example.com/cb").
			SetResponseType("code").
			SetScopes([]string{"openid"}).
			SetPkceChallenge("challenge").
			SetExpiresTime(now.Add(time.Hour)).
			Save(ctx); err != nil {
			t.Fatal(err)
		}
		if _, err := client.AuthFlow.Create().
			SetID("flow-" + suffix).
			SetOwnerHash("owner-hash").
			SetStatus(authflow.StatusActive).
			SetState(authflow.StateAuthenticate).
			SetOidcRequestID(reqID).
			SetExpiresTime(now.Add(time.Hour)).
			SetCreateTime(now).
			SetUpdateTime(now).
			Save(ctx); err != nil {
			t.Fatal(err)
		}
		doneReqID := "done-req-" + suffix
		if _, err := client.OIDCAuthorizationRequest.Create().
			SetID(doneReqID).
			SetClientID(cid).
			SetRedirectURI("https://cascade.example.com/cb").
			SetResponseType("code").
			SetScopes([]string{"openid"}).
			SetPkceChallenge("challenge").
			SetSubject(userID).
			SetLoginSessionID(login.ID).
			SetAuthTime(now).
			SetDone(true).
			SetExpiresTime(now.Add(time.Hour)).
			Save(ctx); err != nil {
			t.Fatal(err)
		}
		if _, err := client.OAuthAuthorizationCode.Create().
			SetID("code-" + suffix).
			SetAuthorizationRequestID(doneReqID).
			SetCodeHash("code-hash-" + suffix).
			SetClientID(cid).
			SetSubject(userID).
			SetRedirectURI("https://cascade.example.com/cb").
			SetScopes([]string{"openid"}).
			SetPkceChallenge("challenge").
			SetExpiresTime(now.Add(5 * time.Minute)).
			Save(ctx); err != nil {
			t.Fatal(err)
		}
		tsID := "ts-" + suffix
		if _, err := client.OAuthTokenSession.Create().
			SetID(tsID).
			SetUserID(userID).
			SetClientID(cid).
			SetLoginSessionID(login.ID).
			SetRefreshFamilyID("fam-" + suffix).
			SetScopes([]string{"openid"}).
			SetAmr([]string{"pwd"}).
			SetAuthTime(now).
			Save(ctx); err != nil {
			t.Fatal(err)
		}
		if _, err := client.OAuthRefreshToken.Create().
			SetID("rt-" + suffix).
			SetTokenSessionID(tsID).
			SetFamilyID("fam-" + suffix).
			SetTokenHash("rt-hash-" + suffix).
			SetExpiresTime(now.Add(time.Hour)).
			Save(ctx); err != nil {
			t.Fatal(err)
		}
		if _, err := client.OAuthAccessToken.Create().
			SetID("at-human-" + suffix).
			SetActorType(oauthaccesstoken.ActorTypeHuman).
			SetTokenSessionID(tsID).
			SetSubject(userID).
			SetClientID(cid).
			SetScopes([]string{"openid"}).
			SetExpiresTime(now.Add(time.Hour)).
			Save(ctx); err != nil {
			t.Fatal(err)
		}
		if _, err := client.OAuthAccessToken.Create().
			SetID("at-svc-" + suffix).
			SetActorType(oauthaccesstoken.ActorTypeService).
			SetSubject(cid).
			SetClientID(cid).
			SetScopes([]string{}).
			SetAudiences([]string{"iam"}).
			SetExpiresTime(now.Add(time.Hour)).
			Save(ctx); err != nil {
			t.Fatal(err)
		}
	}

	if _, err := client.OAuthConsent.Create().
		SetID("consent-cascade").
		SetUserID(userID).
		SetClientID("cascade-app").
		SetGrantedScopes([]string{"openid"}).
		SetCreateTime(now).
		SetUpdateTime(now).
		Save(ctx); err != nil {
		t.Fatal(err)
	}

	seedClientArtifacts("disable", "cascade-app")
	revokeAt := now.Add(5 * time.Minute)
	disabled, err := clientRepo.UpdateStatus(ctx, "cascade-app", dynClient.GetEtag(), iamclientv1.ClientStatus_CLIENT_STATUS_DISABLED, revokeAt)
	if err != nil || disabled.GetStatus() != iamclientv1.ClientStatus_CLIENT_STATUS_DISABLED {
		t.Fatalf("Disable(cascade-app) = (%+v, %v)", disabled, err)
	}

	if f := client.AuthFlow.GetX(ctx, "flow-disable"); f.Status != authflow.StatusInvalidated {
		t.Fatalf("AuthFlow status after Disable = %q, want invalidated", f.Status)
	}
	if client.OIDCAuthorizationRequest.Query().Where(oidcauthorizationrequest.IDEQ("pending-req-disable")).CountX(ctx) != 0 {
		t.Fatal("pending OIDCAuthorizationRequest survived Disable")
	}
	if c := client.OAuthAuthorizationCode.GetX(ctx, "code-disable"); c.ConsumedTime == nil {
		t.Fatal("unconsumed OAuthAuthorizationCode not marked consumed on Disable")
	}
	if ts := client.OAuthTokenSession.GetX(ctx, "ts-disable"); ts.RevokedTime == nil {
		t.Fatal("OAuthTokenSession not revoked on Disable")
	}
	if rt := client.OAuthRefreshToken.GetX(ctx, "rt-disable"); rt.RevokedTime == nil {
		t.Fatal("OAuthRefreshToken not revoked on Disable")
	}
	if at := client.OAuthAccessToken.GetX(ctx, "at-human-disable"); at.RevokedTime == nil {
		t.Fatal("human OAuthAccessToken not revoked on Disable")
	}
	if at := client.OAuthAccessToken.GetX(ctx, "at-svc-disable"); at.RevokedTime == nil {
		t.Fatal("service OAuthAccessToken not revoked on Disable")
	}
	// Secrets & Consent stay intact on Disable so Enable restores usability cleanly.
	if client.OAuthConsent.Query().Where(oauthconsent.ClientIDEQ("cascade-app")).CountX(ctx) != 1 {
		t.Fatal("OAuthConsent unexpectedly deleted on Disable")
	}

	reenabled, err := clientRepo.UpdateStatus(ctx, "cascade-app", disabled.GetEtag(), iamclientv1.ClientStatus_CLIENT_STATUS_ACTIVE, revokeAt.Add(time.Minute))
	if err != nil || reenabled.GetStatus() != iamclientv1.ClientStatus_CLIENT_STATUS_ACTIVE {
		t.Fatalf("Enable(cascade-app) = (%+v, %v)", reenabled, err)
	}

	// Now test Delete cascade + physical removal of secrets, consents, and client!
	seedClientArtifacts("delete", "cascade-app")
	if err := clientRepo.Delete(ctx, "cascade-app", reenabled.GetEtag(), revokeAt.Add(2*time.Minute)); err != nil {
		t.Fatalf("Delete(cascade-app): %v", err)
	}
	if _, err := client.OAuthClient.Get(ctx, "cascade-app"); !entmodel.IsNotFound(err) {
		t.Fatalf("OAuthClient survived Delete: %v", err)
	}
	if client.OAuthClientSecret.Query().Where(oauthclientsecret.ClientIDEQ("cascade-app")).CountX(ctx) != 0 {
		t.Fatal("OAuthClientSecret survived Delete")
	}
	if client.OAuthConsent.Query().Where(oauthconsent.ClientIDEQ("cascade-app")).CountX(ctx) != 0 {
		t.Fatal("OAuthConsent survived Delete")
	}
	if client.AuthFlow.GetX(ctx, "flow-delete").Status != authflow.StatusInvalidated ||
		client.OAuthTokenSession.GetX(ctx, "ts-delete").RevokedTime == nil ||
		client.OAuthAccessToken.GetX(ctx, "at-svc-delete").RevokedTime == nil {
		t.Fatal("Delete did not cascade-revoke active flows/tokens")
	}

	// PurgeStaticClient cascades on static client and refuses dynamic client.
	if _, err := client.OAuthClient.Create().
		SetID("static-purge").
		SetSource(biz.ClientSourceStatic).
		SetStatus(biz.ClientStatusActive).
		SetDisplayName("Static Purge").
		SetApplicationType(biz.AppTypeWeb).
		SetTokenEndpointAuthMethod(biz.AuthMethodBasic).
		SetAllowedGrantTypes([]string{biz.GrantTypeClientCredentials}).
		SetAudiences([]string{"iam"}).
		SetEtag("etag-sp").
		SetCreateTime(now).
		SetUpdateTime(now).
		Save(ctx); err != nil {
		t.Fatal(err)
	}
	seedClientArtifacts("static", "static-purge")
	if err := oauthRepo.PurgeStaticClient(ctx, "static-purge", revokeAt.Add(3*time.Minute)); err != nil {
		t.Fatalf("PurgeStaticClient: %v", err)
	}
	if _, err := client.OAuthClient.Get(ctx, "static-purge"); !entmodel.IsNotFound(err) {
		t.Fatal("static-purge survived PurgeStaticClient")
	}
	if client.OAuthAccessToken.GetX(ctx, "at-svc-static").RevokedTime == nil {
		t.Fatal("PurgeStaticClient did not revoke service token")
	}
}

func TestOAuthConsentRepositoryLifecycleAndAuthorizedAppRevocation(t *testing.T) {
	client, _ := newPostgresTestClient(t)
	ctx := t.Context()
	d := &Data{ent: client}
	consents, err := NewOAuthConsentRepository(d)
	if err != nil {
		t.Fatal(err)
	}
	creator, err := NewInitialUserCreator(d)
	if err != nil {
		t.Fatal(err)
	}

	now := time.Date(2026, 11, 28, 11, 0, 0, 0, time.UTC)
	const userID = "01912345-6789-7abc-8def-000000000888"
	if err := creator.CreateInitialUser(ctx, userID, "alice@example.com", "alice@example.com", "hash", now); err != nil {
		t.Fatal(err)
	}
	if _, err := client.User.UpdateOneID(userID).SetRequiredActions([]string{}).Save(ctx); err != nil {
		t.Fatal(err)
	}
	login, err := client.LoginSession.Create().SetID("login-consent").SetUserID(userID).SetAmr([]string{"pwd"}).SetCreateTime(now).Save(ctx)
	if err != nil {
		t.Fatal(err)
	}

	for _, item := range []struct {
		id     string
		status string
	}{
		{id: "app-one", status: biz.ClientStatusActive},
		{id: "app-two", status: biz.ClientStatusActive},
		{id: "app-disabled", status: biz.ClientStatusDisabled},
	} {
		if _, err := client.OAuthClient.Create().
			SetID(item.id).
			SetSource(biz.ClientSourceDynamic).
			SetStatus(item.status).
			SetDisplayName("Display " + item.id).
			SetDescription("Desc " + item.id).
			SetLogoURI("https://cdn.example.com/" + item.id + ".png").
			SetApplicationType(biz.AppTypeWeb).
			SetTokenEndpointAuthMethod(biz.AuthMethodNone).
			SetRedirectUris([]string{"https://app.example.com/cb"}).
			SetAllowedGrantTypes([]string{biz.GrantTypeAuthorizationCode}).
			SetAllowedResponseTypes([]string{biz.ResponseTypeCode}).
			SetAllowedScopes([]string{"openid", "profile", "email"}).
			SetTrusted(false).
			SetEtag("etag-" + item.id).
			SetCreateTime(now).
			SetUpdateTime(now).
			Save(ctx); err != nil {
			t.Fatal(err)
		}
	}

	ownerHash := biz.HashOpaqueSecret("consent-owner-sqlite")
	if _, err := client.OIDCAuthorizationRequest.Create().
		SetID("req-approve-1").
		SetClientID("app-one").
		SetRedirectURI("https://app.example.com/cb").
		SetResponseType("code").
		SetScopes([]string{"openid", "profile"}).
		SetState("st-1").
		SetPkceChallenge("challenge").
		SetConsentOwnerHash(ownerHash).
		SetExpiresTime(now.Add(time.Hour)).
		Save(ctx); err != nil {
		t.Fatal(err)
	}

	pending, err := consents.GetConsentView(ctx, userID, ownerHash, "req-approve-1", now)
	if err != nil || pending.GetClientName() != "Display app-one" || pending.GetLogoUri() != "https://cdn.example.com/app-one.png" {
		t.Fatalf("GetConsentView = (%+v, %v)", pending, err)
	}
	if _, err := consents.GetConsentView(ctx, userID, biz.HashOpaqueSecret("other-browser"), "req-approve-1", now); !errors.Is(err, biz.ErrOIDCRequestInvalid) {
		t.Fatalf("GetConsentView(wrong owner) = %v, want ErrOIDCRequestInvalid", err)
	}

	cid, err := consents.ApproveConsent(ctx, userID, ownerHash, "req-approve-1", now)
	if err != nil || cid != "app-one" {
		t.Fatalf("ApproveConsent(1) = (%q, %v)", cid, err)
	}
	approvedRow := client.OIDCAuthorizationRequest.GetX(ctx, "req-approve-1")
	if !approvedRow.ConsentApproved || approvedRow.Subject == nil || *approvedRow.Subject != userID {
		t.Fatalf("ApproveConsent did not set consent_approved+subject: %+v", approvedRow)
	}

	// Second request adds "email" -> union must be ["email", "openid", "profile"] in a single row.
	if _, err := client.OIDCAuthorizationRequest.Create().
		SetID("req-approve-2").
		SetClientID("app-one").
		SetRedirectURI("https://app.example.com/cb").
		SetResponseType("code").
		SetScopes([]string{"openid", "email"}).
		SetPkceChallenge("challenge").
		SetConsentOwnerHash(ownerHash).
		SetExpiresTime(now.Add(time.Hour)).
		Save(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := consents.ApproveConsent(ctx, userID, ownerHash, "req-approve-2", now.Add(time.Minute)); err != nil {
		t.Fatalf("ApproveConsent(2) = %v", err)
	}
	view2, err := consents.GetConsentView(ctx, userID, ownerHash, "req-approve-2", now.Add(time.Minute))
	if err != nil || !slices.Equal(view2.GetGrantedScopes(), []string{"email", "openid", "profile"}) {
		t.Fatalf("GetConsentView(2) granted = (%v, %v)", view2, err)
	}

	// Seed consent for app-two and app-disabled; ListAuthorizedApps must omit app-disabled.
	for _, cid := range []string{"app-two", "app-disabled"} {
		if _, err := client.OAuthConsent.Create().
			SetID("consent-" + cid).
			SetUserID(userID).
			SetClientID(cid).
			SetGrantedScopes([]string{"openid"}).
			SetCreateTime(now).
			SetUpdateTime(now).
			Save(ctx); err != nil {
			t.Fatal(err)
		}
	}
	apps, err := consents.ListAuthorizedApps(ctx, userID)
	if err != nil || len(apps) != 2 || apps[0].GetClientId() != "app-one" || apps[1].GetClientId() != "app-two" {
		t.Fatalf("ListAuthorizedApps = (%+v, %v)", apps, err)
	}

	// Seed token sessions for app-one and app-two; RevokeAuthorizedApp(app-one) revokes ONLY app-one!
	for _, cid := range []string{"app-one", "app-two"} {
		if _, err := client.OAuthTokenSession.Create().
			SetID("ts-" + cid).
			SetUserID(userID).
			SetClientID(cid).
			SetLoginSessionID(login.ID).
			SetRefreshFamilyID("fam-" + cid).
			SetScopes([]string{"openid"}).
			SetAmr([]string{"pwd"}).
			SetAuthTime(now).
			Save(ctx); err != nil {
			t.Fatal(err)
		}
		if _, err := client.OAuthAccessToken.Create().
			SetID("at-" + cid).
			SetActorType(oauthaccesstoken.ActorTypeHuman).
			SetTokenSessionID("ts-" + cid).
			SetSubject(userID).
			SetClientID(cid).
			SetScopes([]string{"openid"}).
			SetExpiresTime(now.Add(time.Hour)).
			Save(ctx); err != nil {
			t.Fatal(err)
		}
	}
	if err := consents.RevokeAuthorizedApp(ctx, userID, "app-one", now.Add(2*time.Minute)); err != nil {
		t.Fatalf("RevokeAuthorizedApp(app-one): %v", err)
	}
	if client.OAuthTokenSession.GetX(ctx, "ts-app-one").RevokedTime == nil || client.OAuthAccessToken.GetX(ctx, "at-app-one").RevokedTime == nil {
		t.Fatal("app-one tokens survived RevokeAuthorizedApp")
	}
	if client.OAuthTokenSession.GetX(ctx, "ts-app-two").RevokedTime != nil || client.OAuthAccessToken.GetX(ctx, "at-app-two").RevokedTime != nil {
		t.Fatal("RevokeAuthorizedApp(app-one) wrongly revoked app-two tokens")
	}

	// DenyConsent destroys pending authorization request and invalidates active AuthFlow.
	if _, err := client.OIDCAuthorizationRequest.Create().
		SetID("req-deny-1").
		SetClientID("app-two").
		SetRedirectURI("https://app.example.com/cb").
		SetResponseType("code").
		SetState("deny-st").
		SetScopes([]string{"openid"}).
		SetPkceChallenge("challenge").
		SetConsentOwnerHash(ownerHash).
		SetExpiresTime(now.Add(time.Hour)).
		Save(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := client.AuthFlow.Create().
		SetID("flow-deny-1").
		SetOwnerHash("owner").
		SetStatus(authflow.StatusActive).
		SetState(authflow.StateAuthenticate).
		SetOidcRequestID("req-deny-1").
		SetExpiresTime(now.Add(time.Hour)).
		SetCreateTime(now).
		SetUpdateTime(now).
		Save(ctx); err != nil {
		t.Fatal(err)
	}
	redir, st, deniedCID, err := consents.DenyConsent(ctx, ownerHash, "req-deny-1", now.Add(3*time.Minute))
	if err != nil || redir != "https://app.example.com/cb" || st != "deny-st" || deniedCID != "app-two" {
		t.Fatalf("DenyConsent = (%q, %q, %q, %v)", redir, st, deniedCID, err)
	}
	if client.OIDCAuthorizationRequest.Query().Where(oidcauthorizationrequest.IDEQ("req-deny-1")).CountX(ctx) != 0 {
		t.Fatal("denied OIDCAuthorizationRequest survived DenyConsent")
	}
	if client.AuthFlow.GetX(ctx, "flow-deny-1").Status != authflow.StatusInvalidated {
		t.Fatal("DenyConsent did not invalidate bound AuthFlow")
	}
}
