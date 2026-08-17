package biz

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"

	examplev1 "github.com/Servora-Kit/plateau/api/gen/go/example/service/v1"
	corecrud "github.com/Servora-Kit/servora/core/crud"
	"golang.org/x/crypto/bcrypt"
	fieldmaskpb "google.golang.org/protobuf/types/known/fieldmaskpb"
	timestamppb "google.golang.org/protobuf/types/known/timestamppb"
)

func TestNewUserScopeRejectsUnsupportedWildcard(t *testing.T) {
	t.Parallel()

	for _, tenantID := range []string{"", "-", "a/b"} {
		if _, err := NewUserScope(tenantID); err == nil {
			t.Fatalf("NewUserScope(%q) succeeded", tenantID)
		}
	}

	scope, err := NewUserScope("acme")
	if err != nil {
		t.Fatalf("NewUserScope(acme): %v", err)
	}
	if got := scope.TenantID(); got != "acme" {
		t.Fatalf("TenantID() = %q, want acme", got)
	}
}

func TestUserScopeFingerprintIncludesDeletedVisibility(t *testing.T) {
	t.Parallel()

	scope, err := NewUserScope("acme")
	if err != nil {
		t.Fatalf("NewUserScope(acme): %v", err)
	}
	active := string(scope.Fingerprint(false))
	all := string(scope.Fingerprint(true))
	if active == all {
		t.Fatalf("fingerprints are equal: %q", active)
	}
}

func TestTranslateRepoErrorPreservesStorageCause(t *testing.T) {
	t.Parallel()

	cause := errors.New("driver failure")
	usecase := NewUserUsecase(nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
	tests := []struct {
		name string
		fact error
		is   func(error) bool
	}{
		{name: "not found", fact: ErrNotFound, is: examplev1.IsUserErrorReasonNotFound},
		{name: "already exists", fact: ErrAlreadyExists, is: examplev1.IsUserErrorReasonAlreadyExists},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			translated := usecase.translateRepoError(errors.Join(test.fact, cause))
			if !test.is(translated) {
				t.Fatalf("translated error = %v", translated)
			}
			if !errors.Is(translated, cause) {
				t.Fatalf("translated error does not preserve cause: %v", translated)
			}
		})
	}
}

func TestUserUsecasePassesOnlyPasswordHashToRepository(t *testing.T) {
	t.Parallel()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	plan := corecrud.MustBuildResourcePlan[*examplev1.User](examplev1.UserCRUDDescriptor())
	name := examplev1.NewUserName("acme", "u1")
	canonicalName, err := name.Format()
	if err != nil {
		t.Fatalf("format User name: %v", err)
	}

	t.Run("create", func(t *testing.T) {
		email := "person@example.com"
		plaintext := "temporary-secret"
		prepared, prepareErr := plan.PrepareCreate(&examplev1.User{
			Email: &email, TemporaryPassword: &plaintext,
		})
		if prepareErr != nil {
			t.Fatalf("PrepareCreate: %v", prepareErr)
		}
		repo := new(secretBoundaryRepo)
		if _, createErr := NewUserUsecase(repo, logger).CreateUser(t.Context(), name, prepared); createErr != nil {
			t.Fatalf("CreateUser: %v", createErr)
		}
		assertHashedSecret(t, repo.resource, repo.passwordHash, plaintext)
	})

	t.Run("update", func(t *testing.T) {
		plaintext := "replacement-secret"
		prepared, prepareErr := plan.PrepareUpdate(
			&examplev1.User{Name: canonicalName, TemporaryPassword: &plaintext, Etag: "old-etag"},
			&fieldmaskpb.FieldMask{Paths: []string{examplev1.UserFields.TemporaryPassword.String()}},
			corecrud.UpdateOptions{Etag: "old-etag"},
		)
		if prepareErr != nil {
			t.Fatalf("PrepareUpdate: %v", prepareErr)
		}
		repo := &secretBoundaryRepo{current: &examplev1.User{Name: canonicalName, Etag: "old-etag"}}
		if _, updateErr := NewUserUsecase(repo, logger).UpdateUser(t.Context(), name, prepared, nil); updateErr != nil {
			t.Fatalf("UpdateUser: %v", updateErr)
		}
		assertHashedSecret(t, repo.resource, repo.passwordHash, plaintext)
	})
}

