package biz

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	auditeventpb "github.com/Servora-Kit/plateau/api/gen/go/audit/event/v1"
	securityauditpb "github.com/Servora-Kit/plateau/api/gen/go/plateau/security/audit/v1"
	"github.com/Servora-Kit/plateau/security/audit"
	cloudevents "github.com/cloudevents/sdk-go/v2"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type fakeEventRepo struct {
	saved      []*auditeventpb.Event
	saveErr    error
	byID       map[string]*auditeventpb.Event
	listFn     func(ListEventsFilter) ([]*auditeventpb.Event, error)
	saveCalled int
}

func (f *fakeEventRepo) SaveBatch(_ context.Context, events []*auditeventpb.Event) error {
	f.saveCalled++
	if f.saveErr != nil {
		return f.saveErr
	}
	f.saved = append(f.saved, events...)
	return nil
}

func (f *fakeEventRepo) GetByID(_ context.Context, eventID string) (*auditeventpb.Event, error) {
	ev, ok := f.byID[eventID]
	if !ok {
		return nil, ErrEventNotFound
	}
	return ev, nil
}

func (f *fakeEventRepo) List(_ context.Context, filter ListEventsFilter) ([]*auditeventpb.Event, error) {
	if f.listFn != nil {
		return f.listFn(filter)
	}
	return nil, nil
}

