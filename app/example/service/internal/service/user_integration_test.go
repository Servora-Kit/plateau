//go:build integration

package service_test

import (
	"context"
	"io"
	"log/slog"
	"os"
	"strings"
	"testing"
	"time"

	examplev1 "github.com/Servora-Kit/plateau/api/gen/go/example/service/v1"
	"github.com/Servora-Kit/plateau/app/example/service/internal/biz"
	"github.com/Servora-Kit/plateau/app/example/service/internal/data"
	userservice "github.com/Servora-Kit/plateau/app/example/service/internal/service"
	corev1 "github.com/Servora-Kit/servora/api/gen/go/servora/core/v1"
	crudpb "github.com/Servora-Kit/servora/api/gen/go/servora/crud/v1"
	"google.golang.org/protobuf/types/known/fieldmaskpb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

const exampleSQLiteDSNEnv = "SERVORA_EXAMPLE_SQLITE_DSN"

func TestUserReferenceIntegration(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv(exampleSQLiteDSNEnv))
	if dsn == "" {
		t.Fatalf("%s is required for the explicit integration test", exampleSQLiteDSNEnv)
	}

	service := newLiveUserService(t, dsn)
	ctx := context.Background()
	injectedCreateTime := timestamppb.New(time.Unix(1, 0))
	temporaryPassword := "temporary-secret"

	alpha := createUser(t, ctx, service, "alpha", "Alpha", "alpha@example.com", "developer", &temporaryPassword, injectedCreateTime)
	beta := createUser(t, ctx, service, "beta", "Beta", "beta@example.com", "developer", nil, nil)
	gamma := createUser(t, ctx, service, "gamma", "Gamma*", "gamma@example.com", "enterprise", nil, nil)

	if alpha.GetName() != "tenants/acme/users/alpha" {
		t.Fatalf("created name = %q", alpha.GetName())
	}
	if alpha.TemporaryPassword != nil {
		t.Fatal("CreateUser response contains temporary_password")
	}
	if alpha.GetCreateTime() == nil || alpha.GetCreateTime().AsTime().Equal(injectedCreateTime.AsTime()) {
		t.Fatalf("CreateUser did not replace OUTPUT_ONLY create_time: %v", alpha.GetCreateTime())
	}

	got, err := service.GetUser(ctx, &examplev1.GetUserRequest{Name: alpha.GetName()})
	if err != nil {
		t.Fatalf("GetUser: %v", err)
	}
	if got.GetEmail() != alpha.GetEmail() {
		t.Fatalf("GetUser email = %q, want %q", got.GetEmail(), alpha.GetEmail())
	}

	firstPage, err := service.ListUsers(ctx, &examplev1.ListUsersRequest{
		Parent:       "tenants/acme",
		PageSize:     1,
		Filter:       `email >= "alpha@example.com"`,
		OrderBy:      "display_name",
		IncludeTotal: true,
	})
	if err != nil {
		t.Fatalf("ListUsers first page: %v", err)
	}
	if len(firstPage.GetUsers()) != 1 || firstPage.GetUsers()[0].GetName() != alpha.GetName() {
		t.Fatalf("first page users = %v", userNames(firstPage.GetUsers()))
	}
	if firstPage.GetNextPageToken() == "" {
		t.Fatal("first page next_page_token is empty")
	}
	if firstPage.TotalSize == nil || firstPage.GetTotalSize() != 3 {
		t.Fatalf("first page total_size = %v, want 3", firstPage.TotalSize)
	}

	secondPage, err := service.ListUsers(ctx, &examplev1.ListUsersRequest{
		Parent:    "tenants/acme",
		PageSize:  1,
		PageToken: firstPage.GetNextPageToken(),
		Filter:    `email >= "alpha@example.com"`,
		OrderBy:   "display_name",
	})
	if err != nil {
		t.Fatalf("ListUsers second page: %v", err)
	}
	if len(secondPage.GetUsers()) != 1 || secondPage.GetUsers()[0].GetName() != beta.GetName() {
		t.Fatalf("second page users = %v", userNames(secondPage.GetUsers()))
	}

	_, err = service.ListUsers(ctx, &examplev1.ListUsersRequest{
		Parent:    "tenants/acme",
		PageSize:  1,
		PageToken: firstPage.GetNextPageToken(),
		Filter:    `email = "alpha@example.com"`,
		OrderBy:   "display_name",
	})
	if !crudpb.IsCrudErrorReasonInvalidPageToken(err) {
		t.Fatalf("changed-filter token error = %v, want INVALID_PAGE_TOKEN", err)
	}

	enterprise, err := service.ListUsers(ctx, &examplev1.ListUsersRequest{
		Parent:   "tenants/acme",
		PageSize: 10,
		Filter:   `tenant_plan = "enterprise"`,
	})
	if err != nil {
		t.Fatalf("ListUsers tenant_plan filter: %v", err)
	}
	if len(enterprise.GetUsers()) != 1 || enterprise.GetUsers()[0].GetName() != gamma.GetName() {
		t.Fatalf("enterprise users = %v", userNames(enterprise.GetUsers()))
	}

	prefix, err := service.ListUsers(ctx, &examplev1.ListUsersRequest{
		Parent: "tenants/acme", Filter: `display_name = "Al*"`,
	})
	if err != nil {
		t.Fatalf("ListUsers prefix filter: %v", err)
	}
	if len(prefix.GetUsers()) != 1 || prefix.GetUsers()[0].GetName() != alpha.GetName() {
		t.Fatalf("prefix users = %v, want [%s]", userNames(prefix.GetUsers()), alpha.GetName())
	}

	suffix, err := service.ListUsers(ctx, &examplev1.ListUsersRequest{
		Parent: "tenants/acme", Filter: `email = "*ha@example.com"`, IncludeTotal: true,
	})
	if err != nil {
		t.Fatalf("ListUsers suffix filter: %v", err)
	}
	if suffix.GetTotalSize() != 1 || len(suffix.GetUsers()) != 1 || suffix.GetUsers()[0].GetName() != alpha.GetName() {
		t.Fatalf("suffix result = users %v total %d, want [%s] total 1", userNames(suffix.GetUsers()), suffix.GetTotalSize(), alpha.GetName())
	}

	contains, err := service.ListUsers(ctx, &examplev1.ListUsersRequest{
		Parent: "tenants/acme", Filter: `display_name = "*amm*"`,
	})
	if err != nil {
		t.Fatalf("ListUsers contains filter: %v", err)
	}
	if len(contains.GetUsers()) != 1 || contains.GetUsers()[0].GetName() != gamma.GetName() {
		t.Fatalf("contains users = %v, want [%s]", userNames(contains.GetUsers()), gamma.GetName())
	}

	literalStar, err := service.ListUsers(ctx, &examplev1.ListUsersRequest{
		Parent: "tenants/acme", Filter: `display_name = "Gamma\\*"`,
	})
	if err != nil {
		t.Fatalf("ListUsers literal star filter: %v", err)
	}
	if len(literalStar.GetUsers()) != 1 || literalStar.GetUsers()[0].GetName() != gamma.GetName() {
		t.Fatalf("literal-star users = %v, want [%s]", userNames(literalStar.GetUsers()), gamma.GetName())
	}

	_, err = service.ListUsers(ctx, &examplev1.ListUsersRequest{
		Parent: "tenants/acme", Filter: `display_name < "Al*"`,
	})
	if !crudpb.IsCrudErrorReasonInvalidFilter(err) {
		t.Fatalf("range wildcard error = %v, want INVALID_FILTER", err)
	}

	wildcardFirst, err := service.ListUsers(ctx, &examplev1.ListUsersRequest{
		Parent: "tenants/acme", PageSize: 1, Filter: `email = "**@example.com"`, OrderBy: "display_name",
	})
	if err != nil {
		t.Fatalf("ListUsers wildcard first page: %v", err)
	}
	if len(wildcardFirst.GetUsers()) != 1 || wildcardFirst.GetUsers()[0].GetName() != alpha.GetName() || wildcardFirst.GetNextPageToken() == "" {
		t.Fatalf("wildcard first page = users %v token %q", userNames(wildcardFirst.GetUsers()), wildcardFirst.GetNextPageToken())
	}
	wildcardSecond, err := service.ListUsers(ctx, &examplev1.ListUsersRequest{
		Parent:    "tenants/acme",
		PageSize:  1,
		PageToken: wildcardFirst.GetNextPageToken(),
		Filter:    `email = "*@example.com"`,
		OrderBy:   "display_name",
	})
	if err != nil {
		t.Fatalf("ListUsers canonical wildcard second page: %v", err)
	}
	if len(wildcardSecond.GetUsers()) != 1 || wildcardSecond.GetUsers()[0].GetName() != beta.GetName() {
		t.Fatalf("wildcard second page users = %v, want [%s]", userNames(wildcardSecond.GetUsers()), beta.GetName())
	}
	_, err = service.ListUsers(ctx, &examplev1.ListUsersRequest{
		Parent:    "tenants/acme",
		PageSize:  1,
		PageToken: wildcardFirst.GetNextPageToken(),
		Filter:    `email = "*@example.org"`,
		OrderBy:   "display_name",
	})
	if !crudpb.IsCrudErrorReasonInvalidPageToken(err) {
		t.Fatalf("changed wildcard token error = %v, want INVALID_PAGE_TOKEN", err)
	}

	defaultFirst, err := service.ListUsers(ctx, &examplev1.ListUsersRequest{
		Parent:   "tenants/acme",
		PageSize: 2,
	})
	if err != nil {
		t.Fatalf("ListUsers default first page: %v", err)
	}
	if len(defaultFirst.GetUsers()) != 2 || defaultFirst.GetNextPageToken() == "" {
		t.Fatalf("default first page = users %v token %q", userNames(defaultFirst.GetUsers()), defaultFirst.GetNextPageToken())
	}
	defaultSecond, err := service.ListUsers(ctx, &examplev1.ListUsersRequest{
		Parent:    "tenants/acme",
		PageSize:  2,
		PageToken: defaultFirst.GetNextPageToken(),
	})
	if err != nil {
		t.Fatalf("ListUsers default second page: %v", err)
	}
	if len(defaultSecond.GetUsers()) != 1 {
		t.Fatalf("default second page users = %v, want one remaining User", userNames(defaultSecond.GetUsers()))
	}
	seen := map[string]struct{}{}
	for _, user := range append(defaultFirst.GetUsers(), defaultSecond.GetUsers()...) {
		seen[user.GetName()] = struct{}{}
	}
	if len(seen) != 3 {
		t.Fatalf("default pagination returned duplicate or missing Users: %v", seen)
	}

	displayName := "Alpha Updated"
	tenantPlan := "developer"
	updated, err := service.UpdateUser(ctx, &examplev1.UpdateUserRequest{
		User: &examplev1.User{
			Name:              alpha.GetName(),
			DisplayName:       &displayName,
			TenantPlan:        &tenantPlan,
			TemporaryPassword: stringPointer("replacement-secret"),
			Etag:              alpha.GetEtag(),
			CreateTime:        timestamppb.New(time.Unix(2, 0)),
		},
		UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{
			examplev1.UserFields.DisplayName.String(),
			examplev1.UserFields.Nickname.String(),
			examplev1.UserFields.TenantPlan.String(),
			examplev1.UserFields.TemporaryPassword.String(),
			examplev1.UserFields.CreateTime.String(),
		}},
	})
	if err != nil {
		t.Fatalf("UpdateUser: %v", err)
	}
	if updated.GetDisplayName() != displayName || updated.Nickname != nil {
		t.Fatalf("UpdateUser mutable fields = display_name %q nickname %v", updated.GetDisplayName(), updated.Nickname)
	}
	if updated.TemporaryPassword != nil {
		t.Fatal("UpdateUser response contains temporary_password")
	}
	if updated.GetEtag() == alpha.GetEtag() {
		t.Fatal("UpdateUser did not rotate etag")
	}
	if updated.GetCreateTime().AsTime().Equal(time.Unix(2, 0)) {
		t.Fatal("UpdateUser accepted OUTPUT_ONLY create_time")
	}

	changedPlan := "forbidden-change"
	_, err = service.UpdateUser(ctx, &examplev1.UpdateUserRequest{
		User: &examplev1.User{Name: alpha.GetName(), TenantPlan: &changedPlan, Etag: updated.GetEtag()},
		UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{
			examplev1.UserFields.TenantPlan.String(),
		}},
	})
	if !crudpb.IsCrudErrorReasonInvalidFieldValue(err) {
		t.Fatalf("immutable update error = %v, want INVALID_FIELD_VALUE", err)
	}

	_, err = service.DeleteUser(ctx, &examplev1.DeleteUserRequest{Name: alpha.GetName(), Etag: alpha.GetEtag()})
	if !examplev1.IsUserErrorReasonEtagMismatch(err) {
		t.Fatalf("stale delete error = %v, want ETAG_MISMATCH", err)
	}

	deleted, err := service.DeleteUser(ctx, &examplev1.DeleteUserRequest{Name: alpha.GetName(), Etag: updated.GetEtag()})
	if err != nil {
		t.Fatalf("DeleteUser: %v", err)
	}
	if deleted.GetDeleteTime() == nil {
		t.Fatal("DeleteUser response has no delete_time")
	}

	tombstone, err := service.GetUser(ctx, &examplev1.GetUserRequest{Name: alpha.GetName()})
	if err != nil {
		t.Fatalf("GetUser tombstone: %v", err)
	}
	if tombstone.GetDeleteTime() == nil {
		t.Fatal("GetUser did not return tombstone")
	}

	activeOnly := listAllUsers(t, ctx, service, false)
	if activeOnly.GetTotalSize() != 2 {
		t.Fatalf("active-only total_size = %d, want 2", activeOnly.GetTotalSize())
	}
	withDeleted := listAllUsers(t, ctx, service, true)
	if withDeleted.GetTotalSize() != 3 {
		t.Fatalf("show-deleted total_size = %d, want 3", withDeleted.GetTotalSize())
	}

	restored, err := service.UndeleteUser(ctx, &examplev1.UndeleteUserRequest{Name: alpha.GetName()})
	if err != nil {
		t.Fatalf("UndeleteUser: %v", err)
	}
	if restored.GetDeleteTime() != nil || restored.GetPurgeTime() != nil {
		t.Fatalf("UndeleteUser tombstone fields = delete_time %v purge_time %v", restored.GetDeleteTime(), restored.GetPurgeTime())
	}
	if restored.GetEtag() == deleted.GetEtag() {
		t.Fatal("UndeleteUser did not rotate etag")
	}

}

