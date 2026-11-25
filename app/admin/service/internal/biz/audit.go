package biz

import (
	"context"
	"errors"
	"fmt"

	auditeventpb "github.com/Servora-Kit/plateau/api/gen/go/audit/event/v1"
	"github.com/Servora-Kit/plateau/security"
)

// ErrInvalidAuditArgument indicates invalid query parameters supplied to AuditUsecase.
var ErrInvalidAuditArgument = errors.New("admin: invalid audit query argument")

// AuditEventRepo abstracts downstream audit.service EventService gRPC operations.
type AuditEventRepo interface {
	ListEvents(ctx context.Context, req *auditeventpb.ListEventsRequest) (*auditeventpb.ListEventsResponse, error)
	GetEvent(ctx context.Context, req *auditeventpb.GetEventRequest) (*auditeventpb.Event, error)
}

// AuditUsecase orchestrates read-only control-plane audit event queries for platform administrators.
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
	actor, ok := security.ActorFrom(ctx)
	if !ok || !actor.Valid() || actor.Type != security.ActorTypeHuman {
		return nil, ErrUnauthenticated
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

// GetEvent retrieves one control-plane audit event by validated resource name.
func (uc *AuditUsecase) GetEvent(ctx context.Context, name auditeventpb.EventName) (*auditeventpb.Event, error) {
	actor, ok := security.ActorFrom(ctx)
	if !ok || !actor.Valid() || actor.Type != security.ActorTypeHuman {
		return nil, ErrUnauthenticated
	}
	formatted, err := name.Format()
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidAuditArgument, err)
	}
	return uc.repo.GetEvent(ctx, &auditeventpb.GetEventRequest{Name: formatted})
}
