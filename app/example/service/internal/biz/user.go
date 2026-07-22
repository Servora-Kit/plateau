package biz

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	examplev1 "github.com/Servora-Kit/servora-platform/api/gen/go/example/service/v1"
	corecrud "github.com/Servora-Kit/servora/core/crud"
	kerrors "github.com/go-kratos/kratos/v3/errors"
	"golang.org/x/crypto/bcrypt"
	fieldmaskpb "google.golang.org/protobuf/types/known/fieldmaskpb"
)

var (
	// ErrNotFound indicates that a repository record does not exist.
	ErrNotFound = errors.New("record not found")
	// ErrAlreadyExists indicates that a repository uniqueness constraint was violated.
	ErrAlreadyExists = errors.New("record already exists")
	// ErrMutationMiss indicates that an atomic conditional mutation matched no row.
	ErrMutationMiss = errors.New("conditional mutation matched no rows")
)

const maxUndeleteAttempts = 3

// UserScope is intentionally a minimal example-only resource scope.
// It is not Servora's production tenant/authentication context: it only extracts
// the tenant path variable so this reference service can exercise scoped CRUD,
// Ent predicates, and page-token fingerprints without an IAM dependency.
// Production services MUST obtain tenant identity from authenticated operator
// context and authorization policy instead of copying this type.
type UserScope struct {
	tenantID string
}

// NewUserScope constructs one validated business scope.
func NewUserScope(tenantID string) (UserScope, error) {
	if tenantID == "" || tenantID == "-" || strings.Contains(tenantID, "/") {
		return UserScope{}, fmt.Errorf("tenant id must be non-empty, non-wildcard, and contain no slash")
	}
	return UserScope{tenantID: tenantID}, nil
}

// TenantID returns the storage tenant identity.
func (scope UserScope) TenantID() string { return scope.tenantID }

// Fingerprint binds tenant and tombstone visibility to every page token.
func (scope UserScope) Fingerprint(includeDeleted bool) []byte {
	visibility := "active"
	if includeDeleted {
		visibility = "all"
	}
	return []byte("tenant:" + scope.TenantID() + "\x00deleted:" + visibility)
}

// UserRepo is the persistence port implemented by data.NewUserRepo.
type UserRepo interface {
	CreateUser(context.Context, examplev1.UserName, *examplev1.User, *string) (*examplev1.User, error)
	GetUser(context.Context, examplev1.UserName, bool) (*examplev1.User, error)
	ListUsers(context.Context, UserScope, bool, corecrud.ListQuery) (corecrud.ListResult[*examplev1.User], error)
	UpdateUser(context.Context, examplev1.UserName, *examplev1.User, *fieldmaskpb.FieldMask, *string, string) (*examplev1.User, error)
	DeleteUser(context.Context, examplev1.UserName, string) (*examplev1.User, error)
	UndeleteUser(context.Context, examplev1.UserName, string) (*examplev1.User, error)
}

// UserUsecase owns User business semantics over the repository port.
type UserUsecase struct {
	repo UserRepo
	log  *slog.Logger
}

// NewUserUsecase creates the User business service.
func NewUserUsecase(repo UserRepo, logger *slog.Logger) *UserUsecase {
	return &UserUsecase{repo: repo, log: logger.With("scope", "example/biz/user")}
}

func (usecase *UserUsecase) CreateUser(
	ctx context.Context,
	name examplev1.UserName,
	prepared corecrud.PreparedCreate[*examplev1.User],
) (*examplev1.User, error) {
	resource := prepared.Resource()
	passwordHash, err := hashTemporaryPassword(resource.TemporaryPassword)
	if err != nil {
		return nil, usecase.internalUserError(ctx, "create", err)
	}
	etag, err := newEtag()
	if err != nil {
		return nil, usecase.internalUserError(ctx, "create", err)
	}
	resource.TemporaryPassword = nil
	resource.Etag = etag
	created, err := usecase.repo.CreateUser(ctx, name, resource, passwordHash)
	return created, usecase.translateRepoError(err)
}

func (usecase *UserUsecase) GetUser(ctx context.Context, name examplev1.UserName) (*examplev1.User, error) {
	resource, err := usecase.repo.GetUser(ctx, name, true)
	return resource, usecase.translateRepoError(err)
}

// ListUsers applies the biz-owned tombstone visibility choice to a prepared query.
func (usecase *UserUsecase) ListUsers(
	ctx context.Context,
	scope UserScope,
	query corecrud.ListQuery,
	options corecrud.ListOptions,
) (corecrud.ListResult[*examplev1.User], error) {
	result, err := usecase.repo.ListUsers(ctx, scope, options.ShowDeleted, query)
	if err != nil {
		return corecrud.ListResult[*examplev1.User]{}, usecase.translateRepoError(err)
	}
	return result, nil
}

