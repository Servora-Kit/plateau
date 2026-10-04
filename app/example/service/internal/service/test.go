package service

import (
	"context"

	examplepb "github.com/Servora-Kit/plateau/api/gen/go/example/service/v1"
	testpb "github.com/Servora-Kit/plateau/api/gen/go/test/service/v1"
	"github.com/Servora-Kit/plateau/app/example/service/internal/biz"
)

type ExampleService struct {
	examplepb.UnimplementedExampleServiceServer
	uc *biz.ExampleUsecase
}

func NewExampleService(uc *biz.ExampleUsecase) *ExampleService {
	return &ExampleService{
		uc: uc,
	}
}

func (s *ExampleService) Hello(ctx context.Context, req *testpb.HelloRequest) (*testpb.HelloResponse, error) {
	return s.uc.Hello(ctx, req)
}
