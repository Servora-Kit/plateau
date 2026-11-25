package server

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	auditeventpb "github.com/Servora-Kit/plateau/api/gen/go/audit/event/v1"
	securityauditpb "github.com/Servora-Kit/plateau/api/gen/go/plateau/security/audit/v1"
	jwtconfpb "github.com/Servora-Kit/plateau/api/gen/go/plateau/security/authn/jwt/v1"
	jwtkeypb "github.com/Servora-Kit/plateau/api/gen/go/plateau/security/jwt/v1"
	"github.com/Servora-Kit/plateau/app/audit/service/internal/biz"
	"github.com/Servora-Kit/plateau/app/audit/service/internal/service"
	"github.com/Servora-Kit/plateau/security/audit"
	"github.com/Servora-Kit/plateau/security/authz/openfga"
	corepb "github.com/Servora-Kit/servora/api/gen/go/servora/core/v1"
	"github.com/Servora-Kit/servora/obs/metrics"
	"github.com/go-jose/go-jose/v4"
	jwtlib "github.com/golang-jwt/jwt/v5"
	fgasdk "github.com/openfga/go-sdk"
	fgaclient "github.com/openfga/go-sdk/client"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type memoryRepo struct {
	event *auditeventpb.Event
}

func (m *memoryRepo) SaveBatch(context.Context, []*auditeventpb.Event) error { return nil }
func (m *memoryRepo) GetByID(_ context.Context, id string) (*auditeventpb.Event, error) {
	if id == m.event.GetEventId() {
		return m.event, nil
	}
	return nil, biz.ErrEventNotFound
}
func (m *memoryRepo) List(context.Context, biz.ListEventsFilter) ([]*auditeventpb.Event, error) {
	return []*auditeventpb.Event{m.event}, nil
}
func (m *memoryRepo) ListServices(context.Context) ([]string, error) {
	return []string{m.event.GetService()}, nil
}

