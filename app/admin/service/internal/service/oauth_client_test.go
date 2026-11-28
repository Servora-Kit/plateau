package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	adminclientpb "github.com/Servora-Kit/plateau/api/gen/go/plateau/admin/client/v1"
	crudpb "github.com/Servora-Kit/plateau/api/gen/go/plateau/crud/v1"
	iamclientpb "github.com/Servora-Kit/plateau/api/gen/go/plateau/iam/client/v1"
	userpb "github.com/Servora-Kit/plateau/api/gen/go/plateau/iam/user/v1"
	securityerrorspb "github.com/Servora-Kit/plateau/api/gen/go/plateau/security/errors/v1"
	"github.com/Servora-Kit/plateau/app/admin/service/internal/biz"
	"github.com/Servora-Kit/plateau/security"
	"github.com/Servora-Kit/plateau/security/audit"
	authnsession "github.com/Servora-Kit/plateau/security/authn/session"
	"github.com/Servora-Kit/plateau/security/authz/openfga"
	httpsession "github.com/Servora-Kit/plateau/security/session"
	_ "github.com/Servora-Kit/servora/transport/server/http"
	"github.com/alexedwards/scs/v2"
	"github.com/alexedwards/scs/v2/memstore"
	kerrors "github.com/go-kratos/kratos/v3/errors"
	khttp "github.com/go-kratos/kratos/v3/transport/http"
	fgaclient "github.com/openfga/go-sdk/client"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type fakeIAMOAuthClientRepo struct {
	registerFn  func(context.Context, *iamclientpb.RegisterOAuthClientRequest) (*iamclientpb.RegisterOAuthClientResponse, error)
	getFn       func(context.Context, *iamclientpb.GetOAuthClientRequest) (*iamclientpb.OAuthClient, error)
	listFn      func(context.Context, *iamclientpb.ListOAuthClientsRequest) (*iamclientpb.ListOAuthClientsResponse, error)
	updateFn    func(context.Context, *iamclientpb.UpdateOAuthClientRequest) (*iamclientpb.OAuthClient, error)
	disableFn   func(context.Context, *iamclientpb.DisableOAuthClientRequest) (*iamclientpb.OAuthClient, error)
	enableFn    func(context.Context, *iamclientpb.EnableOAuthClientRequest) (*iamclientpb.OAuthClient, error)
	deleteFn    func(context.Context, *iamclientpb.DeleteOAuthClientRequest) error
	createSecFn func(context.Context, *iamclientpb.CreateOAuthClientSecretRequest) (*iamclientpb.CreateOAuthClientSecretResponse, error)
	deleteSecFn func(context.Context, *iamclientpb.DeleteOAuthClientSecretRequest) error
}

func (f *fakeIAMOAuthClientRepo) RegisterOAuthClient(ctx context.Context, req *iamclientpb.RegisterOAuthClientRequest) (*iamclientpb.RegisterOAuthClientResponse, error) {
	if f.registerFn != nil {
		return f.registerFn(ctx, req)
	}
	return &iamclientpb.RegisterOAuthClientResponse{}, nil
}

func (f *fakeIAMOAuthClientRepo) GetOAuthClient(ctx context.Context, req *iamclientpb.GetOAuthClientRequest) (*iamclientpb.OAuthClient, error) {
	if f.getFn != nil {
		return f.getFn(ctx, req)
	}
	return &iamclientpb.OAuthClient{}, nil
}

func (f *fakeIAMOAuthClientRepo) ListOAuthClients(ctx context.Context, req *iamclientpb.ListOAuthClientsRequest) (*iamclientpb.ListOAuthClientsResponse, error) {
	if f.listFn != nil {
		return f.listFn(ctx, req)
	}
	return &iamclientpb.ListOAuthClientsResponse{}, nil
}

func (f *fakeIAMOAuthClientRepo) UpdateOAuthClient(ctx context.Context, req *iamclientpb.UpdateOAuthClientRequest) (*iamclientpb.OAuthClient, error) {
	if f.updateFn != nil {
		return f.updateFn(ctx, req)
	}
	return &iamclientpb.OAuthClient{}, nil
}

func (f *fakeIAMOAuthClientRepo) DisableOAuthClient(ctx context.Context, req *iamclientpb.DisableOAuthClientRequest) (*iamclientpb.OAuthClient, error) {
	if f.disableFn != nil {
		return f.disableFn(ctx, req)
	}
	return &iamclientpb.OAuthClient{}, nil
}

func (f *fakeIAMOAuthClientRepo) EnableOAuthClient(ctx context.Context, req *iamclientpb.EnableOAuthClientRequest) (*iamclientpb.OAuthClient, error) {
	if f.enableFn != nil {
		return f.enableFn(ctx, req)
	}
	return &iamclientpb.OAuthClient{}, nil
}

