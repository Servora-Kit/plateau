package biz

import (
	"context"
	"errors"
	"testing"
	"time"

	auditeventpb "github.com/Servora-Kit/plateau/api/gen/go/audit/event/v1"
	"github.com/Servora-Kit/plateau/security"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type stubAuditEventRepo struct {
	lastListReq  *auditeventpb.ListEventsRequest
	lastGetReq   *auditeventpb.GetEventRequest
	listResp     *auditeventpb.ListEventsResponse
	getResp      *auditeventpb.Event
	servicesResp *auditeventpb.ListServicesResponse
	err          error
}

func (s *stubAuditEventRepo) ListEvents(_ context.Context, req *auditeventpb.ListEventsRequest) (*auditeventpb.ListEventsResponse, error) {
	s.lastListReq = req
	if s.err != nil {
		return nil, s.err
	}
	return s.listResp, nil
}

func (s *stubAuditEventRepo) GetEvent(_ context.Context, req *auditeventpb.GetEventRequest) (*auditeventpb.Event, error) {
	s.lastGetReq = req
	if s.err != nil {
		return nil, s.err
	}
	return s.getResp, nil
}

func (s *stubAuditEventRepo) ListServices(_ context.Context, _ *auditeventpb.ListServicesRequest) (*auditeventpb.ListServicesResponse, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.servicesResp, nil
}

func TestAuditUsecaseValidationAndPassThrough(t *testing.T) {
	if _, err := NewAuditUsecase(nil); err == nil {
		t.Fatal("NewAuditUsecase(nil) succeeded, want error")
	}

	repo := &stubAuditEventRepo{
		listResp: &auditeventpb.ListEventsResponse{
			Events:        []*auditeventpb.Event{{Name: "events/ev-1", EventId: "ev-1"}},
			NextPageToken: "next-tok",
		},
		getResp: &auditeventpb.Event{Name: "events/ev-1", EventId: "ev-1"},
		servicesResp: &auditeventpb.ListServicesResponse{
			Services: []string{"//admin.service", "//cms.service", "//iam.service"},
		},
	}
	uc, err := NewAuditUsecase(repo)
	if err != nil {
		t.Fatal(err)
	}

	// Reject unauthenticated or service actor contexts.
	if _, err := uc.ListEvents(context.Background(), nil); !errors.Is(err, ErrUnauthenticated) {
		t.Fatalf("ListEvents(unauth) err = %v, want ErrUnauthenticated", err)
	}
	svcCtx := security.WithActor(context.Background(), security.Actor{Type: security.ActorTypeService, ID: "admin"})
	if _, err := uc.GetEvent(svcCtx, auditeventpb.NewEventName("ev-1")); !errors.Is(err, ErrUnauthenticated) {
		t.Fatalf("GetEvent(service actor) err = %v, want ErrUnauthenticated", err)
	}
	if _, err := uc.ListServices(svcCtx); !errors.Is(err, ErrUnauthenticated) {
		t.Fatalf("ListServices(service actor) err = %v, want ErrUnauthenticated", err)
	}

	adminCtx := security.WithActor(context.Background(), security.Actor{Type: security.ActorTypeHuman, ID: "admin-1"})

	// Reject negative page_size and inverted time range.
	if _, err := uc.ListEvents(adminCtx, &auditeventpb.ListEventsRequest{PageSize: -5}); !errors.Is(err, ErrInvalidAuditArgument) {
		t.Fatalf("ListEvents(negative page_size) err = %v, want ErrInvalidAuditArgument", err)
	}
	now := time.Now().UTC()
	if _, err := uc.ListEvents(adminCtx, &auditeventpb.ListEventsRequest{
		StartTime: timestamppb.New(now),
		EndTime:   timestamppb.New(now.Add(-time.Minute)),
	}); !errors.Is(err, ErrInvalidAuditArgument) {
		t.Fatalf("ListEvents(start > end) err = %v, want ErrInvalidAuditArgument", err)
	}

	// Reject invalid EventName.
	if _, err := uc.GetEvent(adminCtx, auditeventpb.NewEventName("bad/id")); !errors.Is(err, ErrInvalidAuditArgument) {
		t.Fatalf("GetEvent(bad/id) err = %v, want ErrInvalidAuditArgument", err)
	}

	// Pass through valid ListEvents without overriding zero page_size (delegated to audit.service).
	successFilter := false
	resp, err := uc.ListEvents(adminCtx, &auditeventpb.ListEventsRequest{
		Service: "iam.service",
		Success: &successFilter,
	})
	if err != nil {
		t.Fatalf("ListEvents(valid) err = %v", err)
	}
	if len(resp.GetEvents()) != 1 || resp.GetNextPageToken() != "next-tok" {
		t.Fatalf("unexpected ListEvents response: %+v", resp)
	}
	if repo.lastListReq.GetPageSize() != 0 || repo.lastListReq.Success == nil || *repo.lastListReq.Success {
		t.Fatalf("unexpected forwarded ListEventsRequest: %+v", repo.lastListReq)
	}

	ev, err := uc.GetEvent(adminCtx, auditeventpb.NewEventName("ev-1"))
	if err != nil || ev.GetName() != "events/ev-1" || repo.lastGetReq.GetName() != "events/ev-1" {
		t.Fatalf("GetEvent(ev-1) ev=%+v req=%+v err=%v", ev, repo.lastGetReq, err)
	}

	services, err := uc.ListServices(adminCtx)
	if err != nil || len(services) != 3 || services[1] != "//cms.service" {
		t.Fatalf("ListServices(admin) = (%v, %v)", services, err)
	}
}
