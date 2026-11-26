package biz

import (
	"crypto/sha256"
	"crypto/subtle"
	"fmt"
	"time"

	auditeventpb "github.com/Servora-Kit/plateau/api/gen/go/audit/event/v1"
	crudpb "github.com/Servora-Kit/plateau/api/gen/go/plateau/crud/v1"
	"github.com/Servora-Kit/plateau/crud"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// TODO(plateau-crud): 待 infra/clickhouse 提供分页与过滤适配器后，
// 将此处的强类型过滤指纹及游标编解码统一收归共享 CRUD 管线（对标 infra/entgo/crud）。

func encodeEventPageToken(fingerprint []byte, last *auditeventpb.Event) (string, error) {
	codec := crud.NewUnsignedPageTokenCodec()
	return codec.Encode(&crudpb.PageTokenPayload{
		Version:            crud.CurrentPageTokenVersion,
		ContextFingerprint: fingerprint,
		Cursor: []*crudpb.CursorValue{
			{Value: &crudpb.CursorValue_Int64Value{Int64Value: last.GetOccurredTime().AsTime().UTC().UnixMicro()}},
			{Value: &crudpb.CursorValue_StringValue{StringValue: last.GetEventId()}},
		},
	})
}

func decodeEventPageToken(token string, fingerprint []byte) (*EventCursor, error) {
	codec := crud.NewUnsignedPageTokenCodec()
	payload, err := codec.Decode(token)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalidPageToken, err)
	}
	if payload == nil ||
		payload.GetVersion() != crud.CurrentPageTokenVersion ||
		subtle.ConstantTimeCompare(payload.GetContextFingerprint(), fingerprint) != 1 ||
		len(payload.GetCursor()) != 2 {
		return nil, ErrInvalidPageToken
	}

	tsVal, ok := payload.GetCursor()[0].GetValue().(*crudpb.CursorValue_Int64Value)
	if !ok {
		return nil, ErrInvalidPageToken
	}
	idVal, ok := payload.GetCursor()[1].GetValue().(*crudpb.CursorValue_StringValue)
	if !ok || idVal.StringValue == "" {
		return nil, ErrInvalidPageToken
	}

	return &EventCursor{
		OccurredAt: time.UnixMicro(tsVal.Int64Value).UTC(),
		EventID:    idVal.StringValue,
	}, nil
}

func queryFingerprint(q ListEventsQuery) ([]byte, error) {
	canonical := &auditeventpb.ListEventsRequest{
		Category:     q.Category,
		Service:      q.Service,
		EventType:    q.EventType,
		Operation:    q.Operation,
		ActorType:    q.ActorType,
		ActorId:      q.ActorID,
		ResourceType: q.ResourceType,
		ResourceName: q.ResourceName,
		Success:      q.Success,
		TraceId:      q.TraceID,
	}
	if q.StartTime != nil {
		canonical.StartTime = timestamppb.New(*q.StartTime)
	}
	if q.EndTime != nil {
		canonical.EndTime = timestamppb.New(*q.EndTime)
	}

	raw, err := proto.MarshalOptions{Deterministic: true}.Marshal(canonical)
	if err != nil {
		return nil, fmt.Errorf("marshal query fingerprint: %w", err)
	}
	sum := sha256.Sum256(raw)
	return sum[:], nil
}