func (f *fakeIAMOAuthClientRepo) DeleteOAuthClient(ctx context.Context, req *iamclientpb.DeleteOAuthClientRequest) error {
	if f.deleteFn != nil {
		return f.deleteFn(ctx, req)
	}
	return nil
}

func (f *fakeIAMOAuthClientRepo) CreateOAuthClientSecret(ctx context.Context, req *iamclientpb.CreateOAuthClientSecretRequest) (*iamclientpb.CreateOAuthClientSecretResponse, error) {
	if f.createSecFn != nil {
		return f.createSecFn(ctx, req)
	}
	return &iamclientpb.CreateOAuthClientSecretResponse{}, nil
}

func (f *fakeIAMOAuthClientRepo) DeleteOAuthClientSecret(ctx context.Context, req *iamclientpb.DeleteOAuthClientSecretRequest) error {
	if f.deleteSecFn != nil {
		return f.deleteSecFn(ctx, req)
	}
	return nil
}

func TestOAuthClientServiceValidationAndErrorMapping(t *testing.T) {
	if _, err := NewOAuthClientService(nil); err == nil {
		t.Fatal("NewOAuthClientService(nil) succeeded, want error")
	}

	repo := &fakeIAMOAuthClientRepo{}
	uc, err := biz.NewOAuthClientUsecase(repo)
	if err != nil {
		t.Fatal(err)
	}
	svc, err := NewOAuthClientService(uc)
	if err != nil {
		t.Fatal(err)
	}

	// Unauthenticated context -> 401 SECURITY_ERROR_REASON_UNAUTHENTICATED.
	if _, err := svc.ListOAuthClients(context.Background(), &adminclientpb.ListOAuthClientsRequest{}); !securityerrorspb.IsSecurityErrorReasonUnauthenticated(err) {
		t.Fatalf("ListOAuthClients(unauth) err = %v, want 401 UNAUTHENTICATED", err)
	}

	adminCtx := security.WithActor(context.Background(), security.Actor{Type: security.ActorTypeHuman, ID: "admin-1"})

	// Request validation checks before downstream RPC.
	for _, badName := range []string{"", "users/123", "oauthClients/", "oauthClients/a/b"} {
		if _, err := svc.GetOAuthClient(adminCtx, &adminclientpb.GetOAuthClientRequest{Name: badName}); !crudpb.IsCrudErrorReasonInvalidResourceName(err) {
			t.Fatalf("GetOAuthClient(%q) err = %v, want CRUD_ERROR_REASON_INVALID_RESOURCE_NAME", badName, err)
		}
		if _, err := svc.UpdateOAuthClient(adminCtx, &adminclientpb.UpdateOAuthClientRequest{
			OauthClient: &iamclientpb.OAuthClient{Name: badName},
		}); !crudpb.IsCrudErrorReasonInvalidResourceName(err) {
			t.Fatalf("UpdateOAuthClient(%q) err = %v, want CRUD_ERROR_REASON_INVALID_RESOURCE_NAME", badName, err)
		}
		if _, err := svc.DisableOAuthClient(adminCtx, &adminclientpb.DisableOAuthClientRequest{Name: badName}); !crudpb.IsCrudErrorReasonInvalidResourceName(err) {
			t.Fatalf("DisableOAuthClient(%q) err = %v, want CRUD_ERROR_REASON_INVALID_RESOURCE_NAME", badName, err)
		}
		if _, err := svc.EnableOAuthClient(adminCtx, &adminclientpb.EnableOAuthClientRequest{Name: badName}); !crudpb.IsCrudErrorReasonInvalidResourceName(err) {
			t.Fatalf("EnableOAuthClient(%q) err = %v, want CRUD_ERROR_REASON_INVALID_RESOURCE_NAME", badName, err)
		}
		if _, err := svc.DeleteOAuthClient(adminCtx, &adminclientpb.DeleteOAuthClientRequest{Name: badName}); !crudpb.IsCrudErrorReasonInvalidResourceName(err) {
			t.Fatalf("DeleteOAuthClient(%q) err = %v, want CRUD_ERROR_REASON_INVALID_RESOURCE_NAME", badName, err)
		}
		if _, err := svc.CreateOAuthClientSecret(adminCtx, &adminclientpb.CreateOAuthClientSecretRequest{Name: badName}); !crudpb.IsCrudErrorReasonInvalidResourceName(err) {
			t.Fatalf("CreateOAuthClientSecret(%q) err = %v, want CRUD_ERROR_REASON_INVALID_RESOURCE_NAME", badName, err)
		}
		if _, err := svc.DeleteOAuthClientSecret(adminCtx, &adminclientpb.DeleteOAuthClientSecretRequest{Name: badName, SecretId: "s1"}); !crudpb.IsCrudErrorReasonInvalidResourceName(err) {
			t.Fatalf("DeleteOAuthClientSecret(%q) err = %v, want CRUD_ERROR_REASON_INVALID_RESOURCE_NAME", badName, err)
		}
	}

	if _, err := svc.CreateOAuthClient(adminCtx, &adminclientpb.CreateOAuthClientRequest{}); kerrors.Code(err) != 400 {
		t.Fatalf("CreateOAuthClient(nil oauth_client) err = %v, want 400", err)
	}
	if _, err := svc.UpdateOAuthClient(adminCtx, &adminclientpb.UpdateOAuthClientRequest{}); kerrors.Code(err) != 400 {
		t.Fatalf("UpdateOAuthClient(nil oauth_client) err = %v, want 400", err)
	}
	if _, err := svc.DeleteOAuthClientSecret(adminCtx, &adminclientpb.DeleteOAuthClientSecretRequest{
		Name:     "oauthClients/app-1",
		SecretId: "   ",
	}); kerrors.Code(err) != 400 {
		t.Fatalf("DeleteOAuthClientSecret(blank secret_id) err = %v, want 400", err)
	}
	if _, err := svc.CreateOAuthClientSecret(adminCtx, &adminclientpb.CreateOAuthClientSecretRequest{
		Name:        "oauthClients/app-1",
		ExpiresTime: &timestamppb.Timestamp{Nanos: -1},
	}); !adminclientpb.IsAdminClientErrorReasonInvalidConfig(err) {
		t.Fatalf("CreateOAuthClientSecret(invalid timestamp) err = %v, want 400 INVALID_CONFIG", err)
	}

	// Complete downstream ClientErrorReason, CRUD_ERROR_REASON_*, and 503 isolation matrix.
	cases := []struct {
		name       string
		err        error
		wantCode   int32
		wantReason string
	}{
		{
			name:       "NOT_FOUND -> 404 ADMIN_CLIENT_ERROR_REASON_NOT_FOUND",
			err:        iamclientpb.ErrorClientErrorReasonNotFound("missing"),
			wantCode:   404,
			wantReason: adminclientpb.AdminClientErrorReason_ADMIN_CLIENT_ERROR_REASON_NOT_FOUND.String(),
		},
		{
			name:       "ALREADY_EXISTS -> 409 ADMIN_CLIENT_ERROR_REASON_ALREADY_EXISTS",
			err:        iamclientpb.ErrorClientErrorReasonAlreadyExists("duplicate client_id"),
			wantCode:   409,
			wantReason: adminclientpb.AdminClientErrorReason_ADMIN_CLIENT_ERROR_REASON_ALREADY_EXISTS.String(),
		},
		{
			name:       "STATIC_CLIENT_IMMUTABLE -> 403 ADMIN_CLIENT_ERROR_REASON_STATIC_CLIENT_IMMUTABLE",
			err:        iamclientpb.ErrorClientErrorReasonStaticClientImmutable("static"),
			wantCode:   403,
			wantReason: adminclientpb.AdminClientErrorReason_ADMIN_CLIENT_ERROR_REASON_STATIC_CLIENT_IMMUTABLE.String(),
		},
		{
			name:       "INVALID_CONFIG -> 400 ADMIN_CLIENT_ERROR_REASON_INVALID_CONFIG",
			err:        iamclientpb.ErrorClientErrorReasonInvalidConfig("redirect_uris required"),
			wantCode:   400,
			wantReason: adminclientpb.AdminClientErrorReason_ADMIN_CLIENT_ERROR_REASON_INVALID_CONFIG.String(),
		},
		{
			name:       "INVALID_STATE -> 409 ADMIN_CLIENT_ERROR_REASON_INVALID_STATE",
			err:        iamclientpb.ErrorClientErrorReasonInvalidState("public client has no secrets"),
			wantCode:   409,
			wantReason: adminclientpb.AdminClientErrorReason_ADMIN_CLIENT_ERROR_REASON_INVALID_STATE.String(),
		},
		{
			name:       "ETAG_MISMATCH -> 409 ADMIN_CLIENT_ERROR_REASON_ETAG_MISMATCH",
			err:        iamclientpb.ErrorClientErrorReasonEtagMismatch("stale etag"),
			wantCode:   409,
			wantReason: adminclientpb.AdminClientErrorReason_ADMIN_CLIENT_ERROR_REASON_ETAG_MISMATCH.String(),
		},
		{
			name:       "SECRET_LIMIT_EXCEEDED -> 409 ADMIN_CLIENT_ERROR_REASON_SECRET_LIMIT_EXCEEDED",
			err:        iamclientpb.ErrorClientErrorReasonSecretLimitExceeded("max 5 secrets"),
			wantCode:   409,
			wantReason: adminclientpb.AdminClientErrorReason_ADMIN_CLIENT_ERROR_REASON_SECRET_LIMIT_EXCEEDED.String(),
		},
		{
			name:       "LAST_ACTIVE_SECRET -> 409 ADMIN_CLIENT_ERROR_REASON_LAST_ACTIVE_SECRET",
			err:        iamclientpb.ErrorClientErrorReasonLastActiveSecret("last secret"),
			wantCode:   409,
			wantReason: adminclientpb.AdminClientErrorReason_ADMIN_CLIENT_ERROR_REASON_LAST_ACTIVE_SECRET.String(),
		},
		{
			name:       "CRUD_ERROR_REASON_INVALID_FILTER -> 400 pass-through reason",
			err:        crudpb.ErrorCrudErrorReasonInvalidFilter("unsupported filter"),
			wantCode:   400,
			wantReason: crudpb.CrudErrorReason_CRUD_ERROR_REASON_INVALID_FILTER.String(),
		},
		{
			name:       "CRUD_ERROR_REASON_INVALID_FIELD_MASK -> 400 pass-through reason",
			err:        crudpb.ErrorCrudErrorReasonInvalidFieldMask("immutable field application_type"),
			wantCode:   400,
			wantReason: crudpb.CrudErrorReason_CRUD_ERROR_REASON_INVALID_FIELD_MASK.String(),
		},
		{
			name:       "CRUD_ERROR_REASON_INVALID_PAGE_TOKEN -> 400 pass-through reason",
			err:        crudpb.ErrorCrudErrorReasonInvalidPageToken("bad cursor"),
			wantCode:   400,
			wantReason: crudpb.CrudErrorReason_CRUD_ERROR_REASON_INVALID_PAGE_TOKEN.String(),
		},
		{
			name:       "CRUD_ERROR_REASON_INTERNAL (500) -> 503 DOWNSTREAM_UNAVAILABLE",
			err:        crudpb.ErrorCrudErrorReasonInternal("internal crud invariant"),
			wantCode:   503,
			wantReason: adminclientpb.AdminClientErrorReason_ADMIN_CLIENT_ERROR_REASON_DOWNSTREAM_UNAVAILABLE.String(),
		},
		{
			name:       "gRPC InvalidArgument -> 400 INVALID_ARGUMENT",
			err:        status.Error(codes.InvalidArgument, "malformed field"),
			wantCode:   400,
			wantReason: "INVALID_ARGUMENT",
		},
		{
			name:       "gRPC Unavailable -> 503 DOWNSTREAM_UNAVAILABLE",
			err:        status.Error(codes.Unavailable, "iam unreachable"),
			wantCode:   503,
			wantReason: adminclientpb.AdminClientErrorReason_ADMIN_CLIENT_ERROR_REASON_DOWNSTREAM_UNAVAILABLE.String(),
		},
		{
			name:       "gRPC DeadlineExceeded with live caller ctx -> 503 DOWNSTREAM_UNAVAILABLE",
			err:        status.Error(codes.DeadlineExceeded, "downstream rpc timeout"),
			wantCode:   503,
			wantReason: adminclientpb.AdminClientErrorReason_ADMIN_CLIENT_ERROR_REASON_DOWNSTREAM_UNAVAILABLE.String(),
		},
		{
			name:       "context.DeadlineExceeded from downstream with live caller ctx -> 503 DOWNSTREAM_UNAVAILABLE",
			err:        context.DeadlineExceeded,
			wantCode:   503,
			wantReason: adminclientpb.AdminClientErrorReason_ADMIN_CLIENT_ERROR_REASON_DOWNSTREAM_UNAVAILABLE.String(),
		},
		{
			name:       "downstream service JWT 401 -> 503 DOWNSTREAM_UNAVAILABLE",
			err:        securityerrorspb.ErrorSecurityErrorReasonUnauthenticated("bad m2m token"),
			wantCode:   503,
			wantReason: adminclientpb.AdminClientErrorReason_ADMIN_CLIENT_ERROR_REASON_DOWNSTREAM_UNAVAILABLE.String(),
		},
		{
			name:       "downstream OpenFGA 403 -> 503 DOWNSTREAM_UNAVAILABLE",
			err:        securityerrorspb.ErrorSecurityErrorReasonPermissionDenied("missing service:admin manage_clients"),
			wantCode:   503,
			wantReason: adminclientpb.AdminClientErrorReason_ADMIN_CLIENT_ERROR_REASON_DOWNSTREAM_UNAVAILABLE.String(),
		},
		{
			name:       "raw internal error -> 503 DOWNSTREAM_UNAVAILABLE without leaking message",
			err:        errors.New("sensitive sql connection string leaked"),
			wantCode:   503,
			wantReason: adminclientpb.AdminClientErrorReason_ADMIN_CLIENT_ERROR_REASON_DOWNSTREAM_UNAVAILABLE.String(),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo.listFn = func(context.Context, *iamclientpb.ListOAuthClientsRequest) (*iamclientpb.ListOAuthClientsResponse, error) {
				return nil, tc.err
			}
			_, gotErr := svc.ListOAuthClients(adminCtx, &adminclientpb.ListOAuthClientsRequest{})
			se := kerrors.FromError(gotErr)
			if se == nil || se.Code != tc.wantCode || se.Reason != tc.wantReason {
				t.Fatalf("ListOAuthClients(%s) = %+v, want (%d, %q)", tc.name, se, tc.wantCode, tc.wantReason)
			}
			if strings.Contains(se.Message, "sensitive sql connection string leaked") {
				t.Fatalf("raw internal error leaked into client message: %q", se.Message)
			}
		})
	}

	// Caller's own canceled or deadline-exceeded context -> 499 / 504.
	canceledCtx, cancel := context.WithCancel(adminCtx)
	cancel()
	repo.listFn = func(ctx context.Context, _ *iamclientpb.ListOAuthClientsRequest) (*iamclientpb.ListOAuthClientsResponse, error) {
		return nil, ctx.Err()
	}
	if _, err := svc.ListOAuthClients(canceledCtx, &adminclientpb.ListOAuthClientsRequest{}); kerrors.Code(err) != 499 {
		t.Fatalf("caller canceled code = %d, want 499", kerrors.Code(err))
	}

	expiredCtx, cancelDeadline := context.WithDeadline(adminCtx, time.Now().Add(-time.Second))
	defer cancelDeadline()
	if _, err := svc.ListOAuthClients(expiredCtx, &adminclientpb.ListOAuthClientsRequest{}); kerrors.Code(err) != 504 {
		t.Fatalf("caller deadline exceeded code = %d, want 504", kerrors.Code(err))
	}
}

