package server

import (
	"bytes"
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	authnpb "github.com/Servora-Kit/plateau/api/gen/go/iam/authn/v1"

	"github.com/go-kratos/kratos/v3/middleware/logging"
	khttp "github.com/go-kratos/kratos/v3/transport/http"
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

func TestRequestLoggingDoesNotExposeInteractionQuery(t *testing.T) {
	t.Parallel()

	const interactionID = "opaque-interaction-reference"
	var output bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&output, nil))
	seenID := ""
	server := khttp.NewServer(khttp.Middleware(logging.Server(logger)))
	server.Route("/").Handle(http.MethodGet, "/authorize/callback", func(ctx khttp.Context) error {
		khttp.SetOperation(ctx, "/iam.oidc/AuthorizeCallback")
		handler := ctx.Middleware(func(context.Context, any) (any, error) {
			request, ok := khttp.RequestFromServerContext(ctx)
			if !ok {
				t.Fatal("处理器无法读取原始 HTTP 请求")
			}
			seenID = request.URL.Query().Get("id")
			return struct{}{}, nil
		})
		if _, err := handler(ctx, struct{}{}); err != nil {
			return err
		}
		return ctx.String(http.StatusOK, "正常")
	})

	request := httptest.NewRequest(http.MethodGet, "/authorize/callback?id="+interactionID, nil)
	response := httptest.NewRecorder()
	server.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("状态码为 %d，期望 %d", response.Code, http.StatusOK)
	}
	if seenID != interactionID {
		t.Fatalf("业务路由读取的交互引用为 %q", seenID)
	}
	if strings.Contains(output.String(), interactionID) {
		t.Fatalf("请求日志泄露了交互引用：%s", output.String())
	}
}
