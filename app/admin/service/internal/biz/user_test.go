package biz

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"

	adminconfpb "github.com/Servora-Kit/plateau/api/gen/go/admin/conf/v1"
	adminuserpb "github.com/Servora-Kit/plateau/api/gen/go/admin/user/v1"
	userpb "github.com/Servora-Kit/plateau/api/gen/go/iam/user/v1"
	"github.com/Servora-Kit/plateau/security"
	"github.com/Servora-Kit/plateau/security/authz/openfga"
	fgaclient "github.com/openfga/go-sdk/client"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/fieldmaskpb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type fakeIAMUserRepo struct {
	listFn   func(context.Context, *userpb.ListUsersRequest) (*userpb.ListUsersResponse, error)
	createFn func(context.Context, *userpb.CreateUserRequest) (*userpb.User, error)
	emailFn  func(context.Context, *userpb.SendUserVerificationEmailRequest) error
	updateFn func(context.Context, *userpb.UpdateUserRequest) (*userpb.User, error)
}

func (f *fakeIAMUserRepo) ListUsers(ctx context.Context, req *userpb.ListUsersRequest) (*userpb.ListUsersResponse, error) {
	if f.listFn != nil {
		return f.listFn(ctx, req)
	}
	return &userpb.ListUsersResponse{}, nil
}

func (f *fakeIAMUserRepo) GetUser(context.Context, *userpb.GetUserRequest) (*userpb.User, error) {
	return &userpb.User{}, nil
}

func (f *fakeIAMUserRepo) CreateUser(ctx context.Context, req *userpb.CreateUserRequest) (*userpb.User, error) {
	if f.createFn != nil {
		return f.createFn(ctx, req)
	}
	return &userpb.User{Name: "users/u-1", UserId: "u-1"}, nil
}

func (f *fakeIAMUserRepo) SendUserVerificationEmail(ctx context.Context, req *userpb.SendUserVerificationEmailRequest) error {
	if f.emailFn != nil {
		return f.emailFn(ctx, req)
	}
	return nil
}

func (f *fakeIAMUserRepo) UpdateUser(ctx context.Context, req *userpb.UpdateUserRequest) (*userpb.User, error) {
	if f.updateFn != nil {
		return f.updateFn(ctx, req)
	}
	return req.GetUser(), nil
}

func (f *fakeIAMUserRepo) SetUserPassword(context.Context, *userpb.SetUserPasswordRequest) error {
	return nil
}

func (f *fakeIAMUserRepo) RevokeUserSessions(context.Context, *userpb.RevokeUserSessionsRequest) error {
	return nil
}

func (f *fakeIAMUserRepo) DisableUser(context.Context, *userpb.DisableUserRequest) (*userpb.User, error) {
	return &userpb.User{}, nil
}

func (f *fakeIAMUserRepo) EnableUser(context.Context, *userpb.EnableUserRequest) (*userpb.User, error) {
	return &userpb.User{}, nil
}

func (f *fakeIAMUserRepo) DeleteUser(context.Context, *userpb.DeleteUserRequest) (*userpb.User, error) {
	return &userpb.User{}, nil
}

func (f *fakeIAMUserRepo) UndeleteUser(context.Context, *userpb.UndeleteUserRequest) (*userpb.User, error) {
	return &userpb.User{}, nil
}

func (f *fakeIAMUserRepo) ExpungeUser(context.Context, *userpb.ExpungeUserRequest) error {
	return nil
}

type fakeSeedRepo struct {
	seededIDs []string
}

func (f *fakeSeedRepo) EnsureAdminTuple(_ context.Context, userID string) error {
	f.seededIDs = append(f.seededIDs, userID)
	return nil
}

func newTestAuthorizer(t *testing.T, handler http.HandlerFunc) *openfga.Authorizer {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	sdk, err := fgaclient.NewSdkClient(&fgaclient.ClientConfiguration{
		ApiUrl:               srv.URL,
		StoreId:              "01H00000000000000000000000",
		AuthorizationModelId: "01H00000000000000000000001",
	})
	if err != nil {
		t.Fatal(err)
	}
	authz, err := openfga.NewDefault(sdk)
	if err != nil {
		t.Fatal(err)
	}
	return authz
}