func TestOAuthClientServiceHTTPMiddlewareAuditAndRedaction(t *testing.T) {
	const (
		initialPlaintextSecret = "{{SECRET_0011initialclientsecret}}"
		rotatedPlaintextSecret = "{{SECRET_0012rotatedclientsecret}}"
		adminOrigin            = "http://localhost:10012"
	)

	// Verify protoc-gen-redact / audit.RedactClone clears both one-time secret fields without mutating originals.
	origCreateResp := &adminclientpb.CreateOAuthClientResponse{
		Client:       &iamclientpb.OAuthClient{Name: "oauthClients/app-1", ClientId: "app-1"},
		ClientSecret: initialPlaintextSecret,
	}
	redactedCreateResp := audit.RedactClone(origCreateResp).(*adminclientpb.CreateOAuthClientResponse)
	if redactedCreateResp.GetClientSecret() != "" || origCreateResp.GetClientSecret() != initialPlaintextSecret {
		t.Fatalf("CreateOAuthClientResponse RedactClone broken: redacted=%q orig=%q", redactedCreateResp.GetClientSecret(), origCreateResp.GetClientSecret())
	}

	origSecResp := &adminclientpb.CreateOAuthClientSecretResponse{
		Secret:      &iamclientpb.OAuthClientSecret{SecretId: "sec-2", SecretPrefix: "pcs_abcd1234"},
		SecretValue: rotatedPlaintextSecret,
	}
	redactedSecResp := audit.RedactClone(origSecResp).(*adminclientpb.CreateOAuthClientSecretResponse)
	if redactedSecResp.GetSecretValue() != "" || origSecResp.GetSecretValue() != rotatedPlaintextSecret {
		t.Fatalf("CreateOAuthClientSecretResponse RedactClone broken: redacted=%q orig=%q", redactedSecResp.GetSecretValue(), origSecResp.GetSecretValue())
	}

	// Build Kratos HTTP server with real AuthN -> Audit -> OpenFGA AuthZ rules for OAuthClientService.
	fgaSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload struct {
			TupleKey struct {
				User     string `json:"user"`
				Relation string `json:"relation"`
				Object   string `json:"object"`
			} `json:"tuple_key"`
		}
		_ = json.NewDecoder(r.Body).Decode(&payload)
		w.Header().Set("Content-Type", "application/json")
		allowed := payload.TupleKey.User == "user:admin-1" &&
			payload.TupleKey.Relation == "manage_clients" &&
			payload.TupleKey.Object == "admin:global"
		if allowed {
			_, _ = w.Write([]byte(`{"allowed":true}`))
			return
		}
		_, _ = w.Write([]byte(`{"allowed":false}`))
	}))
	t.Cleanup(fgaSrv.Close)

	fgaSDK, err := fgaclient.NewSdkClient(&fgaclient.ClientConfiguration{
		ApiUrl:               fgaSrv.URL,
		StoreId:              "01H00000000000000000000000",
		AuthorizationModelId: "01H00000000000000000000001",
	})
	if err != nil {
		t.Fatal(err)
	}
	authorizer, err := openfga.NewDefault(fgaSDK)
	if err != nil {
		t.Fatal(err)
	}

	var logBuf bytes.Buffer
	prevDefaultLogger := slog.Default()
	logger := slog.New(slog.NewJSONHandler(&logBuf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	slog.SetDefault(logger)
	t.Cleanup(func() { slog.SetDefault(prevDefaultLogger) })

	repo := &fakeIAMOAuthClientRepo{
		listFn: func(_ context.Context, _ *iamclientpb.ListOAuthClientsRequest) (*iamclientpb.ListOAuthClientsResponse, error) {
			total := int64(1)
			return &iamclientpb.ListOAuthClientsResponse{
				Clients:   []*iamclientpb.OAuthClient{{Name: "oauthClients/app-1", ClientId: "app-1"}},
				TotalSize: &total,
			}, nil
		},
		getFn: func(_ context.Context, req *iamclientpb.GetOAuthClientRequest) (*iamclientpb.OAuthClient, error) {
			return &iamclientpb.OAuthClient{Name: req.GetName(), ClientId: "app-1"}, nil
		},
		registerFn: func(_ context.Context, req *iamclientpb.RegisterOAuthClientRequest) (*iamclientpb.RegisterOAuthClientResponse, error) {
			if req.GetOauthClientId() == "dup-app" {
				return nil, iamclientpb.ErrorClientErrorReasonAlreadyExists("already exists")
			}
			id := req.GetOauthClientId()
			if id == "" {
				id = "app-1"
			}
			return &iamclientpb.RegisterOAuthClientResponse{
				Client:       &iamclientpb.OAuthClient{Name: "oauthClients/" + id, ClientId: id, Etag: "etag-1"},
				ClientSecret: initialPlaintextSecret,
			}, nil
		},
		updateFn: func(_ context.Context, req *iamclientpb.UpdateOAuthClientRequest) (*iamclientpb.OAuthClient, error) {
			return &iamclientpb.OAuthClient{
				Name:        req.GetOauthClient().GetName(),
				ClientId:    "app-1",
				DisplayName: proto.String(req.GetOauthClient().GetDisplayName()),
				Etag:        "etag-2",
			}, nil
		},
		disableFn: func(_ context.Context, req *iamclientpb.DisableOAuthClientRequest) (*iamclientpb.OAuthClient, error) {
			return &iamclientpb.OAuthClient{
				Name:   req.GetName(),
				Status: iamclientpb.ClientStatus_CLIENT_STATUS_DISABLED,
				Etag:   "etag-3",
			}, nil
		},
		enableFn: func(_ context.Context, req *iamclientpb.EnableOAuthClientRequest) (*iamclientpb.OAuthClient, error) {
			return &iamclientpb.OAuthClient{
				Name:   req.GetName(),
				Status: iamclientpb.ClientStatus_CLIENT_STATUS_ACTIVE,
				Etag:   "etag-4",
			}, nil
		},
		createSecFn: func(_ context.Context, req *iamclientpb.CreateOAuthClientSecretRequest) (*iamclientpb.CreateOAuthClientSecretResponse, error) {
			return &iamclientpb.CreateOAuthClientSecretResponse{
				Secret:      &iamclientpb.OAuthClientSecret{SecretId: "sec-2", Name: req.GetSecretName(), SecretPrefix: "pcs_rot_pref"},
				SecretValue: rotatedPlaintextSecret,
			}, nil
		},
	}

	uc, err := biz.NewOAuthClientUsecase(repo)
	if err != nil {
		t.Fatal(err)
	}
	svc, err := NewOAuthClientService(uc)
	if err != nil {
		t.Fatal(err)
	}

	mgr := scs.New()
	mgr.Store = memstore.NewWithCleanupInterval(0)
	userAuthn, err := authnsession.NewUserAuthenticator(mgr)
	if err != nil {
		t.Fatal(err)
	}
	recAuditor := &authCaptureAuditor{}
	browserFilter, err := httpsession.NewFilter(mgr, httpsession.FilterConfig{
		PublicOrigin: adminOrigin,
		JSONPrefixes: []string{"/v1/admin/"},
	})
	if err != nil {
		t.Fatal(err)
	}

	srv := khttp.NewServer(
		khttp.Filter(browserFilter),
		khttp.Middleware(
			authnsession.Server(userAuthn, security.WithRulesFuncs(adminclientpb.AuthnRules)),
			audit.Server(recAuditor, security.WithRulesFuncs(adminclientpb.AuditRules)),
			openfga.Server(authorizer, security.WithRulesFuncs(adminclientpb.AuthzRules)),
		),
	)
	adminclientpb.RegisterOAuthClientServiceHTTPServer(srv, svc)

	issueSessionCookie := func(userID, email string) *http.Cookie {
		w := httptest.NewRecorder()
		httpsession.LoadAndSave(mgr)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if saveErr := authnsession.SaveUser(r.Context(), mgr, &userpb.User{
				Name:   "users/" + userID,
				UserId: userID,
				Email:  &email,
			}); saveErr != nil {
				t.Errorf("SaveUser(%s): %v", userID, saveErr)
			}
			w.WriteHeader(http.StatusNoContent)
		})).ServeHTTP(w, httptest.NewRequest(http.MethodGet, adminOrigin+"/", nil))
		for _, ck := range w.Result().Cookies() {
			if ck.Name == mgr.Cookie.Name {
				return ck
			}
		}
		t.Fatalf("missing session cookie for %s", userID)
		return nil
	}

	execHTTP := func(method, path, body string, ck *http.Cookie) *httptest.ResponseRecorder {
		var reader *strings.Reader
		if body != "" {
			reader = strings.NewReader(body)
		} else {
			reader = strings.NewReader("")
		}
		req := httptest.NewRequest(method, adminOrigin+path, reader)
		if method != http.MethodGet && method != http.MethodHead {
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Origin", adminOrigin)
		}
		if ck != nil {
			req.AddCookie(ck)
		}
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, req)
		return w
	}

	// 1. Unauthenticated request -> 401.
	if w := execHTTP(http.MethodGet, "/v1/admin/oauth-clients", "", nil); w.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated status = %d, want 401", w.Code)
	}

	// 2. Non-authorized user -> 403 + authz denied audit event even on AUDIT_MODE_DISABLED endpoint.
	viewerCookie := issueSessionCookie("viewer-1", "viewer@plateau.local")
	if w := execHTTP(http.MethodGet, "/v1/admin/oauth-clients", "", viewerCookie); w.Code != http.StatusForbidden {
		t.Fatalf("unauthorized viewer status = %d, want 403", w.Code)
	}
	if len(recAuditor.events) != 1 ||
		recAuditor.events[0].Type() != audit.AuthzDeniedEventType ||
		recAuditor.events[0].Extensions()[audit.ExtActorID] != "viewer-1" {
		t.Fatalf("expected 1 authz denied event for viewer-1, got %+v", recAuditor.events)
	}

	// 3. Authorized admin read-only endpoints -> 200 OK & zero audit events.
	recAuditor.events = nil
	adminCookie := issueSessionCookie("admin-1", "admin@plateau.local")
	if w := execHTTP(http.MethodGet, "/v1/admin/oauth-clients?pageSize=20&includeTotal=true", "", adminCookie); w.Code != http.StatusOK {
		t.Fatalf("ListOAuthClients status = %d body = %s", w.Code, w.Body.String())
	}
	if w := execHTTP(http.MethodGet, "/v1/admin/oauthClients/app-1", "", adminCookie); w.Code != http.StatusOK {
		t.Fatalf("GetOAuthClient status = %d body = %s", w.Code, w.Body.String())
	}
	if len(recAuditor.events) != 0 {
		t.Fatalf("read-only endpoints emitted %d audit events, want 0", len(recAuditor.events))
	}

	// 4. Execute all 7 mutating operations + 1 failed CreateOAuthClient and verify audit enrichment & zero secret leakage.
	createW := execHTTP(http.MethodPost, "/v1/admin/oauth-clients", `{
		"oauthClientId": "app-1",
		"initialSecretName": "初始密钥",
		"oauthClient": {
			"displayName": "App One",
			"applicationType": "APPLICATION_TYPE_WEB",
			"tokenEndpointAuthMethod": "TOKEN_ENDPOINT_AUTH_METHOD_CLIENT_SECRET_BASIC",
			"redirectUris": ["https://app1.example.com/cb"],
			"allowedGrantTypes": ["authorization_code"]
		}
	}`, adminCookie)
	if createW.Code != http.StatusOK {
		t.Fatalf("CreateOAuthClient status = %d body = %s", createW.Code, createW.Body.String())
	}
	gotCreate := new(adminclientpb.CreateOAuthClientResponse)
	if err := protojson.Unmarshal(createW.Body.Bytes(), gotCreate); err != nil {
		t.Fatal(err)
	}
	if gotCreate.GetClientSecret() != initialPlaintextSecret {
		t.Fatalf("CreateOAuthClient clientSecret = %q, want %q", gotCreate.GetClientSecret(), initialPlaintextSecret)
	}

	if w := execHTTP(http.MethodPatch, "/v1/admin/oauthClients/app-1?updateMask=display_name", `{"displayName":"App One Renamed","etag":"etag-1"}`, adminCookie); w.Code != http.StatusOK {
		t.Fatalf("UpdateOAuthClient status = %d body = %s", w.Code, w.Body.String())
	}
	if w := execHTTP(http.MethodPost, "/v1/admin/oauthClients/app-1:disable", `{"etag":"etag-2"}`, adminCookie); w.Code != http.StatusOK {
		t.Fatalf("DisableOAuthClient status = %d body = %s", w.Code, w.Body.String())
	}
	if w := execHTTP(http.MethodPost, "/v1/admin/oauthClients/app-1:enable", `{"etag":"etag-3"}`, adminCookie); w.Code != http.StatusOK {
		t.Fatalf("EnableOAuthClient status = %d body = %s", w.Code, w.Body.String())
	}

	createSecW := execHTTP(http.MethodPost, "/v1/admin/oauthClients/app-1/secrets", `{"secretName":"轮换密钥"}`, adminCookie)
	if createSecW.Code != http.StatusOK {
		t.Fatalf("CreateOAuthClientSecret status = %d body = %s", createSecW.Code, createSecW.Body.String())
	}
	gotSec := new(adminclientpb.CreateOAuthClientSecretResponse)
	if err := protojson.Unmarshal(createSecW.Body.Bytes(), gotSec); err != nil {
		t.Fatal(err)
	}
	if gotSec.GetSecretValue() != rotatedPlaintextSecret {
		t.Fatalf("CreateOAuthClientSecret secretValue = %q, want %q", gotSec.GetSecretValue(), rotatedPlaintextSecret)
	}

	if w := execHTTP(http.MethodDelete, "/v1/admin/oauthClients/app-1/secrets/sec-1", "", adminCookie); w.Code != http.StatusOK {
		t.Fatalf("DeleteOAuthClientSecret status = %d body = %s", w.Code, w.Body.String())
	}
	if w := execHTTP(http.MethodDelete, "/v1/admin/oauthClients/app-1?etag=etag-4", "", adminCookie); w.Code != http.StatusOK {
		t.Fatalf("DeleteOAuthClient status = %d body = %s", w.Code, w.Body.String())
	}

	// Failed CreateOAuthClient with duplicate slug -> 409 and audit records candidate resource name.
	if w := execHTTP(http.MethodPost, "/v1/admin/oauth-clients", `{
		"oauthClientId": "dup-app",
		"oauthClient": {"displayName": "Dup"}
	}`, adminCookie); w.Code != http.StatusConflict {
		t.Fatalf("CreateOAuthClient(dup) status = %d, want 409", w.Code)
	}

	wantOps := []struct {
		subject string
		resName string
		outcome string
		errCode string
	}{
		{adminclientpb.OperationOAuthClientServiceCreateOAuthClient, "oauthClients/app-1", audit.OutcomeSuccess, ""},
		{adminclientpb.OperationOAuthClientServiceUpdateOAuthClient, "oauthClients/app-1", audit.OutcomeSuccess, ""},
		{adminclientpb.OperationOAuthClientServiceDisableOAuthClient, "oauthClients/app-1", audit.OutcomeSuccess, ""},
		{adminclientpb.OperationOAuthClientServiceEnableOAuthClient, "oauthClients/app-1", audit.OutcomeSuccess, ""},
		{adminclientpb.OperationOAuthClientServiceCreateOAuthClientSecret, "oauthClients/app-1", audit.OutcomeSuccess, ""},
		{adminclientpb.OperationOAuthClientServiceDeleteOAuthClientSecret, "oauthClients/app-1", audit.OutcomeSuccess, ""},
		{adminclientpb.OperationOAuthClientServiceDeleteOAuthClient, "oauthClients/app-1", audit.OutcomeSuccess, ""},
		{adminclientpb.OperationOAuthClientServiceCreateOAuthClient, "oauthClients/dup-app", audit.OutcomeFailure, "ADMIN_CLIENT_ERROR_REASON_ALREADY_EXISTS"},
	}
	if len(recAuditor.events) != len(wantOps) {
		t.Fatalf("captured %d audit events, want %d", len(recAuditor.events), len(wantOps))
	}

	for i, want := range wantOps {
		ev := recAuditor.events[i]
		exts := ev.Extensions()
		if ev.Type() != audit.DefaultRPCEventType ||
			ev.Subject() != want.subject ||
			exts[audit.ExtCategory] != audit.CategoryResource ||
			exts[audit.ExtActorType] != "human" ||
			exts[audit.ExtActorID] != "admin-1" ||
			exts[audit.ExtResourceType] != oauthClientResourceType ||
			exts[audit.ExtResourceName] != want.resName ||
			exts[audit.ExtOutcome] != want.outcome {
			t.Fatalf("event[%d] mismatch: subject=%q exts=%+v, want %+v", i, ev.Subject(), exts, want)
		}
		if want.errCode != "" && exts[audit.ExtErrorCode] != want.errCode {
			t.Fatalf("event[%d] errorcode = %v, want %q", i, exts[audit.ExtErrorCode], want.errCode)
		}

		rawEventJSON, err := json.Marshal(ev)
		if err != nil {
			t.Fatal(err)
		}
		combined := string(rawEventJSON) + "\n" + string(ev.Data())
		for _, forbidden := range []string{
			initialPlaintextSecret,
			rotatedPlaintextSecret,
			"client_secret",
			"clientSecret",
			"secret_value",
			"secretValue",
		} {
			if strings.Contains(combined, forbidden) {
				t.Fatalf("event[%d] (%s) leaked %q into CloudEvents payload: %s", i, ev.Subject(), forbidden, combined)
			}
		}
	}

	logs := logBuf.String()
	for _, secret := range []string{initialPlaintextSecret, rotatedPlaintextSecret} {
		if strings.Contains(logs, secret) {
			t.Fatalf("plaintext secret %q leaked into slog output: %s", secret, logs)
		}
	}
}
