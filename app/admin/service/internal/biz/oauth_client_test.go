package biz

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"testing"
	"time"

	adminclientpb "github.com/Servora-Kit/plateau/api/gen/go/plateau/admin/client/v1"
	iamclientpb "github.com/Servora-Kit/plateau/api/gen/go/plateau/iam/client/v1"
	"github.com/Servora-Kit/plateau/security"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/fieldmaskpb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type stubIAMOAuthClientRepo struct {
	calls         int
	lastRegister  *iamclientpb.RegisterOAuthClientRequest
	lastGet       *iamclientpb.GetOAuthClientRequest
	lastList      *iamclientpb.ListOAuthClientsRequest
	lastUpdate    *iamclientpb.UpdateOAuthClientRequest
	lastDisable   *iamclientpb.DisableOAuthClientRequest
	lastEnable    *iamclientpb.EnableOAuthClientRequest
	lastDelete    *iamclientpb.DeleteOAuthClientRequest
	lastCreateSec *iamclientpb.CreateOAuthClientSecretRequest
	lastDeleteSec *iamclientpb.DeleteOAuthClientSecretRequest
	registerResp  *iamclientpb.RegisterOAuthClientResponse
	getResp       *iamclientpb.OAuthClient
	listResp      *iamclientpb.ListOAuthClientsResponse
	updateResp    *iamclientpb.OAuthClient
	disableResp   *iamclientpb.OAuthClient
	enableResp    *iamclientpb.OAuthClient
	createSecResp *iamclientpb.CreateOAuthClientSecretResponse
	downstreamErr error
}

func (s *stubIAMOAuthClientRepo) RegisterOAuthClient(_ context.Context, req *iamclientpb.RegisterOAuthClientRequest) (*iamclientpb.RegisterOAuthClientResponse, error) {
	s.calls++
	s.lastRegister = req
	if s.downstreamErr != nil {
		return nil, s.downstreamErr
	}
	return s.registerResp, nil
}

func (s *stubIAMOAuthClientRepo) GetOAuthClient(_ context.Context, req *iamclientpb.GetOAuthClientRequest) (*iamclientpb.OAuthClient, error) {
	s.calls++
	s.lastGet = req
	if s.downstreamErr != nil {
		return nil, s.downstreamErr
	}
	return s.getResp, nil
}

func (s *stubIAMOAuthClientRepo) ListOAuthClients(_ context.Context, req *iamclientpb.ListOAuthClientsRequest) (*iamclientpb.ListOAuthClientsResponse, error) {
	s.calls++
	s.lastList = req
	if s.downstreamErr != nil {
		return nil, s.downstreamErr
	}
	return s.listResp, nil
}

func (s *stubIAMOAuthClientRepo) UpdateOAuthClient(_ context.Context, req *iamclientpb.UpdateOAuthClientRequest) (*iamclientpb.OAuthClient, error) {
	s.calls++
	s.lastUpdate = req
	if s.downstreamErr != nil {
		return nil, s.downstreamErr
	}
	return s.updateResp, nil
}

func (s *stubIAMOAuthClientRepo) DisableOAuthClient(_ context.Context, req *iamclientpb.DisableOAuthClientRequest) (*iamclientpb.OAuthClient, error) {
	s.calls++
	s.lastDisable = req
	if s.downstreamErr != nil {
		return nil, s.downstreamErr
	}
	return s.disableResp, nil
}

func (s *stubIAMOAuthClientRepo) EnableOAuthClient(_ context.Context, req *iamclientpb.EnableOAuthClientRequest) (*iamclientpb.OAuthClient, error) {
	s.calls++
	s.lastEnable = req
	if s.downstreamErr != nil {
		return nil, s.downstreamErr
	}
	return s.enableResp, nil
}

func (s *stubIAMOAuthClientRepo) DeleteOAuthClient(_ context.Context, req *iamclientpb.DeleteOAuthClientRequest) error {
	s.calls++
	s.lastDelete = req
	return s.downstreamErr
}

func (s *stubIAMOAuthClientRepo) CreateOAuthClientSecret(_ context.Context, req *iamclientpb.CreateOAuthClientSecretRequest) (*iamclientpb.CreateOAuthClientSecretResponse, error) {
	s.calls++
	s.lastCreateSec = req
	if s.downstreamErr != nil {
		return nil, s.downstreamErr
	}
	return s.createSecResp, nil
}

func (s *stubIAMOAuthClientRepo) DeleteOAuthClientSecret(_ context.Context, req *iamclientpb.DeleteOAuthClientSecretRequest) error {
	s.calls++
	s.lastDeleteSec = req
	return s.downstreamErr
}