func newLiveUserService(t *testing.T, dsn string) *userservice.UserService {
	t.Helper()

	driver, err := data.NewEntDriver(&corev1.Data{Database: &corev1.Data_Database{
		Driver: "sqlite",
		Source: dsn,
	}})
	if err != nil {
		t.Fatalf("NewEntDriver: %v", err)
	}
	client, err := data.NewDBClient(driver)
	if err != nil {
		_ = driver.Close()
		t.Fatalf("NewDBClient: %v", err)
	}
	transaction, err := client.Tx(context.Background())
	if err != nil {
		_ = client.Close()
		t.Fatalf("begin fixture transaction: %v", err)
	}
	t.Cleanup(func() {
		if err := transaction.Rollback(); err != nil {
			t.Errorf("rollback fixture transaction: %v", err)
		}
		if err := client.Close(); err != nil {
			t.Errorf("close fixture client: %v", err)
		}
	})
	store, _, err := data.NewData(transaction.Client())
	if err != nil {
		t.Fatalf("NewData: %v", err)
	}

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	repository, err := data.NewUserRepo(store, logger)
	if err != nil {
		t.Fatalf("NewUserRepo: %v", err)
	}
	service, err := userservice.NewUserService(biz.NewUserUsecase(repository, logger))
	if err != nil {
		t.Fatalf("NewUserService: %v", err)
	}
	return service
}

