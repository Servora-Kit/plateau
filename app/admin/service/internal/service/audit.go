package service

import (
	"context"
	"errors"
	"fmt"
	"strings"

	adminauditpb "github.com/Servora-Kit/plateau/api/gen/go/plateau/admin/audit/v1"
	auditeventpb "github.com/Servora-Kit/plateau/api/gen/go/plateau/audit/event/v1"
	securityerrorspb "github.com/Servora-Kit/plateau/api/gen/go/plateau/security/errors/v1"
	"github.com/Servora-Kit/plateau/app/admin/service/internal/biz"
	kerrors "github.com/go-kratos/kratos/v3/errors"
)

// AuditService implements plateau.admin.audit.v1.AuditServiceHTTPServer.
type AuditService struct {
	adminauditpb.UnimplementedAuditServiceServer
	uc *biz.AuditUsecase
}

// NewAuditService constructs AuditService backed by AuditUsecase.
func NewAuditService(uc *biz.AuditUsecase) (*AuditService, error) {
	if uc == nil {
		return nil, fmt.Errorf("admin audit service: usecase is nil")
	}
	return &AuditService{uc: uc}, nil
}

func (s *AuditService) ListEvents(ctx context.Context, req *adminauditpb.ListEventsRequest) (*adminauditpb.ListEventsResponse, error) {
	var downstreamReq *auditeventpb.ListEventsRequest
	if req == nil {
		downstreamReq = &auditeventpb.ListEventsRequest{}
	} else {
		downstreamReq = &auditeventpb.ListEventsRequest{
			StartTime:    req.GetStartTime(),
			EndTime:      req.GetEndTime(),
			Category:     req.GetCategory(),
			Service:      req.GetService(),
			EventType:    req.GetEventType(),
			Operation:    req.GetOperation(),
			ActorType:    req.GetActorType(),
			ActorId:      req.GetActorId(),
			ResourceType: req.GetResourceType(),
			ResourceName: req.GetResourceName(),
			Success:      req.Success,
			TraceId:      req.GetTraceId(),
			PageSize:     req.GetPageSize(),
			PageToken:    req.GetPageToken(),
		}
	}
	resp, err := s.uc.ListEvents(ctx, downstreamReq)
	if err != nil {
		return nil, auditError(ctx, err)
	}
	return &adminauditpb.ListEventsResponse{
		Events:        resp.GetEvents(),
		NextPageToken: resp.GetNextPageToken(),
	}, nil
}

func (s *AuditService) GetEvent(ctx context.Context, req *adminauditpb.GetEventRequest) (*auditeventpb.Event, error) {
	name, err := auditeventpb.ParseEventName(req.GetName())
	if err != nil {
		return nil, auditError(ctx, err)
	}
	ev, err := s.uc.GetEvent(ctx, name)
	if err != nil {
		return nil, auditError(ctx, err)
	}
	return ev, nil
}

func (s *AuditService) ListServices(ctx context.Context, _ *adminauditpb.ListServicesRequest) (*adminauditpb.ListServicesResponse, error) {
	services, err := s.uc.ListServices(ctx)
	if err != nil {
		return nil, auditError(ctx, err)
	}
	return &adminauditpb.ListServicesResponse{Services: services}, nil
}

func auditError(ctx context.Context, err error) error {
	var ctxErr error
	if ctx != nil {
		ctxErr = ctx.Err()
	}
	se := kerrors.FromError(err)

	switch {
	case errors.Is(ctxErr, context.Canceled):
		return kerrors.ClientClosed("CLIENT_CLOSED", "client closed request").WithCause(err)
	case errors.Is(ctxErr, context.DeadlineExceeded):
		return kerrors.GatewayTimeout("GATEWAY_TIMEOUT", "request deadline exceeded").WithCause(err)
	case errors.Is(err, biz.ErrUnauthenticated):
		return securityerrorspb.ErrorSecurityErrorReasonUnauthenticated("authentication required").WithCause(err)
	case auditeventpb.IsEventErrorReasonNotFound(err):
		return adminauditpb.ErrorAdminAuditErrorReasonEventNotFound("audit event not found").WithCause(err)
	case auditeventpb.IsEventErrorReasonInvalidPageToken(err):
		return adminauditpb.ErrorAdminAuditErrorReasonInvalidPageToken("invalid page_token").WithCause(err)
	case errors.Is(err, biz.ErrInvalidAuditArgument),
		auditeventpb.IsEventErrorReasonInvalidArgument(err),
		se != nil && strings.HasPrefix(se.Reason, "CRUD_ERROR_REASON_"),
		se != nil && se.Code == 400:
		return adminauditpb.ErrorAdminAuditErrorReasonInvalidArgument("invalid audit query argument").WithCause(err)
	default:
		return adminauditpb.ErrorAdminAuditErrorReasonDownstreamUnavailable("downstream audit service is temporarily unavailable").WithCause(err)
	}
}
