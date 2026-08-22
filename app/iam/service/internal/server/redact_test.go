package server

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"

	authnpb "github.com/Servora-Kit/plateau/api/gen/go/iam/authn/v1"

	"github.com/go-kratos/kratos/v3/middleware/logging"
)

func TestRequestLoggingUsesGeneratedRedact(t *testing.T) {
	t.Parallel()

	var output bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&output, nil))
	request := &authnpb.LoginRequest{
		Email:    "person@example.com",
		Password: "plain-password",
	}

	handler := logging.Server(logger)(func(context.Context, any) (any, error) {
		return &authnpb.LoginResponse{}, nil
	})
	if _, err := handler(t.Context(), request); err != nil {
		t.Fatalf("logging handler: %v", err)
	}

	logged := output.String()
	if strings.Contains(logged, "plain-password") {
		t.Fatalf("request log contains plaintext password: %s", logged)
	}
	if !strings.Contains(logged, "person@example.com") {
		t.Fatalf("request log omitted safe email field: %s", logged)
	}
	if request.Password != "plain-password" {
		t.Fatalf("logging mutated original request password: %q", request.Password)
	}
}
