package server

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"database/sql"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	adminauditpb "github.com/Servora-Kit/plateau/api/gen/go/plateau/admin/audit/v1"
	adminconfpb "github.com/Servora-Kit/plateau/api/gen/go/plateau/admin/conf/v1"
	adminuserpb "github.com/Servora-Kit/plateau/api/gen/go/plateau/admin/user/v1"
	auditeventpb "github.com/Servora-Kit/plateau/api/gen/go/plateau/audit/event/v1"
	userpb "github.com/Servora-Kit/plateau/api/gen/go/plateau/iam/user/v1"
	clickhousepb "github.com/Servora-Kit/plateau/api/gen/go/plateau/infra/clickhouse/v1"
	kafkapb "github.com/Servora-Kit/plateau/api/gen/go/plateau/infra/kafka/v1"
	auditpb "github.com/Servora-Kit/plateau/api/gen/go/plateau/security/audit/v1"
	oidcpb "github.com/Servora-Kit/plateau/api/gen/go/plateau/security/oidc/v1"
	sessionpb "github.com/Servora-Kit/plateau/api/gen/go/plateau/security/session/v1"
	"github.com/Servora-Kit/plateau/app/admin/service/internal/biz"
	"github.com/Servora-Kit/plateau/app/admin/service/internal/data"
	"github.com/Servora-Kit/plateau/app/admin/service/internal/service"
	"github.com/Servora-Kit/plateau/app/admin/service/internal/startup"
	pkgch "github.com/Servora-Kit/plateau/infra/clickhouse"
	infrakafka "github.com/Servora-Kit/plateau/infra/kafka"
	auditkafka "github.com/Servora-Kit/plateau/security/audit/kafka"
	authnsession "github.com/Servora-Kit/plateau/security/authn/session"
	"github.com/Servora-Kit/plateau/security/authz/openfga"
	httpsession "github.com/Servora-Kit/plateau/security/session"
	corepb "github.com/Servora-Kit/servora/api/gen/go/servora/core/v1"
	"github.com/go-kratos/kratos/v3"
	"github.com/google/uuid"
	fgaclient "github.com/openfga/go-sdk/client"
	"github.com/twmb/franz-go/pkg/kmsg"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/durationpb"
)

const (
	defaultKafkaBroker    = "127.0.0.1:29092"
	defaultClickHouseAddr = "127.0.0.1:19000"
)

func buildAuditBinary(t *testing.T) string {
	t.Helper()
	binPath := filepath.Join(t.TempDir(), "audit-server")
	repoRoot, err := filepath.Abs("../../../../..")
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("go", "build", "-o", binPath, "./app/audit/service/cmd/server")
	cmd.Dir = repoRoot
	cmd.Env = append(os.Environ(), "CC=/usr/bin/clang")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("build audit server binary: %v\n%s", err, string(out))
	}
	return binPath
}

func envOrFallback(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}

func prepareAuditInfra(t *testing.T) (kafkaBroker, topic, chAddr, chUser, chPass, chDB string) {
	t.Helper()
	kafkaBroker = envOrFallback("KAFKA_BROKER_ADDR", defaultKafkaBroker)
	if conn, err := net.DialTimeout("tcp", kafkaBroker, 500*time.Millisecond); err != nil {
		t.Skipf("skipping cross-service audit integration test (Kafka %s unreachable: %v)", kafkaBroker, err)
	} else {
		_ = conn.Close()
	}

	chAddr = envOrFallback("CLICKHOUSE_ADDR", defaultClickHouseAddr)
	if conn, err := net.DialTimeout("tcp", chAddr, 500*time.Millisecond); err != nil {
		t.Skipf("skipping cross-service audit integration test (ClickHouse %s unreachable: %v)", chAddr, err)
	} else {
		_ = conn.Close()
	}

	chUser = envOrFallback("CLICKHOUSE_USER", "clickhouse")
	chPass = envOrFallback("CLICKHOUSE_PASSWORD", "clickhouse")
	chDB = "audit_e2e_" + strings.ReplaceAll(uuid.NewString(), "-", "")[:12]

	adminConn, err := pkgch.NewConnOptional(t.Context(), &clickhousepb.ClickHouse{
		Addrs:       []string{chAddr},
		Database:    "default",
		Username:    chUser,
		Password:    chPass,
		DialTimeout: durationpb.New(5 * time.Second),
	})
	if err != nil || adminConn == nil {
		t.Skipf("skipping cross-service audit integration test (ClickHouse auth failed: %v)", err)
	}
	if err := adminConn.Exec(t.Context(), "CREATE DATABASE "+chDB); err != nil {
		_ = adminConn.Close()
		t.Fatalf("CREATE DATABASE %s: %v", chDB, err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = adminConn.Exec(ctx, "DROP DATABASE IF EXISTS "+chDB)
		_ = adminConn.Close()
	})

	topic = "plateau.audit.events.e2e." + strings.ReplaceAll(uuid.NewString(), "-", "")[:12]
	kClient, closeK, err := infrakafka.New(&kafkapb.Kafka{
		Brokers:  []string{kafkaBroker},
		ClientId: "audit-e2e-admin",
	}, slog.Default())
	if err != nil {
		t.Fatalf("connect Kafka for topic setup: %v", err)
	}
	createReq := kmsg.NewPtrCreateTopicsRequest()
	tp := kmsg.NewCreateTopicsRequestTopic()
	tp.Topic = topic
	tp.NumPartitions = 1
	tp.ReplicationFactor = 1
	createReq.Topics = []kmsg.CreateTopicsRequestTopic{tp}
	if _, err := createReq.RequestWith(t.Context(), kClient); err != nil {
		closeK()
		t.Fatalf("create Kafka topic %s: %v", topic, err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		delReq := kmsg.NewPtrDeleteTopicsRequest()
		delTopic := kmsg.NewDeleteTopicsRequestTopic()
		delTopic.Topic = &topic
		delReq.TopicNames = []string{topic}
		delReq.Topics = []kmsg.DeleteTopicsRequestTopic{delTopic}
		_, _ = delReq.RequestWith(ctx, kClient)
		closeK()
	})

	return kafkaBroker, topic, chAddr, chUser, chPass, chDB
}

