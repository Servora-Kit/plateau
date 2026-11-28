package data

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	authnpb "github.com/Servora-Kit/plateau/api/gen/go/plateau/iam/authn/v1"
	"github.com/Servora-Kit/plateau/app/iam/service/internal/biz"
	"github.com/Servora-Kit/plateau/app/iam/service/internal/data/ent/authflow"
	"github.com/Servora-Kit/plateau/app/iam/service/internal/data/ent/migrate"
	"github.com/Servora-Kit/plateau/app/iam/service/internal/data/ent/passwordauthenticator"
	"github.com/Servora-Kit/plateau/security/password"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func TestAuthFlowSchemaNoSecretColumns(t *testing.T) {
	for _, col := range migrate.AuthFlowsTable.Columns {
		lower := strings.ToLower(col.Name)
		if lower == "password" || strings.Contains(lower, "password_hash") {
			t.Fatalf("auth_flows schema contains forbidden secret column %q", col.Name)
		}
	}
}

func TestPostgresAuthFlowSnapshotsResetAndCancel(t *testing.T) {
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
	credentials, err := NewCredentialRepository(d)
	if err != nil {
		t.Fatal(err)
	}
	flows, err := NewAuthFlowRepository(d)
	if err != nil {
		t.Fatal(err)
	}

	now := time.Date(2026, 11, 27, 9, 0, 0, 123456789, time.UTC)
	initHash, err := password.Hash("initial horse battery staple")
	if err != nil {
		t.Fatal(err)
	}
	const userID = "01912345-6789-7abc-8def-000000000901"
	if err := creator.CreateInitialUser(ctx, userID, "Alice@Example.com", "alice@example.com", initHash, now); err != nil {
		t.Fatal(err)
	}

	if _, err := client.OAuthClient.Create().
		SetID("admin-web").
		SetSource(biz.ClientSourceStatic).
		SetStatus(biz.ClientStatusActive).
		SetDisplayName("Admin Console").
		SetLogoURI("https://admin.example/logo.png").
		SetApplicationType(biz.AppTypeWeb).
		SetTokenEndpointAuthMethod(biz.AuthMethodBasic).
		SetRedirectUris([]string{"https://admin.example/callback"}).
		SetAllowedGrantTypes([]string{biz.GrantTypeAuthorizationCode}).
		SetAllowedResponseTypes([]string{biz.ResponseTypeCode}).
		SetAllowedScopes([]string{"openid"}).
		SetTrusted(true).
		SetEtag("etag-admin-web").
		SetCreateTime(now).
		SetUpdateTime(now).
		Save(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := client.OIDCAuthorizationRequest.Create().
		SetID("oidc-req-1").
		SetClientID("admin-web").
		SetRedirectURI("https://admin.example/callback").
		SetResponseType("code").
		SetScopes([]string{"openid"}).
		SetPkceChallenge("challenge").
		SetState("state-xyz").
		SetExpiresTime(now.Add(20 * time.Minute)).
		Save(ctx); err != nil {
		t.Fatal(err)
	}

	ownerHash := biz.HashOpaqueSecret("owner-1")
	otherOwner := biz.HashOpaqueSecret("owner-2")

	newSeed := func(id string) *authnpb.AuthFlow {
		f := biz.EvaluateFlowProgress(nil, biz.DefaultAuthPolicy(), nil)
		f.FlowId = id
		f.ExpiresTime = timestamppb.New(now.Add(30 * time.Minute))
		return f
	}

	created, err := flows.Create(ctx, newSeed("flow-oidc"), ownerHash, "oidc-req-1", now)
	if err != nil {
		t.Fatalf("Create(oidc): %v", err)
	}
	if created.GetOidc().GetClientId() != "admin-web" ||
		created.GetOidc().GetClientName() != "Admin Console" ||
		created.GetOidc().GetLogoUri() != "https://admin.example/logo.png" ||
		!created.GetExpiresTime().AsTime().Equal(now.Add(20*time.Minute).Truncate(time.Microsecond)) {
		t.Fatalf("Create(oidc) = %#v", created)
	}

	person, err := users.Get(ctx, userID)
	if err != nil {
		t.Fatal(err)
	}
	cred, err := credentials.FindActivePassword(ctx, userID)
	if err != nil {
		t.Fatal(err)
	}

	gated, login, err := flows.AdvanceAfterPassword(ctx, "flow-oidc", ownerHash, *cred, person.GetEtag(), []string{"pwd"}, 10*time.Minute, now)
	if err != nil || login != nil || gated.GetState() != authnpb.AuthFlowState_AUTH_FLOW_STATE_REQUIRED_ACTION {
		t.Fatalf("AdvanceAfterPassword() flow=%#v login=%#v err=%v", gated, login, err)
	}
	if gated.GetEmail() != "Alice@Example.com" || gated.GetOidc().GetClientId() != "admin-web" {
		t.Fatalf("gated flow metadata = %#v", gated)
	}

	nextHash, err := password.Hash("replacement horse battery staple")
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := flows.CompleteUpdatePassword(ctx, "flow-oidc", otherOwner, nextHash, 10*time.Minute, now.Add(time.Minute)); !errors.Is(err, biz.ErrAuthFlowNotFound) {
		t.Fatalf("CompleteUpdatePassword(wrong owner) = %v, want ErrAuthFlowNotFound", err)
	}

	if _, _, err := flows.CompleteUpdatePassword(ctx, "flow-oidc", ownerHash, nextHash, 10*time.Minute, now.Add(11*time.Minute)); !errors.Is(err, biz.ErrAuthFlowExpired) {
		t.Fatalf("CompleteUpdatePassword(step expired) = %v, want ErrAuthFlowExpired", err)
	}
	resetFlow, resetActor, err := flows.Reset(ctx, "flow-oidc", "flow-reset", ownerHash, now.Add(11*time.Minute))
	if err != nil || resetActor != userID || resetFlow.GetFlowId() != "flow-reset" || resetFlow.GetState() != authnpb.AuthFlowState_AUTH_FLOW_STATE_AUTHENTICATE || resetFlow.GetOidc().GetClientId() != "admin-web" {
		t.Fatalf("Reset() flow=%#v actor=%q err=%v", resetFlow, resetActor, err)
	}
	if oldSnap, err := flows.Get(ctx, "flow-oidc"); err != nil || oldSnap.Status != string(authflow.StatusReset) {
		t.Fatalf("old flow after Reset = %#v, err=%v", oldSnap, err)
	}

	if _, _, err := flows.AdvanceAfterPassword(ctx, "flow-reset", ownerHash, *cred, person.GetEtag(), []string{"pwd"}, 10*time.Minute, now.Add(11*time.Minute)); err != nil {
		t.Fatalf("AdvanceAfterPassword(flow-reset): %v", err)
	}
	if _, err := client.PasswordAuthenticator.Update().
		Where(passwordauthenticator.IDEQ(cred.PasswordAuthenticatorID)).
		SetChangedTime(now.Add(12 * time.Minute).Truncate(time.Microsecond)).
		Save(ctx); err != nil {
		t.Fatal(err)
	}
	if _, _, err := flows.CompleteUpdatePassword(ctx, "flow-reset", ownerHash, nextHash, 10*time.Minute, now.Add(12*time.Minute)); !errors.Is(err, biz.ErrAuthFlowInvalidated) {
		t.Fatalf("CompleteUpdatePassword(password_changed_time drift) = %v, want ErrAuthFlowInvalidated", err)
	}
	if driftedSnap, err := flows.Get(ctx, "flow-reset"); err != nil || driftedSnap.Status != biz.AuthFlowStatusInvalidated {
		t.Fatalf("drifted flow status = %#v, err=%v", driftedSnap, err)
	}

	if _, err := flows.Create(ctx, newSeed("flow-cancel"), ownerHash, "oidc-req-1", now.Add(12*time.Minute)); err != nil {
		t.Fatal(err)
	}
	redirectURI, stateVal, _, err := flows.Cancel(ctx, "flow-cancel", ownerHash, now.Add(13*time.Minute))
	if err != nil || redirectURI != "https://admin.example/callback" || stateVal != "state-xyz" {
		t.Fatalf("Cancel() redirect=%q state=%q err=%v", redirectURI, stateVal, err)
	}
	if _, _, _, err := flows.Cancel(ctx, "flow-cancel", ownerHash, now.Add(13*time.Minute)); !errors.Is(err, biz.ErrAuthFlowNotFound) {
		t.Fatalf("Cancel(already cancelled) = %v, want ErrAuthFlowNotFound", err)
	}
}

func TestPostgresAuthFlowConcurrentUpdatePasswordCompetition(t *testing.T) {
	client, _ := newPostgresTestClient(t)
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()

	d := &Data{ent: client}
	creator, err := NewInitialUserCreator(d)
	if err != nil {
		t.Fatal(err)
	}
	users, err := NewUserRepository(d)
	if err != nil {
		t.Fatal(err)
	}
	credentials, err := NewCredentialRepository(d)
	if err != nil {
		t.Fatal(err)
	}
	flows, err := NewAuthFlowRepository(d)
	if err != nil {
		t.Fatal(err)
	}

	now := time.Now().UTC().Truncate(time.Microsecond)
	initHash, err := password.Hash("initial horse battery staple")
	if err != nil {
		t.Fatal(err)
	}
	const userID = "01912345-6789-7abc-8def-000000000902"
	if err := creator.CreateInitialUser(ctx, userID, "race@example.com", "race@example.com", initHash, now); err != nil {
		t.Fatal(err)
	}
	person, err := users.Get(ctx, userID)
	if err != nil {
		t.Fatal(err)
	}
	cred, err := credentials.FindActivePassword(ctx, userID)
	if err != nil {
		t.Fatal(err)
	}

	ownerA := biz.HashOpaqueSecret("owner-race-a")
	ownerB := biz.HashOpaqueSecret("owner-race-b")
	for _, item := range []struct {
		id    string
		owner string
	}{
		{id: "race-flow-a", owner: ownerA},
		{id: "race-flow-b", owner: ownerB},
	} {
		seed := biz.EvaluateFlowProgress(nil, biz.DefaultAuthPolicy(), nil)
		seed.FlowId = item.id
		seed.ExpiresTime = timestamppb.New(now.Add(30 * time.Minute))
		if _, err := flows.Create(ctx, seed, item.owner, "", now); err != nil {
			t.Fatal(err)
		}
		if _, _, err := flows.AdvanceAfterPassword(ctx, item.id, item.owner, *cred, person.GetEtag(), []string{"pwd"}, 10*time.Minute, now); err != nil {
			t.Fatal(err)
		}
	}

	hashA, err := password.Hash("winner or loser password A")
	if err != nil {
		t.Fatal(err)
	}
	hashB, err := password.Hash("winner or loser password B")
	if err != nil {
		t.Fatal(err)
	}

	type outcome struct {
		flow  *authnpb.AuthFlow
		login *biz.LoginSession
		err   error
	}
	start := make(chan struct{})
	results := make([]outcome, 2)
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		<-start
		f, l, e := flows.CompleteUpdatePassword(ctx, "race-flow-a", ownerA, hashA, 10*time.Minute, now.Add(time.Second))
		results[0] = outcome{flow: f, login: l, err: e}
	}()
	go func() {
		defer wg.Done()
		<-start
		f, l, e := flows.CompleteUpdatePassword(ctx, "race-flow-b", ownerB, hashB, 10*time.Minute, now.Add(time.Second))
		results[1] = outcome{flow: f, login: l, err: e}
	}()
	close(start)
	wg.Wait()

	var wins, invalidated int
	for _, r := range results {
		switch {
		case r.err == nil && r.login != nil && r.flow.GetState() == authnpb.AuthFlowState_AUTH_FLOW_STATE_COMPLETED:
			wins++
		case errors.Is(r.err, biz.ErrAuthFlowInvalidated):
			invalidated++
		default:
			t.Fatalf("unexpected concurrent CompleteUpdatePassword outcome: flow=%#v login=%#v err=%v", r.flow, r.login, r.err)
		}
	}
	if wins != 1 || invalidated != 1 {
		t.Fatalf("concurrent CompleteUpdatePassword wins=%d invalidated=%d, want 1 and 1", wins, invalidated)
	}
	updatedUser, err := users.Get(ctx, userID)
	if err != nil || len(updatedUser.GetRequiredActions()) != 0 {
		t.Fatalf("updatedUser after race = %#v, err=%v", updatedUser, err)
	}
}
