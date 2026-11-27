package audit

import (
	"context"
	"maps"
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
	detail       map[string]any
	hasError     bool
	errorCode    string
	errorMessage string
	eventType    string
	category     string
	skip         bool
}

// SetActor enriches the current RPC or HTTP audit event with a verified execution Actor.
// Intended for AUTHN_MODE_PUBLIC endpoints (such as SubmitPassword, CompleteUpdatePassword,
// VerifyEmail, or ConfirmPasswordReset) and native HTTP protocol endpoints that verify
// identity inside the handler.
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
// the current RPC or HTTP audit event.
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

// SetDetail merges non-sensitive key-value metadata into an HTTPHandler-wrapped
// audit event's JSON data payload.
func SetDetail(ctx context.Context, detail map[string]any) {
	if ctx == nil || len(detail) == 0 {
		return
	}
	if slot, ok := ctx.Value(slotContextKey{}).(*requestSlot); ok && slot != nil {
		slot.mu.Lock()
		if slot.detail == nil {
			slot.detail = make(map[string]any, len(detail))
		}
		maps.Copy(slot.detail, detail)
		slot.mu.Unlock()
	}
}

// SetError marks an HTTPHandler-wrapped audit event as failed with an explicit
// error code and sanitized message (for HTTP handlers that redirect on error
// rather than returning a 4xx/5xx status code).
func SetError(ctx context.Context, code, message string) {
	if ctx == nil {
		return
	}
	if slot, ok := ctx.Value(slotContextKey{}).(*requestSlot); ok && slot != nil {
		slot.mu.Lock()
		slot.hasError = true
		if code != "" {
			slot.errorCode = code
		}
		if message != "" {
			slot.errorMessage = message
		}
		slot.mu.Unlock()
	}
}

// SetEventOverride overrides the default CloudEvents type and control-plane
// category on an HTTPHandler-wrapped audit event (for example upgrading an
// authentication callback rejection to AuthzDeniedEventType + CategoryAuthz).
func SetEventOverride(ctx context.Context, eventType, category string) {
	if ctx == nil {
		return
	}
	if slot, ok := ctx.Value(slotContextKey{}).(*requestSlot); ok && slot != nil {
		slot.mu.Lock()
		if eventType != "" {
			slot.eventType = eventType
		}
		if category != "" {
			slot.category = category
		}
		slot.mu.Unlock()
	}
}

// Skip suppresses emission of an HTTPHandler-wrapped audit event (for example
// normal unauthenticated browser redirects to /login during OIDC authorization).
func Skip(ctx context.Context) {
	if ctx == nil {
		return
	}
	if slot, ok := ctx.Value(slotContextKey{}).(*requestSlot); ok && slot != nil {
		slot.mu.Lock()
		slot.skip = true
		slot.mu.Unlock()
	}
}
