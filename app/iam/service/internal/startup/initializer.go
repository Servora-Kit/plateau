package startup

import (
	"context"
	"fmt"

	"github.com/Servora-Kit/plateau/app/iam/service/internal/biz"
	"github.com/Servora-Kit/plateau/app/iam/service/internal/oidc"
)

// Initializer runs IAM startup tasks in their required order before traffic is served.
type Initializer struct {
	admin *biz.AdminInitializer
	oidc  *oidc.OIDCInitializer
}

// NewInitializer validates the application startup dependencies.
func NewInitializer(admin *biz.AdminInitializer, oidcInitializer *oidc.OIDCInitializer) (*Initializer, error) {
	if admin == nil || oidcInitializer == nil {
		return nil, fmt.Errorf("IAM startup initializer: dependency is nil")
	}
	return &Initializer{admin: admin, oidc: oidcInitializer}, nil
}

// Initialize creates the initial administrator before reconciling OIDC state.
func (initializer *Initializer) Initialize(ctx context.Context) error {
	if err := initializer.admin.Initialize(ctx); err != nil {
		return err
	}
	if err := initializer.oidc.Initialize(ctx); err != nil {
		return err
	}
	return nil
}
