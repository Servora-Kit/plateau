package data

import (
	"context"
	"errors"
	"testing"
	"time"

	"entgo.io/ent"
	"github.com/Servora-Kit/plateau/app/iam/service/internal/biz"
	entmodel "github.com/Servora-Kit/plateau/app/iam/service/internal/data/ent"
)

type transactionOrderKey struct{}

func TestPostgresOAuthIssuanceSerializesWithLogout(t *testing.T) {
	for _, grantKind := range []string{"code", "refresh"} {
		for _, first := range []string{"issue", "logout"} {
			t.Run(grantKind+"/"+first, func(t *testing.T) {
				client, _ := newPostgresTestClient(t)
				_, _, sessions, _, logins := seedLoginState(t, client)
				repo, err := NewOAuthRepository(&Data{ent: client})
				if err != nil {
					t.Fatal(err)
				}
				grant := prepareOAuthGrant(t, client, grantKind, logins[0])
				issuedAt := time.Now()
				issue := biz.OAuthTokenIssue{AccessID: "issued", RefreshID: "rotated", RefreshHash: "new-refresh-hash", IssuedAt: issuedAt, AccessExpiresAt: issuedAt.Add(time.Minute), RefreshExpiresAt: issuedAt.Add(time.Hour)}
				ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
				defer cancel()
				firstBlocked, secondStarted, release := make(chan struct{}), make(chan struct{}), make(chan struct{})
				client.OAuthAccessToken.Use(func(next ent.Mutator) ent.Mutator {
					return ent.MutateFunc(func(ctx context.Context, mutation ent.Mutation) (ent.Value, error) {
						if ctx.Value(transactionOrderKey{}) == "first" {
							close(firstBlocked)
							select {
							case <-release:
							case <-ctx.Done():
								return nil, ctx.Err()
							}
						}
						return next.Mutate(ctx, mutation)
					})
				})
				client.User.Intercept(ent.InterceptFunc(func(next ent.Querier) ent.Querier {
					return ent.QuerierFunc(func(ctx context.Context, query ent.Query) (ent.Value, error) {
						if ctx.Value(transactionOrderKey{}) == "second" {
							close(secondStarted)
						}
						return next.Query(ctx, query)
					})
				}))
				issueOp := func(ctx context.Context) error { return repo.Issue(ctx, grant, issue) }
				logoutOp := func(ctx context.Context) error { return sessions.Revoke(ctx, logins[0].ID, time.Now()) }
				firstOp, secondOp := issueOp, logoutOp
				if first == "logout" {
					firstOp, secondOp = logoutOp, issueOp
				}
				firstResult, secondResult := make(chan error, 1), make(chan error, 1)
				go func() { firstResult <- firstOp(context.WithValue(ctx, transactionOrderKey{}, "first")) }()
				select {
				case <-firstBlocked:
				case err := <-firstResult:
					t.Fatalf("first operation failed before write: %v", err)
				case <-ctx.Done():
					t.Fatal(ctx.Err())
				}
				go func() { secondResult <- secondOp(context.WithValue(ctx, transactionOrderKey{}, "second")) }()
				select {
				case <-secondStarted:
				case <-ctx.Done():
					t.Fatal(ctx.Err())
				}
				close(release)
				if err := <-firstResult; err != nil {
					t.Fatalf("first commit: %v", err)
				}
				err = <-secondResult
				if first == "logout" {
					if !errors.Is(err, biz.ErrOAuthGrantInvalid) {
						t.Fatalf("issuance after revocation: %v", err)
					}
					if _, err := client.OAuthAccessToken.Get(t.Context(), "issued"); !entmodel.IsNotFound(err) {
						t.Fatal("revocation-first request persisted a token")
					}
				} else {
					if err != nil {
						t.Fatal(err)
					}
					access := client.OAuthAccessToken.GetX(t.Context(), "issued")
					refresh := client.OAuthRefreshToken.GetX(t.Context(), "rotated")
					if access.RevokedTime == nil || refresh.RevokedTime == nil {
						t.Fatal("logout failed to revoke tokens committed before it")
					}
				}
			})
		}
	}
}

func prepareOAuthGrant(t *testing.T, client *entmodel.Client, kind string, login *biz.LoginSession) biz.OAuthGrant {
	t.Helper()
	grant := biz.OAuthGrant{Subject: login.UserID, ClientID: "web", LoginID: login.ID, Scopes: []string{"openid"}, Audiences: []string{"web"}, AuthTime: login.AuthTime, AMR: []string{"pwd"}}
	if kind == "refresh" {
		grant.TokenSessionID, grant.RefreshTokenID = login.ID, login.ID
		return grant
	}
	_, err := client.OIDCAuthorizationRequest.Create().SetID("request").SetClientID("web").
		SetRedirectURI("https://web.example/callback").SetResponseType("code").SetScopes(grant.Scopes).
		SetPkceChallenge("challenge").SetSubject(login.UserID).SetIamLoginSessionID(login.ID).SetAuthTime(login.AuthTime).
		SetDone(true).SetExpiresTime(time.Now().Add(time.Hour)).Save(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.OAuthAuthorizationCode.Create().SetID("code").SetCodeHash("code-hash").SetAuthorizationRequestID("request").
		SetClientID("web").SetSubject(login.UserID).SetRedirectURI("https://web.example/callback").SetScopes(grant.Scopes).
		SetPkceChallenge("challenge").SetExpiresTime(time.Now().Add(time.Hour)).Save(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	grant.CodeID = "code"
	return grant
}
