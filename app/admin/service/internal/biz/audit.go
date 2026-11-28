package biz

import (
	"context"
	"errors"
	"fmt"

	auditeventpb "github.com/Servora-Kit/plateau/api/gen/go/plateau/audit/event/v1"
)

// ErrInvalidAuditArgument indicates invalid query parameters supplied to AuditUsecase.
var ErrInvalidAuditArgument = errors.New("admin: invalid audit query argument")

// AuditEventRepo abstracts downstream audit.service EventService gRPC operations.
type AuditEventRepo interface {
	ListEvents(ctx context.Context, req *auditeventpb.ListEventsRequest) (*auditeventpb.ListEventsResponse, error)
	GetEvent(ctx context.Context, req *auditeventpb.GetEventRequest) (*auditeventpb.Event, error)
	ListServices(ctx context.Context, req *auditeventpb.ListServicesRequest) (*auditeventpb.ListServicesResponse, error)
}

// AuditUsecase orchestrates read-only platform audit event queries for platform administrators.
type AuditUsecase struct {
	repo AuditEventRepo
}

// NewAuditUsecase constructs AuditUsecase backed by AuditEventRepo.
func NewAuditUsecase(repo AuditEventRepo) (*AuditUsecase, error) {
	if repo == nil {
		return nil, fmt.Errorf("admin audit usecase: repo is nil")
	}
	return &AuditUsecase{repo: repo}, nil
}

// ListEvents validates basic query constraints and delegates pagination/filtering to audit.service.
func (uc *AuditUsecase) ListEvents(ctx context.Context, req *auditeventpb.ListEventsRequest) (*auditeventpb.ListEventsResponse, error) {
	if _, err := requireHumanActor(ctx); err != nil {
		return nil, err
	}
	if req == nil {
		req = &auditeventpb.ListEventsRequest{}
	}
	if req.GetPageSize() < 0 {
		return nil, fmt.Errorf("%w: page_size must not be negative", ErrInvalidAuditArgument)
	}
	if req.GetStartTime() != nil {
		if err := req.GetStartTime().CheckValid(); err != nil {
			return nil, fmt.Errorf("%w: invalid start_time: %v", ErrInvalidAuditArgument, err)
		}
	}
	if req.GetEndTime() != nil {
		if err := req.GetEndTime().CheckValid(); err != nil {
			return nil, fmt.Errorf("%w: invalid end_time: %v", ErrInvalidAuditArgument, err)
		}
	}
	if req.GetStartTime() != nil && req.GetEndTime() != nil && req.GetStartTime().AsTime().After(req.GetEndTime().AsTime()) {
		return nil, fmt.Errorf("%w: start_time must not be after end_time", ErrInvalidAuditArgument)
	}
	return uc.repo.ListEvents(ctx, req)
}

// GetEvent retrieves one platform audit event by validated resource name.
func (uc *AuditUsecase) GetEvent(ctx context.Context, name auditeventpb.EventName) (*auditeventpb.Event, error) {
	if _, err := requireHumanActor(ctx); err != nil {
		return nil, err
	}
	formatted, err := name.Format()
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidAuditArgument, err)
	}
	return uc.repo.GetEvent(ctx, &auditeventpb.GetEventRequest{Name: formatted})
}

// ListServices retrieves distinct producer service identifiers for human administrators.
func (uc *AuditUsecase) ListServices(ctx context.Context) ([]string, error) {
	if _, err := requireHumanActor(ctx); err != nil {
		return nil, err
	}
	resp, err := uc.repo.ListServices(ctx, &auditeventpb.ListServicesRequest{})
	if err != nil {
		return nil, err
	}
	return resp.GetServices(), nil
}
