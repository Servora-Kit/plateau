package audit

import (
	"context"
	"sync"

	security "github.com/Servora-Kit/plateau/security"
)

type slotContextKey struct{}

type requestSlot struct {
	mu           sync.Mutex
	hasActor     bool
	actor        security.Actor
	resourceType string
	resourceName string
}

// SetActor enriches the current RPC audit event with a verified execution Actor.
// Intended for AUTHN_MODE_PUBLIC endpoints (such as Login, ChangeInitialPassword,
// VerifyEmail, or ConfirmPasswordReset) that verify identity inside the handler.
func SetActor(ctx context.Context, actor security.Actor) {
	if ctx == nil || !actor.Valid() {
		return
	}
	if slot, ok := ctx.Value(slotContextKey{}).(*requestSlot); ok && slot != nil {
		slot.mu.Lock()
		slot.hasActor = true
		slot.actor = actor
		slot.mu.Unlock()
	}
}

// SetResource enriches or overrides the target resource metadata recorded on
// the current RPC audit event.
func SetResource(ctx context.Context, resourceType, resourceName string) {
	if ctx == nil {
		return
	}
	if slot, ok := ctx.Value(slotContextKey{}).(*requestSlot); ok && slot != nil {
		slot.mu.Lock()
		if resourceType != "" {
			slot.resourceType = resourceType
		}
		if resourceName != "" {
			slot.resourceName = resourceName
		}
		slot.mu.Unlock()
	}
}
