package service

import (
	"context"
	"fmt"

	testpb "github.com/Servora-Kit/plateau/api/gen/go/test/service/v1"
)

type TestService struct {
	testpb.UnimplementedTestServiceServer
}

func NewTestService() *TestService {
	return &TestService{}
}

func (s *TestService) Hello(ctx context.Context, req *testpb.HelloRequest) (*testpb.HelloResponse, error) {
	reply := fmt.Sprintf("test says hello, %s", req.GetGreeting())

	return &testpb.HelloResponse{Reply: reply}, nil
}