func TestOAuthClientUsecaseOrchestrationAndGuards(t *testing.T) {
	if _, err := NewOAuthClientUsecase(nil); err == nil {
		t.Fatal("NewOAuthClientUsecase(nil) succeeded, want error")
	}

	total := int64(2)
	expires := timestamppb.New(time.Date(2027, 1, 2, 3, 4, 5, 0, time.UTC))
	repo := &stubIAMOAuthClientRepo{
		listResp: &iamclientpb.ListOAuthClientsResponse{
			Clients:       []*iamclientpb.OAuthClient{{Name: "oauthClients/admin", ClientId: "admin"}},
			NextPageToken: "cursor-2",
			TotalSize:     &total,
		},
		getResp: &iamclientpb.OAuthClient{Name: "oauthClients/blog-web", ClientId: "blog-web"},
		registerResp: &iamclientpb.RegisterOAuthClientResponse{
			Client:       &iamclientpb.OAuthClient{Name: "oauthClients/blog-web", ClientId: "blog-web"},
			ClientSecret: "pcs_initial_secret_once",
		},
		updateResp:  &iamclientpb.OAuthClient{Name: "oauthClients/blog-web", DisplayName: proto.String("Blog Updated")},
		disableResp: &iamclientpb.OAuthClient{Name: "oauthClients/blog-web", Status: iamclientpb.ClientStatus_CLIENT_STATUS_DISABLED},
		enableResp:  &iamclientpb.OAuthClient{Name: "oauthClients/blog-web", Status: iamclientpb.ClientStatus_CLIENT_STATUS_ACTIVE},
		createSecResp: &iamclientpb.CreateOAuthClientSecretResponse{
			Secret:      &iamclientpb.OAuthClientSecret{SecretId: "sec-2", Name: "轮换密钥", SecretPrefix: "pcs_abcd1234"},
			SecretValue: "pcs_rotated_secret_once",
		},
	}

	uc, err := NewOAuthClientUsecase(repo)
	if err != nil {
		t.Fatal(err)
	}

	// Unauthenticated and service actors must be rejected across all 9 methods without calling downstream.
	targetName := iamclientpb.NewOAuthClientName("blog-web")
	for _, badCtx := range []context.Context{
		context.Background(),
		security.WithActor(context.Background(), security.Actor{Type: security.ActorTypeService, ID: "admin"}),
	} {
		if _, err := uc.ListOAuthClients(badCtx, &adminclientpb.ListOAuthClientsRequest{}); !errors.Is(err, ErrUnauthenticated) {
			t.Fatalf("ListOAuthClients err = %v, want ErrUnauthenticated", err)
		}
		if _, err := uc.GetOAuthClient(badCtx, targetName); !errors.Is(err, ErrUnauthenticated) {
			t.Fatalf("GetOAuthClient err = %v, want ErrUnauthenticated", err)
		}
		if _, err := uc.CreateOAuthClient(badCtx, &adminclientpb.CreateOAuthClientRequest{}); !errors.Is(err, ErrUnauthenticated) {
			t.Fatalf("CreateOAuthClient err = %v, want ErrUnauthenticated", err)
		}
		if _, err := uc.UpdateOAuthClient(badCtx, targetName, &iamclientpb.OAuthClient{}, nil); !errors.Is(err, ErrUnauthenticated) {
			t.Fatalf("UpdateOAuthClient err = %v, want ErrUnauthenticated", err)
		}
		if _, err := uc.DisableOAuthClient(badCtx, targetName, "e1"); !errors.Is(err, ErrUnauthenticated) {
			t.Fatalf("DisableOAuthClient err = %v, want ErrUnauthenticated", err)
		}
		if _, err := uc.EnableOAuthClient(badCtx, targetName, "e1"); !errors.Is(err, ErrUnauthenticated) {
			t.Fatalf("EnableOAuthClient err = %v, want ErrUnauthenticated", err)
		}
		if err := uc.DeleteOAuthClient(badCtx, targetName, "e1"); !errors.Is(err, ErrUnauthenticated) {
			t.Fatalf("DeleteOAuthClient err = %v, want ErrUnauthenticated", err)
		}
		if _, err := uc.CreateOAuthClientSecret(badCtx, targetName, "k", nil); !errors.Is(err, ErrUnauthenticated) {
			t.Fatalf("CreateOAuthClientSecret err = %v, want ErrUnauthenticated", err)
		}
		if err := uc.DeleteOAuthClientSecret(badCtx, targetName, "sec-1"); !errors.Is(err, ErrUnauthenticated) {
			t.Fatalf("DeleteOAuthClientSecret err = %v, want ErrUnauthenticated", err)
		}
	}
	if repo.calls != 0 {
		t.Fatalf("downstream repo called %d times for unauthorized actors, want 0", repo.calls)
	}

	adminCtx := security.WithActor(context.Background(), security.Actor{Type: security.ActorTypeHuman, ID: "admin-1"})

	// 1. ListOAuthClients forwards pagination/filtering/sorting and preserves optional TotalSize.
	listResp, err := uc.ListOAuthClients(adminCtx, &adminclientpb.ListOAuthClientsRequest{
		PageSize:     25,
		PageToken:    "cursor-1",
		Filter:       `source = "CLIENT_SOURCE_DYNAMIC"`,
		OrderBy:      "create_time desc",
		IncludeTotal: true,
	})
	if err != nil {
		t.Fatalf("ListOAuthClients err = %v", err)
	}
	if repo.lastList.GetPageSize() != 25 || repo.lastList.GetPageToken() != "cursor-1" ||
		repo.lastList.GetFilter() != `source = "CLIENT_SOURCE_DYNAMIC"` || repo.lastList.GetOrderBy() != "create_time desc" ||
		!repo.lastList.GetIncludeTotal() {
		t.Fatalf("unexpected ListOAuthClientsRequest: %+v", repo.lastList)
	}
	if len(listResp.GetClients()) != 1 || listResp.GetNextPageToken() != "cursor-2" || listResp.TotalSize == nil || listResp.GetTotalSize() != 2 {
		t.Fatalf("unexpected ListOAuthClientsResponse: %+v", listResp)
	}

	// 2. GetOAuthClient formats canonical resource name and rejects invalid OAuthClientName.
	if _, err := uc.GetOAuthClient(adminCtx, iamclientpb.NewOAuthClientName("bad/id")); err == nil {
		t.Fatal("GetOAuthClient(bad/id) succeeded, want validation error")
	}
	gotClient, err := uc.GetOAuthClient(adminCtx, targetName)
	if err != nil || gotClient.GetClientId() != "blog-web" || repo.lastGet.GetName() != "oauthClients/blog-web" {
		t.Fatalf("GetOAuthClient got=%+v req=%+v err=%v", gotClient, repo.lastGet, err)
	}

	// 3. CreateOAuthClient maps to downstream RegisterOAuthClient and returns one-time client_secret.
	createIn := &iamclientpb.OAuthClient{
		DisplayName:             proto.String("Blog Web"),
		ApplicationType:         iamclientpb.ApplicationType_APPLICATION_TYPE_WEB.Enum(),
		TokenEndpointAuthMethod: iamclientpb.TokenEndpointAuthMethod_TOKEN_ENDPOINT_AUTH_METHOD_CLIENT_SECRET_BASIC.Enum(),
		RedirectUris:            []string{"https://blog.example.com/callback"},
		AllowedGrantTypes:       []string{"authorization_code"},
		AllowedScopes:           []string{"openid", "profile"},
	}
	createResp, err := uc.CreateOAuthClient(adminCtx, &adminclientpb.CreateOAuthClientRequest{
		OauthClientId:     "blog-web",
		OauthClient:       createIn,
		InitialSecretName: "生产密钥",
	})
	if err != nil {
		t.Fatalf("CreateOAuthClient err = %v", err)
	}
	if repo.lastRegister.GetOauthClientId() != "blog-web" ||
		repo.lastRegister.GetInitialSecretName() != "生产密钥" ||
		repo.lastRegister.GetOauthClient().GetDisplayName() != "Blog Web" {
		t.Fatalf("unexpected RegisterOAuthClientRequest: %+v", repo.lastRegister)
	}
	if createResp.GetClient().GetName() != "oauthClients/blog-web" || createResp.GetClientSecret() != "pcs_initial_secret_once" {
		t.Fatalf("unexpected CreateOAuthClientResponse: %+v", createResp)
	}

	// 4. UpdateOAuthClient clones input without mutating caller's struct and enforces canonical Name.
	callerPatch := &iamclientpb.OAuthClient{
		Name:        "tampered/name",
		DisplayName: proto.String("Blog Updated"),
		Etag:        "etag-1",
	}
	mask := &fieldmaskpb.FieldMask{Paths: []string{"display_name"}}
	updated, err := uc.UpdateOAuthClient(adminCtx, targetName, callerPatch, mask)
	if err != nil {
		t.Fatalf("UpdateOAuthClient err = %v", err)
	}
	if callerPatch.Name != "tampered/name" {
		t.Fatalf("caller OAuthClient mutated: Name = %q", callerPatch.Name)
	}
	if repo.lastUpdate.GetOauthClient().GetName() != "oauthClients/blog-web" ||
		repo.lastUpdate.GetOauthClient().GetEtag() != "etag-1" ||
		!slices.Equal(repo.lastUpdate.GetUpdateMask().GetPaths(), []string{"display_name"}) ||
		updated.GetDisplayName() != "Blog Updated" {
		t.Fatalf("unexpected UpdateOAuthClient result: req=%+v resp=%+v", repo.lastUpdate, updated)
	}

	// 5. DisableOAuthClient, EnableOAuthClient, DeleteOAuthClient.
	disabled, err := uc.DisableOAuthClient(adminCtx, targetName, "etag-2")
	if err != nil || disabled.GetStatus() != iamclientpb.ClientStatus_CLIENT_STATUS_DISABLED ||
		repo.lastDisable.GetName() != "oauthClients/blog-web" || repo.lastDisable.GetEtag() != "etag-2" {
		t.Fatalf("DisableOAuthClient resp=%+v req=%+v err=%v", disabled, repo.lastDisable, err)
	}

	enabled, err := uc.EnableOAuthClient(adminCtx, targetName, "etag-3")
	if err != nil || enabled.GetStatus() != iamclientpb.ClientStatus_CLIENT_STATUS_ACTIVE ||
		repo.lastEnable.GetName() != "oauthClients/blog-web" || repo.lastEnable.GetEtag() != "etag-3" {
		t.Fatalf("EnableOAuthClient resp=%+v req=%+v err=%v", enabled, repo.lastEnable, err)
	}

	if err := uc.DeleteOAuthClient(adminCtx, targetName, "etag-4"); err != nil ||
		repo.lastDelete.GetName() != "oauthClients/blog-web" || repo.lastDelete.GetEtag() != "etag-4" {
		t.Fatalf("DeleteOAuthClient req=%+v err=%v", repo.lastDelete, err)
	}

	// 6. CreateOAuthClientSecret and DeleteOAuthClientSecret.
	secResp, err := uc.CreateOAuthClientSecret(adminCtx, targetName, "轮换密钥", expires)
	if err != nil {
		t.Fatalf("CreateOAuthClientSecret err = %v", err)
	}
	if repo.lastCreateSec.GetName() != "oauthClients/blog-web" ||
		repo.lastCreateSec.GetSecretName() != "轮换密钥" ||
		!repo.lastCreateSec.GetExpiresTime().AsTime().Equal(expires.AsTime()) ||
		secResp.GetSecret().GetSecretId() != "sec-2" ||
		secResp.GetSecretValue() != "pcs_rotated_secret_once" {
		t.Fatalf("CreateOAuthClientSecret req=%+v resp=%+v", repo.lastCreateSec, secResp)
	}

	if err := uc.DeleteOAuthClientSecret(adminCtx, targetName, "sec-1"); err != nil ||
		repo.lastDeleteSec.GetName() != "oauthClients/blog-web" ||
		repo.lastDeleteSec.GetSecretId() != "sec-1" {
		t.Fatalf("DeleteOAuthClientSecret req=%+v err=%v", repo.lastDeleteSec, err)
	}
}

