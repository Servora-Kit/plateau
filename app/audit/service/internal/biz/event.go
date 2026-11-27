package biz

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	auditeventpb "github.com/Servora-Kit/plateau/api/gen/go/plateau/audit/event/v1"
	securityauditpb "github.com/Servora-Kit/plateau/api/gen/go/plateau/security/audit/v1"
	"github.com/Servora-Kit/plateau/security/audit"
	cloudevents "github.com/cloudevents/sdk-go/v2"
	cetypes "github.com/cloudevents/sdk-go/v2/types"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/protobuf/types/known/timestamppb"
)

const (
	defaultPageSize = 50
	maxPageSize     = 200
)

// EventCursor positions pagination after a prior (OccurredAt, EventID) tuple.
type EventCursor struct {
	OccurredAt time.Time
	EventID    string
}

// ListEventsQuery carries caller-supplied list parameters before pagination token decoding.
type ListEventsQuery struct {
	StartTime    *time.Time
	EndTime      *time.Time
	Category     securityauditpb.AuditCategory
	Service      string
	EventType    string
	Operation    string
	ActorType    string
	ActorID      string
	ResourceType string
	ResourceName string
	Success      *bool
	TraceID      string
	PageSize     int
	PageToken    string
}

// ListEventsFilter carries validated exact-match predicates and decoded cursor to EventRepo.
type ListEventsFilter struct {
	StartTime    *time.Time
	EndTime      *time.Time
	Category     string
	Service      string
	EventType    string
	Operation    string
	ActorType    string
	ActorID      string
	ResourceType string
	ResourceName string
	Success      *bool
	TraceID      string
	Limit        int
	Cursor       *EventCursor
}

// EventRepo defines the persistence port owned by biz and implemented by data.
type EventRepo interface {
	SaveBatch(ctx context.Context, events []*auditeventpb.Event) error
	GetByID(ctx context.Context, eventID string) (*auditeventpb.Event, error)
	List(ctx context.Context, filter ListEventsFilter) ([]*auditeventpb.Event, error)
	ListServices(ctx context.Context) ([]string, error)
}

// EventUsecase coordinates CloudEvents ingestion projection and read-only event retrieval.
type EventUsecase struct {
	repo EventRepo
	log  *slog.Logger
}

// NewEventUsecase constructs an EventUsecase.
func NewEventUsecase(repo EventRepo, l *slog.Logger) (*EventUsecase, error) {
	if repo == nil {
		return nil, fmt.Errorf("audit biz: event repository is nil")
	}
	if l == nil {
		l = slog.Default()
	}
	return &EventUsecase{
		repo: repo,
		log:  l.With("scope", "audit/biz/event"),
	}, nil
}

