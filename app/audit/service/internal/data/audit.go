package data

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"time"

	auditsvcpb "github.com/Servora-Kit/servora-platform/api/gen/go/audit/service/v1"
	"github.com/Servora-Kit/servora-platform/app/audit/service/internal/biz"
	logger "github.com/Servora-Kit/servora/obs/logging"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// auditRepo provides read access to ClickHouse audit_events.
type auditRepo struct {
	data *Data
	log  *logger.Helper
}

// NewAuditRepo creates a new AuditRepo. Returns the biz.AuditRepo interface
// so Wire resolves the dependency without wire.Bind — matching IAM's pattern.
func NewAuditRepo(d *Data, l logger.Logger) biz.AuditRepo {
	return &auditRepo{
		data: d,
		log:  logger.For(l, "audit/data"),
	}
}

// pageToken encodes the cursor for pagination.
type pageToken struct {
	OccurredAt time.Time `json:"occurred_at"`
	EventID    string    `json:"event_id"`
}

func encodePageToken(t time.Time, eventID string) string {
	raw, _ := json.Marshal(pageToken{OccurredAt: t, EventID: eventID})
	return base64.StdEncoding.EncodeToString(raw)
}

func decodePageToken(token string) (*pageToken, error) {
	raw, err := base64.StdEncoding.DecodeString(token)
	if err != nil {
		return nil, err
	}
	var pt pageToken
	if err := json.Unmarshal(raw, &pt); err != nil {
		return nil, err
	}
	return &pt, nil
}

// ListEvents queries audit events with filters and cursor-based pagination.
func (r *auditRepo) ListEvents(ctx context.Context, req *auditsvcpb.ListAuditEventsRequest) ([]*auditsvcpb.AuditEventItem, string, error) {
	if r.data.ClickHouse() == nil {
		return nil, "", nil
	}

	pageSize := int(req.PageSize)
	if pageSize <= 0 {
		pageSize = 50
	}
	if pageSize > 200 {
		pageSize = 200
	}

	var startTime, endTime time.Time
	if req.StartTime != nil {
		startTime = req.StartTime.AsTime()
	}
	if req.EndTime != nil {
		endTime = req.EndTime.AsTime()
	}
	where, args, err := buildWhere(startTime, endTime, req.EventTypes, req.ActorId, req.Service, req.PageToken)
	if err != nil {
		return nil, "", err
	}

	query := fmt.Sprintf(`
SELECT event_id, event_type, event_version, occurred_at,
       service, operation,
       actor_id, actor_type, actor_display_name,
       target_type, target_id, target_name,
       success, error_code, error_message,
       trace_id, request_id, detail
FROM audit_events
%s
ORDER BY occurred_at ASC, event_id ASC
LIMIT %d
`, where, pageSize+1)

	rows, err := r.data.ClickHouse().Query(ctx, query, args...)
	if err != nil {
		return nil, "", fmt.Errorf("query audit events: %w", err)
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			r.log.WithContext(ctx).Warnf("failed to close rows: %v", closeErr)
		}
	}()

	var items []*auditsvcpb.AuditEventItem
	for rows.Next() {
		var (
			eventID, eventType, eventVersion string
			occurredAt                       time.Time
			svc, operation                   string
			actorID, actorType, actorDisplay string
			targetType, targetID, targetName string
			success                          bool
			errorCode, errorMessage          string
			traceID, requestID, detail       string
		)
		if err := rows.Scan(
			&eventID, &eventType, &eventVersion, &occurredAt,
			&svc, &operation,
			&actorID, &actorType, &actorDisplay,
			&targetType, &targetID, &targetName,
			&success, &errorCode, &errorMessage,
			&traceID, &requestID, &detail,
		); err != nil {
			r.log.WithContext(ctx).Warnf("failed to scan row: %v", err)
			continue
		}
		items = append(items, &auditsvcpb.AuditEventItem{
			EventId:          eventID,
			EventType:        eventType,
			EventVersion:     eventVersion,
			OccurredAt:       timestamppb.New(occurredAt),
			Service:          svc,
			Operation:        operation,
			ActorId:          actorID,
			ActorType:        actorType,
			ActorDisplayName: actorDisplay,
			TargetType:       targetType,
			TargetId:         targetID,
			TargetName:       targetName,
			Success:          success,
			ErrorCode:        errorCode,
			ErrorMessage:     errorMessage,
			TraceId:          traceID,
			RequestId:        requestID,
			Detail:           detail,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, "", fmt.Errorf("iterate audit events: %w", err)
	}

	var nextToken string
	if len(items) > pageSize {
		last := items[pageSize-1]
		nextToken = encodePageToken(last.OccurredAt.AsTime(), last.EventId)
		items = items[:pageSize]
	}

	return items, nextToken, nil
}

// CountEvents returns the count of audit events matching the given filters.
func (r *auditRepo) CountEvents(ctx context.Context, req *auditsvcpb.CountAuditEventsRequest) (int64, error) {
	if r.data.ClickHouse() == nil {
		return 0, nil
	}

	var startTime, endTime time.Time
	if req.StartTime != nil {
		startTime = req.StartTime.AsTime()
	}
	if req.EndTime != nil {
		endTime = req.EndTime.AsTime()
	}
	where, args, err := buildWhere(startTime, endTime, req.EventTypes, req.ActorId, req.Service, "")
	if err != nil {
		return 0, err
	}

	query := fmt.Sprintf("SELECT count() FROM audit_events %s", where)

	var count uint64
	if err := r.data.ClickHouse().QueryRow(ctx, query, args...).Scan(&count); err != nil {
		return 0, fmt.Errorf("count audit events: %w", err)
	}
	return int64(count), nil
}

// buildWhere constructs the WHERE clause and argument list from filter parameters.
// Returns an error if pageToken is non-empty but invalid.
func buildWhere(startTime, endTime time.Time, eventTypes []string, actorID, service, pageToken string) (string, []interface{}, error) {
	var conditions []string
	var args []interface{}

	if !startTime.IsZero() {
		conditions = append(conditions, "occurred_at >= ?")
		args = append(args, startTime)
	}
	if !endTime.IsZero() {
		conditions = append(conditions, "occurred_at < ?")
		args = append(args, endTime)
	}
	if len(eventTypes) > 0 {
		conditions = append(conditions, "event_type IN (?)")
		args = append(args, eventTypes)
	}
	if actorID != "" {
		conditions = append(conditions, "actor_id = ?")
		args = append(args, actorID)
	}
	if service != "" {
		conditions = append(conditions, "service = ?")
		args = append(args, service)
	}
	if pageToken != "" {
		pt, err := decodePageToken(pageToken)
		if err != nil {
			return "", nil, fmt.Errorf("invalid page_token: %w", err)
		}
		conditions = append(conditions, "(occurred_at, event_id) > (?, ?)")
		args = append(args, pt.OccurredAt, pt.EventID)
	}

	if len(conditions) == 0 {
		return "", args, nil
	}

	where := "WHERE "
	for i, c := range conditions {
		if i > 0 {
			where += " AND "
		}
		where += c
	}
	return where, args, nil
}
