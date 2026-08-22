package data

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"testing"
	"time"

	"entgo.io/ent/dialect/sql"
	userpb "github.com/Servora-Kit/plateau/api/gen/go/iam/user/v1"
	"github.com/Servora-Kit/plateau/app/iam/service/internal/biz"
	entmodel "github.com/Servora-Kit/plateau/app/iam/service/internal/data/ent"
	"github.com/Servora-Kit/plateau/app/iam/service/internal/data/ent/user"
	"github.com/Servora-Kit/plateau/security/password"
	redispb "github.com/Servora-Kit/servora/api/gen/go/servora/contrib/db/redis/v1"
	_ "github.com/mattn/go-sqlite3"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/fieldmaskpb"
)

func newTestEntClient(t *testing.T) *entmodel.Client {
	t.Helper()

	driver, err := sql.Open("sqlite3", fmt.Sprintf("file:%s?mode=memory&cache=shared&_fk=1", t.Name()))
	if err != nil {
		t.Fatalf("open SQLite driver: %v", err)
	}
	client, cleanup, err := NewDBClient(driver)
	if err != nil {
		t.Fatalf("create test schema: %v", err)
	}
	t.Cleanup(cleanup)
	return client
}

func TestSchemaCreateIsIdempotentAndNonDestructive(t *testing.T) {

	client := newTestEntClient(t)
	ctx := t.Context()
	if _, err := client.User.Create().SetID("schema-user").SetEtag("etag-1").Save(ctx); err != nil {
		t.Fatalf("create user: %v", err)
	}
	if err := client.Schema.Create(ctx); err != nil {
		t.Fatalf("repeat schema creation: %v", err)
	}
	if _, err := client.User.Get(ctx, "schema-user"); err != nil {
		t.Fatalf("user disappeared after repeated schema creation: %v", err)
	}
}

func TestNewDBClientClosesDriverWhenMigrationFails(t *testing.T) {

	driver, err := sql.Open("sqlite3", fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name()))
	if err != nil {
		t.Fatalf("open SQLite driver: %v", err)
	}
	if err := driver.Close(); err != nil {
		t.Fatalf("close SQLite driver: %v", err)
	}

	client, cleanup, err := NewDBClient(driver)
	if err == nil {
		if cleanup != nil {
			cleanup()
		}
		t.Fatal("NewDBClient() error = nil, want migration failure")
	}
	if client != nil || cleanup != nil {
		t.Fatalf("NewDBClient() returned client=%t cleanup=%t after failure", client != nil, cleanup != nil)
	}
}

func TestInTxCommitsRollsBackAndSupportsConditionalUpdate(t *testing.T) {

	client := newTestEntClient(t)
	data := &Data{ent: client}
	ctx := t.Context()

	rollbackErr := errors.New("rollback transaction")
	err := data.InTx(ctx, func(tx *entmodel.Tx) error {
		if _, err := tx.User.Create().SetID("rolled-back-user").SetEtag("etag-1").Save(ctx); err != nil {
			return err
		}
		return rollbackErr
	})
	if !errors.Is(err, rollbackErr) {
		t.Fatalf("InTx() error = %v, want rollback error", err)
	}
	if _, err := client.User.Get(ctx, "rolled-back-user"); !entmodel.IsNotFound(err) {
		t.Fatalf("rolled-back user lookup error = %v, want not found", err)
	}

	if err := data.InTx(ctx, func(tx *entmodel.Tx) error {
		_, err := tx.User.Create().SetID("committed-user").SetEtag("etag-1").Save(ctx)
		return err
	}); err != nil {
		t.Fatalf("commit transaction: %v", err)
	}

	updated, err := client.User.Update().
		Where(user.IDEQ("committed-user"), user.EtagEQ("etag-1")).
		SetStatus("active").
		SetEtag("etag-2").
		Save(ctx)
	if err != nil {
		t.Fatalf("conditional update: %v", err)
	}
	if updated != 1 {
		t.Fatalf("conditional update count = %d, want 1", updated)
	}
	stale, err := client.User.Update().
		Where(user.IDEQ("committed-user"), user.EtagEQ("etag-1")).
		SetStatus("disabled").
		Save(ctx)
	if err != nil {
		t.Fatalf("stale conditional update: %v", err)
	}
	if stale != 0 {
		t.Fatalf("stale conditional update count = %d, want 0", stale)
	}

	if query := client.User.Query().ForUpdate(); query == nil {
		t.Fatal("generated User query does not expose row locking")
	}
}