// IngestEvents validates required CloudEvents v1.0 attributes, normalizes extensions
// and payloads, drops malformed events with structured diagnostics, and persists valid events.
func (uc *EventUsecase) IngestEvents(ctx context.Context, rawEvents []cloudevents.Event) error {
	if len(rawEvents) == 0 {
		return nil
	}

	valid := make([]*auditeventpb.Event, 0, len(rawEvents))
	for i := range rawEvents {
		ev := &rawEvents[i]
		eventID := strings.TrimSpace(ev.ID())
		eventType := strings.TrimSpace(ev.Type())
		service := strings.TrimSpace(ev.Source())
		if eventID == "" || eventType == "" || service == "" || ev.Time().IsZero() {
			uc.log.WarnContext(ctx, "dropping cloud event missing required attributes",
				"event_id", eventID,
				"event_type", eventType,
				"source", service,
				"has_time", !ev.Time().IsZero(),
			)
			continue
		}

		cat := securityauditpb.AuditCategory_AUDIT_CATEGORY_UNSPECIFIED
		switch strings.ToLower(strings.TrimSpace(extString(ev, audit.ExtCategory))) {
		case audit.CategoryAuthn:
			cat = securityauditpb.AuditCategory_AUDIT_CATEGORY_AUTHN
		case audit.CategoryResource:
			cat = securityauditpb.AuditCategory_AUDIT_CATEGORY_RESOURCE
		case audit.CategoryAuthz:
			cat = securityauditpb.AuditCategory_AUDIT_CATEGORY_AUTHZ
		}

		errorCode := strings.TrimSpace(extString(ev, audit.ExtErrorCode))
		errorMessage := strings.TrimSpace(extString(ev, audit.ExtErrorMessage))
		outcome := strings.ToLower(strings.TrimSpace(extString(ev, audit.ExtOutcome)))
		success := errorCode == "" && errorMessage == ""
		switch outcome {
		case audit.OutcomeSuccess:
			success = true
		case audit.OutcomeFailure:
			success = false
		}

		var traceID string
		if tp := strings.ToLower(strings.TrimSpace(extString(ev, audit.ExtTraceParent))); len(tp) == 55 &&
			tp[2] == '-' && tp[35] == '-' && tp[52] == '-' && tp[:2] != "ff" {
			if tid, err := trace.TraceIDFromHex(tp[3:35]); err == nil {
				if _, spanErr := trace.SpanIDFromHex(tp[36:52]); spanErr == nil {
					traceID = tid.String()
				}
			}
		}
		if traceID == "" {
			traceID = strings.ToLower(strings.TrimSpace(extString(ev, "traceid")))
		}

		detailJSON := "{}"
		if raw := ev.Data(); len(bytes.TrimSpace(raw)) > 0 {
			ct := strings.ToLower(strings.TrimSpace(ev.DataContentType()))
			if base, _, found := strings.Cut(ct, ";"); found {
				ct = strings.TrimSpace(base)
			}
			encoded := false
			if ct == "" || ct == cloudevents.ApplicationJSON || strings.HasSuffix(ct, "+json") {
				if json.Valid(raw) {
					var compact bytes.Buffer
					if err := json.Compact(&compact, raw); err == nil && compact.Len() > 0 {
						if compact.Bytes()[0] == '{' {
							detailJSON = compact.String()
							encoded = true
						} else if wrapped, err := json.Marshal(struct {
							Value json.RawMessage `json:"value"`
						}{Value: compact.Bytes()}); err == nil {
							detailJSON = string(wrapped)
							encoded = true
						}
					}
				}
			}
			if !encoded {
				if envelope, err := json.Marshal(struct {
					ContentType string `json:"_content_type"`
					DataSchema  string `json:"_dataschema"`
					Base64      string `json:"_base64"`
				}{
					ContentType: strings.TrimSpace(ev.DataContentType()),
					DataSchema:  strings.TrimSpace(ev.DataSchema()),
					Base64:      base64.StdEncoding.EncodeToString(raw),
				}); err == nil {
					detailJSON = string(envelope)
				}
			}
		}

		name, err := auditeventpb.NewEventName(eventID).Format()
		if err != nil {
			name = "events/" + eventID
		}

		valid = append(valid, &auditeventpb.Event{
			Name:         name,
			EventId:      eventID,
			EventType:    eventType,
			Category:     cat,
			OccurredTime: timestamppb.New(ev.Time().UTC().Truncate(time.Microsecond)),
			Service:      service,
			Operation:    strings.TrimSpace(ev.Subject()),
			ActorType:    strings.TrimSpace(extString(ev, audit.ExtActorType)),
			ActorId:      strings.TrimSpace(extString(ev, audit.ExtActorID)),
			ResourceType: strings.TrimSpace(extString(ev, audit.ExtResourceType)),
			ResourceName: strings.TrimSpace(extString(ev, audit.ExtResourceName)),
			Success:      success,
			ErrorCode:    errorCode,
			ErrorMessage: errorMessage,
			TraceId:      traceID,
			RequestId:    strings.TrimSpace(extString(ev, audit.ExtRequestID)),
			ClientIp:     strings.TrimSpace(extString(ev, audit.ExtClientIP)),
			UserAgent:    strings.TrimSpace(extString(ev, audit.ExtUserAgent)),
			DetailJson:   detailJSON,
		})
	}

	if len(valid) == 0 {
		return nil
	}
	return uc.repo.SaveBatch(ctx, valid)
}

// GetEvent retrieves one audit event by bare event_id.
func (uc *EventUsecase) GetEvent(ctx context.Context, eventID string) (*auditeventpb.Event, error) {
	eventID = strings.TrimSpace(eventID)
	if eventID == "" {
		return nil, ErrInvalidArgument
	}
	return uc.repo.GetByID(ctx, eventID)
}

