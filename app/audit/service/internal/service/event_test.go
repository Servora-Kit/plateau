package service

import (
	"context"
	"errors"
	"testing"
	"time"

	auditeventpb "github.com/Servora-Kit/plateau/api/gen/go/plateau/audit/event/v1"
	securityauditpb "github.com/Servora-Kit/plateau/api/gen/go/plateau/security/audit/v1"
	"github.com/Servora-Kit/plateau/app/audit/service/internal/biz"
	"github.com/Servora-Kit/plateau/security/audit"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type stubRepo struct {
	getFn      func(string) (*auditeventpb.Event, error)
	listFn     func(biz.ListEventsFilter) ([]*auditeventpb.Event, error)
	servicesFn func() ([]string, error)
}

func (s *stubRepo) SaveBatch(context.Context, []*auditeventpb.Event) error { return nil }
func (s *stubRepo) GetByID(_ context.Context, id string) (*auditeventpb.Event, error) {
	return s.getFn(id)
}
func (s *stubRepo) List(_ context.Context, f biz.ListEventsFilter) ([]*auditeventpb.Event, error) {
	return s.listFn(f)
}
func (s *stubRepo) ListServices(context.Context) ([]string, error) {
	if s.servicesFn != nil {
		return s.servicesFn()
	}
	return []string{"//admin.service", "//iam.service"}, nil
}

func TestEventServiceValidationAndErrorMapping(t *testing.T) {
	ts := timestamppb.New(time.Date(2026, 11, 24, 9, 30, 0, 0, time.UTC))
	uc, err := biz.NewEventUsecase(&stubRepo{
		getFn: func(id string) (*auditeventpb.Event, error) {
			if id == "ev-1" {
				return &auditeventpb.Event{
					Name:         "events/ev-1",
					EventId:      "ev-1",
					EventType:    audit.DefaultRPCEventType,
					Category:     securityauditpb.AuditCategory_AUDIT_CATEGORY_AUTHZ,
					OccurredTime: ts,
					Service:      "iam.service",
					ActorType:    "service",
					ActorId:      "admin",
					Success:      false,
					ErrorCode:    "SECURITY_ERROR_REASON_PERMISSION_DENIED",
					DetailJson:   "{}",
				}, nil
			}
			if id == "boom" {
				return nil, biz.ErrStorageUnavailable
			}
			if id == "secret" {
				return nil, errors.New("clickhouse tcp 10.0.0.8:9000 internal fault")
			}
			return nil, biz.ErrEventNotFound
		},
		listFn: func(f biz.ListEventsFilter) ([]*auditeventpb.Event, error) {
			if f.Success == nil || *f.Success {
				t.Fatalf("expected Success=false filter, got %v", f.Success)
			}
			return []*auditeventpb.Event{{
				Name:         "events/ev-1",
				EventId:      "ev-1",
				Category:     securityauditpb.AuditCategory_AUDIT_CATEGORY_AUTHN,
				OccurredTime: ts,
				DetailJson:   "{}",
			}}, nil
		},
	}, nil)
	if err != nil {
		t.Fatalf("NewEventUsecase: %v", err)
	}
	svc, err := NewEventService(uc)
	if err != nil {
		t.Fatalf("NewEventService: %v", err)
	}

	for _, badName := range []string{"", "users/123", "events/", "events/a/b"} {
		if _, err := svc.GetEvent(context.Background(), &auditeventpb.GetEventRequest{Name: badName}); !auditeventpb.IsEventErrorReasonInvalidArgument(err) {
			t.Fatalf("GetEvent(%q) error = %v, want INVALID_ARGUMENT", badName, err)
		}
	}

	if _, err := svc.GetEvent(context.Background(), &auditeventpb.GetEventRequest{Name: "events/missing"}); !auditeventpb.IsEventErrorReasonNotFound(err) {
		t.Fatalf("GetEvent(missing) error = %v, want NOT_FOUND", err)
	}
	if _, err := svc.GetEvent(context.Background(), &auditeventpb.GetEventRequest{Name: "events/boom"}); !auditeventpb.IsEventErrorReasonStorageUnavailable(err) {
		t.Fatalf("GetEvent(boom) error = %v, want STORAGE_UNAVAILABLE", err)
	}
	unknownErrResp, unknownErr := svc.GetEvent(context.Background(), &auditeventpb.GetEventRequest{Name: "events/secret"})
	if unknownErrResp != nil || !auditeventpb.IsEventErrorReasonUnspecified(unknownErr) {
		t.Fatalf("GetEvent(secret) = (%v, %v), want UNSPECIFIED", unknownErrResp, unknownErr)
	}

	got, err := svc.GetEvent(context.Background(), &auditeventpb.GetEventRequest{Name: "events/ev-1"})
	if err != nil {
		t.Fatalf("GetEvent(ev-1) error = %v", err)
	}
	if got.GetName() != "events/ev-1" || got.GetCategory() != securityauditpb.AuditCategory_AUDIT_CATEGORY_AUTHZ || got.GetActorId() != "admin" {
		t.Fatalf("unexpected GetEvent proto: %+v", got)
	}

	wantSuccess := false
	listResp, err := svc.ListEvents(context.Background(), &auditeventpb.ListEventsRequest{
		Category: securityauditpb.AuditCategory_AUDIT_CATEGORY_AUTHN,
		Success:  &wantSuccess,
	})
	if err != nil || len(listResp.GetEvents()) != 1 {
		t.Fatalf("ListEvents() = (%+v, %v)", listResp, err)
	}

	if _, err := svc.ListEvents(context.Background(), &auditeventpb.ListEventsRequest{
		PageToken: "not-a-valid-token",
	}); !auditeventpb.IsEventErrorReasonInvalidPageToken(err) {
		t.Fatalf("ListEvents(bad token) error = %v, want INVALID_PAGE_TOKEN", err)
	}

	svcResp, err := svc.ListServices(context.Background(), &auditeventpb.ListServicesRequest{})
	if err != nil || len(svcResp.GetServices()) != 2 || svcResp.GetServices()[0] != "//admin.service" {
		t.Fatalf("ListServices() = (%+v, %v)", svcResp, err)
	}
}
