package biz

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
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

func (creator *fakeBootstrapCreator) CreateInitialAdmin(_ context.Context, userID, email, _ string, passwordHash string, _ time.Time) error {
	creator.calls++
	creator.passwordHash = passwordHash
	creator.users.user = &userpb.User{
		Name: "users/" + userID, UserId: userID, Email: &email,
		Status: userpb.UserStatus_USER_STATUS_ACTIVE, EmailVerified: true,
	}
	return nil
}

type fakeAdminRelations struct {
	calls  int
	userID string
	err    error
}

func (relations *fakeAdminRelations) EnsurePlatformAdmin(_ context.Context, userID string) error {
	relations.calls++
	relations.userID = userID
	return relations.err
}

func TestAdminBootstrapCreatesOnceAndReusesIdentity(t *testing.T) {
	users := new(fakeAccountUsers)
	creator := &fakeBootstrapCreator{users: users}
	relations := new(fakeAdminRelations)
	var output bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&output, nil))
	bootstrap, err := NewAdminInitializer(
		&iamconfpb.IAM{BootstrapAdminEmail: " Admin@Example.com "},
		users, creator, relations, logger,
	)
	if err != nil {
		t.Fatalf("NewAdminInitializer() error = %v", err)
	}
	bootstrap.now = func() time.Time { return time.Unix(1_700_000_000, 0) }

	if err := bootstrap.Initialize(t.Context()); err != nil {
		t.Fatalf("first Initialize() error = %v", err)
	}
	if creator.calls != 1 || relations.calls != 1 || relations.userID != users.user.GetUserId() {
		t.Fatalf("creator calls=%d relation calls=%d user=%q", creator.calls, relations.calls, relations.userID)
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
	if creator.calls != 1 || relations.calls != 2 {
		t.Fatalf("repeated run creator calls=%d relation calls=%d", creator.calls, relations.calls)
	}
	if strings.Count(output.String(), "initial_password") != 1 {
		t.Fatalf("initial password emitted more than once: %s", output.String())
	}
}

func TestAdminBootstrapFailsWhenRelationCannotBeEnsured(t *testing.T) {
	users := &fakeAccountUsers{user: &userpb.User{
		UserId: "user-1", Email: stringPtr("admin@example.com"),
		Status: userpb.UserStatus_USER_STATUS_ACTIVE, EmailVerified: true,
	}}
	relations := &fakeAdminRelations{err: errors.New("OpenFGA unavailable")}
	bootstrap, err := NewAdminInitializer(
		&iamconfpb.IAM{BootstrapAdminEmail: "admin@example.com"},
		users, &fakeBootstrapCreator{users: users}, relations, nil,
	)
	if err != nil {
		t.Fatalf("NewAdminInitializer() error = %v", err)
	}
	if err := bootstrap.Initialize(t.Context()); err == nil || !strings.Contains(err.Error(), "ensure bootstrap admin relation") {
		t.Fatalf("Initialize() error = %v", err)
	}
}
