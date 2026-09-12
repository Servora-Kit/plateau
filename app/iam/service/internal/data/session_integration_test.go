package data

import (
	"context"
	"errors"
	"testing"
	"time"

	"entgo.io/ent"
	userpb "github.com/Servora-Kit/plateau/api/gen/go/iam/user/v1"
	sessionpb "github.com/Servora-Kit/plateau/api/gen/go/plateau/security/session/v1"
	"github.com/Servora-Kit/plateau/app/iam/service/internal/biz"
	entmodel "github.com/Servora-Kit/plateau/app/iam/service/internal/data/ent"
	"github.com/Servora-Kit/plateau/app/iam/service/internal/data/ent/passwordauthenticator"
	"github.com/Servora-Kit/plateau/app/iam/service/internal/data/ent/passwordresettoken"
	"github.com/alexedwards/scs/postgresstore"
	"google.golang.org/protobuf/types/known/durationpb"
)

func TestPostgresSCSStoreUsesEntSchema(t *testing.T) {
	client, db := newPostgresTestClient(t)
	manager, cleanup, err := NewHTTPSessionManager(&sessionpb.Session{
		Lifetime: durationpb.New(time.Hour),
		Cookie:   &sessionpb.Cookie{Name: "__Host-iam_session"},
	}, db, client)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(cleanup)
	ctx, err := manager.Load(t.Context(), "")
	if err != nil {
		t.Fatal(err)
	}
	manager.Put(ctx, "login", "login-1")
	token, expiry, err := manager.Commit(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if token == "" || !expiry.After(time.Now()) {
		t.Fatal("missing session token or deadline")
	}
	loaded, err := manager.Load(t.Context(), token)
	if err != nil {
		t.Fatal(err)
	}
	if manager.GetString(loaded, "login") != "login-1" {
		t.Fatal("session data was not persisted")
	}
	if err := manager.RenewToken(loaded); err != nil {
		t.Fatal(err)
	}
	next, _, err := manager.Commit(loaded)
	if err != nil || next == token {
		t.Fatalf("rotation: same=%v error=%v", next == token, err)
	}
	old, err := manager.Load(t.Context(), token)
	if err != nil {
		t.Fatal(err)
	}
	if manager.GetString(old, "login") != "" {
		t.Fatal("old token remains usable")
	}
	if _, err := db.ExecContext(t.Context(), "UPDATE sessions SET expiry = $1", time.Now().Add(-time.Hour)); err != nil {
		t.Fatal(err)
	}
	expired, err := manager.Load(t.Context(), next)
	if err != nil {
		t.Fatal(err)
	}
	if manager.GetString(expired, "login") != "" {
		t.Fatal("expired session remains usable")
	}
	collector := postgresstore.NewWithCleanupInterval(db, 10*time.Millisecond)
	t.Cleanup(collector.StopCleanup)
	limit := time.Now().Add(time.Second)
	for {
		count := client.HTTPSession.Query().CountX(t.Context())
		if count == 0 {
			break
		}
		if time.Now().After(limit) {
			t.Fatal("official store did not clean expired sessions")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if err := db.PingContext(t.Context()); err != nil {
		t.Fatal(err)
	}
}

func seedLoginState(t *testing.T, client *entmodel.Client) (*userpb.User, biz.UserRepo, biz.SessionRepo, biz.CredentialRepo, []*biz.LoginSession) {
	t.Helper()
	d := &Data{ent: client}
	users, err := NewUserRepository(d)
	if err != nil {
		t.Fatal(err)
	}
	creator, err := NewInitialUserCreator(d)
	if err != nil {
		t.Fatal(err)
	}
	if err := creator.CreateInitialUser(t.Context(), "alice", "alice@example.com", "alice@example.com", "old-hash", time.Now()); err != nil {
		t.Fatal(err)
	}
	person, err := users.Get(t.Context(), "alice")
	if err != nil {
		t.Fatal(err)
	}
	sessions, err := NewSessionRepository(d)
	if err != nil {
		t.Fatal(err)
	}
	passwords, err := NewCredentialRepository(d)
	if err != nil {
		t.Fatal(err)
	}
	logins := make([]*biz.LoginSession, 2)
	for index := range logins {
		login, err := sessions.Create(t.Context(), "alice", time.Now())
		if err != nil {
			t.Fatal(err)
		}
		logins[index] = login
		_, err = client.OAuthTokenSession.Create().SetID(login.ID).
			SetUserID("alice").SetClientID("web").SetIamLoginSessionID(login.ID).
			SetRefreshFamilyID(login.ID).SetScopes([]string{"openid"}).SetAmr([]string{"pwd"}).SetAuthTime(login.AuthTime).Save(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		_, err = client.OAuthAccessToken.Create().SetID(login.ID).SetSubject("alice").SetClientID("web").
			SetTokenSessionID(login.ID).SetScopes([]string{"openid"}).SetExpiresTime(time.Now().Add(time.Hour)).Save(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		_, err = client.OAuthRefreshToken.Create().SetID(login.ID).SetTokenSessionID(login.ID).
			SetFamilyID(login.ID).SetTokenHash(login.ID).SetExpiresTime(time.Now().Add(time.Hour)).Save(t.Context())
		if err != nil {
			t.Fatal(err)
		}
	}
	return person, users, sessions, passwords, logins
}

func TestPostgresSessionReplacementPreservesIdentity(t *testing.T) {
	client, db := newPostgresTestClient(t)
	person, users, logins, passwords, previous := seedLoginState(t, client)
	ctx := t.Context()
	// Simulate a required legacy column that Schema.Create deliberately does not drop.
	if _, err := db.ExecContext(ctx, "ALTER TABLE iam_login_sessions ADD COLUMN token_hash text NOT NULL DEFAULT 'legacy'"); err != nil {
		t.Fatal(err)
	}
	// These statements run only inside this test's isolated schema, with IAM stopped.
	for _, statement := range []string{
		"DELETE FROM oauth_refresh_tokens",
		"DELETE FROM oauth_access_tokens",
		"DELETE FROM oauth_token_sessions",
		"DELETE FROM oauth_authorization_codes",
		"DELETE FROM oidc_authorization_requests",
		"DROP TABLE iam_login_sessions",
	} {
		if _, err := db.ExecContext(ctx, statement); err != nil {
			t.Fatal(err)
		}
	}
	if err := client.Schema.Create(ctx); err != nil {
		t.Fatal(err)
	}
	current, err := users.FindByEmail(ctx, person.GetEmail())
	if err != nil || current.GetUserId() != person.GetUserId() || !current.GetEmailVerified() {
		t.Fatalf("identity changed during session replacement: %v", err)
	}
	proof, err := passwords.FindActivePassword(ctx, person.GetUserId())
	if err != nil || proof.PasswordHash != "old-hash" {
		t.Fatalf("password changed during session replacement: %v", err)
	}
	if _, err := logins.Find(ctx, previous[0].ID); !errors.Is(err, biz.ErrNotFound) {
		t.Fatalf("old login survived replacement: %v", err)
	}
	if _, err := logins.Create(ctx, person.GetUserId(), time.Now()); err != nil {
		t.Fatalf("new session schema is unusable: %v", err)
	}
}

func TestPostgresIdentityMutationsAreAtomic(t *testing.T) {
	for _, operation := range []string{"password", "reset", "disable", "logout"} {
		t.Run(operation, func(t *testing.T) {
			client, _ := newPostgresTestClient(t)
			person, users, sessions, passwords, logins := seedLoginState(t, client)
			proof, err := passwords.FindActivePassword(t.Context(), "alice")
			if err != nil {
				t.Fatal(err)
			}
			reset, err := NewPasswordResetTokenRepository(&Data{ent: client})
			if err != nil {
				t.Fatal(err)
			}
			if err := reset.Create(t.Context(), "alice", "reset-proof", time.Now().Add(time.Hour)); err != nil {
				t.Fatal(err)
			}
			injected := errors.New("injected OAuth revocation failure")
			fail := true
			client.OAuthAccessToken.Use(func(next ent.Mutator) ent.Mutator {
				return ent.MutateFunc(func(ctx context.Context, mutation ent.Mutation) (ent.Value, error) {
					if fail && mutation.Op().Is(ent.OpUpdate|ent.OpUpdateOne) {
						return nil, injected
					}
					return next.Mutate(ctx, mutation)
				})
			})
			mutate := func() error {
				switch operation {
				case "password":
					return passwords.ReplacePassword(t.Context(), "alice", proof.AuthenticatorID, "old-hash", "new-hash", logins[0].ID, time.Now())
				case "reset":
					_, err := reset.ConsumeAndReplacePassword(t.Context(), "reset-proof", "new-hash", time.Now())
					return err
				case "disable":
					_, err := users.UpdateStatus(t.Context(), "alice", person.GetEtag(), userpb.UserStatus_USER_STATUS_DISABLED, time.Now())
					return err
				default:
					return sessions.Revoke(t.Context(), logins[0].ID, time.Now())
				}
			}
			if err := mutate(); !errors.Is(err, injected) {
				t.Fatalf("expected storage failure, got %v", err)
			}
			assertState := func(committed bool) {
				t.Helper()
				for index, login := range logins {
					got, err := sessions.Find(t.Context(), login.ID)
					if err != nil {
						t.Fatal(err)
					}
					wantLoginRevoked := committed && (operation == "reset" || operation == "disable" || operation == "logout" && index == 0 || operation == "password" && index == 1)
					if (got.RevokedAt != nil) != wantLoginRevoked {
						t.Fatalf("login %d revoked=%v", index, got.RevokedAt != nil)
					}
					wantOAuthRevoked := committed && (operation != "logout" || index == 0)
					session := client.OAuthTokenSession.GetX(t.Context(), login.ID)
					access := client.OAuthAccessToken.GetX(t.Context(), login.ID)
					refresh := client.OAuthRefreshToken.GetX(t.Context(), login.ID)
					if (session.RevokedTime != nil) != wantOAuthRevoked || (access.RevokedTime != nil) != wantOAuthRevoked || (refresh.RevokedTime != nil) != wantOAuthRevoked {
						t.Fatalf("OAuth revocation mismatch for login %d", index)
					}
				}
				stored := client.PasswordAuthenticator.Query().Where(passwordauthenticator.AuthenticatorIDEQ(proof.AuthenticatorID)).OnlyX(t.Context())
				wantHash := "old-hash"
				if committed && (operation == "password" || operation == "reset") {
					wantHash = "new-hash"
				}
				if stored.PasswordHash != wantHash {
					t.Fatal("password mutation did not follow transaction")
				}
				token := client.PasswordResetToken.Query().Where(passwordresettoken.TokenHashEQ("reset-proof")).OnlyX(t.Context())
				if (token.ConsumedTime != nil) != (committed && operation == "reset") {
					t.Fatal("reset token consumption did not follow transaction")
				}
				storedUser := client.User.GetX(t.Context(), "alice")
				if (storedUser.Status == biz.UserStatusDisabled) != (committed && operation == "disable") {
					t.Fatal("status mutation did not follow transaction")
				}
			}
			assertState(false)
			fail = false
			if err := mutate(); err != nil {
				t.Fatal(err)
			}
			assertState(true)
		})
	}
}
