package service

import (
	"context"
	"errors"
	"fmt"
	"strings"

	auditeventpb "github.com/Servora-Kit/plateau/api/gen/go/audit/event/v1"
	"github.com/Servora-Kit/plateau/app/audit/service/internal/biz"
	cloudevents "github.com/cloudevents/sdk-go/v2"
)

// EventService adapts gRPC EventService calls and inbound Kafka batches to biz.EventUsecase.
type EventService struct {
	auditeventpb.UnimplementedEventServiceServer

	uc *biz.EventUsecase
}

// NewEventService constructs an EventService.
func NewEventService(uc *biz.EventUsecase) (*EventService, error) {
	if uc == nil {
		return nil, fmt.Errorf("audit service: event usecase is nil")
	}
	return &EventService{uc: uc}, nil
}

// IngestEvents forwards decoded CloudEvents batches from the Kafka transport server to the domain usecase.
func (s *EventService) IngestEvents(ctx context.Context, events []cloudevents.Event) error {
	return s.uc.IngestEvents(ctx, events)
}

// GetEvent retrieves a single control-plane audit event by canonical resource name ("events/{event}").
func (s *EventService) GetEvent(ctx context.Context, req *auditeventpb.GetEventRequest) (*auditeventpb.Event, error) {
	if req == nil {
		return nil, auditeventpb.ErrorEventErrorReasonInvalidArgument("request is required")
	}
	parsed, err := auditeventpb.ParseEventName(strings.TrimSpace(req.GetName()))
	if err != nil || strings.TrimSpace(parsed.Event) == "" {
		return nil, auditeventpb.ErrorEventErrorReasonInvalidArgument("invalid event resource name")
	}

	ev, err := s.uc.GetEvent(ctx, parsed.Event)
	if err != nil {
		return nil, eventError(err)
	}
	return ev, nil
}

// ListEvents returns a filtered page of control-plane audit events.
func (s *EventService) ListEvents(ctx context.Context, req *auditeventpb.ListEventsRequest) (*auditeventpb.ListEventsResponse, error) {
	if req == nil {
		req = &auditeventpb.ListEventsRequest{}
	}

	query := biz.ListEventsQuery{
		Category:     req.GetCategory(),
		Service:      req.GetService(),
		EventType:    req.GetEventType(),
		Operation:    req.GetOperation(),
		ActorType:    req.GetActorType(),
		ActorID:      req.GetActorId(),
		ResourceType: req.GetResourceType(),
		ResourceName: req.GetResourceName(),
		TraceID:      req.GetTraceId(),
		PageSize:     int(req.GetPageSize()),
		PageToken:    req.GetPageToken(),
	}
	if ts := req.GetStartTime(); ts != nil {
		if !ts.IsValid() {
			return nil, auditeventpb.ErrorEventErrorReasonInvalidArgument("invalid start_time")
		}
		t := ts.AsTime()
		query.StartTime = &t
	}
	if ts := req.GetEndTime(); ts != nil {
		if !ts.IsValid() {
			return nil, auditeventpb.ErrorEventErrorReasonInvalidArgument("invalid end_time")
		}
		t := ts.AsTime()
		query.EndTime = &t
	}
	if req.Success != nil {
		v := req.GetSuccess()
		query.Success = &v
	}

	items, nextToken, err := s.uc.ListEvents(ctx, query)
	if err != nil {
		return nil, eventError(err)
	}

	return &auditeventpb.ListEventsResponse{
		Events:        items,
		NextPageToken: nextToken,
	}, nil
}

func eventError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	switch {
	case errors.Is(err, biz.ErrEventNotFound):
		return auditeventpb.ErrorEventErrorReasonNotFound("audit event not found").WithCause(err)
	case errors.Is(err, biz.ErrInvalidPageToken):
		return auditeventpb.ErrorEventErrorReasonInvalidPageToken("invalid page token").WithCause(err)
	case errors.Is(err, biz.ErrInvalidArgument):
		return auditeventpb.ErrorEventErrorReasonInvalidArgument("invalid request argument").WithCause(err)
	case errors.Is(err, biz.ErrStorageUnavailable):
		return auditeventpb.ErrorEventErrorReasonStorageUnavailable("audit storage unavailable").WithCause(err)
	default:
		return auditeventpb.ErrorEventErrorReasonUnspecified("internal server error").WithCause(err)
	}
}
