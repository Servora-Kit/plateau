package data

import (
	"context"
	"fmt"

	auditeventpb "github.com/Servora-Kit/plateau/api/gen/go/plateau/audit/event/v1"
	"github.com/Servora-Kit/plateau/app/admin/service/internal/biz"
	gogrpc "google.golang.org/grpc"
)

const auditServiceName = "audit.service"

type auditEventRepo struct {
	conn   *gogrpc.ClientConn
	client auditeventpb.EventServiceClient
}

// NewAuditEventRepo dials audit.service via Servora's shared gRPC Dialer and returns biz.AuditEventRepo with cleanup.
func NewAuditEventRepo(data *Data) (biz.AuditEventRepo, func(), error) {
	if data == nil || data.grpcDialer == nil {
		return nil, nil, fmt.Errorf("admin audit event repo: dialer is nil")
	}
	conn, err := data.grpcDialer.Dial(context.Background(), auditServiceName)
	if err != nil {
		return nil, nil, fmt.Errorf("dial %s: %w", auditServiceName, err)
	}
	cleanup := func() { _ = conn.Close() }
	return &auditEventRepo{conn: conn, client: auditeventpb.NewEventServiceClient(conn)}, cleanup, nil
}

func (r *auditEventRepo) ListEvents(ctx context.Context, req *auditeventpb.ListEventsRequest) (*auditeventpb.ListEventsResponse, error) {
	recoverConnTransientFailure(ctx, r.conn)
	return r.client.ListEvents(ctx, req)
}

func (r *auditEventRepo) GetEvent(ctx context.Context, req *auditeventpb.GetEventRequest) (*auditeventpb.Event, error) {
	recoverConnTransientFailure(ctx, r.conn)
	return r.client.GetEvent(ctx, req)
}

func (r *auditEventRepo) ListServices(ctx context.Context, req *auditeventpb.ListServicesRequest) (*auditeventpb.ListServicesResponse, error) {
	recoverConnTransientFailure(ctx, r.conn)
	return r.client.ListServices(ctx, req)
}
