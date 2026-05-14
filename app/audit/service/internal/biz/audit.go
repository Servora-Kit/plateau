package biz

import (
	"context"

	auditsvcpb "github.com/Servora-Kit/servora-platform/api/gen/go/audit/service/v1"
)

// AuditRepo defines the read-side repository for querying persisted audit events.
type AuditRepo interface {
	ListEvents(ctx context.Context, req *auditsvcpb.ListAuditEventsRequest) ([]*auditsvcpb.AuditEventItem, string, error)
	CountEvents(ctx context.Context, req *auditsvcpb.CountAuditEventsRequest) (int64, error)
}

// AuditUsecase encapsulates query use cases for the audit service.
type AuditUsecase struct {
	repo AuditRepo
}

// NewAuditUsecase creates a new AuditUsecase.
func NewAuditUsecase(repo AuditRepo) *AuditUsecase {
	return &AuditUsecase{repo: repo}
}

// ListEvents delegates to the repository.
func (uc *AuditUsecase) ListEvents(ctx context.Context, req *auditsvcpb.ListAuditEventsRequest) ([]*auditsvcpb.AuditEventItem, string, error) {
	return uc.repo.ListEvents(ctx, req)
}

// CountEvents delegates to the repository.
func (uc *AuditUsecase) CountEvents(ctx context.Context, req *auditsvcpb.CountAuditEventsRequest) (int64, error) {
	return uc.repo.CountEvents(ctx, req)
}