func assertHashedSecret(t *testing.T, resource *examplev1.User, passwordHash *string, plaintext string) {
	t.Helper()
	if resource.GetTemporaryPassword() != "" || resource.TemporaryPassword != nil {
		t.Fatalf("repository resource contains temporary_password")
	}
	if passwordHash == nil {
		t.Fatal("repository password hash is nil")
	}
	if err := bcrypt.CompareHashAndPassword([]byte(*passwordHash), []byte(plaintext)); err != nil {
		t.Fatalf("password hash does not match plaintext: %v", err)
	}
}

func TestDeleteUserAllowMissingReturnsConcurrentTombstone(t *testing.T) {
	t.Parallel()

	name := examplev1.NewUserName("acme", "u1")
	active := &examplev1.User{Etag: "old-etag"}
	tombstone := &examplev1.User{Etag: "old-etag", DeleteTime: timestamppb.Now()}
	repo := &secretBoundaryRepo{
		getResponses: []*examplev1.User{active, tombstone},
		deleteErr:    ErrMutationMiss,
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	got, err := NewUserUsecase(repo, logger).DeleteUser(t.Context(), name, corecrud.DeleteOptions{AllowMissing: true})
	if err != nil {
		t.Fatalf("DeleteUser: %v", err)
	}
	if got != tombstone {
		t.Fatalf("DeleteUser result = %p, want tombstone %p", got, tombstone)
	}
}

func TestDeleteUserAllowMissingValidatesTombstoneEtag(t *testing.T) {
	t.Parallel()

	repo := &secretBoundaryRepo{current: &examplev1.User{
		Etag:       "current-etag",
		DeleteTime: timestamppb.Now(),
	}}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	_, err := NewUserUsecase(repo, logger).DeleteUser(
		t.Context(),
		examplev1.NewUserName("acme", "u1"),
		corecrud.DeleteOptions{AllowMissing: true, Etag: "stale-etag"},
	)
	if !examplev1.IsUserErrorReasonEtagMismatch(err) {
		t.Fatalf("DeleteUser error = %v, want ETAG_MISMATCH", err)
	}
}

func TestDeleteUserPropagatesConcurrentProbeError(t *testing.T) {
	t.Parallel()

	probeErr := errors.New("probe database failure")
	repo := &secretBoundaryRepo{
		getResponses: []*examplev1.User{{Etag: "old-etag"}},
		getErrors:    []error{nil, probeErr},
		deleteErr:    ErrMutationMiss,
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	_, err := NewUserUsecase(repo, logger).DeleteUser(
		t.Context(), examplev1.NewUserName("acme", "u1"), corecrud.DeleteOptions{AllowMissing: true},
	)
	if !errors.Is(err, probeErr) {
		t.Fatalf("DeleteUser error = %v, want probe error", err)
	}
}

func TestDeleteUserMutationMissClassifiesActiveAsEtagMismatch(t *testing.T) {
	t.Parallel()

	before := &examplev1.User{Etag: "old-etag"}
	after := &examplev1.User{Etag: "new-etag"}
	repo := &secretBoundaryRepo{
		getResponses: []*examplev1.User{before, after},
		deleteErr:    ErrMutationMiss,
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	_, err := NewUserUsecase(repo, logger).DeleteUser(
		t.Context(), examplev1.NewUserName("acme", "u1"), corecrud.DeleteOptions{},
	)
	if !examplev1.IsUserErrorReasonEtagMismatch(err) {
		t.Fatalf("DeleteUser error = %v, want ETAG_MISMATCH", err)
	}
	if !errors.Is(err, ErrMutationMiss) {
		t.Fatalf("DeleteUser error does not preserve mutation miss: %v", err)
	}
}

func TestUndeleteUserMutationMissReturnsConcurrentActiveResource(t *testing.T) {
	t.Parallel()

	tombstone := &examplev1.User{Etag: "old-etag", DeleteTime: timestamppb.Now()}
	active := &examplev1.User{Etag: "new-etag"}
	repo := &secretBoundaryRepo{
		getResponses: []*examplev1.User{tombstone, active},
		undeleteErr:  ErrMutationMiss,
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	got, err := NewUserUsecase(repo, logger).UndeleteUser(
		t.Context(), examplev1.NewUserName("acme", "u1"),
	)
	if err != nil {
		t.Fatalf("UndeleteUser: %v", err)
	}
	if got != active {
		t.Fatalf("UndeleteUser result = %p, want active %p", got, active)
	}
}

func TestUndeleteUserRetriesConcurrentRedelete(t *testing.T) {
	t.Parallel()

	tombstone := &examplev1.User{Etag: "old-etag", DeleteTime: timestamppb.Now()}
	active := &examplev1.User{Etag: "restored-etag"}
	repo := &secretBoundaryRepo{
		getResponses:      []*examplev1.User{tombstone, tombstone},
		undeleteResponses: []*examplev1.User{nil, active},
		undeleteErrors:    []error{ErrMutationMiss, nil},
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	got, err := NewUserUsecase(repo, logger).UndeleteUser(
		t.Context(), examplev1.NewUserName("acme", "u1"),
	)
	if err != nil {
		t.Fatalf("UndeleteUser: %v", err)
	}
	if got != active {
		t.Fatalf("UndeleteUser result = %p, want active %p", got, active)
	}
	if repo.undeleteIndex != 2 {
		t.Fatalf("UndeleteUser calls = %d, want 2", repo.undeleteIndex)
	}
}

func TestUndeleteUserRetriesTombstoneReturnedAfterMutation(t *testing.T) {
	t.Parallel()

	tombstone := &examplev1.User{Etag: "old-etag", DeleteTime: timestamppb.Now()}
	active := &examplev1.User{Etag: "restored-etag"}
	repo := &secretBoundaryRepo{
		getResponses:      []*examplev1.User{tombstone, tombstone},
		undeleteResponses: []*examplev1.User{tombstone, active},
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	got, err := NewUserUsecase(repo, logger).UndeleteUser(
		t.Context(), examplev1.NewUserName("acme", "u1"),
	)
	if err != nil {
		t.Fatalf("UndeleteUser: %v", err)
	}
	if got != active {
		t.Fatalf("UndeleteUser result = %p, want active %p", got, active)
	}
	if repo.undeleteIndex != 2 {
		t.Fatalf("UndeleteUser calls = %d, want 2", repo.undeleteIndex)
	}
}

func TestUndeleteUserExhaustedLifecycleRaceReturnsEtagMismatch(t *testing.T) {
	t.Parallel()

	tombstone := &examplev1.User{Etag: "old-etag", DeleteTime: timestamppb.Now()}
	repo := &secretBoundaryRepo{
		getResponses:   []*examplev1.User{tombstone, tombstone, tombstone, tombstone},
		undeleteErrors: []error{ErrMutationMiss, ErrMutationMiss, ErrMutationMiss},
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	_, err := NewUserUsecase(repo, logger).UndeleteUser(
		t.Context(), examplev1.NewUserName("acme", "u1"),
	)
	if !examplev1.IsUserErrorReasonEtagMismatch(err) {
		t.Fatalf("UndeleteUser error = %v, want ETAG_MISMATCH", err)
	}
	if !errors.Is(err, ErrMutationMiss) {
		t.Fatalf("UndeleteUser error does not preserve mutation miss: %v", err)
	}
	if repo.undeleteIndex != maxUndeleteAttempts {
		t.Fatalf("UndeleteUser calls = %d, want %d", repo.undeleteIndex, maxUndeleteAttempts)
	}
	if repo.getIndex != maxUndeleteAttempts+1 {
		t.Fatalf("GetUser calls = %d, want %d", repo.getIndex, maxUndeleteAttempts+1)
	}
}

func TestUndeleteUserRejectsNilRepositoryResource(t *testing.T) {
	t.Parallel()

	tombstone := &examplev1.User{Etag: "old-etag", DeleteTime: timestamppb.Now()}
	tests := []struct {
		name string
		repo *secretBoundaryRepo
	}{
		{name: "initial read", repo: &secretBoundaryRepo{}},
		{
			name: "final probe",
			repo: &secretBoundaryRepo{
				getResponses:   []*examplev1.User{tombstone, tombstone, tombstone},
				undeleteErrors: []error{ErrMutationMiss, ErrMutationMiss, ErrMutationMiss},
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			logger := slog.New(slog.NewTextHandler(io.Discard, nil))
			resource, err := NewUserUsecase(test.repo, logger).UndeleteUser(
				t.Context(), examplev1.NewUserName("acme", "u1"),
			)
			if err == nil {
				t.Fatal("UndeleteUser succeeded with nil repository resource")
			}
			if resource != nil {
				t.Fatalf("UndeleteUser result = %v, want nil", resource)
			}
		})
	}
}

type secretBoundaryRepo struct {
	current           *examplev1.User
	resource          *examplev1.User
	passwordHash      *string
	getResponses      []*examplev1.User
	getErrors         []error
	getIndex          int
	deleteErr         error
	undeleteErr       error
	undeleteResponses []*examplev1.User
	undeleteErrors    []error
	undeleteIndex     int
}

func (repo *secretBoundaryRepo) CreateUser(_ context.Context, _ examplev1.UserName, resource *examplev1.User, passwordHash *string) (*examplev1.User, error) {
	repo.resource = resource
	repo.passwordHash = passwordHash
	return resource, nil
}

func (repo *secretBoundaryRepo) GetUser(context.Context, examplev1.UserName, bool) (*examplev1.User, error) {
	if repo.getIndex < len(repo.getErrors) {
		err := repo.getErrors[repo.getIndex]
		if err != nil {
			repo.getIndex++
			return nil, err
		}
	}
	if repo.getIndex < len(repo.getResponses) {
		resource := repo.getResponses[repo.getIndex]
		repo.getIndex++
		return resource, nil
	}
	return repo.current, nil
}

func (*secretBoundaryRepo) ListUsers(context.Context, UserScope, bool, corecrud.ListQuery) (corecrud.ListResult[*examplev1.User], error) {
	panic("unexpected ListUsers call")
}

func (repo *secretBoundaryRepo) UpdateUser(_ context.Context, _ examplev1.UserName, resource *examplev1.User, _ *fieldmaskpb.FieldMask, passwordHash *string, _ string) (*examplev1.User, error) {
	repo.resource = resource
	repo.passwordHash = passwordHash
	return resource, nil
}

func (repo *secretBoundaryRepo) DeleteUser(context.Context, examplev1.UserName, string) (*examplev1.User, error) {
	return nil, repo.deleteErr
}

func (repo *secretBoundaryRepo) UndeleteUser(context.Context, examplev1.UserName, string) (*examplev1.User, error) {
	index := repo.undeleteIndex
	repo.undeleteIndex++
	if index < len(repo.undeleteErrors) && repo.undeleteErrors[index] != nil {
		return nil, repo.undeleteErrors[index]
	}
	if index < len(repo.undeleteResponses) {
		return repo.undeleteResponses[index], nil
	}
	return nil, repo.undeleteErr
}
