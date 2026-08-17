package service

import (
	"context"

	auditpb "github.com/Servora-Kit/plateau/api/gen/go/audit/service/v1"
	"github.com/Servora-Kit/plateau/app/audit/service/internal/biz"
)

// AuditService implements the generated AuditQueryService gRPC and HTTP contract.
type AuditService struct {
	auditpb.UnimplementedAuditQueryServiceServer
	uc *biz.AuditUsecase
}

// NewAuditService creates a new AuditService.
func NewAuditService(uc *biz.AuditUsecase) *AuditService {
	return &AuditService{uc: uc}
}

func (s *AuditService) ListAuditEvents(ctx context.Context, req *auditpb.ListAuditEventsRequest) (*auditpb.ListAuditEventsResponse, error) {
	items, nextToken, err := s.uc.ListEvents(ctx, req)
	if err != nil {
		return nil, err
	}
	return &auditpb.ListAuditEventsResponse{
		Events:        items,
		NextPageToken: nextToken,
	}, nil
}

func (s *AuditService) CountAuditEvents(ctx context.Context, req *auditpb.CountAuditEventsRequest) (*auditpb.CountAuditEventsResponse, error) {
	count, err := s.uc.CountEvents(ctx, req)
	if err != nil {
		return nil, err
	}
	return &auditpb.CountAuditEventsResponse{TotalCount: count}, nil
}