func TestNewRedisClientPropagatesPingFailure(t *testing.T) {
	t.Parallel()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	client, cleanup, err := NewRedisClient(&redispb.Redis{
		Addr:         "127.0.0.1:1",
		DialTimeout:  durationpb.New(20 * time.Millisecond),
		ReadTimeout:  durationpb.New(20 * time.Millisecond),
		WriteTimeout: durationpb.New(20 * time.Millisecond),
	}, logger)
	if err == nil {
		if cleanup != nil {
			cleanup()
		}
		t.Fatal("NewRedisClient() error = nil, want Ping failure")
	}
	if client != nil || cleanup != nil {
		t.Fatalf("NewRedisClient() returned client=%t cleanup=%t after failure", client != nil, cleanup != nil)
	}
}

func TestInTxRejectsNilFunction(t *testing.T) {
	t.Parallel()

	data := &Data{}
	if err := data.InTx(context.Background(), nil); err == nil {
		t.Fatal("InTx() error = nil, want nil function error")
	}
}

func TestUserAndCredentialRepositoriesCreateAndFind(t *testing.T) {
	client := newTestEntClient(t)
	data := &Data{ent: client}
	users, err := NewUserRepository(data)
	if err != nil {
		t.Fatalf("NewUserRepository() error = %v", err)
	}
	credentials, err := NewCredentialRepository(data)
	if err != nil {
		t.Fatalf("NewCredentialRepository() error = %v", err)
	}
	hash, err := password.Hash("correct horse battery staple")
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	email := "Person@Example.com"
	resource := &userpb.User{
		Name:   "users/01912345-6789-7abc-8def-0123456789ab",
		UserId: "01912345-6789-7abc-8def-0123456789ab",
		Email:  &email,
		Status: userpb.UserStatus_USER_STATUS_PENDING_EMAIL_VERIFICATION,
	}
	created, err := users.Create(t.Context(), resource, hash, "person@example.com")
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if created.GetStatus() != userpb.UserStatus_USER_STATUS_PENDING_EMAIL_VERIFICATION || created.GetEmailVerified() {
		t.Fatalf("created user = %#v, want pending and unverified", created)
	}
	found, err := users.FindByEmail(t.Context(), "person@example.com")
	if err != nil {
		t.Fatalf("FindByEmail() error = %v", err)
	}
	if found.GetUserId() != created.GetUserId() || found.GetEmail() != "Person@Example.com" {
		t.Fatalf("FindByEmail() = %#v, want display email and stable ID", found)
	}
	credential, err := credentials.FindActivePassword(t.Context(), created.GetUserId())
	if err != nil {
		t.Fatalf("FindActivePassword() error = %v", err)
	}
	match, _, err := password.Compare("correct horse battery staple", credential.PasswordHash)
	if err != nil || !match {
		t.Fatalf("stored password hash match=%t error=%v", match, err)
	}
}

func TestUserRepositoryUpdateClearsOnlySelectedProfileFields(t *testing.T) {
	client := newTestEntClient(t)
	users, err := NewUserRepository(&Data{ent: client})
	if err != nil {
		t.Fatalf("NewUserRepository() error = %v", err)
	}
	email := "person@example.com"
	created, err := users.Create(t.Context(), &userpb.User{
		UserId: "01912345-6789-7abc-8def-0123456789ad",
		Email:  &email,
		Profile: &userpb.UserProfile{
			Name:     proto.String("Person Name"),
			Nickname: proto.String("person"),
		},
	}, "password-hash", email)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	updated, err := users.UpdateUser(
		t.Context(),
		userpb.NewUserName(created.GetUserId()),
		&userpb.User{Profile: &userpb.UserProfile{Name: proto.String("Updated Name")}},
		&fieldmaskpb.FieldMask{Paths: []string{
			userpb.UserFields.ProfileName.String(),
			userpb.UserFields.ProfileNickname.String(),
		}},
		created.GetEtag(),
	)
	if err != nil {
		t.Fatalf("UpdateUser() error = %v", err)
	}
	if got := updated.GetProfile().GetName(); got != "Updated Name" {
		t.Fatalf("profile.name = %q, want Updated Name", got)
	}
	if updated.GetProfile().Nickname != nil {
		t.Fatalf("profile.nickname = %q, want absent", updated.GetProfile().GetNickname())
	}
}

