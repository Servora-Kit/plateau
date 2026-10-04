package data

import (
	"context"
	"fmt"
	"log/slog"

	testpb "github.com/Servora-Kit/plateau/api/gen/go/test/service/v1"
	"github.com/Servora-Kit/plateau/app/example/service/internal/biz"
)

const testServiceName = "test.service"

// exampleRepe implements biz.TestRepo through the Test gRPC service.
type exampleRepe struct {
	client testpb.TestServiceClient
	log    *slog.Logger
}

func NewExampleRepo(data *Data, l *slog.Logger) (biz.ExampleRepo, func(), error) {
	conn, err := data.grpcDialer.Dial(context.Background(), testServiceName)
	if err != nil {
		return nil, nil, fmt.Errorf("create test grpc conn: %w", err)
	}

	repo := &exampleRepe{
		client: testpb.NewTestServiceClient(conn),
		log:    l.With("scope", "data/example"),
	}
	cleanup := func() { _ = conn.Close() }

	return repo, cleanup, nil
}

func (r *exampleRepe) Hello(
	ctx context.Context,
	req *testpb.HelloRequest,
) (*testpb.HelloResponse, error) {
	resp, err := r.client.Hello(ctx, req)
	if err != nil {
		r.log.Error("test hello failed", "err", err)
		return nil, fmt.Errorf("call test hello: %w", err)
	}
	return resp, nil
}
