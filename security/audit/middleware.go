package audit

import (
	"context"
	"log/slog"

	auditpb "github.com/Servora-Kit/plateau/api/gen/go/plateau/security/audit/v1"
	securityerrorspb "github.com/Servora-Kit/plateau/api/gen/go/plateau/security/errors/v1"
	security "github.com/Servora-Kit/plateau/security"
	"github.com/go-kratos/kratos/v3/middleware"
	"github.com/go-kratos/kratos/v3/transport"
	"google.golang.org/protobuf/proto"
)

// Server constructs Kratos server middleware that emits CloudEvents audit
// records according to generated Plateau Audit rules, and unconditionally
// records 403 permission-denied rejections across all operations.
//
// Place Server after AuthN middleware and before AuthZ middleware so that:
//  1. Verified Actor identity from AuthN is already present in ctx;
//  2. Downstream AuthZ PERMISSION_DENIED errors are intercepted and audited
//     even on read-only endpoints whose AuditMode is DISABLED.
func Server(auditor Auditor, opts ...security.RulesOption[*auditpb.AuditRule]) middleware.Middleware {
	if auditor == nil {
		panic("audit: auditor is nil")
	}
	rules := security.NewRules(opts...)

	return func(handler middleware.Handler) middleware.Handler {
		return func(ctx context.Context, req any) (any, error) {
			var operation string
			if tr, ok := transport.FromServerContext(ctx); ok && tr != nil {
				operation = tr.Operation()
			}

			rule := rules[operation]
			if rule == nil || rule.GetMode() != auditpb.AuditMode_AUDIT_MODE_ENABLED {
				resp, err := handler(ctx, req)
				if err != nil && securityerrorspb.IsSecurityErrorReasonPermissionDenied(err) {
					emit(ctx, auditor, operation, nil, nil, req, resp, err)
				}
				return resp, err
			}

			slot := &requestSlot{}
			enrichedCtx := context.WithValue(ctx, slotContextKey{}, slot)
			resp, err := handler(enrichedCtx, req)
			emit(enrichedCtx, auditor, operation, rule, slot, req, resp, err)
			return resp, err
		}
	}
}

func emit(
	ctx context.Context,
	auditor Auditor,
	operation string,
	rule *auditpb.AuditRule,
	slot *requestSlot,
	req any,
	resp any,
	err error,
) {
	eventType := DefaultRPCEventType
	category := ruleCategory(rule)
	if err != nil && securityerrorspb.IsSecurityErrorReasonPermissionDenied(err) {
		eventType = AuthzDeniedEventType
		category = CategoryAuthz
	}

	var resType, resName string
	reqMsg, reqIsProto := req.(proto.Message)
	if reqIsProto {
		resType, resName = ExtractResource(reqMsg)
	}
	if (resType == "" || resName == "") && err == nil {
		if respMsg, respIsProto := resp.(proto.Message); respIsProto {
			respType, respName := ExtractResource(respMsg)
			if resType == "" {
				resType = respType
			}
			if resName == "" {
				resName = respName
			}
		}
	}

	opts := []EventOption{
		WithType(eventType),
		WithSubject(operation),
		WithCategory(category),
	}
	if slot != nil {
		slot.mu.Lock()
		if slot.resourceType != "" {
			resType = slot.resourceType
		}
		if slot.resourceName != "" {
			resName = slot.resourceName
		}
		if slot.hasActor {
			opts = append(opts, WithActor(slot.actor))
		}
		slot.mu.Unlock()
	}
	opts = append(opts, WithResource(resType, resName), WithError(err))

	event := NewEvent(ctx, opts...)
	if reqIsProto {
		if dataErr := SetJSONProtoData(&event, reqMsg); dataErr != nil {
			slog.WarnContext(ctx, "audit marshal request data failed",
				slog.String("operation", operation),
				slog.String("error", dataErr.Error()),
			)
		}
	}
	if emitErr := auditor.Emit(ctx, event); emitErr != nil {
		slog.ErrorContext(ctx, "audit emit failed",
			slog.String("operation", operation),
			slog.String("error", emitErr.Error()),
		)
	}
}

func ruleCategory(rule *auditpb.AuditRule) string {
	switch rule.GetCategory() {
	case auditpb.AuditCategory_AUDIT_CATEGORY_AUTHN:
		return CategoryAuthn
	case auditpb.AuditCategory_AUDIT_CATEGORY_AUTHZ:
		return CategoryAuthz
	default:
		return CategoryResource
	}
}