func TestIngestEventsNormalizationAndPoisonIsolation(t *testing.T) {
	repo := &fakeEventRepo{}
	uc, err := NewEventUsecase(repo, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatalf("NewEventUsecase() error = %v", err)
	}

	now := time.Date(2026, 11, 24, 10, 0, 0, 123456789, time.UTC)

	evAdmin := cloudevents.NewEvent()
	evAdmin.SetID("ev-admin-1")
	evAdmin.SetType(audit.DefaultRPCEventType)
	evAdmin.SetSource("admin.service")
	evAdmin.SetSubject("/admin.user.v1.UserService/DisableUser")
	evAdmin.SetTime(now)
	evAdmin.SetExtension(audit.ExtCategory, "resource")
	evAdmin.SetExtension(audit.ExtActorType, "human")
	evAdmin.SetExtension(audit.ExtActorID, "user-1")
	evAdmin.SetExtension(audit.ExtResourceType, "iam.plateau.dev/User")
	evAdmin.SetExtension(audit.ExtResourceName, "users/user-2")
	evAdmin.SetExtension(audit.ExtOutcome, audit.OutcomeSuccess)
	evAdmin.SetExtension(audit.ExtTraceParent, "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01")
	if err := evAdmin.SetData(cloudevents.ApplicationJSON, []byte(` { "name" : "users/user-2" } `)); err != nil {
		t.Fatalf("SetData: %v", err)
	}

	evIAM := cloudevents.NewEvent()
	evIAM.SetID("ev-iam-1")
	evIAM.SetType(audit.DefaultRPCEventType)
	evIAM.SetSource("iam.service")
	evIAM.SetSubject("/iam.user.v1.UserService/DisableUser")
	evIAM.SetTime(now.Add(5 * time.Millisecond))
	evIAM.SetExtension(audit.ExtCategory, "custom_unknown")
	evIAM.SetExtension(audit.ExtActorType, "service")
	evIAM.SetExtension(audit.ExtActorID, "admin")
	evIAM.SetExtension(audit.ExtOutcome, audit.OutcomeFailure)
	evIAM.SetExtension(audit.ExtErrorCode, "USER_ERROR_REASON_NOT_FOUND")
	evIAM.SetExtension(audit.ExtTraceParent, "00-4bf92f3577b34da6a3ce929d0e0e4736-b7ad6b7169203331-00")
	if err := evIAM.SetData(cloudevents.ApplicationJSON, []byte(`["a","b"]`)); err != nil {
		t.Fatalf("SetData: %v", err)
	}

	evBin := cloudevents.NewEvent()
	evBin.SetID("ev-bin-1")
	evBin.SetType("custom.binary.v1")
	evBin.SetSource("iam.service")
	evBin.SetTime(now.Add(10 * time.Millisecond))
	evBin.SetDataSchema("plateau://schema/v1")
	evBin.SetExtension("traceid", "ABCDEF0123456789ABCDEF0123456789")
	if err := evBin.SetData("application/octet-stream", []byte{0xde, 0xad, 0xbe, 0xef}); err != nil {
		t.Fatalf("SetData: %v", err)
	}

	evBad := cloudevents.NewEvent()
	evBad.SetID("ev-bad")
	evBad.SetType("plateau.audit.rpc.v1")
	evBad.SetTime(now)

	if err := uc.IngestEvents(context.Background(), []cloudevents.Event{evBad, evAdmin, evIAM, evBin}); err != nil {
		t.Fatalf("IngestEvents() error = %v", err)
	}

	if len(repo.saved) != 3 {
		t.Fatalf("saved count = %d, want 3", len(repo.saved))
	}

	gotAdmin, gotIAM, gotBin := repo.saved[0], repo.saved[1], repo.saved[2]
	if gotAdmin.GetTraceId() != "4bf92f3577b34da6a3ce929d0e0e4736" || gotIAM.GetTraceId() != gotAdmin.GetTraceId() {
		t.Fatalf("trace_id mismatch across hops: admin=%q iam=%q", gotAdmin.GetTraceId(), gotIAM.GetTraceId())
	}
	if gotAdmin.GetOccurredTime().AsTime().Nanosecond()%1000 != 0 {
		t.Fatalf("OccurredTime not truncated to microsecond: %v", gotAdmin.GetOccurredTime().AsTime())
	}
	if gotAdmin.GetName() != "events/ev-admin-1" ||
		gotAdmin.GetCategory() != securityauditpb.AuditCategory_AUDIT_CATEGORY_RESOURCE ||
		!gotAdmin.GetSuccess() ||
		gotAdmin.GetDetailJson() != `{"name":"users/user-2"}` {
		t.Fatalf("unexpected admin projection: %+v", gotAdmin)
	}
	if gotIAM.GetCategory() != securityauditpb.AuditCategory_AUDIT_CATEGORY_UNSPECIFIED ||
		gotIAM.GetSuccess() ||
		gotIAM.GetDetailJson() != `{"value":["a","b"]}` {
		t.Fatalf("unexpected iam projection: %+v", gotIAM)
	}
	if gotBin.GetTraceId() != "abcdef0123456789abcdef0123456789" {
		t.Fatalf("unexpected fallback trace_id: %q", gotBin.GetTraceId())
	}

	for _, detail := range []string{gotAdmin.GetDetailJson(), gotIAM.GetDetailJson(), gotBin.GetDetailJson()} {
		var obj map[string]any
		if err := json.Unmarshal([]byte(detail), &obj); err != nil {
			t.Fatalf("detail_json %q is not a JSON object: %v", detail, err)
		}
	}

	repo.saveCalled = 0
	if err := uc.IngestEvents(context.Background(), []cloudevents.Event{evBad}); err != nil {
		t.Fatalf("IngestEvents(all invalid) error = %v", err)
	}
	if repo.saveCalled != 0 {
		t.Fatalf("SaveBatch called %d times on all-invalid batch, want 0", repo.saveCalled)
	}
}

