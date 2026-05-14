package service

import (
	"context"

	auditsvcpb "github.com/Servora-Kit/servora-platform/api/gen/go/audit/service/v1"
	"github.com/Servora-Kit/servora-platform/app/audit/service/internal/biz"
)

// AuditService implements both AuditQueryService (gRPC) and AuditHTTPService (HTTP).
type AuditService struct {
	auditsvcpb.UnimplementedAuditQueryServiceServer
	auditsvcpb.UnimplementedAuditHTTPServiceServer
	uc *biz.AuditUsecase
}

// NewAuditService creates a new AuditService.
func NewAuditService(uc *biz.AuditUsecase) *AuditService {
	return &AuditService{uc: uc}
}

func (s *AuditService) ListAuditEvents(ctx context.Context, req *auditsvcpb.ListAuditEventsRequest) (*auditsvcpb.ListAuditEventsResponse, error) {
	items, nextToken, err := s.uc.ListEvents(ctx, req)
	if err != nil {
		return nil, err
	}
	return &auditsvcpb.ListAuditEventsResponse{
		Events:        items,
		NextPageToken: nextToken,
	}, nil
}

func (s *AuditService) CountAuditEvents(ctx context.Context, req *auditsvcpb.CountAuditEventsRequest) (*auditsvcpb.CountAuditEventsResponse, error) {
	count, err := s.uc.CountEvents(ctx, req)
	if err != nil {
		return nil, err
	}
	return &auditsvcpb.CountAuditEventsResponse{TotalCount: count}, nil
}