func createUser(
	t *testing.T,
	ctx context.Context,
	service *userservice.UserService,
	userID string,
	displayName string,
	email string,
	tenantPlan string,
	temporaryPassword *string,
	createTime *timestamppb.Timestamp,
) *examplev1.User {
	t.Helper()

	resource, err := service.CreateUser(ctx, &examplev1.CreateUserRequest{
		Parent: "tenants/acme",
		UserId: userID,
		User: &examplev1.User{
			DisplayName:       &displayName,
			Email:             &email,
			TenantPlan:        &tenantPlan,
			Nickname:          stringPointer(strings.ToLower(displayName)),
			TemporaryPassword: temporaryPassword,
			CreateTime:        createTime,
		},
	})
	if err != nil {
		t.Fatalf("CreateUser(%s): %v", userID, err)
	}
	return resource
}

func listAllUsers(
	t *testing.T,
	ctx context.Context,
	service *userservice.UserService,
	showDeleted bool,
) *examplev1.ListUsersResponse {
	t.Helper()

	response, err := service.ListUsers(ctx, &examplev1.ListUsersRequest{
		Parent:       "tenants/acme",
		PageSize:     10,
		IncludeTotal: true,
		ShowDeleted:  showDeleted,
	})
	if err != nil {
		t.Fatalf("ListUsers(show_deleted=%t): %v", showDeleted, err)
	}
	return response
}

func userNames(users []*examplev1.User) []string {
	names := make([]string, len(users))
	for index, user := range users {
		names[index] = user.GetName()
	}
	return names
}

func stringPointer(value string) *string { return &value }
