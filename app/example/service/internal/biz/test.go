package biz

import (
	"context"
	"fmt"

	"log/slog"

	testpb "github.com/Servora-Kit/plateau/api/gen/go/test/service/v1"
)

type ExampleRepo interface {
	Hello(ctx context.Context, req *testpb.HelloRequest) (*testpb.HelloResponse, error)
}

type ExampleUsecase struct {
	worker ExampleRepo
	log    *slog.Logger
}

func NewExampleUsecase(worker ExampleRepo, l *slog.Logger) *ExampleUsecase {
	return &ExampleUsecase{
		worker: worker,
		log:    l.With("scope", "biz/test"),
	}
}

func (uc *ExampleUsecase) Hello(ctx context.Context, req *testpb.HelloRequest) (*testpb.HelloResponse, error) {
	resp, err := uc.worker.Hello(ctx, req)
	if err != nil {
		uc.log.Error("relay worker hello failed", "err", err)
		return nil, fmt.Errorf("relay worker hello: %w", err)
	}

	return &testpb.HelloResponse{Reply: "master relay -> " + resp.GetReply()}, nil
}