func TestUserUsecaseProtectionCreateAndUpdateRules(t *testing.T) {
	adminUsers := map[string]bool{"user:fellow-admin": true}
	var failFGA bool
	authz := newTestAuthorizer(t, func(w http.ResponseWriter, r *http.Request) {
		if failFGA {
			http.Error(w, "openfga down", http.StatusServiceUnavailable)
			return
		}
		var payload struct {
			TupleKey struct {
				User string `json:"user"`
			} `json:"tuple_key"`
		}
		_ = json.NewDecoder(r.Body).Decode(&payload)
		w.Header().Set("Content-Type", "application/json")
		if adminUsers[payload.TupleKey.User] {
			_, _ = w.Write([]byte(`{"allowed":true}`))
			return
		}
		_, _ = w.Write([]byte(`{"allowed":false}`))
	})

	var lastUpdate *userpb.UpdateUserRequest
	var lastCreate *userpb.CreateUserRequest
	var emailSent bool
	var emailErr error
	repo := &fakeIAMUserRepo{
		createFn: func(_ context.Context, req *userpb.CreateUserRequest) (*userpb.User, error) {
			lastCreate = req
			return &userpb.User{Name: "users/new-1", UserId: "new-1"}, nil
		},
		emailFn: func(context.Context, *userpb.SendUserVerificationEmailRequest) error {
			emailSent = true
			return emailErr
		},
		updateFn: func(_ context.Context, req *userpb.UpdateUserRequest) (*userpb.User, error) {
			lastUpdate = req
			return req.GetUser(), nil
		},
	}

	uc, err := NewUserUsecase(repo, authz, nil)
	if err != nil {
		t.Fatal(err)
	}

	// Missing actor is rejected.
	if _, err := uc.DisableUser(t.Context(), userpb.NewUserName("target-1"), "", ""); !errors.Is(err, ErrUnauthenticated) {
		t.Fatalf("missing actor err = %v, want ErrUnauthenticated", err)
	}

	ctx := security.WithActor(t.Context(), security.Actor{Type: security.ActorTypeHuman, ID: "operator-1"})

	// Self-protection across DisableUser, DeleteUser, ExpungeUser.
	if _, err := uc.DisableUser(ctx, userpb.NewUserName("operator-1"), "", ""); !errors.Is(err, ErrSelfActionForbidden) {
		t.Fatalf("self disable err = %v, want ErrSelfActionForbidden", err)
	}
	if _, err := uc.DeleteUser(ctx, userpb.NewUserName("operator-1")); !errors.Is(err, ErrSelfActionForbidden) {
		t.Fatalf("self delete err = %v, want ErrSelfActionForbidden", err)
	}
	if err := uc.ExpungeUser(ctx, userpb.NewUserName("operator-1")); !errors.Is(err, ErrSelfActionForbidden) {
		t.Fatalf("self expunge err = %v, want ErrSelfActionForbidden", err)
	}

	// Fellow-admin protection across DisableUser, DeleteUser, ExpungeUser.
	if _, err := uc.DisableUser(ctx, userpb.NewUserName("fellow-admin"), "", ""); !errors.Is(err, ErrTargetIsAdmin) {
		t.Fatalf("fellow admin disable err = %v, want ErrTargetIsAdmin", err)
	}
	if _, err := uc.DeleteUser(ctx, userpb.NewUserName("fellow-admin")); !errors.Is(err, ErrTargetIsAdmin) {
		t.Fatalf("fellow admin delete err = %v, want ErrTargetIsAdmin", err)
	}
	if err := uc.ExpungeUser(ctx, userpb.NewUserName("fellow-admin")); !errors.Is(err, ErrTargetIsAdmin) {
		t.Fatalf("fellow admin expunge err = %v, want ErrTargetIsAdmin", err)
	}

	// OpenFGA outage during protection check fails closed by propagating the error.
	failFGA = true
	if _, err := uc.DeleteUser(ctx, userpb.NewUserName("normal-user")); err == nil {
		t.Fatal("expected OpenFGA failure to fail closed")
	}
	failFGA = false

	// CreateUser preserves tri-state require_password_change and degrades gracefully on email failure.
	emailErr = userpb.ErrorUserErrorReasonVerificationEmailFailed("smtp down")
	created, sent, err := uc.CreateUser(ctx, &adminuserpb.CreateUserRequest{
		User:                  &userpb.User{Email: proto.String("u@example.com")},
		Password:              "StrongPass!123",
		RequirePasswordChange: proto.Bool(false),
	})
	if err != nil || sent || created.GetUserId() != "new-1" || lastCreate.RequirePasswordChange == nil || lastCreate.GetRequirePasswordChange() {
		t.Fatalf("CreateUser(email fail) created=%+v sent=%v err=%v", created, sent, err)
	}
	if !emailSent {
		t.Fatal("expected SendUserVerificationEmail to be attempted")
	}

	// UpdateUser enforces the 7-field profile whitelist and strips email from payload.
	for _, badMask := range [][]string{
		nil,
		{"email"},
		{"profile.name", "status"},
		{"profile"},
	} {
		if _, err := uc.UpdateUser(ctx, userpb.NewUserName("u-1"), &userpb.User{}, &fieldmaskpb.FieldMask{Paths: badMask}); !errors.Is(err, ErrInvalidUpdateMask) {
			t.Fatalf("UpdateUser(badMask=%v) err = %v, want ErrInvalidUpdateMask", badMask, err)
		}
	}

	allSeven := []string{
		"profile.name",
		"profile.given_name",
		"profile.family_name",
		"profile.nickname",
		"profile.preferred_username",
		"profile.picture",
		"profile.locale",
		"profile.name", // duplicate should be deduplicated
	}
	_, err = uc.UpdateUser(ctx, userpb.NewUserName("u-1"), &userpb.User{
		Email:   proto.String("tamper@example.com"),
		Profile: &userpb.UserProfile{Name: proto.String("Alice")},
	}, &fieldmaskpb.FieldMask{Paths: allSeven})
	if err != nil {
		t.Fatalf("UpdateUser(all 7 fields) error = %v", err)
	}
	if lastUpdate.GetUser().Email != nil {
		t.Fatal("UpdateUser did not strip Email field from payload")
	}
	if len(lastUpdate.GetUpdateMask().GetPaths()) != 7 {
		t.Fatalf("UpdateMask paths = %v, want 7 unique paths", lastUpdate.GetUpdateMask().GetPaths())
	}
}