func startSilentSinkListener(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			_ = conn.Close()
		}
	}()
	return ln.Addr().String()
}

func startRealIAMWithAudit(
	t *testing.T,
	iamBin, iamHTTPAddr, iamGRPCAddr, iamDSN string,
	fgaURL, storeID, modelID string,
	smtpAddr, adminCallbackURI string,
	kafkaBroker, auditTopic, traceEndpoint string,
) (string, string) {
	t.Helper()
	confDir := t.TempDir()
	secretsDir := t.TempDir()

	rsaKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	signingPath := filepath.Join(secretsDir, "oidc-signing.pem")
	if err := os.WriteFile(signingPath, pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(rsaKey),
	}), 0o600); err != nil {
		t.Fatal(err)
	}
	cryptoBytes := make([]byte, 32)
	if _, err := rand.Read(cryptoBytes); err != nil {
		t.Fatal(err)
	}
	cryptoPath := filepath.Join(secretsDir, "oidc-crypto.key")
	if err := os.WriteFile(cryptoPath, cryptoBytes, 0o600); err != nil {
		t.Fatal(err)
	}

	smtpHost, smtpPort, err := net.SplitHostPort(smtpAddr)
	if err != nil {
		t.Fatal(err)
	}
	iamOrigin := "http://" + iamHTTPAddr

	files := map[string]string{
		"bootstrap.yaml": fmt.Sprintf(`server:
  http:
    listen:
      addr: %q
      timeout: 15s
  grpc:
    listen:
      addr: %q
      timeout: 15s
data:
  database:
    driver: postgresql
    source: %q
app:
  name: iam.service
  version: dev
  env: test
  external_url: %q
obs:
  log:
    level: 0
    backends:
      - stdout: {}
  trace:
    endpoint: %q
    insecure: true
    sampling_ratio: 1.0
  metrics:
    enable: false
`, iamHTTPAddr, iamGRPCAddr, iamDSN, iamOrigin, traceEndpoint),
		"iam.yaml": fmt.Sprintf("iam:\n  bootstrap_user_email: %q\n", adminBootstrapEml),
		"oidc.yaml": fmt.Sprintf(`oidc:
  issuer: %q
  signing_key_path: %q
  crypto_key_path: %q
  clients:
    - client_id: admin
      client_secret: %q
      redirect_uris:
        - %q
      allowed_scopes: [openid, profile, email]
      trusted: true
      allowed_grant_types: [authorization_code, client_credentials]
      audiences: [iam, audit]
`, iamOrigin, signingPath, cryptoPath, oidcClientSecret, adminCallbackURI),
		"openfga.yaml":       fmt.Sprintf("open_fga:\n  api_url: %q\n  store_id: %q\n  model_id: %q\n", fgaURL, storeID, modelID),
		"session.yaml":       "session:\n  lifetime: 86400s\n  idle_timeout: 3600s\n  cookie:\n    name: iam_session\n    secure: false\n",
		"redis.yaml":         fmt.Sprintf("redis:\n  network: tcp\n  addr: %q\n  password: %q\n  db: 0\n", defaultRedisAddr, defaultRedisPass),
		"cap.yaml":           fmt.Sprintf("cap:\n  signing_secret: %q\n  redis_key_prefix: %q\n", "iam-test-cap-secret-at-least-32-bytes", "it_aud_"+uuid.NewString()+":"),
		"kafka.yaml":         fmt.Sprintf("kafka:\n  brokers:\n    - %q\n  client_id: iam.service\n  required_acks: -1\n", kafkaBroker),
		"audit_emitter.yaml": fmt.Sprintf("audit_emitter:\n  enabled: true\n  backend: kafka\n  topic: %q\n", auditTopic),
		"mail.yaml": fmt.Sprintf(`mail:
  smtp:
    host: %q
    port: %s
    tls: false
    send_timeout: 5s
  from:
    address: "no-reply@plateau.local"
    name: "Plateau IAM"
`, smtpHost, smtpPort),
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(confDir, name), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	var logs syncBuffer
	cmd := exec.Command(iamBin, "-conf", confDir)
	cmd.Stdout = &logs
	cmd.Stderr = &logs
	if err := cmd.Start(); err != nil {
		t.Fatalf("start iam-server: %v", err)
	}
	t.Cleanup(func() {
		stopSubprocess(cmd)
	})

	initPassRe := regexp.MustCompile(`initial_password[=:"\s]+([A-Za-z0-9_-]{16,})`)
	deadline := time.Now().Add(15 * time.Second)
	var initialPassword string
	for time.Now().Before(deadline) {
		if initialPassword == "" {
			if m := initPassRe.FindStringSubmatch(logs.String()); len(m) == 2 {
				initialPassword = m[1]
			}
		}
		res, err := http.Get(iamOrigin + "/.well-known/openid-configuration")
		if err == nil {
			_ = res.Body.Close()
			if res.StatusCode == http.StatusOK && initialPassword != "" {
				if grpcConn, dialErr := net.DialTimeout("tcp", iamGRPCAddr, 100*time.Millisecond); dialErr == nil {
					_ = grpcConn.Close()
					return iamOrigin, initialPassword
				}
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("IAM server did not become ready in time; logs:\n%s", logs.String())
	return "", ""
}

func startRealAuditProcess(
	t *testing.T,
	auditBin, auditHTTPAddr, auditGRPCAddr string,
	iamOrigin string,
	fgaURL, storeID, modelID string,
	kafkaBroker, auditTopic string,
	chAddr, chUser, chPass, chDB string,
) func() {
	t.Helper()
	confDir := t.TempDir()
	groupID := "audit-e2e-cg-" + strings.ReplaceAll(uuid.NewString(), "-", "")[:10]

	files := map[string]string{
		"bootstrap.yaml": fmt.Sprintf(`server:
  http:
    listen:
      addr: %q
      timeout: 15s
  grpc:
    listen:
      addr: %q
      timeout: 15s
app:
  name: audit.service
  version: dev
  env: test
obs:
  log:
    level: 0
    backends:
      - stdout: {}
  metrics:
    enable: false
`, auditHTTPAddr, auditGRPCAddr),
		"audit.yaml": fmt.Sprintf(`audit:
  topic: %q
  batch_size: 20
  flush_interval: 0.2s
  retention_days: 180
`, auditTopic),
		"kafka.yaml": fmt.Sprintf(`kafka:
  brokers:
    - %q
  client_id: audit.service
  consumer_group: %q
  required_acks: -1
`, kafkaBroker, groupID),
		"clickhouse.yaml": fmt.Sprintf(`click_house:
  addrs:
    - %q
  database: %q
  username: %q
  password: %q
  dial_timeout: 5s
  read_timeout: 10s
`, chAddr, chDB, chUser, chPass),
		"jwt_authn.yaml": fmt.Sprintf(`jwt_authn_config:
  issuer: %q
  audience: audit
  jwks:
    uri: %q
`, iamOrigin, iamOrigin+"/keys"),
		"openfga.yaml": fmt.Sprintf("open_fga:\n  api_url: %q\n  store_id: %q\n  model_id: %q\n", fgaURL, storeID, modelID),
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(confDir, name), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	var logs syncBuffer
	cmd := exec.Command(auditBin, "-conf", confDir)
	cmd.Stdout = &logs
	cmd.Stderr = &logs
	if err := cmd.Start(); err != nil {
		t.Fatalf("start audit-server: %v", err)
	}
	stop := func() {
		stopSubprocess(cmd)
	}
	t.Cleanup(stop)

	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		if conn, err := net.DialTimeout("tcp", auditGRPCAddr, 100*time.Millisecond); err == nil {
			_ = conn.Close()
			return stop
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("Audit server did not become ready in time; logs:\n%s", logs.String())
	return stop
}

func stopSubprocess(cmd *exec.Cmd) {
	if cmd == nil || cmd.Process == nil {
		return
	}
	if cmd.ProcessState != nil && cmd.ProcessState.Exited() {
		return
	}
	_ = cmd.Process.Signal(os.Interrupt)
	done := make(chan struct{})
	go func() {
		_ = cmd.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		_ = cmd.Process.Kill()
		<-done
	}
}

func buildAuditEnabledAdminInstance(
	t *testing.T,
	adminDB *sql.DB,
	adminDataCfg *corepb.Data,
	iamOrigin, iamGRPCAddr, auditGRPCAddr string,
	fgaURL, storeID, modelID string,
	kafkaBroker, auditTopic, traceEndpoint string,
	listener *httptest.Server,
) *adminInstance {
	t.Helper()
	logs := &syncBuffer{}
	logger := slog.New(slog.NewJSONHandler(logs, &slog.HandlerOptions{Level: slog.LevelDebug}))

	adminOrigin := "http://" + listener.Listener.Addr().String()
	obsCfg := &corepb.Observability{
		Trace: &corepb.Trace{
			Endpoint:      traceEndpoint,
			Insecure:      true,
			SamplingRatio: proto.Float64(1.0),
		},
	}

	dialCfg := proto.Clone(adminDataCfg).(*corepb.Data)
	dialCfg.Client = &corepb.Data_Client{
		Services: []*corepb.Data_Client_Service{
			{
				Name: "iam.service",
				Endpoints: []*corepb.Data_Client_Endpoint{
					{Protocol: "grpc", Endpoint: iamGRPCAddr, Timeout: durationpb.New(5 * time.Second)},
				},
			},
			{
				Name: "audit.service",
				Endpoints: []*corepb.Data_Client_Endpoint{
					{Protocol: "grpc", Endpoint: auditGRPCAddr, Timeout: durationpb.New(5 * time.Second)},
				},
			},
		},
	}

	driver, err := data.NewEntDriver(dialCfg, adminDB)
	if err != nil {
		t.Fatal(err)
	}
	entClient, closeEnt, err := data.NewDBClient(driver)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(closeEnt)

	sessionMgr, closeSession, err := data.NewHTTPSessionManager(&sessionpb.Session{
		Lifetime:    durationpb.New(24 * time.Hour),
		IdleTimeout: durationpb.New(8 * time.Hour),
		Cookie: &sessionpb.Cookie{
			Name:   proto.String("admin_session"),
			Secure: proto.Bool(false),
		},
	}, adminDB, entClient)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(closeSession)

	oidcCfg := &oidcpb.OIDCClient{
		Issuer:       proto.String(iamOrigin),
		ClientId:     proto.String("admin"),
		ClientSecret: proto.String(oidcClientSecret),
		RedirectUri:  adminOrigin + "/v1/admin/auth/callback",
		CookieSecret: oidcCookieSecret,
		Scopes:       []string{"openid", "profile", "email"},
		HttpTimeout:  durationpb.New(5 * time.Second),
	}

	dialer, closeDialer, err := data.NewDialer(dialCfg, obsCfg, nil, oidcCfg, nil, logger)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(closeDialer)

	fgaClient, err := fgaclient.NewSdkClient(&fgaclient.ClientConfiguration{
		ApiUrl:               fgaURL,
		StoreId:              storeID,
		AuthorizationModelId: modelID,
	})
	if err != nil {
		t.Fatal(err)
	}

	d, err := data.NewData(dialer, entClient, fgaClient)
	if err != nil {
		t.Fatal(err)
	}
	iamRepo, closeIAMRepo, err := data.NewIAMUserRepo(d)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(closeIAMRepo)

	auditRepo, closeAuditRepo, err := data.NewAuditEventRepo(d)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(closeAuditRepo)

	seedRepo, err := data.NewAdminSeedRepo(d)
	if err != nil {
		t.Fatal(err)
	}

	kafkaClient, closeKafka, err := infrakafka.New(&kafkapb.Kafka{
		Brokers:      []string{kafkaBroker},
		ClientId:     "admin.service",
		RequiredAcks: -1,
	}, logger)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(closeKafka)

	realAuditor, err := auditkafka.NewDefault(&auditpb.AuditEmitter{
		Enabled: true,
		Backend: proto.String("kafka"),
		Topic:   proto.String(auditTopic),
	}, kafkaClient, logger)
	if err != nil {
		t.Fatal(err)
	}

	authorizer, err := openfga.NewDefault(fgaClient)
	if err != nil {
		t.Fatal(err)
	}
	authUC, err := biz.NewAuthUsecase(authorizer)
	if err != nil {
		t.Fatal(err)
	}
	userUC, err := biz.NewUserUsecase(iamRepo, authorizer, logger)
	if err != nil {
		t.Fatal(err)
	}
	auditUC, err := biz.NewAuditUsecase(auditRepo)
	if err != nil {
		t.Fatal(err)
	}
	seedInit, err := biz.NewSeedInitializer(&adminconfpb.Admin{
		BootstrapAdminEmail: proto.String(adminBootstrapEml),
	}, iamRepo, seedRepo, logger)
	if err != nil {
		t.Fatal(err)
	}
	startupInit, err := startup.NewInitializer(seedInit, logger)
	if err != nil {
		t.Fatal(err)
	}
	sessionAuthn, err := authnsession.NewUserAuthenticator(sessionMgr)
	if err != nil {
		t.Fatal(err)
	}
	authSvc, err := service.NewAuthService(authUC, seedInit, oidcCfg, sessionMgr, logger)
	if err != nil {
		t.Fatal(err)
	}
	userSvc, err := service.NewUserService(userUC)
	if err != nil {
		t.Fatal(err)
	}
	auditSvc, err := service.NewAuditService(auditUC)
	if err != nil {
		t.Fatal(err)
	}

	httpServer, err := NewHTTPServer(
		&corepb.Server{Http: &corepb.Server_HTTP{Listen: &corepb.Server_Listen{Timeout: durationpb.New(15 * time.Second)}}},
		obsCfg,
		&corepb.App{Name: "admin.service", ExternalUrl: adminOrigin},
		nil,
		sessionMgr,
		sessionAuthn,
		realAuditor,
		authorizer,
		authSvc,
		userSvc,
		auditSvc,
		logger,
	)
	if err != nil {
		t.Fatal(err)
	}
	appInfo := kratos.New(kratos.Name("admin.service"), kratos.Version("dev"))
	listener.Config.BaseContext = func(net.Listener) context.Context {
		return kratos.NewContext(context.Background(), appInfo)
	}
	listener.Config.Handler = httpServer
	listener.Start()

	return &adminInstance{
		server:  listener,
		origin:  adminOrigin,
		manager: sessionMgr,
		seed:    seedInit,
		init:    startupInit,
		logs:    logs,
	}
}

func TestCrossServiceAuditEndToEnd(t *testing.T) {
	iamDSN := prepareIsolatedSchema(t, "IAM_TEST_POSTGRES_DSN", defaultIAMDSN, "iam_aud_e2e")
	adminDSN := prepareIsolatedSchema(t, "ADMIN_DATABASE_SOURCE", defaultAdminDSN, "admin_aud_e2e")
	kafkaBroker, auditTopic, chAddr, chUser, chPass, chDB := prepareAuditInfra(t)
	fgaURL, storeID, modelID, fgaSDK := createTempOpenFGAStore(t)

	// Grant service:admin read_events on audit:global (matching docs/iam.md).
	if _, err := fgaSDK.Write(t.Context()).Body(fgaclient.ClientWriteRequest{
		Writes: []fgaclient.ClientTupleKey{
			{User: "service:admin", Relation: "read_events", Object: "audit:global"},
		},
	}).Execute(); err != nil {
		t.Fatalf("grant service:admin read_events: %v", err)
	}

	// Enable W3C TraceContext propagation in-process so Admin HTTP -> gRPC -> IAM shares trace_id!
	prevTP := otel.GetTracerProvider()
	prevProp := otel.GetTextMapPropagator()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSampler(sdktrace.AlwaysSample()))
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.TraceContext{})
	t.Cleanup(func() {
		_ = tp.Shutdown(context.Background())
		otel.SetTracerProvider(prevTP)
		otel.SetTextMapPropagator(prevProp)
	})

	traceSinkAddr := startSilentSinkListener(t)
	smtp := startMockSMTPServer(t)
	iamBin := buildIAMBinary(t)
	auditBin := buildAuditBinary(t)

	adminListener := httptest.NewUnstartedServer(nil)
	t.Cleanup(adminListener.Close)
	adminOrigin := "http://" + adminListener.Listener.Addr().String()

	iamHTTPAddr := reserveTCPAddr(t)
	iamGRPCAddr := reserveTCPAddr(t)
	iamOrigin, initialPassword := startRealIAMWithAudit(
		t, iamBin, iamHTTPAddr, iamGRPCAddr, iamDSN,
		fgaURL, storeID, modelID, smtp.addr,
		adminOrigin+"/v1/admin/auth/callback",
		kafkaBroker, auditTopic, traceSinkAddr,
	)

	auditHTTPAddr := reserveTCPAddr(t)
	auditGRPCAddr := reserveTCPAddr(t)
	stopAudit := startRealAuditProcess(
		t, auditBin, auditHTTPAddr, auditGRPCAddr, iamOrigin,
		fgaURL, storeID, modelID,
		kafkaBroker, auditTopic,
		chAddr, chUser, chPass, chDB,
	)

	adminDataCfg := &corepb.Data{Database: &corepb.Data_Database{Driver: "pgx", Source: adminDSN}}
	adminDB, closeAdminDB, err := data.NewSQLDB(adminDataCfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(closeAdminDB)

	adminApp := buildAuditEnabledAdminInstance(
		t, adminDB, adminDataCfg,
		iamOrigin, iamGRPCAddr, auditGRPCAddr,
		fgaURL, storeID, modelID,
		kafkaBroker, auditTopic, traceSinkAddr,
		adminListener,
	)
	if err := adminApp.init.Initialize(t.Context()); err != nil {
		t.Fatal(err)
	}

	noRedirect := &http.Client{
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}

	// 1. Unauthenticated GET /v1/admin/audit/events -> 401 UNAUTHENTICATED.
	unauthAuditRes, _ := doJSON(t, http.MethodGet, adminOrigin+"/v1/admin/audit/events", adminOrigin, "")
	if unauthAuditRes.StatusCode != http.StatusUnauthorized {
		t.Fatalf("unauthenticated ListEvents status = %d, want 401", unauthAuditRes.StatusCode)
	}

	// 2. Failed IAM login (wrong password) + Auth Flow CompleteUpdatePassword + Admin OIDC browser login.
	const wrongPass = "{{SECRET_0008wrongpass}}"
	badFlowRes, badFlowRaw := doJSON(t, http.MethodPost, iamOrigin+"/v1/iam/authn/flows", iamOrigin, "{}")
	if badFlowRes.StatusCode != http.StatusOK {
		t.Fatalf("CreateAuthFlow(wrong pass) status=%d body=%s", badFlowRes.StatusCode, string(badFlowRaw))
	}
	var badFlowPayload struct {
		Flow struct {
			FlowID string `json:"flowId"`
		} `json:"flow"`
	}
	if err := json.Unmarshal(badFlowRaw, &badFlowPayload); err != nil {
		t.Fatal(err)
	}
	badOwnerCookie := cookieByName(badFlowRes.Cookies(), "iam_session")
	badLoginRes, _ := doJSON(t, http.MethodPost, iamOrigin+"/v1/iam/authn/flows/"+badFlowPayload.Flow.FlowID+":submitPassword", iamOrigin,
		fmt.Sprintf(`{"email":%q,"password":%q}`, adminBootstrapEml, wrongPass), badOwnerCookie)
	if badLoginRes.StatusCode != http.StatusUnauthorized {
		t.Fatalf("wrong password SubmitPassword status = %d, want 401", badLoginRes.StatusCode)
	}

	// Trigger OIDC login before IAM session exists -> unauthenticated /authorize/callback 302 /login must NOT emit noise.
	loginStartRes, err := noRedirect.Get(adminOrigin + "/v1/admin/auth/login")
	if err != nil {
		t.Fatal(err)
	}
	_ = loginStartRes.Body.Close()
	handshakeCookie := cookieByName(loginStartRes.Cookies(), "admin_oidc_handshake")
	iamAuthRes, err := noRedirect.Get(loginStartRes.Header.Get("Location"))
	if err != nil {
		t.Fatal(err)
	}
	_ = iamAuthRes.Body.Close()
	cbURL := iamAuthRes.Header.Get("Location")
	if !strings.HasPrefix(cbURL, "http") {
		cbURL = iamOrigin + cbURL
	}
	unauthIAMCBRes, err := noRedirect.Get(cbURL)
	if err != nil {
		t.Fatal(err)
	}
	_ = unauthIAMCBRes.Body.Close()
	if unauthIAMCBRes.StatusCode != http.StatusFound || !strings.Contains(unauthIAMCBRes.Header.Get("Location"), "/login?") {
		t.Fatalf("expected 302 /login redirect on unauthenticated IAM callback, got %d %q", unauthIAMCBRes.StatusCode, unauthIAMCBRes.Header.Get("Location"))
	}

	// Complete initial password change on IAM to obtain IAM session cookie.
	iamCookie := performIAMAuthFlowLogin(t, iamOrigin, adminBootstrapEml, initialPassword, adminNewPassword)

	// Complete OIDC callback on IAM & Admin.
	iamCBReq, _ := http.NewRequest(http.MethodGet, cbURL, nil)
	iamCBReq.AddCookie(iamCookie)
	iamCBRes, err := noRedirect.Do(iamCBReq)
	if err != nil {
		t.Fatal(err)
	}
	_ = iamCBRes.Body.Close()
	adminCBReq, _ := http.NewRequest(http.MethodGet, iamCBRes.Header.Get("Location"), nil)
	if handshakeCookie != nil {
		adminCBReq.AddCookie(handshakeCookie)
	}
	adminCBRes, err := noRedirect.Do(adminCBReq)
	if err != nil {
		t.Fatal(err)
	}
	_ = adminCBRes.Body.Close()
	adminCookie := cookieByName(adminCBRes.Cookies(), "admin_session")
	if adminCookie == nil {
		t.Fatalf("missing admin_session cookie after callback (status=%d loc=%q)", adminCBRes.StatusCode, adminCBRes.Header.Get("Location"))
	}

	// 3. Read-only endpoints (GetSession, ListUsers) must succeed and produce zero audit events.
	if sRes, _ := doJSON(t, http.MethodGet, adminOrigin+"/v1/admin/auth/session", adminOrigin, "", adminCookie); sRes.StatusCode != http.StatusOK {
		t.Fatalf("GetSession status = %d", sRes.StatusCode)
	}
	if lRes, _ := doJSON(t, http.MethodGet, adminOrigin+"/v1/admin/users", adminOrigin, "", adminCookie); lRes.StatusCode != http.StatusOK {
		t.Fatalf("ListUsers status = %d", lRes.StatusCode)
	}

	// 4. Cross-service mutations: CreateUser, UpdateUser, SetUserPassword, plus non-admin 403 rejection.
	const (
		bobPass      = "{{SECRET_0009bobinit}}"
		bobResetPass = "{{SECRET_0010bobreset}}"
	)
	createRes, createRaw := doJSON(t, http.MethodPost, adminOrigin+"/v1/admin/users", adminOrigin,
		fmt.Sprintf(`{"user":{"email":"bob-audit@plateau.local","profile":{"nickname":"BobAudit"}},"password":%q}`, bobPass),
		adminCookie)
	if createRes.StatusCode != http.StatusOK {
		t.Fatalf("CreateUser status=%d body=%s", createRes.StatusCode, string(createRaw))
	}
	createdBob := new(adminuserpb.CreateUserResponse)
	if err := protojson.Unmarshal(createRaw, createdBob); err != nil {
		t.Fatal(err)
	}
	bobName := createdBob.GetUser().GetName()

	patchRes, patchRaw := doJSON(t, http.MethodPatch, adminOrigin+"/v1/admin/"+bobName+"?updateMask=profile.nickname", adminOrigin,
		fmt.Sprintf(`{"name":%q,"etag":%q,"profile":{"nickname":"BobUpdated"}}`, bobName, createdBob.GetUser().GetEtag()),
		adminCookie)
	if patchRes.StatusCode != http.StatusOK {
		t.Fatalf("UpdateUser status=%d body=%s", patchRes.StatusCode, string(patchRaw))
	}

	setPwdRes, setPwdRaw := doJSON(t, http.MethodPost, adminOrigin+"/v1/admin/"+bobName+":setPassword", adminOrigin,
		fmt.Sprintf(`{"newPassword":%q}`, bobResetPass),
		adminCookie)
	if setPwdRes.StatusCode != http.StatusOK {
		t.Fatalf("SetUserPassword status=%d body=%s", setPwdRes.StatusCode, string(setPwdRaw))
	}

	// Forge a non-admin SCS session on Admin and verify GET /v1/admin/audit/events returns 403 and emits authz denied event.
	bobEmail := "bob-audit@plateau.local"
	seedBobW := httptest.NewRecorder()
	httpsession.LoadAndSave(adminApp.manager)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if saveErr := authnsession.SaveUser(r.Context(), adminApp.manager, &userpb.User{
			Name:   bobName,
			UserId: createdBob.GetUser().GetUserId(),
			Email:  &bobEmail,
		}); saveErr != nil {
			t.Errorf("SaveUser(bob): %v", saveErr)
		}
		w.WriteHeader(http.StatusNoContent)
	})).ServeHTTP(seedBobW, httptest.NewRequest(http.MethodGet, adminOrigin+"/", nil))
	nonAdminCookie := cookieByName(seedBobW.Result().Cookies(), "admin_session")
	if nonAdminCookie == nil {
		t.Fatal("failed to mint non-admin session cookie")
	}
	deniedRes, _ := doJSON(t, http.MethodGet, adminOrigin+"/v1/admin/audit/events", adminOrigin, "", nonAdminCookie)
	if deniedRes.StatusCode != http.StatusForbidden {
		t.Fatalf("non-admin ListEvents status = %d, want 403", deniedRes.StatusCode)
	}

	// 5. Poll GET /v1/admin/audit/events until Kafka -> Audit -> ClickHouse flushes all expected events.
	var allEvents []*auditeventpb.Event
	pollDeadline := time.Now().Add(12 * time.Second)
	for time.Now().Before(pollDeadline) {
		listRes, listRaw := doJSON(t, http.MethodGet, adminOrigin+"/v1/admin/audit/events?pageSize=100", adminOrigin, "", adminCookie)
		if listRes.StatusCode == http.StatusOK {
			payload := new(adminauditpb.ListEventsResponse)
			if err := protojson.Unmarshal(listRaw, payload); err == nil {
				allEvents = payload.GetEvents()
				hasAdminUpdate := false
				hasIAMUpdate := false
				hasAuthzDenied := false
				for _, ev := range allEvents {
					if ev.GetOperation() == "/plateau.admin.user.v1.UserService/UpdateUser" && ev.GetService() == "//admin.service" {
						hasAdminUpdate = true
					}
					if ev.GetOperation() == "/plateau.iam.user.v1.UserService/UpdateUser" && ev.GetService() == "//iam.service" {
						hasIAMUpdate = true
					}
					if ev.GetOperation() == "/plateau.admin.audit.v1.AuditService/ListEvents" && ev.GetEventType() == "plateau.security.authz.denied.v1" {
						hasAuthzDenied = true
					}
				}
				if hasAdminUpdate && hasIAMUpdate && hasAuthzDenied {
					break
				}
			}
		}
		time.Sleep(150 * time.Millisecond)
	}
	if len(allEvents) == 0 {
		t.Fatal("no audit events returned from GET /v1/admin/audit/events")
	}

	// Verify AC-P1 & AC-P2 across persisted ClickHouse events:
	var adminUpdateEv, iamUpdateEv *auditeventpb.Event
	var hasOIDCAuthorize, hasAdminCallback, hasFailedLogin bool
	for _, ev := range allEvents {
		// Read-only RPCs and successful token exchanges must NEVER appear!
		switch ev.GetOperation() {
		case "/plateau.admin.user.v1.UserService/ListUsers",
			"/plateau.admin.user.v1.UserService/GetUser",
			"/plateau.iam.user.v1.UserService/ListUsers",
			"/plateau.iam.user.v1.UserService/GetUser",
			"/plateau.admin.auth.v1.AuthService/GetSession",
			"/plateau.iam.account.v1.AccountService/GetProfile",
			"/plateau.iam.authn.v1.AuthnService/CreateAuthFlow",
			"/plateau.iam.authn.v1.AuthnService/GetAuthFlow",
			"POST /oauth/token":
			t.Fatalf("unexpected noise event persisted for read-only/token operation: %+v", ev)
		}
		if ev.GetOperation() == "GET /authorize/callback" && !ev.GetSuccess() {
			t.Fatalf("unauthenticated 302 /login redirect leaked as failed callback event: %+v", ev)
		}
		if ev.GetOperation() == "GET /authorize/callback" && ev.GetSuccess() {
			hasOIDCAuthorize = true
		}
		if ev.GetOperation() == "GET /v1/admin/auth/callback" && ev.GetSuccess() {
			hasAdminCallback = true
		}
		if ev.GetOperation() == "/plateau.iam.authn.v1.AuthnService/SubmitPassword" && !ev.GetSuccess() && ev.GetActorType() == "anonymous" {
			hasFailedLogin = true
		}
		if ev.GetOperation() == "/plateau.admin.user.v1.UserService/UpdateUser" && ev.GetService() == "//admin.service" {
			adminUpdateEv = ev
		}
		if ev.GetOperation() == "/plateau.iam.user.v1.UserService/UpdateUser" && ev.GetService() == "//iam.service" {
			iamUpdateEv = ev
		}

		// Sensitive credential zero-leakage check across every persisted event!
		rawEv, _ := protojson.Marshal(ev)
		for _, secret := range []string{
			initialPassword,
			adminNewPassword,
			wrongPass,
			bobPass,
			bobResetPass,
			oidcClientSecret,
			oidcCookieSecret,
			iamCookie.Value,
			adminCookie.Value,
		} {
			if secret != "" && strings.Contains(string(rawEv), secret) {
				t.Fatalf("secret %q leaked into persisted audit event: %s", secret, string(rawEv))
			}
		}
	}

	if !hasOIDCAuthorize || !hasAdminCallback || !hasFailedLogin {
		t.Fatalf("missing key authn audit events: oidcAuthorize=%v adminCallback=%v failedLogin=%v", hasOIDCAuthorize, hasAdminCallback, hasFailedLogin)
	}
	if adminUpdateEv == nil || iamUpdateEv == nil {
		t.Fatalf("missing cross-service UpdateUser events: admin=%v iam=%v", adminUpdateEv, iamUpdateEv)
	}
	if adminUpdateEv.GetActorType() != "human" || adminUpdateEv.GetActorId() == "" || adminUpdateEv.GetClientIp() == "" {
		t.Fatalf("unexpected Admin UpdateUser actor/ip: %+v", adminUpdateEv)
	}
	if iamUpdateEv.GetActorType() != "service" || iamUpdateEv.GetActorId() != "admin" {
		t.Fatalf("unexpected IAM UpdateUser actor: %+v", iamUpdateEv)
	}
	if adminUpdateEv.GetTraceId() == "" || adminUpdateEv.GetTraceId() != iamUpdateEv.GetTraceId() {
		t.Fatalf("cross-service trace_id mismatch: admin=%q iam=%q", adminUpdateEv.GetTraceId(), iamUpdateEv.GetTraceId())
	}

	// Verify traceId filter and GetEvent detail endpoint.
	traceFilterURL := adminOrigin + "/v1/admin/audit/events?traceId=" + url.QueryEscape(adminUpdateEv.GetTraceId())
	traceRes, traceRaw := doJSON(t, http.MethodGet, traceFilterURL, adminOrigin, "", adminCookie)
	if traceRes.StatusCode != http.StatusOK {
		t.Fatalf("ListEvents(traceId) status=%d body=%s", traceRes.StatusCode, string(traceRaw))
	}
	tracePayload := new(adminauditpb.ListEventsResponse)
	if err := protojson.Unmarshal(traceRaw, tracePayload); err != nil {
		t.Fatal(err)
	}
	if len(tracePayload.GetEvents()) < 2 {
		t.Fatalf("expected >= 2 correlated events for trace_id %q, got %d", adminUpdateEv.GetTraceId(), len(tracePayload.GetEvents()))
	}

	getRes, getRaw := doJSON(t, http.MethodGet, adminOrigin+"/v1/admin/audit/"+adminUpdateEv.GetName(), adminOrigin, "", adminCookie)
	if getRes.StatusCode != http.StatusOK {
		t.Fatalf("GetEvent(%s) status=%d body=%s", adminUpdateEv.GetName(), getRes.StatusCode, string(getRaw))
	}
	singleEv := new(auditeventpb.Event)
	if err := protojson.Unmarshal(getRaw, singleEv); err != nil || singleEv.GetEventId() != adminUpdateEv.GetEventId() {
		t.Fatalf("GetEvent unmarshal err=%v got=%+v", err, singleEv)
	}

	// 6. AC-P3: Remove service:admin read_events -> 503 DOWNSTREAM_UNAVAILABLE while preserving Admin browser session!
	if _, err := fgaSDK.Write(t.Context()).Body(fgaclient.ClientWriteRequest{
		Deletes: []fgaclient.ClientTupleKeyWithoutCondition{
			{User: "service:admin", Relation: "read_events", Object: "audit:global"},
		},
	}).Execute(); err != nil {
		t.Fatal(err)
	}
	degradedRes, degradedRaw := doJSON(t, http.MethodGet, adminOrigin+"/v1/admin/audit/events", adminOrigin, "", adminCookie)
	if degradedRes.StatusCode != http.StatusServiceUnavailable || !strings.Contains(string(degradedRaw), "ADMIN_AUDIT_ERROR_REASON_DOWNSTREAM_UNAVAILABLE") {
		t.Fatalf("expected 503 DOWNSTREAM_UNAVAILABLE when service:admin lacks read_events, got status=%d body=%s", degradedRes.StatusCode, string(degradedRaw))
	}
	if sessAfter503, _ := doJSON(t, http.MethodGet, adminOrigin+"/v1/admin/auth/session", adminOrigin, "", adminCookie); sessAfter503.StatusCode != http.StatusOK {
		t.Fatalf("Admin session was invalidated after downstream 403->503: status=%d", sessAfter503.StatusCode)
	}

	// Restore tuple -> 200 OK again.
	if _, err := fgaSDK.Write(t.Context()).Body(fgaclient.ClientWriteRequest{
		Writes: []fgaclient.ClientTupleKey{
			{User: "service:admin", Relation: "read_events", Object: "audit:global"},
		},
	}).Execute(); err != nil {
		t.Fatal(err)
	}
	if okAgain, _ := doJSON(t, http.MethodGet, adminOrigin+"/v1/admin/audit/events", adminOrigin, "", adminCookie); okAgain.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK after restoring service:admin read_events, got %d", okAgain.StatusCode)
	}

	// Stop audit.service -> 503 DOWNSTREAM_UNAVAILABLE and Admin session STILL 200 OK!
	stopAudit()
	downRes, downRaw := doJSON(t, http.MethodGet, adminOrigin+"/v1/admin/audit/events", adminOrigin, "", adminCookie)
	if downRes.StatusCode != http.StatusServiceUnavailable || !strings.Contains(string(downRaw), "ADMIN_AUDIT_ERROR_REASON_DOWNSTREAM_UNAVAILABLE") {
		t.Fatalf("expected 503 DOWNSTREAM_UNAVAILABLE when audit.service is down, got status=%d body=%s", downRes.StatusCode, string(downRaw))
	}
	if sessAfterDown, _ := doJSON(t, http.MethodGet, adminOrigin+"/v1/admin/auth/session", adminOrigin, "", adminCookie); sessAfterDown.StatusCode != http.StatusOK {
		t.Fatalf("Admin session was invalidated after audit.service shutdown: status=%d", sessAfterDown.StatusCode)
	}
}