func TestAuthUsecasePermissionsIncludesManageClients(t *testing.T) {
	granted := map[string]bool{
		PermAccess:        true,
		PermManageClients: true,
	}
	authz := newTestAuthorizer(t, func(w http.ResponseWriter, r *http.Request) {
		var payload struct {
			Checks []struct {
				TupleKey struct {
					Relation string `json:"relation"`
				} `json:"tuple_key"`
				CorrelationID string `json:"correlation_id"`
			} `json:"checks"`
		}
		_ = json.NewDecoder(r.Body).Decode(&payload)
		result := make(map[string]any, len(payload.Checks))
		for _, item := range payload.Checks {
			result[item.CorrelationID] = map[string]bool{
				"allowed": granted[item.TupleKey.Relation],
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"result": result})
	})

	authUC, err := NewAuthUsecase(authz)
	if err != nil {
		t.Fatal(err)
	}
	actor := security.Actor{Type: security.ActorTypeHuman, ID: "admin-1"}

	partial, err := authUC.Permissions(t.Context(), actor)
	if err != nil {
		t.Fatalf("Permissions(partial) err = %v", err)
	}
	if !slices.Equal(partial, []string{"access", "manage_clients"}) {
		t.Fatalf("Permissions(partial) = %v, want [access manage_clients]", partial)
	}

	granted = map[string]bool{
		PermAccess:        true,
		PermManageUsers:   true,
		PermExpungeUsers:  true,
		PermManageClients: true,
		PermReadAudit:     true,
	}
	all, err := authUC.Permissions(t.Context(), actor)
	if err != nil {
		t.Fatalf("Permissions(all) err = %v", err)
	}
	wantAll := []string{"access", "manage_users", "expunge_users", "manage_clients", "read_audit"}
	if !slices.Equal(all, wantAll) {
		t.Fatalf("Permissions(all) = %v, want %v", all, wantAll)
	}
}
