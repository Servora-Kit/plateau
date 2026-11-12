package biz

import (
	"context"
	"fmt"

	"github.com/Servora-Kit/plateau/security"
	"github.com/Servora-Kit/plateau/security/authz/openfga"
)

const (
	AdminResourceType = "admin"
	AdminResourceID   = "global"

	RelationAdmin    = "admin"
	PermAccess       = "access"
	PermManageUsers  = "manage_users"
	PermExpungeUsers = "expunge_users"
)

var adminPermissionOrder = []string{
	PermAccess,
	PermManageUsers,
	PermExpungeUsers,
}

// AuthUsecase evaluates real-time OpenFGA permissions for Admin BFF sessions.
type AuthUsecase struct {
	authorizer *openfga.Authorizer
}

// NewAuthUsecase constructs an AuthUsecase backed by the shared OpenFGA Authorizer.
func NewAuthUsecase(authorizer *openfga.Authorizer) (*AuthUsecase, error) {
	if authorizer == nil {
		return nil, fmt.Errorf("admin auth usecase: authorizer is nil")
	}
	return &AuthUsecase{authorizer: authorizer}, nil
}

// CheckAccess verifies whether a human user holds access on admin:global.
func (uc *AuthUsecase) CheckAccess(ctx context.Context, userID string) (bool, error) {
	return uc.authorizer.Check(
		ctx,
		security.Actor{Type: security.ActorTypeHuman, ID: userID},
		PermAccess,
		AdminResourceType,
		AdminResourceID,
	)
}

// Permissions queries OpenFGA BatchCheck for all Admin permission codes in deterministic order.
func (uc *AuthUsecase) Permissions(ctx context.Context, actor security.Actor) ([]string, error) {
	reqs := make([]openfga.Request, len(adminPermissionOrder))
	for i, action := range adminPermissionOrder {
		reqs[i] = openfga.Request{
			Actor:        actor,
			Action:       action,
			ResourceType: AdminResourceType,
			ResourceID:   AdminResourceID,
		}
	}
	allowed, err := uc.authorizer.BatchCheck(ctx, reqs)
	if err != nil {
		return nil, err
	}
	perms := make([]string, 0, len(adminPermissionOrder))
	for i, ok := range allowed {
		if ok {
			perms = append(perms, adminPermissionOrder[i])
		}
	}
	return perms, nil
}
