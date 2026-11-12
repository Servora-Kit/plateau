// Package startup coordinates non-blocking Admin startup initialization.
package startup

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/Servora-Kit/plateau/app/admin/service/internal/biz"
	"github.com/google/wire"
)

const startupSeedTimeout = 3 * time.Second

// ProviderSet provides Admin startup initialization hooks.
var ProviderSet = wire.NewSet(NewInitializer)

// Initializer runs a non-blocking Ensure-Seed attempt during Kratos BeforeStart.
type Initializer struct {
	seed *biz.SeedInitializer
	log  *slog.Logger
}

// NewInitializer constructs Initializer for bootstrap administrator seed synchronization.
func NewInitializer(seed *biz.SeedInitializer, l *slog.Logger) (*Initializer, error) {
	if seed == nil {
		return nil, fmt.Errorf("admin startup initializer: seed initializer is nil")
	}
	if l == nil {
		l = slog.Default()
	}
	return &Initializer{
		seed: seed,
		log:  l.With("scope", "admin/startup"),
	}, nil
}

// Initialize attempts to seed bootstrap_admin_email with a 3s timeout; if IAM or OpenFGA
// is still warming up, it logs a warning and allows startup to proceed (lazy retry happens on login callback).
func (i *Initializer) Initialize(ctx context.Context) error {
	seedCtx, cancel := context.WithTimeout(ctx, startupSeedTimeout)
	defer cancel()

	if err := i.seed.Ensure(seedCtx); err != nil {
		i.log.WarnContext(ctx, "startup admin seed sync deferred until OIDC callback", "err", err)
	}
	return nil
}
