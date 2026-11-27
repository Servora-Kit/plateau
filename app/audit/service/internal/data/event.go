package data

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	auditeventpb "github.com/Servora-Kit/plateau/api/gen/go/plateau/audit/event/v1"
	securityauditpb "github.com/Servora-Kit/plateau/api/gen/go/plateau/security/audit/v1"
	"github.com/Servora-Kit/plateau/app/audit/service/internal/biz"
	"github.com/Servora-Kit/plateau/app/audit/service/internal/data/schema"
	"github.com/Servora-Kit/plateau/security/audit"
	"google.golang.org/protobuf/types/known/timestamppb"
)

const selectEventColumns = `event_id, event_type, category, occurred_at, ingested_at,
       service, operation,
       actor_type, actor_id,
       resource_type, resource_name,
       success, error_code, error_message,
       trace_id, request_id, client_ip, user_agent, detail_json`

type eventRepo struct {
	data *Data
}

// NewEventRepo constructs the ClickHouse-backed biz.EventRepo implementation.
func NewEventRepo(d *Data) (biz.EventRepo, error) {
	if d == nil || d.ClickHouse() == nil {
		return nil, fmt.Errorf("audit data: ClickHouse connection is required")
	}
	return &eventRepo{data: d}, nil
}

func (r *eventRepo) SaveBatch(ctx context.Context, events []*auditeventpb.Event) error {
	if len(events) == 0 {
		return nil
	}

	batch, err := r.data.ClickHouse().PrepareBatch(ctx, "INSERT INTO "+schema.TableEvents)
	if err != nil {
		return err
	}

	ingestedAt := time.Now().UTC().Truncate(time.Microsecond)
	for _, ev := range events {
		var cat string
		switch ev.GetCategory() {
		case securityauditpb.AuditCategory_AUDIT_CATEGORY_AUTHN:
			cat = audit.CategoryAuthn
		case securityauditpb.AuditCategory_AUDIT_CATEGORY_RESOURCE:
			cat = audit.CategoryResource
		case securityauditpb.AuditCategory_AUDIT_CATEGORY_AUTHZ:
			cat = audit.CategoryAuthz
		}
		row := schema.EventRow{
			EventID:      ev.GetEventId(),
			EventType:    ev.GetEventType(),
			Category:     cat,
			OccurredAt:   ev.GetOccurredTime().AsTime().UTC(),
			IngestedAt:   ingestedAt,
			Service:      ev.GetService(),
			Operation:    ev.GetOperation(),
			ActorType:    ev.GetActorType(),
			ActorID:      ev.GetActorId(),
			ResourceType: ev.GetResourceType(),
			ResourceName: ev.GetResourceName(),
			Success:      ev.GetSuccess(),
			ErrorCode:    ev.GetErrorCode(),
			ErrorMessage: ev.GetErrorMessage(),
			TraceID:      ev.GetTraceId(),
			RequestID:    ev.GetRequestId(),
			ClientIP:     ev.GetClientIp(),
			UserAgent:    ev.GetUserAgent(),
			DetailJSON:   ev.GetDetailJson(),
		}
		if appendErr := batch.AppendStruct(&row); appendErr != nil {
			_ = batch.Abort()
			return appendErr
		}
	}

	return batch.Send()
}

func (r *eventRepo) GetByID(ctx context.Context, eventID string) (*auditeventpb.Event, error) {
	query := fmt.Sprintf(
		"SELECT %s FROM %s WHERE event_id = ? ORDER BY ingested_at DESC LIMIT 1",
		selectEventColumns,
		schema.TableEvents,
	)

	var rows []schema.EventRow
	if err := r.data.ClickHouse().Select(ctx, &rows, query, eventID); err != nil {
		if ctx.Err() != nil {
			return nil, err
		}
		return nil, errors.Join(biz.ErrStorageUnavailable, err)
	}
	if len(rows) == 0 {
		return nil, biz.ErrEventNotFound
	}
	return toProto(&rows[0]), nil
}

