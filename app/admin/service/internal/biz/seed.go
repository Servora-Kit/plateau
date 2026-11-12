package biz

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"sync/atomic"

	adminconfpb "github.com/Servora-Kit/plateau/api/gen/go/admin/conf/v1"
	userpb "github.com/Servora-Kit/plateau/api/gen/go/iam/user/v1"
	"github.com/Servora-Kit/plateau/security"
	"golang.org/x/sync/singleflight"
)

// AdminSeedRepo writes the initial administrator relation tuple idempotently.
type AdminSeedRepo interface {
	EnsureAdminTuple(ctx context.Context, userID string) error
}

// SeedInitializer reconciles bootstrap_admin_email to OpenFGA admin:global in Ensure-Seed mode.
type SeedInitializer struct {
	displayEmail   string
	canonicalEmail string
	users          IAMUserRepo
	seedRepo       AdminSeedRepo
	log            *slog.Logger

	seeded atomic.Bool
	sf     singleflight.Group
}

// NewSeedInitializer validates Admin bootstrap configuration and constructs SeedInitializer.
func NewSeedInitializer(cfg *adminconfpb.Admin, users IAMUserRepo, seedRepo AdminSeedRepo, l *slog.Logger) (*SeedInitializer, error) {
	if users == nil || seedRepo == nil {
		return nil, fmt.Errorf("admin seed initializer: repository dependency is nil")
	}
	display := strings.TrimSpace(cfg.GetBootstrapAdminEmail())
	canonical := security.NormalizeEmail(display)
	if canonical == "" {
		return nil, fmt.Errorf("admin seed initializer: bootstrap_admin_email is empty")
	}
	if l == nil {
		l = slog.Default()
	}
	return &SeedInitializer{
		displayEmail:   display,
		canonicalEmail: canonical,
		users:          users,
		seedRepo:       seedRepo,
		log:            l.With("scope", "admin/biz/seed"),
	}, nil
}

// Seeded reports whether the bootstrap administrator tuple has been confirmed in this process.
func (s *SeedInitializer) Seeded() bool {
	return s.seeded.Load()
}

// Ensure locates the active/undeleted IAM user matching bootstrap_admin_email and ensures
// user:<id> admin admin:global exists in OpenFGA without deleting any other tuples.
func (s *SeedInitializer) Ensure(ctx context.Context) error {
	if s.seeded.Load() {
		return nil
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	ch := s.sf.DoChan("seed", func() (any, error) {
		if s.seeded.Load() {
			return nil, nil
		}
		resp, err := s.users.ListUsers(ctx, &userpb.ListUsersRequest{
			Filter:      fmt.Sprintf("email = %s", strconv.Quote(s.canonicalEmail)),
			ShowDeleted: true,
			PageSize:    2,
		})
		if err != nil {
			return nil, fmt.Errorf("lookup bootstrap admin user in IAM: %w", err)
		}

		var matched []*userpb.User
		for _, candidate := range resp.GetUsers() {
			if security.NormalizeEmail(candidate.GetEmail()) == s.canonicalEmail {
				matched = append(matched, candidate)
			}
		}
		if len(matched) == 0 {
			s.log.WarnContext(ctx, "bootstrap admin user not found in IAM yet; will retry lazily", "email", s.displayEmail)
			return nil, nil
		}
		if len(matched) > 1 {
			return nil, fmt.Errorf("ambiguous IAM users matched bootstrap_admin_email")
		}

		target := matched[0]
		if target.GetDeleteTime() != nil {
			s.log.WarnContext(ctx, "bootstrap admin user is soft-deleted in IAM; skipping seed", "user_id", target.GetUserId())
			return nil, nil
		}
		if target.GetUserId() == "" {
			return nil, fmt.Errorf("matched IAM user has empty user_id")
		}

		if err := s.seedRepo.EnsureAdminTuple(ctx, target.GetUserId()); err != nil {
			return nil, fmt.Errorf("ensure OpenFGA admin tuple: %w", err)
		}
		s.seeded.Store(true)
		return nil, nil
	})

	select {
	case <-ctx.Done():
		return ctx.Err()
	case res := <-ch:
		return res.Err
	}
}