func TestBootstrapUserCreatorCreatesActiveVerifiedAdmin(t *testing.T) {
	client := newTestEntClient(t)
	data := &Data{ent: client}
	creator, err := NewInitialAdminCreator(data)
	if err != nil {
		t.Fatalf("NewInitialAdminCreator() error = %v", err)
	}
	users, err := NewUserRepository(data)
	if err != nil {
		t.Fatalf("NewUserRepository() error = %v", err)
	}

	const userID = "01912345-6789-7abc-8def-0123456789ae"
	if err := creator.CreateInitialAdmin(t.Context(), userID, "Admin@Example.com", "admin@example.com", "password-hash", time.Unix(1_700_000_000, 0)); err != nil {
		t.Fatalf("CreateInitialAdmin() error = %v", err)
	}
	admin, err := users.FindByEmail(t.Context(), "admin@example.com")
	if err != nil {
		t.Fatalf("FindByEmail() error = %v", err)
	}
	if admin.GetUserId() != userID || admin.GetStatus() != userpb.UserStatus_USER_STATUS_ACTIVE || !admin.GetEmailVerified() {
		t.Fatalf("bootstrap admin = %#v", admin)
	}
}

func TestPasswordResetTokenConsumptionReplacesPasswordOnce(t *testing.T) {
	client := newTestEntClient(t)
	data := &Data{ent: client}
	users, err := NewUserRepository(data)
	if err != nil {
		t.Fatalf("NewUserRepository() error = %v", err)
	}
	credentials, err := NewCredentialRepository(data)
	if err != nil {
		t.Fatalf("NewCredentialRepository() error = %v", err)
	}
	resetTokens, err := NewPasswordResetTokenRepository(data)
	if err != nil {
		t.Fatalf("NewPasswordResetTokenRepository() error = %v", err)
	}
	oldHash, err := password.Hash("correct horse battery staple")
	if err != nil {
		t.Fatalf("hash old password: %v", err)
	}
	email := "person@example.com"
	resource := &userpb.User{UserId: "01912345-6789-7abc-8def-0123456789ac", Email: &email}
	created, err := users.Create(t.Context(), resource, oldHash, email)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	tokenHash := biz.HashOpaqueSecret("reset-token")
	if err := resetTokens.Create(t.Context(), created.GetUserId(), tokenHash, time.Now().Add(time.Hour)); err != nil {
		t.Fatalf("Create reset token: %v", err)
	}
	newHash, err := password.Hash("new correct horse battery staple")
	if err != nil {
		t.Fatalf("hash new password: %v", err)
	}
	userID, err := resetTokens.ConsumeAndReplacePassword(t.Context(), tokenHash, newHash, time.Now())
	if err != nil || userID != created.GetUserId() {
		t.Fatalf("ConsumeAndReplacePassword() user=%q error=%v", userID, err)
	}
	credential, err := credentials.FindActivePassword(t.Context(), userID)
	if err != nil {
		t.Fatalf("FindActivePassword() error = %v", err)
	}
	match, _, err := password.Compare("new correct horse battery staple", credential.PasswordHash)
	if err != nil || !match {
		t.Fatalf("replacement password match=%t error=%v", match, err)
	}
	if _, err := resetTokens.ConsumeAndReplacePassword(t.Context(), tokenHash, oldHash, time.Now()); !errors.Is(err, biz.ErrMutationMiss) {
		t.Fatalf("replay error = %v, want conditional mutation miss", err)
	}
}