// ListEvents validates query filters, verifies page_token integrity, and fetches one page.
func (uc *EventUsecase) ListEvents(ctx context.Context, q ListEventsQuery) ([]*auditeventpb.Event, string, error) {
	if q.PageSize < 0 {
		return nil, "", ErrInvalidArgument
	}
	pageSize := q.PageSize
	if pageSize == 0 {
		pageSize = defaultPageSize
	} else if pageSize > maxPageSize {
		pageSize = maxPageSize
	}

	var category string
	switch q.Category {
	case securityauditpb.AuditCategory_AUDIT_CATEGORY_UNSPECIFIED:
	case securityauditpb.AuditCategory_AUDIT_CATEGORY_AUTHN:
		category = audit.CategoryAuthn
	case securityauditpb.AuditCategory_AUDIT_CATEGORY_RESOURCE:
		category = audit.CategoryResource
	case securityauditpb.AuditCategory_AUDIT_CATEGORY_AUTHZ:
		category = audit.CategoryAuthz
	default:
		return nil, "", ErrInvalidArgument
	}

	var startTime, endTime *time.Time
	if q.StartTime != nil {
		t := q.StartTime.UTC().Truncate(time.Microsecond)
		startTime = &t
	}
	if q.EndTime != nil {
		t := q.EndTime.UTC().Truncate(time.Microsecond)
		endTime = &t
	}
	if startTime != nil && endTime != nil && startTime.After(*endTime) {
		return nil, "", ErrInvalidArgument
	}

	normalizedQuery := ListEventsQuery{
		StartTime:    startTime,
		EndTime:      endTime,
		Category:     q.Category,
		Service:      strings.TrimSpace(q.Service),
		EventType:    strings.TrimSpace(q.EventType),
		Operation:    strings.TrimSpace(q.Operation),
		ActorType:    strings.TrimSpace(q.ActorType),
		ActorID:      strings.TrimSpace(q.ActorID),
		ResourceType: strings.TrimSpace(q.ResourceType),
		ResourceName: strings.TrimSpace(q.ResourceName),
		Success:      q.Success,
		TraceID:      strings.ToLower(strings.TrimSpace(q.TraceID)),
	}

	fp, err := queryFingerprint(normalizedQuery)
	if err != nil {
		return nil, "", err
	}

	filter := ListEventsFilter{
		StartTime:    normalizedQuery.StartTime,
		EndTime:      normalizedQuery.EndTime,
		Category:     category,
		Service:      normalizedQuery.Service,
		EventType:    normalizedQuery.EventType,
		Operation:    normalizedQuery.Operation,
		ActorType:    normalizedQuery.ActorType,
		ActorID:      normalizedQuery.ActorID,
		ResourceType: normalizedQuery.ResourceType,
		ResourceName: normalizedQuery.ResourceName,
		Success:      normalizedQuery.Success,
		TraceID:      normalizedQuery.TraceID,
		Limit:        pageSize + 1,
	}

	if token := strings.TrimSpace(q.PageToken); token != "" {
		cursor, err := decodeEventPageToken(token, fp)
		if err != nil {
			return nil, "", err
		}
		filter.Cursor = cursor
	}

	rows, err := uc.repo.List(ctx, filter)
	if err != nil {
		return nil, "", err
	}

	var nextToken string
	if len(rows) > pageSize {
		rows = rows[:pageSize]
		nextToken, err = encodeEventPageToken(fp, rows[pageSize-1])
		if err != nil {
			return nil, "", fmt.Errorf("encode next page token: %w", err)
		}
	}
	return rows, nextToken, nil
}

// ListServices returns distinct non-empty producer service identifiers observed in storage.
func (uc *EventUsecase) ListServices(ctx context.Context) ([]string, error) {
	return uc.repo.ListServices(ctx)
}

func extString(ev *cloudevents.Event, key string) string {
	val, err := ev.Context.GetExtension(key)
	if err != nil || val == nil {
		return ""
	}
	s, err := cetypes.ToString(val)
	if err != nil {
		return fmt.Sprint(val)
	}
	return s
}