func (usecase *UserUsecase) UpdateUser(
	ctx context.Context,
	name examplev1.UserName,
	prepared corecrud.PreparedUpdate[*examplev1.User],
	missingCreate *corecrud.PreparedCreate[*examplev1.User],
) (*examplev1.User, error) {
	current, err := usecase.repo.GetUser(ctx, name, false)
	if err != nil {
		if errors.Is(err, ErrNotFound) && prepared.Options().AllowMissing && missingCreate != nil {
			return usecase.CreateUser(ctx, name, *missingCreate)
		}
		return nil, usecase.translateRepoError(err)
	}
	if err := prepared.ValidateImmutable(current); err != nil {
		return nil, err
	}
	if expected := prepared.Options().Etag; expected != "" && expected != current.GetEtag() {
		return nil, examplev1.ErrorUserErrorReasonEtagMismatch("user etag does not match")
	}
	resource := prepared.Resource()
	passwordHash, err := hashTemporaryPassword(resource.TemporaryPassword)
	if err != nil {
		return nil, usecase.internalUserError(ctx, "update", err)
	}
	etag, err := newEtag()
	if err != nil {
		return nil, usecase.internalUserError(ctx, "update", err)
	}
	resource.TemporaryPassword = nil
	resource.Etag = etag
	updated, err := usecase.repo.UpdateUser(ctx, name, resource, prepared.WriteMask(), passwordHash, current.GetEtag())
	if errors.Is(err, ErrMutationMiss) {
		latest, probeErr := usecase.repo.GetUser(ctx, name, true)
		if probeErr != nil {
			return nil, usecase.translateRepoError(probeErr)
		}
		if latest.GetDeleteTime() != nil {
			return nil, examplev1.ErrorUserErrorReasonNotFound("user not found").WithCause(err)
		}
		return nil, examplev1.ErrorUserErrorReasonEtagMismatch("user etag does not match").WithCause(err)
	}
	return updated, usecase.translateRepoError(err)
}

// DeleteUser soft-deletes an active row and returns its persisted tombstone.
func (usecase *UserUsecase) DeleteUser(
	ctx context.Context,
	name examplev1.UserName,
	options corecrud.DeleteOptions,
) (*examplev1.User, error) {
	current, err := usecase.repo.GetUser(ctx, name, true)
	if err != nil {
		return nil, usecase.translateRepoError(err)
	}
	if options.Etag != "" && options.Etag != current.GetEtag() {
		return nil, examplev1.ErrorUserErrorReasonEtagMismatch("user etag does not match")
	}
	if current.GetDeleteTime() != nil {
		if options.AllowMissing {
			return current, nil
		}
		return nil, examplev1.ErrorUserErrorReasonNotFound("user not found")
	}
	deleted, err := usecase.repo.DeleteUser(ctx, name, current.GetEtag())
	if errors.Is(err, ErrMutationMiss) {
		latest, probeErr := usecase.repo.GetUser(ctx, name, true)
		if probeErr != nil {
			return nil, usecase.translateRepoError(probeErr)
		}
		if latest.GetDeleteTime() != nil {
			if options.AllowMissing {
				return latest, nil
			}
			return nil, examplev1.ErrorUserErrorReasonNotFound("user not found").WithCause(err)
		}
		return nil, examplev1.ErrorUserErrorReasonEtagMismatch("user etag does not match").WithCause(err)
	}
	return deleted, usecase.translateRepoError(err)
}

// UndeleteUser restores a tombstone and is idempotent for an already-active row.
func (usecase *UserUsecase) UndeleteUser(ctx context.Context, name examplev1.UserName) (*examplev1.User, error) {
	var lastMiss error
	for range maxUndeleteAttempts {
		current, err := usecase.repo.GetUser(ctx, name, true)
		if err != nil {
			return nil, usecase.translateRepoError(err)
		}
		if current == nil {
			return nil, usecase.internalUserError(ctx, "undelete", errors.New("repository returned nil User without error"))
		}
		if current.GetDeleteTime() == nil {
			return current, nil
		}

		etag, err := newEtag()
		if err != nil {
			return nil, usecase.internalUserError(ctx, "undelete", err)
		}
		restored, err := usecase.repo.UndeleteUser(ctx, name, etag)
		if err == nil {
			if restored == nil {
				return nil, usecase.internalUserError(ctx, "undelete", errors.New("repository returned nil User without error"))
			}
			if restored.GetDeleteTime() == nil {
				return restored, nil
			}
			lastMiss = ErrMutationMiss
			continue
		}
		if !errors.Is(err, ErrMutationMiss) {
			return nil, usecase.translateRepoError(err)
		}
		lastMiss = err
	}

	latest, err := usecase.repo.GetUser(ctx, name, true)
	if err != nil {
		return nil, usecase.translateRepoError(err)
	}
	if latest == nil {
		return nil, usecase.internalUserError(ctx, "undelete", errors.New("repository returned nil User without error"))
	}
	if latest.GetDeleteTime() == nil {
		return latest, nil
	}
	return nil, examplev1.ErrorUserErrorReasonEtagMismatch("user lifecycle changed concurrently").WithCause(lastMiss)
}

func hashTemporaryPassword(plaintext *string) (*string, error) {
	if plaintext == nil {
		return nil, nil
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(*plaintext), bcrypt.DefaultCost)
	if err != nil {
		return nil, fmt.Errorf("hash temporary password: %w", err)
	}
	value := string(hash)
	return &value, nil
}

func newEtag() (string, error) {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", fmt.Errorf("generate user etag: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(value[:]), nil
}

func (*UserUsecase) translateRepoError(err error) error {
	if err == nil {
		return nil
	}
	switch {
	case errors.Is(err, ErrNotFound):
		return examplev1.ErrorUserErrorReasonNotFound("user not found").WithCause(err)
	case errors.Is(err, ErrAlreadyExists):
		return examplev1.ErrorUserErrorReasonAlreadyExists("user already exists").WithCause(err)
	}
	var apiError *kerrors.Error
	if errors.As(err, &apiError) {
		return err
	}
	return kerrors.InternalServer("USER_INTERNAL", "user operation failed").WithCause(err)
}

func (usecase *UserUsecase) internalUserError(ctx context.Context, operation string, cause error) error {
	usecase.log.ErrorContext(ctx, "User operation failed", "operation", operation, "err", cause)
	return kerrors.InternalServer("USER_INTERNAL", "user operation failed").WithCause(cause)
}