func TestSeedInitializerEnsureModes(t *testing.T) {
	var users []*userpb.User
	repo := &fakeIAMUserRepo{
		listFn: func(context.Context, *userpb.ListUsersRequest) (*userpb.ListUsersResponse, error) {
			return &userpb.ListUsersResponse{Users: users}, nil
		},
	}
	seedRepo := &fakeSeedRepo{}
	seed, err := NewSeedInitializer(&adminconfpb.Admin{
		BootstrapAdminEmail: proto.String("Admin@Plateau.Local"),
	}, repo, seedRepo, nil)
	if err != nil {
		t.Fatal(err)
	}

	// 1. User not created in IAM yet -> returns nil, Seeded() stays false.
	if err := seed.Ensure(t.Context()); err != nil || seed.Seeded() {
		t.Fatalf("Ensure(missing user) err=%v seeded=%v", err, seed.Seeded())
	}

	// 2. Soft-deleted user in IAM -> skipped, Seeded() stays false.
	users = []*userpb.User{{
		UserId:     "deleted-admin",
		Email:      proto.String("admin@plateau.local"),
		DeleteTime: timestamppb.Now(),
	}}
	if err := seed.Ensure(t.Context()); err != nil || seed.Seeded() {
		t.Fatalf("Ensure(soft-deleted user) err=%v seeded=%v", err, seed.Seeded())
	}

	// 3. Active user -> writes tuple once and marks Seeded() == true.
	users = []*userpb.User{{
		UserId: "active-admin",
		Email:  proto.String("admin@plateau.local"),
	}}
	if err := seed.Ensure(t.Context()); err != nil || !seed.Seeded() {
		t.Fatalf("Ensure(active user) err=%v seeded=%v", err, seed.Seeded())
	}
	if err := seed.Ensure(t.Context()); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(seedRepo.seededIDs, []string{"active-admin"}) {
		t.Fatalf("seededIDs = %v, want [active-admin]", seedRepo.seededIDs)
	}
}
