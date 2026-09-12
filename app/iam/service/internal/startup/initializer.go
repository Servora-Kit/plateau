package startup

import (
	"context"
	"fmt"

	"github.com/Servora-Kit/plateau/app/iam/service/internal/biz"
	"github.com/Servora-Kit/plateau/app/iam/service/internal/oidc"
)

// Initializer runs IAM startup tasks in their required order before traffic is served.
type Initializer struct {
	user *biz.UserInitializer
	oidc *oidc.OIDCInitializer
}

// NewInitializer validates the application startup dependencies.
func NewInitializer(user *biz.UserInitializer, oidcInitializer *oidc.OIDCInitializer) (*Initializer, error) {
	if user == nil || oidcInitializer == nil {
		return nil, fmt.Errorf("IAM startup initializer: dependency is nil")
	}
	return &Initializer{user: user, oidc: oidcInitializer}, nil
}

// Initialize creates the initial user before reconciling OIDC state.
func (initializer *Initializer) Initialize(ctx context.Context) error {
	if err := initializer.user.Initialize(ctx); err != nil {
		return err
	}
	if err := initializer.oidc.Initialize(ctx); err != nil {
		return err
	}
	return nil
}
