package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	accountpb "github.com/Servora-Kit/plateau/api/gen/go/iam/account/v1"
	authnpb "github.com/Servora-Kit/plateau/api/gen/go/iam/authn/v1"
	oidcconf "github.com/Servora-Kit/plateau/api/gen/go/iam/oidc/conf/v1"
	sessionpb "github.com/Servora-Kit/plateau/api/gen/go/iam/session/v1"
	bootstrapconfig "github.com/Servora-Kit/servora/core/bootstrap/config"
	khttp "github.com/go-kratos/kratos/v3/transport/http"
)

const originTestPublicOrigin = "https://iam.example"

func TestCookieOriginProtectionRejectsEveryProtectedOperationBeforeHandler(t *testing.T) {
	t.Parallel()

	operations := []string{
		authnpb.OperationAuthnServiceLogin,
		accountpb.OperationAccountServiceUpdateProfile,
		accountpb.OperationAccountServiceChangePassword,
		sessionpb.OperationSessionServiceLogout,
	}
	for _, operation := range operations {
		operation := operation
		t.Run(operation, func(t *testing.T) {
			t.Parallel()
			status, calls := exerciseOriginProtection(t, operation, nil)
			if status != http.StatusForbidden {
				t.Fatalf("状态码为 %d，期望 %d", status, http.StatusForbidden)
			}
			if calls != 0 {
				t.Fatalf("来源拒绝后业务处理器被调用 %d 次", calls)
			}
		})
	}
}

func TestCookieOriginProtectionHeaderRules(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		headers    http.Header
		wantStatus int
		wantCalls  int
	}{
		{
			name:       "匹配的 Origin",
			headers:    http.Header{"Origin": {originTestPublicOrigin}},
			wantStatus: http.StatusOK,
			wantCalls:  1,
		},
		{
			name:       "Origin 缺失时匹配的 Referer",
			headers:    http.Header{"Referer": {originTestPublicOrigin + "/login/?request_id=opaque"}},
			wantStatus: http.StatusOK,
			wantCalls:  1,
		},
		{
			name: "Origin 优先且忽略 Referer",
			headers: http.Header{
				"Origin":  {originTestPublicOrigin},
				"Referer": {"畸形来源"},
			},
			wantStatus: http.StatusOK,
			wantCalls:  1,
		},
		{
			name: "冲突的 Origin 优先拒绝",
			headers: http.Header{
				"Origin":  {"https://attacker.example"},
				"Referer": {originTestPublicOrigin + "/account/"},
			},
			wantStatus: http.StatusForbidden,
		},
		{
			name:       "null Origin 拒绝",
			headers:    http.Header{"Origin": {"null"}},
			wantStatus: http.StatusForbidden,
		},
		{
			name:       "畸形 Origin 拒绝",
			headers:    http.Header{"Origin": {"https://%"}},
			wantStatus: http.StatusForbidden,
		},
		{
			name: "空 Origin 不回退 Referer",
			headers: http.Header{
				"Origin":  {""},
				"Referer": {originTestPublicOrigin + "/account/"},
			},
			wantStatus: http.StatusForbidden,
		},
		{
			name:       "Origin 必须精确匹配且不得带路径",
			headers:    http.Header{"Origin": {originTestPublicOrigin + "/"}},
			wantStatus: http.StatusForbidden,
		},
		{
			name:       "重复 Origin 拒绝",
			headers:    http.Header{"Origin": {originTestPublicOrigin, originTestPublicOrigin}},
			wantStatus: http.StatusForbidden,
		},
		{
			name:       "来源缺失拒绝",
			wantStatus: http.StatusForbidden,
		},
		{
			name:       "相对 Referer 拒绝",
			headers:    http.Header{"Referer": {"/account/"}},
			wantStatus: http.StatusForbidden,
		},
		{
			name: "不信任转发头",
			headers: http.Header{
				"X-Forwarded-Host":  {"iam.example"},
				"X-Forwarded-Proto": {"https"},
			},
			wantStatus: http.StatusForbidden,
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			status, calls := exerciseOriginProtection(t, authnpb.OperationAuthnServiceLogin, test.headers)
			if status != test.wantStatus {
				t.Fatalf("状态码为 %d，期望 %d", status, test.wantStatus)
			}
			if calls != test.wantCalls {
				t.Fatalf("业务处理器调用 %d 次，期望 %d 次", calls, test.wantCalls)
			}
		})
	}
}

func TestCookieOriginProtectionDoesNotCoverOIDCOperations(t *testing.T) {
	t.Parallel()

	status, calls := exerciseOriginProtection(t, "/oidc.protocol/Token", nil)
	if status != http.StatusOK || calls != 1 {
		t.Fatalf("OIDC 操作状态码为 %d，业务处理器调用 %d 次", status, calls)
	}
}

func exerciseOriginProtection(t *testing.T, operation string, headers http.Header) (int, int) {
	t.Helper()

	calls := 0
	server := khttp.NewServer(khttp.Middleware(cookieOriginProtection(originTestPublicOrigin)))
	server.Route("/").Handle(http.MethodPost, "/operation", func(ctx khttp.Context) error {
		khttp.SetOperation(ctx, operation)
		handler := ctx.Middleware(func(context.Context, any) (any, error) {
			calls++
			return struct{}{}, nil
		})
		if _, err := handler(ctx, struct{}{}); err != nil {
			return err
		}
		return ctx.String(http.StatusOK, "正常")
	})

	request := httptest.NewRequest(http.MethodPost, "/operation", nil)
	request.Header = headers.Clone()
	response := httptest.NewRecorder()
	server.ServeHTTP(response, request)
	return response.Code, calls
}

func TestLocalConfigAcceptsWebOriginAndUsesItForMail(t *testing.T) {
	for _, tt := range []struct {
		name, override, origin string
	}{
		{name: "默认开发入口", origin: "http://localhost:10002"},
		{name: "显式公开入口", override: "https://iam.example:8443", origin: "https://iam.example:8443"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("IAM_PUBLIC_ORIGIN", tt.override)
			if tt.override == "" {
				if err := os.Unsetenv("IAM_PUBLIC_ORIGIN"); err != nil {
					t.Fatal(err)
				}
			}
			bc, config, err := bootstrapconfig.LoadBootstrap("../../configs/local", "iam.service", false)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = config.Close() })
			var oidcSettings oidcconf.OIDC
			if err := config.Value("oidc").Scan(&oidcSettings); err != nil {
				t.Fatal(err)
			}
			calls := 0
			server := khttp.NewServer(khttp.Middleware(cookieOriginProtection(oidcSettings.GetIssuer())))
			server.Route("/").POST("/login", func(ctx khttp.Context) error {
				khttp.SetOperation(ctx, authnpb.OperationAuthnServiceLogin)
				_, err := ctx.Middleware(func(context.Context, any) (any, error) {
					calls++
					return nil, nil
				})(ctx, nil)
				if err != nil {
					return err
				}
				return ctx.String(http.StatusOK, "已允许登录处理")
			})
			request := httptest.NewRequest(http.MethodPost, "/login", nil)
			request.Header.Set("Origin", tt.origin)
			response := httptest.NewRecorder()
			server.ServeHTTP(response, request)
			if response.Code != http.StatusOK || calls != 1 {
				t.Fatalf("Web 来源被拒绝：status=%d calls=%d", response.Code, calls)
			}
			if got := bc.GetApp().GetExternalUrl(); got != tt.origin {
				t.Fatalf("邮件入口 %q 与浏览器入口 %q 不一致", got, tt.origin)
			}
		})
	}
}