func (r *eventRepo) List(ctx context.Context, f biz.ListEventsFilter) ([]*auditeventpb.Event, error) {
	conds := make([]string, 0, 13)
	args := make([]any, 0, 16)

	if f.StartTime != nil {
		conds = append(conds, "occurred_at >= ?")
		args = append(args, *f.StartTime)
	}
	if f.EndTime != nil {
		conds = append(conds, "occurred_at <= ?")
		args = append(args, *f.EndTime)
	}
	if f.Category != "" {
		conds = append(conds, "category = ?")
		args = append(args, f.Category)
	}
	if f.Service != "" {
		conds = append(conds, "service = ?")
		args = append(args, f.Service)
	}
	if f.EventType != "" {
		conds = append(conds, "event_type = ?")
		args = append(args, f.EventType)
	}
	if f.Operation != "" {
		conds = append(conds, "operation = ?")
		args = append(args, f.Operation)
	}
	if f.ActorType != "" {
		conds = append(conds, "actor_type = ?")
		args = append(args, f.ActorType)
	}
	if f.ActorID != "" {
		conds = append(conds, "actor_id = ?")
		args = append(args, f.ActorID)
	}
	if f.ResourceType != "" {
		conds = append(conds, "resource_type = ?")
		args = append(args, f.ResourceType)
	}
	if f.ResourceName != "" {
		conds = append(conds, "resource_name = ?")
		args = append(args, f.ResourceName)
	}
	if f.Success != nil {
		conds = append(conds, "success = ?")
		args = append(args, *f.Success)
	}
	if f.TraceID != "" {
		conds = append(conds, "trace_id = ?")
		args = append(args, f.TraceID)
	}
	if f.Cursor != nil {
		conds = append(conds, "(occurred_at < ? OR (occurred_at = ? AND event_id < ?))")
		args = append(args, f.Cursor.OccurredAt, f.Cursor.OccurredAt, f.Cursor.EventID)
	}

	var whereClause string
	if len(conds) > 0 {
		whereClause = "WHERE " + strings.Join(conds, " AND ")
	}
	args = append(args, f.Limit)

	query := fmt.Sprintf(`SELECT %s
FROM %s FINAL
%s
ORDER BY occurred_at DESC, event_id DESC
LIMIT ?
SETTINGS do_not_merge_across_partitions_select_final = 1`,
		selectEventColumns,
		schema.TableEvents,
		whereClause,
	)

	var rows []schema.EventRow
	if err := r.data.ClickHouse().Select(ctx, &rows, query, args...); err != nil {
		if ctx.Err() != nil {
			return nil, err
		}
		return nil, errors.Join(biz.ErrStorageUnavailable, err)
	}

	out := make([]*auditeventpb.Event, len(rows))
	for i := range rows {
		out[i] = toProto(&rows[i])
	}
	return out, nil
}

func (r *eventRepo) ListServices(ctx context.Context) ([]string, error) {
	query := fmt.Sprintf(
		"SELECT DISTINCT service FROM %s WHERE service != '' ORDER BY service ASC",
		schema.TableEvents,
	)
	var rows []struct {
		Service string `ch:"service"`
	}
	if err := r.data.ClickHouse().Select(ctx, &rows, query); err != nil {
		if ctx.Err() != nil {
			return nil, err
		}
		return nil, errors.Join(biz.ErrStorageUnavailable, err)
	}
	services := make([]string, len(rows))
	for i := range rows {
		services[i] = rows[i].Service
	}
	return services, nil
}

func toProto(row *schema.EventRow) *auditeventpb.Event {
	name, err := auditeventpb.NewEventName(row.EventID).Format()
	if err != nil {
		name = "events/" + row.EventID
	}
	cat := securityauditpb.AuditCategory_AUDIT_CATEGORY_UNSPECIFIED
	switch row.Category {
	case audit.CategoryAuthn:
		cat = securityauditpb.AuditCategory_AUDIT_CATEGORY_AUTHN
	case audit.CategoryResource:
		cat = securityauditpb.AuditCategory_AUDIT_CATEGORY_RESOURCE
	case audit.CategoryAuthz:
		cat = securityauditpb.AuditCategory_AUDIT_CATEGORY_AUTHZ
	}
	return &auditeventpb.Event{
		Name:         name,
		EventId:      row.EventID,
		EventType:    row.EventType,
		Category:     cat,
		OccurredTime: timestamppb.New(row.OccurredAt.UTC()),
		Service:      row.Service,
		Operation:    row.Operation,
		ActorType:    row.ActorType,
		ActorId:      row.ActorID,
		ResourceType: row.ResourceType,
		ResourceName: row.ResourceName,
		Success:      row.Success,
		ErrorCode:    row.ErrorCode,
		ErrorMessage: row.ErrorMessage,
		TraceId:      row.TraceID,
		RequestId:    row.RequestID,
		ClientIp:     row.ClientIP,
		UserAgent:    row.UserAgent,
		DetailJson:   row.DetailJSON,
	}
}