func TestListEventsPaginationAndFilterFingerprintBinding(t *testing.T) {
	t1 := time.Date(2026, 11, 24, 12, 0, 2, 0, time.UTC)
	t2 := time.Date(2026, 11, 24, 12, 0, 1, 0, time.UTC)
	t3 := time.Date(2026, 11, 24, 12, 0, 0, 0, time.UTC)
	all := []*auditeventpb.Event{
		{EventId: "ev-3", OccurredTime: timestamppb.New(t1), Service: "iam.service", Category: securityauditpb.AuditCategory_AUDIT_CATEGORY_AUTHN, Success: true},
		{EventId: "ev-2", OccurredTime: timestamppb.New(t2), Service: "iam.service", Category: securityauditpb.AuditCategory_AUDIT_CATEGORY_AUTHN, Success: true},
		{EventId: "ev-1", OccurredTime: timestamppb.New(t3), Service: "iam.service", Category: securityauditpb.AuditCategory_AUDIT_CATEGORY_AUTHN, Success: true},
	}

	repo := &fakeEventRepo{
		listFn: func(f ListEventsFilter) ([]*auditeventpb.Event, error) {
			var out []*auditeventpb.Event
			for _, ev := range all {
				if f.Cursor != nil {
					evTime := ev.GetOccurredTime().AsTime()
					beforeCursor := evTime.Before(f.Cursor.OccurredAt) ||
						(evTime.Equal(f.Cursor.OccurredAt) && ev.GetEventId() < f.Cursor.EventID)
					if !beforeCursor {
						continue
					}
				}
				out = append(out, ev)
				if len(out) == f.Limit {
					break
				}
			}
			return out, nil
		},
	}
	uc, err := NewEventUsecase(repo, nil)
	if err != nil {
		t.Fatalf("NewEventUsecase() error = %v", err)
	}

	baseQuery := ListEventsQuery{
		Category: securityauditpb.AuditCategory_AUDIT_CATEGORY_AUTHN,
		Service:  "iam.service",
		PageSize: 2,
	}
	page1, nextToken, err := uc.ListEvents(context.Background(), baseQuery)
	if err != nil {
		t.Fatalf("ListEvents(page1) error = %v", err)
	}
	if len(page1) != 2 || page1[0].GetEventId() != "ev-3" || page1[1].GetEventId() != "ev-2" || nextToken == "" {
		t.Fatalf("unexpected page1: events=%+v token=%q", page1, nextToken)
	}

	fBool := false
	start := t3
	mutatedQueries := []ListEventsQuery{
		{StartTime: &start, Category: securityauditpb.AuditCategory_AUDIT_CATEGORY_AUTHN, Service: "iam.service", PageToken: nextToken},
		{Category: securityauditpb.AuditCategory_AUDIT_CATEGORY_RESOURCE, Service: "iam.service", PageToken: nextToken},
		{Category: securityauditpb.AuditCategory_AUDIT_CATEGORY_AUTHN, Service: "admin.service", PageToken: nextToken},
		{Category: securityauditpb.AuditCategory_AUDIT_CATEGORY_AUTHN, Service: "iam.service", ActorID: "u1", PageToken: nextToken},
		{Category: securityauditpb.AuditCategory_AUDIT_CATEGORY_AUTHN, Service: "iam.service", Success: &fBool, PageToken: nextToken},
		{Category: securityauditpb.AuditCategory_AUDIT_CATEGORY_AUTHN, Service: "iam.service", TraceID: "4bf92f3577b34da6a3ce929d0e0e4736", PageToken: nextToken},
	}
	for i, mq := range mutatedQueries {
		if _, _, err := uc.ListEvents(context.Background(), mq); !errors.Is(err, ErrInvalidPageToken) {
			t.Fatalf("mutated query[%d] error = %v, want ErrInvalidPageToken", i, err)
		}
	}

	page2Query := baseQuery
	page2Query.PageSize = 5
	page2Query.PageToken = nextToken
	page2, nextToken2, err := uc.ListEvents(context.Background(), page2Query)
	if err != nil {
		t.Fatalf("ListEvents(page2) error = %v", err)
	}
	if len(page2) != 1 || page2[0].GetEventId() != "ev-1" || nextToken2 != "" {
		t.Fatalf("unexpected page2: events=%+v token=%q", page2, nextToken2)
	}

	if _, _, err := uc.ListEvents(context.Background(), ListEventsQuery{
		StartTime: &t1,
		EndTime:   &t3,
	}); !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("inverted time range error = %v, want ErrInvalidArgument", err)
	}
}