func TestGRPCServerServiceJWTAndOpenFGAAuthorization(t *testing.T) {
	const (
		issuer  = "http://localhost:10002"
		keyID   = "test-kid-1"
		storeID = "01ARZ3NDEKTSV4RRFFQ69G5FAV"
		modelID = "01ARZ3NDEKTSV4RRFFQ69G5FAW"
	)

	privKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("rsa.GenerateKey: %v", err)
	}

	jwksSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(jose.JSONWebKeySet{
			Keys: []jose.JSONWebKey{{
				Key:       &privKey.PublicKey,
				KeyID:     keyID,
				Algorithm: "RS256",
				Use:       "sig",
			}},
		})
	}))
	t.Cleanup(jwksSrv.Close)

	fgaSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			TupleKey struct {
				User     string `json:"user"`
				Relation string `json:"relation"`
				Object   string `json:"object"`
			} `json:"tuple_key"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		allowed := body.TupleKey.User == "service:admin" &&
			body.TupleKey.Relation == "read_events" &&
			body.TupleKey.Object == "audit:global"
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]bool{"allowed": allowed})
	}))
	t.Cleanup(fgaSrv.Close)

	jwtAuthn, cleanupJWT, err := NewServiceAuthenticator(&jwtconfpb.JwtAuthnConfig{
		Issuer:   issuer,
		Audience: "audit",
		Jwks:     &jwtkeypb.JWKS{Uri: jwksSrv.URL},
	})
	if err != nil {
		t.Fatalf("NewServiceAuthenticator: %v", err)
	}
	t.Cleanup(cleanupJWT)

	fgaSDK, err := fgaclient.NewSdkClient(&fgaclient.ClientConfiguration{
		ApiUrl:               fgaSrv.URL,
		StoreId:              storeID,
		AuthorizationModelId: modelID,
		RetryParams:          &fgasdk.RetryParams{MaxRetry: 1, MinWaitInMs: 1},
	})
	if err != nil {
		t.Fatalf("NewSdkClient: %v", err)
	}
	authorizer, err := openfga.NewDefault(fgaSDK)
	if err != nil {
		t.Fatalf("openfga.NewDefault: %v", err)
	}

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	uc, err := biz.NewEventUsecase(&memoryRepo{
		event: &auditeventpb.Event{
			Name:         "events/ev-authz-ok",
			EventId:      "ev-authz-ok",
			EventType:    audit.DefaultRPCEventType,
			Category:     securityauditpb.AuditCategory_AUDIT_CATEGORY_RESOURCE,
			OccurredTime: timestamppb.New(time.Date(2026, 11, 24, 10, 0, 0, 0, time.UTC)),
			Service:      "iam.service",
			DetailJson:   "{}",
		},
	}, logger)
	if err != nil {
		t.Fatalf("NewEventUsecase: %v", err)
	}
	svc, err := service.NewEventService(uc)
	if err != nil {
		t.Fatalf("NewEventService: %v", err)
	}

	gs := NewGRPCServer(
		&corepb.Server{Grpc: &corepb.Server_GRPC{Listen: &corepb.Server_Listen{Addr: new("127.0.0.1:0")}}},
		&corepb.Observability{},
		nil,
		jwtAuthn,
		authorizer,
		svc,
		logger,
	)

	endpoint, err := gs.Endpoint()
	if err != nil {
		t.Fatalf("gs.Endpoint: %v", err)
	}

	srvCtx, cancelSrv := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = gs.Start(srvCtx)
	}()
	t.Cleanup(func() {
		_ = gs.Stop(context.Background())
		cancelSrv()
		<-done
	})

	conn, err := grpc.NewClient(endpoint.Host, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("grpc.NewClient: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	client := auditeventpb.NewEventServiceClient(conn)

	signToken := func(claims jwtlib.MapClaims) string {
		t.Helper()
		tok := jwtlib.NewWithClaims(jwtlib.SigningMethodRS256, claims)
		tok.Header["kid"] = keyID
		tok.Header["typ"] = "JWT"
		signed, signErr := tok.SignedString(privKey)
		if signErr != nil {
			t.Fatalf("SignedString: %v", signErr)
		}
		return signed
	}
	now := time.Now()
	withBearer := func(token string) context.Context {
		return metadata.AppendToOutgoingContext(context.Background(), "authorization", "Bearer "+token)
	}

	if _, err := client.ListEvents(context.Background(), &auditeventpb.ListEventsRequest{}); status.Code(err) != codes.Unauthenticated {
		t.Fatalf("missing token code = %v, want Unauthenticated (err=%v)", status.Code(err), err)
	}

	humanTok := signToken(jwtlib.MapClaims{
		"iss":        issuer,
		"aud":        []string{"audit"},
		"sub":        "user-123",
		"jti":        "jti-human",
		"iat":        now.Unix(),
		"exp":        now.Add(5 * time.Minute).Unix(),
		"token_use":  "access",
		"actor_type": "human",
	})
	if _, err := client.ListEvents(withBearer(humanTok), &auditeventpb.ListEventsRequest{}); status.Code(err) != codes.Unauthenticated {
		t.Fatalf("human token code = %v, want Unauthenticated (err=%v)", status.Code(err), err)
	}

	wrongAudTok := signToken(jwtlib.MapClaims{
		"iss":        issuer,
		"aud":        []string{"iam"},
		"sub":        "admin",
		"client_id":  "admin",
		"jti":        "jti-wrong-aud",
		"iat":        now.Unix(),
		"exp":        now.Add(5 * time.Minute).Unix(),
		"token_use":  "access",
		"actor_type": "service",
	})
	if _, err := client.ListEvents(withBearer(wrongAudTok), &auditeventpb.ListEventsRequest{}); status.Code(err) != codes.Unauthenticated {
		t.Fatalf("wrong audience code = %v, want Unauthenticated (err=%v)", status.Code(err), err)
	}

	otherSvcTok := signToken(jwtlib.MapClaims{
		"iss":        issuer,
		"aud":        []string{"audit"},
		"sub":        "other",
		"client_id":  "other",
		"jti":        "jti-other",
		"iat":        now.Unix(),
		"exp":        now.Add(5 * time.Minute).Unix(),
		"token_use":  "access",
		"actor_type": "service",
	})
	if _, err := client.GetEvent(withBearer(otherSvcTok), &auditeventpb.GetEventRequest{Name: "events/ev-authz-ok"}); status.Code(err) != codes.PermissionDenied {
		t.Fatalf("unauthorized service code = %v, want PermissionDenied (err=%v)", status.Code(err), err)
	}

	adminSvcTok := signToken(jwtlib.MapClaims{
		"iss":        issuer,
		"aud":        []string{"audit"},
		"sub":        "admin",
		"client_id":  "admin",
		"jti":        "jti-admin",
		"iat":        now.Unix(),
		"exp":        now.Add(5 * time.Minute).Unix(),
		"token_use":  "access",
		"actor_type": "service",
	})
	gotEvent, err := client.GetEvent(withBearer(adminSvcTok), &auditeventpb.GetEventRequest{Name: "events/ev-authz-ok"})
	if err != nil || gotEvent.GetName() != "events/ev-authz-ok" {
		t.Fatalf("authorized GetEvent = (%+v, %v)", gotEvent, err)
	}
	listResp, err := client.ListEvents(withBearer(adminSvcTok), &auditeventpb.ListEventsRequest{})
	if err != nil || len(listResp.GetEvents()) != 1 {
		t.Fatalf("authorized ListEvents = (%+v, %v)", listResp, err)
	}
	svcResp, err := client.ListServices(withBearer(adminSvcTok), &auditeventpb.ListServicesRequest{})
	if err != nil || len(svcResp.GetServices()) != 1 || svcResp.GetServices()[0] != gotEvent.GetService() {
		t.Fatalf("authorized ListServices = (%+v, %v)", svcResp, err)
	}
}

func TestHTTPServerExposesOnlyInternalOpsRoutes(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	mtc, cleanupMtc, err := metrics.New(&corepb.Observability{Metrics: &corepb.Metrics{Enable: true}}, &corepb.App{Name: "audit.service"}, logger)
	if err != nil {
		t.Fatalf("metrics.New: %v", err)
	}
	t.Cleanup(cleanupMtc)

	hs := NewHTTPServer(
		&corepb.Server{Http: &corepb.Server_HTTP{Listen: &corepb.Server_Listen{Addr: new("127.0.0.1:0")}}},
		&corepb.Observability{},
		mtc,
		logger,
	)

	for _, path := range []string{"/healthz", "/readyz", "/metrics"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rec := httptest.NewRecorder()
		hs.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("GET %s status = %d, want 200", path, rec.Code)
		}
	}

	req := httptest.NewRequest(http.MethodGet, "/v1/audit/events", nil)
	rec := httptest.NewRecorder()
	hs.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("GET /v1/audit/events status = %d, want 404", rec.Code)
	}
}
