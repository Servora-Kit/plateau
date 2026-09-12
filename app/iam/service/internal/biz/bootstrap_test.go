package biz

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"
	"time"

	iamconfpb "github.com/Servora-Kit/plateau/api/gen/go/iam/conf/v1"
	userpb "github.com/Servora-Kit/plateau/api/gen/go/iam/user/v1"
	"github.com/Servora-Kit/plateau/security/password"
)

type fakeBootstrapCreator struct {
	users        *fakeAccountUsers
	calls        int
	passwordHash string
}

func (creator *fakeBootstrapCreator) CreateInitialUser(_ context.Context, userID, email, _ string, passwordHash string, _ time.Time) error {
	creator.calls++
	creator.passwordHash = passwordHash
	creator.users.user = &userpb.User{
		Name: "users/" + userID, UserId: userID, Email: &email,
		Status: userpb.UserStatus_USER_STATUS_ACTIVE, EmailVerified: true,
	}
	return nil
}

func TestUserBootstrapCreatesOnceAndReusesIdentity(t *testing.T) {
	users := new(fakeAccountUsers)
	creator := &fakeBootstrapCreator{users: users}
	var output bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&output, nil))
	bootstrap, err := NewUserInitializer(
		&iamconfpb.IAM{BootstrapUserEmail: " Admin@Example.com "},
		users, creator, logger,
	)
	if err != nil {
		t.Fatalf("NewUserInitializer() error = %v", err)
	}
	bootstrap.now = func() time.Time { return time.Unix(1_700_000_000, 0) }

	if err := bootstrap.Initialize(t.Context()); err != nil {
		t.Fatalf("first Initialize() error = %v", err)
	}
	if creator.calls != 1 {
		t.Fatalf("creator calls=%d", creator.calls)
	}
	var entry map[string]any
	if err := json.Unmarshal(output.Bytes(), &entry); err != nil {
		t.Fatalf("decode startup credential log: %v", err)
	}
	initialPassword, _ := entry["initial_password"].(string)
	match, _, err := password.Compare(initialPassword, creator.passwordHash)
	if err != nil || !match || initialPassword == creator.passwordHash {
		t.Fatalf("initial password handoff match=%t error=%v", match, err)
	}

	if err := bootstrap.Initialize(t.Context()); err != nil {
		t.Fatalf("second Initialize() error = %v", err)
	}
	if creator.calls != 1 {
		t.Fatalf("repeated creator calls=%d", creator.calls)
	}
	if strings.Count(output.String(), "initial_password") != 1 {
		t.Fatalf("initial password emitted more than once: %s", output.String())
	}
}

func TestUserBootstrapRejectsInactiveExistingUser(t *testing.T) {
	users := &fakeAccountUsers{user: &userpb.User{UserId: "user-1", Status: userpb.UserStatus_USER_STATUS_DISABLED}}
	creator := &fakeBootstrapCreator{users: users}
	bootstrap, err := NewUserInitializer(&iamconfpb.IAM{BootstrapUserEmail: "alice@example.com"}, users, creator, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := bootstrap.Initialize(t.Context()); err == nil {
		t.Fatal("inactive existing user accepted")
	}
	if creator.calls != 0 {
		t.Fatal("existing user was replaced")
	}
}
