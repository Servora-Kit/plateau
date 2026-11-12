package server

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"database/sql"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	adminauthpb "github.com/Servora-Kit/plateau/api/gen/go/admin/auth/v1"
	adminconfpb "github.com/Servora-Kit/plateau/api/gen/go/admin/conf/v1"
	adminuserpb "github.com/Servora-Kit/plateau/api/gen/go/admin/user/v1"
	userpb "github.com/Servora-Kit/plateau/api/gen/go/iam/user/v1"
	openfgapb "github.com/Servora-Kit/plateau/api/gen/go/plateau/infra/openfga/v1"
	oidcpb "github.com/Servora-Kit/plateau/api/gen/go/plateau/security/oidc/v1"
	sessionpb "github.com/Servora-Kit/plateau/api/gen/go/plateau/security/session/v1"
	adminauthn "github.com/Servora-Kit/plateau/app/admin/service/internal/authn"
	"github.com/Servora-Kit/plateau/app/admin/service/internal/biz"
	"github.com/Servora-Kit/plateau/app/admin/service/internal/data"
	"github.com/Servora-Kit/plateau/app/admin/service/internal/service"
	"github.com/Servora-Kit/plateau/app/admin/service/internal/startup"
	"github.com/Servora-Kit/plateau/security/authz/openfga"
	corepb "github.com/Servora-Kit/servora/api/gen/go/servora/core/v1"
	"github.com/alexedwards/scs/v2"
	"github.com/google/uuid"
	fgaclient "github.com/openfga/go-sdk/client"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/durationpb"
)

const (
	defaultIAMDSN     = "postgres://postgres:postgres@127.0.0.1:5432/plateau_iam?sslmode=disable"
	defaultAdminDSN   = "postgres://postgres:postgres@127.0.0.1:5432/plateau_admin?sslmode=disable"
	defaultFGAURL     = "http://127.0.0.1:18080"
	defaultRedisAddr  = "127.0.0.1:6379"
	defaultRedisPass  = "servora_redis_pwd"
	adminBootstrapEml = "admin@plateau.local"
	adminNewPassword  = "AdminPermanentPass!2026"
	oidcClientSecret  = "integration-admin-oidc-secret-32b"
	oidcCookieSecret  = "integration-admin-cookie-secret-32b"
)

func buildIAMBinary(t *testing.T) string {
	t.Helper()
	binPath := filepath.Join(t.TempDir(), "iam-server")
	repoRoot, err := filepath.Abs("../../../../..")
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("go", "build", "-o", binPath, "./app/iam/service/cmd/server")
	cmd.Dir = repoRoot
	cmd.Env = append(os.Environ(), "CC=/usr/bin/clang")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("build iam server binary: %v\n%s", err, string(out))
	}
	return binPath
}

type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

type mockSMTPServer struct {
	addr         string
	failDelivery atomic.Bool

	mu       sync.Mutex
	messages []string
}

func startMockSMTPServer(t *testing.T) *mockSMTPServer {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	s := &mockSMTPServer{addr: ln.Addr().String()}
	t.Cleanup(func() { _ = ln.Close() })

	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go s.handleConn(conn)
		}
	}()
	return s
}

func (s *mockSMTPServer) handleConn(conn net.Conn) {
	defer func() { _ = conn.Close() }()
	_ = conn.SetDeadline(time.Now().Add(10 * time.Second))
	r := bufio.NewReader(conn)
	writeLine := func(line string) {
		_, _ = io.WriteString(conn, line+"\r\n")
	}
	writeLine("220 localhost ESMTP ready")
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return
		}
		upper := strings.ToUpper(strings.TrimSpace(line))
		switch {
		case strings.HasPrefix(upper, "EHLO"), strings.HasPrefix(upper, "HELO"):
			writeLine("250-localhost")
			writeLine("250 8BITMIME")
		case strings.HasPrefix(upper, "MAIL FROM:"), strings.HasPrefix(upper, "RCPT TO:"), strings.HasPrefix(upper, "RSET"), strings.HasPrefix(upper, "NOOP"):
			writeLine("250 2.1.0 OK")
		case upper == "DATA":
			writeLine("354 End data with <CR><LF>.<CR><LF>")
			var msg strings.Builder
			for {
				dl, err := r.ReadString('\n')
				if err != nil {
					return
				}
				if strings.TrimRight(dl, "\r\n") == "." {
					break
				}
				msg.WriteString(dl)
			}
			if s.failDelivery.Load() {
				writeLine("554 5.0.0 Simulated SMTP delivery failure")
				continue
			}
			s.mu.Lock()
			s.messages = append(s.messages, msg.String())
			s.mu.Unlock()
			writeLine("250 2.0.0 Queued")
		case strings.HasPrefix(upper, "QUIT"):
			writeLine("221 2.0.0 Bye")
			return
		default:
			writeLine("250 OK")
		}
	}
}

func (s *mockSMTPServer) messageCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.messages)
}

func (s *mockSMTPServer) latestVerifyToken(t *testing.T) string {
	t.Helper()
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.messages) == 0 {
		t.Fatal("expected at least one captured SMTP message")
	}
	raw := strings.ReplaceAll(s.messages[len(s.messages)-1], "=\r\n", "")
	raw = strings.ReplaceAll(raw, "=\n", "")
	m := regexp.MustCompile(`/verify-email#([A-Za-z0-9_-]+)`).FindStringSubmatch(raw)
	if len(m) < 2 {
		t.Fatalf("failed to extract verification token from email:\n%s", raw)
	}
	return m[1]
}

func reserveTCPAddr(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	_ = ln.Close()
	return addr
}

func prepareIsolatedSchema(t *testing.T, envName, fallbackDSN, prefix string) string {
	t.Helper()
	explicit := os.Getenv(envName)
	dsn := explicit
	if dsn == "" {
		dsn = fallbackDSN
	}
	cfg := &corepb.Data{Database: &corepb.Data_Database{Driver: "pgx", Source: dsn}}
	adminDB, closeAdmin, err := data.NewSQLDB(cfg)
	if err != nil {
		if explicit == "" {
			t.Skipf("%s not set and default local Postgres unreachable: %v", envName, err)
		}
		t.Fatalf("connect %s: %v", envName, err)
	}
	schema := prefix + "_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if _, err := adminDB.ExecContext(t.Context(), "CREATE SCHEMA "+schema); err != nil {
		closeAdmin()
		t.Fatalf("create schema %s: %v", schema, err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_, _ = adminDB.ExecContext(ctx, "DROP SCHEMA "+schema+" CASCADE")
		closeAdmin()
	})
	sep := "?"
	if strings.Contains(dsn, "?") {
		sep = "&"
	}
	return dsn + sep + "search_path=" + schema
}

func createTempOpenFGAStore(t *testing.T) (string, string, string, *fgaclient.OpenFgaClient) {
	t.Helper()
	fgaURL := os.Getenv("FGA_API_URL")
	if fgaURL == "" {
		fgaURL = defaultFGAURL
	}

	body := strings.NewReader(`{"name":"admin_e2e_` + strings.ReplaceAll(uuid.NewString(), "-", "") + `"}`)
	resp, err := http.Post(fgaURL+"/stores", "application/json", body)
	if err != nil {
		t.Fatalf("create temporary OpenFGA store at %s: %v", fgaURL, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode >= 300 {
		raw, _ := io.ReadAll(resp.Body)
		t.Fatalf("create OpenFGA store status=%d body=%s", resp.StatusCode, string(raw))
	}
	var created struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&created); err != nil || created.ID == "" {
		t.Fatalf("decode OpenFGA store response: %v", err)
	}
	storeID := created.ID
	t.Cleanup(func() {
		req, err := http.NewRequest(http.MethodDelete, fgaURL+"/stores/"+storeID, nil)
		if err == nil {
			res, err := http.DefaultClient.Do(req)
			if err == nil {
				_ = res.Body.Close()
			}
		}
	})

	modelPath, err := filepath.Abs("../../../../../manifests/openfga/fga.mod")
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("fga", "--api-url", fgaURL, "model", "write", "--store-id", storeID, "--file", modelPath)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("fga model write failed: %v\n%s", err, string(out))
	}
	var written struct {
		AuthorizationModelID string `json:"authorization_model_id"`
		AuthorizationModel   struct {
			ID string `json:"id"`
		} `json:"authorization_model"`
		ID string `json:"id"`
	}
	if err := json.Unmarshal(out, &written); err != nil {
		t.Fatalf("parse fga model write output: %v\n%s", err, string(out))
	}
	modelID := written.AuthorizationModelID
	if modelID == "" {
		modelID = written.AuthorizationModel.ID
	}
	if modelID == "" {
		modelID = written.ID
	}
	if modelID == "" {
		t.Fatalf("empty model ID from fga model write: %s", string(out))
	}

	sdk, err := fgaclient.NewSdkClient(&fgaclient.ClientConfiguration{
		ApiUrl:               fgaURL,
		StoreId:              storeID,
		AuthorizationModelId: modelID,
	})
	if err != nil {
		t.Fatal(err)
	}

	// Pre-seed IAM service tuples for service:admin (design.md §6.2.3).
	// Human admin tuple user:<id> admin admin:global is intentionally NOT pre-seeded so SeedInitializer is proven!
	if _, err := sdk.Write(t.Context()).Body(fgaclient.ClientWriteRequest{
		Writes: []fgaclient.ClientTupleKey{
			{User: "service:admin", Relation: "manage_users", Object: "iam:global"},
			{User: "service:admin", Relation: "expunge_users", Object: "iam:global"},
		},
	}).Execute(); err != nil {
		t.Fatalf("seed service:admin tuples: %v", err)
	}

	return fgaURL, storeID, modelID, sdk
}

func startRealIAMProcess(
	t *testing.T,
	iamBin string,
	iamHTTPAddr string,
	iamGRPCAddr string,
	iamDSN string,
	fgaURL, storeID, modelID string,
	smtpAddr string,
	adminCallbackURI string,
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
  metrics:
    enable: false
`, iamHTTPAddr, iamGRPCAddr, iamDSN, iamOrigin),
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
      audiences: [iam]
`, iamOrigin, signingPath, cryptoPath, oidcClientSecret, adminCallbackURI),
		"openfga.yaml": fmt.Sprintf("open_fga:\n  api_url: %q\n  store_id: %q\n  model_id: %q\n", fgaURL, storeID, modelID),
		"session.yaml": "session:\n  lifetime: 86400s\n  idle_timeout: 3600s\n  cookie:\n    name: iam_session\n    secure: false\n",
		"redis.yaml":   fmt.Sprintf("redis:\n  network: tcp\n  addr: %q\n  password: %q\n  db: 0\n", defaultRedisAddr, defaultRedisPass),
		"cap.yaml":     fmt.Sprintf("cap:\n  signing_secret: %q\n  redis_key_prefix: %q\n", "iam-test-cap-secret-at-least-32-bytes", "it_"+uuid.NewString()+":"),
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
		if cmd.Process != nil {
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
	})

	// Wait for IAM discovery endpoint & extract initial_password emitted once at bootstrap.
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

type adminInstance struct {
	server  *httptest.Server
	origin  string
	manager *scs.SessionManager
	seed    *biz.SeedInitializer
	init    *startup.Initializer
	logs    *syncBuffer
}

func buildAdminInstance(
	t *testing.T,
	adminDB *sql.DB,
	adminDataCfg *corepb.Data,
	iamOrigin string,
	iamGRPCAddr string,
	clientSecret string,
	fgaURL, storeID, modelID string,
	existingListener *httptest.Server,
) *adminInstance {
	t.Helper()
	logs := &syncBuffer{}
	logger := slog.New(slog.NewJSONHandler(logs, &slog.HandlerOptions{Level: slog.LevelDebug}))

	srv := existingListener
	if srv == nil {
		srv = httptest.NewUnstartedServer(nil)
		t.Cleanup(srv.Close)
	}
	adminOrigin := "http://" + srv.Listener.Addr().String()

	dialCfg := proto.Clone(adminDataCfg).(*corepb.Data)
	dialCfg.Client = &corepb.Data_Client{
		Services: []*corepb.Data_Client_Service{
			{
				Name: "iam.service",
				Endpoints: []*corepb.Data_Client_Endpoint{
					{
						Protocol: "grpc",
						Endpoint: iamGRPCAddr,
						Timeout:  durationpb.New(5 * time.Second),
					},
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
		ClientSecret: proto.String(clientSecret),
		RedirectUri:  adminOrigin + "/v1/admin/auth/callback",
		CookieSecret: oidcCookieSecret,
		Scopes:       []string{"openid", "profile", "email"},
		HttpTimeout:  durationpb.New(5 * time.Second),
	}

	dialer, closeDialer, err := data.NewDialer(dialCfg, nil, nil, oidcCfg, nil, logger)
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
	_ = openfgapb.OpenFGA{}

	d, err := data.NewData(dialer, entClient, fgaClient)
	if err != nil {
		t.Fatal(err)
	}
	iamRepo, closeIAMRepo, err := data.NewIAMUserRepo(d)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(closeIAMRepo)

	seedRepo, err := data.NewAdminSeedRepo(d)
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
	sessionAuthn, err := adminauthn.NewSessionAuthenticator(sessionMgr)
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

	httpServer, err := NewHTTPServer(
		&corepb.Server{Http: &corepb.Server_HTTP{Listen: &corepb.Server_Listen{Timeout: durationpb.New(15 * time.Second)}}},
		nil,
		&corepb.App{Name: "admin.service", ExternalUrl: adminOrigin},
		nil,
		sessionMgr,
		sessionAuthn,
		authorizer,
		authSvc,
		userSvc,
		logger,
	)
	if err != nil {
		t.Fatal(err)
	}
	srv.Config.Handler = httpServer
	srv.Start()

	return &adminInstance{
		server:  srv,
		origin:  adminOrigin,
		manager: sessionMgr,
		seed:    seedInit,
		init:    startupInit,
		logs:    logs,
	}
}

func doJSON(t *testing.T, method, fullURL, origin, body string, cookies ...*http.Cookie) (*http.Response, []byte) {
	t.Helper()
	var reader io.Reader
	if body != "" {
		reader = strings.NewReader(body)
	}
	req, err := http.NewRequest(method, fullURL, reader)
	if err != nil {
		t.Fatal(err)
	}
	if method != http.MethodGet && method != http.MethodHead {
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Origin", origin)
	}
	for _, c := range cookies {
		if c != nil {
			req.AddCookie(c)
		}
	}
	client := &http.Client{
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	res, err := client.Do(req)
	if err != nil {
		t.Fatalf("%s %s error: %v", method, fullURL, err)
	}
	defer func() { _ = res.Body.Close() }()
	raw, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}
	return res, raw
}

func cookieByName(cookies []*http.Cookie, name string) *http.Cookie {
	for _, c := range cookies {
		if c.Name == name {
			return c
		}
	}
	return nil
}

func performBrowserOIDCLogin(t *testing.T, adminOrigin, iamOrigin, returnTo string, iamSession *http.Cookie, preAdminCookie *http.Cookie) (*http.Response, []*http.Cookie) {
	t.Helper()
	noRedirect := &http.Client{
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	loginURL := adminOrigin + "/v1/admin/auth/login"
	if returnTo != "" {
		loginURL += "?return_to=" + url.QueryEscape(returnTo)
	}
	req1, _ := http.NewRequest(http.MethodGet, loginURL, nil)
	res1, err := noRedirect.Do(req1)
	if err != nil {
		t.Fatal(err)
	}
	_ = res1.Body.Close()
	if res1.StatusCode != http.StatusFound {
		t.Fatalf("BeginLogin status = %d, want 302", res1.StatusCode)
	}
	handshakeCookie := cookieByName(res1.Cookies(), "admin_oidc_handshake")
	if handshakeCookie == nil || handshakeCookie.Value == "" {
		t.Fatalf("missing admin_oidc_handshake cookie: %+v", res1.Cookies())
	}
	authURL, err := url.Parse(res1.Header.Get("Location"))
	if err != nil {
		t.Fatal(err)
	}
	if authURL.Query().Get("code_challenge_method") != "S256" || authURL.Query().Get("code_challenge") == "" || authURL.Query().Get("nonce") == "" || authURL.Query().Get("state") == "" {
		t.Fatalf("authorize redirect missing PKCE/nonce/state: %s", authURL.String())
	}

	// Step 2: GET IAM /authorize with IAM browser session cookie.
	req2, _ := http.NewRequest(http.MethodGet, authURL.String(), nil)
	if iamSession != nil {
		req2.AddCookie(iamSession)
	}
	res2, err := noRedirect.Do(req2)
	if err != nil {
		t.Fatal(err)
	}
	_ = res2.Body.Close()
	if res2.StatusCode != http.StatusFound {
		t.Fatalf("IAM /authorize status = %d, want 302", res2.StatusCode)
	}
	iamCallbackLoc := res2.Header.Get("Location")
	if strings.HasPrefix(iamCallbackLoc, "/") {
		iamCallbackLoc = iamOrigin + iamCallbackLoc
	}

	// Step 3: GET IAM /authorize/callback with IAM browser session cookie.
	req3, _ := http.NewRequest(http.MethodGet, iamCallbackLoc, nil)
	if iamSession != nil {
		req3.AddCookie(iamSession)
	}
	res3, err := noRedirect.Do(req3)
	if err != nil {
		t.Fatal(err)
	}
	_ = res3.Body.Close()
	if res3.StatusCode != http.StatusFound {
		t.Fatalf("IAM /authorize/callback status = %d, want 302 (loc=%s)", res3.StatusCode, iamCallbackLoc)
	}
	adminCallbackLoc := res3.Header.Get("Location")
	if !strings.HasPrefix(adminCallbackLoc, adminOrigin+"/v1/admin/auth/callback") {
		t.Fatalf("unexpected callback redirect location: %s", adminCallbackLoc)
	}

	// Step 4: GET Admin /v1/admin/auth/callback with handshake cookie (simulating cross-site redirect).
	req4, _ := http.NewRequest(http.MethodGet, adminCallbackLoc, nil)
	req4.Header.Set("Sec-Fetch-Site", "cross-site")
	req4.AddCookie(handshakeCookie)
	if preAdminCookie != nil {
		req4.AddCookie(preAdminCookie)
	}
	res4, err := noRedirect.Do(req4)
	if err != nil {
		t.Fatal(err)
	}
	_ = res4.Body.Close()
	return res4, res4.Cookies()
}

func assertErrorReason(t *testing.T, status int, raw []byte, wantStatus int, wantReason string) {
	t.Helper()
	if status != wantStatus {
		t.Fatalf("status = %d, want %d; body = %s", status, wantStatus, string(raw))
	}
	var payload struct {
		Reason string `json:"reason"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatalf("decode error JSON: %v; body = %s", err, string(raw))
	}
	if payload.Reason != wantReason {
		t.Fatalf("error reason = %q, want %q; body = %s", payload.Reason, wantReason, string(raw))
	}
}

func TestAdminEndToEndIntegration(t *testing.T) {
	iamDSN := prepareIsolatedSchema(t, "IAM_TEST_POSTGRES_DSN", defaultIAMDSN, "iam_e2e")
	adminDSN := prepareIsolatedSchema(t, "ADMIN_TEST_POSTGRES_DSN", defaultAdminDSN, "admin_e2e")
	fgaURL, storeID, modelID, fgaSDK := createTempOpenFGAStore(t)
	smtp := startMockSMTPServer(t)

	iamBin := buildIAMBinary(t)

	iamHTTPAddr := reserveTCPAddr(t)
	iamGRPCAddr := reserveTCPAddr(t)
	iamOrigin := "http://" + iamHTTPAddr

	adminUnstarted := httptest.NewUnstartedServer(nil)
	t.Cleanup(adminUnstarted.Close)
	adminOrigin := "http://" + adminUnstarted.Listener.Addr().String()

	adminDataCfg := &corepb.Data{Database: &corepb.Data_Database{Driver: "postgresql", Source: adminDSN}}
	adminDB, closeAdminDB, err := data.NewSQLDB(adminDataCfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(closeAdminDB)

	// Start Admin BEFORE IAM is running to prove non-blocking startup & lazy seed sync on OIDC callback (AC-B1)!
	adminApp := buildAdminInstance(t, adminDB, adminDataCfg, iamOrigin, iamGRPCAddr, oidcClientSecret, fgaURL, storeID, modelID, adminUnstarted)
	if err := adminApp.init.Initialize(t.Context()); err != nil {
		t.Fatalf("non-blocking startup Initialize failed while IAM was down: %v", err)
	}
	if adminApp.seed.Seeded() {
		t.Fatal("expected seed.Seeded() == false while IAM was unreachable at startup")
	}

	// Now start the real IAM server process.
	_, initialPassword := startRealIAMProcess(
		t,
		iamBin,
		iamHTTPAddr,
		iamGRPCAddr,
		iamDSN,
		fgaURL, storeID, modelID,
		smtp.addr,
		adminOrigin+"/v1/admin/auth/callback",
	)

	// Unauthenticated Admin requests must return 401 SECURITY_ERROR_REASON_UNAUTHENTICATED.
	res, raw := doJSON(t, http.MethodGet, adminOrigin+"/v1/admin/auth/session", adminOrigin, "")
	assertErrorReason(t, res.StatusCode, raw, http.StatusUnauthorized, "SECURITY_ERROR_REASON_UNAUTHENTICATED")
	res, raw = doJSON(t, http.MethodGet, adminOrigin+"/v1/admin/users", adminOrigin, "")
	assertErrorReason(t, res.StatusCode, raw, http.StatusUnauthorized, "SECURITY_ERROR_REASON_UNAUTHENTICATED")

	// Complete initial password change for bootstrap user on IAM to obtain IAM browser session.
	iamInitRes, iamInitBody := doJSON(t, http.MethodPost, iamOrigin+"/v1/iam/account/change-initial-password", iamOrigin, fmt.Sprintf(
		`{"email":%q,"currentPassword":%q,"newPassword":%q}`,
		adminBootstrapEml, initialPassword, adminNewPassword,
	))
	if iamInitRes.StatusCode != http.StatusOK {
		t.Fatalf("IAM ChangeInitialPassword status=%d body=%s", iamInitRes.StatusCode, string(iamInitBody))
	}
	adminIAMCookie := cookieByName(iamInitRes.Cookies(), "iam_session")
	if adminIAMCookie == nil || adminIAMCookie.Value == "" {
		t.Fatal("expected iam_session cookie after ChangeInitialPassword")
	}

	// AC-B1: Perform full browser OIDC Authorization Code + PKCE S256 + state + nonce login.
	// Callback lazily seeds user:<adminID> admin admin:global and creates Admin's local SCS session.
	cbRes, cbCookies := performBrowserOIDCLogin(t, adminOrigin, iamOrigin, "/users?tab=all", adminIAMCookie, nil)
	if cbRes.StatusCode != http.StatusFound || cbRes.Header.Get("Location") != "/users?tab=all" {
		t.Fatalf("Admin OIDC callback status=%d loc=%q, want 302 /users?tab=all\nadmin logs:\n%s", cbRes.StatusCode, cbRes.Header.Get("Location"), adminApp.logs.String())
	}
	if !adminApp.seed.Seeded() {
		t.Fatal("expected seed.Seeded() == true after lazy callback synchronization")
	}
	adminCookie := cookieByName(cbCookies, "admin_session")
	if adminCookie == nil || adminCookie.Value == "" {
		t.Fatalf("missing admin_session cookie on callback: %+v", cbCookies)
	}
	clearedHandshake := cookieByName(cbCookies, "admin_oidc_handshake")
	if clearedHandshake == nil || clearedHandshake.MaxAge >= 0 {
		t.Fatalf("handshake cookie was not deleted on callback: %+v", cbCookies)
	}

	// Verify GetSession returns AdminPrincipal & BatchCheck permissions ["access", "manage_users", "expunge_users"].
	sessRes, sessRaw := doJSON(t, http.MethodGet, adminOrigin+"/v1/admin/auth/session", adminOrigin, "", adminCookie)
	if sessRes.StatusCode != http.StatusOK {
		t.Fatalf("GetSession status=%d body=%s", sessRes.StatusCode, string(sessRaw))
	}
	sessionPayload := new(adminauthpb.GetSessionResponse)
	if err := protojson.Unmarshal(sessRaw, sessionPayload); err != nil {
		t.Fatal(err)
	}
	adminUserID := sessionPayload.GetUser().GetUserId()
	if adminUserID == "" || sessionPayload.GetUser().GetEmail() != adminBootstrapEml {
		t.Fatalf("unexpected GetSession user: %+v", sessionPayload.GetUser())
	}
	wantPerms := []string{"access", "manage_users", "expunge_users"}
	if !slices.Equal(sessionPayload.GetPermissions(), wantPerms) {
		t.Fatalf("GetSession permissions = %v, want %v", sessionPayload.GetPermissions(), wantPerms)
	}

	// AC-B1 / B2: Ensure-Seed must be non-destructive (existing other admin tuples remain untouched).
	if _, err := fgaSDK.Write(t.Context()).Body(fgaclient.ClientWriteRequest{
		Writes: []fgaclient.ClientTupleKey{{User: "user:preserved-admin", Relation: "admin", Object: "admin:global"}},
	}).Execute(); err != nil {
		t.Fatal(err)
	}
	secondAdmin := buildAdminInstance(t, adminDB, adminDataCfg, iamOrigin, iamGRPCAddr, oidcClientSecret, fgaURL, storeID, modelID, nil)
	if err := secondAdmin.seed.Ensure(t.Context()); err != nil {
		t.Fatalf("re-run Ensure seed failed: %v", err)
	}
	preservedCheck, err := fgaSDK.Check(t.Context()).Body(fgaclient.ClientCheckRequest{
		User: "user:preserved-admin", Relation: "admin", Object: "admin:global",
	}).Execute()
	if err != nil || !preservedCheck.GetAllowed() {
		t.Fatalf("Ensure-Seed clobbered existing tuple: allowed=%v err=%v", preservedCheck.GetAllowed(), err)
	}

	// AC-B2: IAM logout does NOT invalidate Admin's local session!
	iamLogoutRes, _ := doJSON(t, http.MethodPost, iamOrigin+"/v1/iam/sessions/current:logout", iamOrigin, "{}", adminIAMCookie)
	if iamLogoutRes.StatusCode != http.StatusOK {
		t.Fatalf("IAM logout status = %d", iamLogoutRes.StatusCode)
	}
	iamProfAfterLogout, _ := doJSON(t, http.MethodGet, iamOrigin+"/v1/iam/account/profile", iamOrigin, "", adminIAMCookie)
	if iamProfAfterLogout.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected IAM session to be 401 after IAM logout, got %d", iamProfAfterLogout.StatusCode)
	}
	sessStillOK, _ := doJSON(t, http.MethodGet, adminOrigin+"/v1/admin/auth/session", adminOrigin, "", adminCookie)
	if sessStillOK.StatusCode != http.StatusOK {
		t.Fatalf("Admin session was broken by IAM logout: status=%d", sessStillOK.StatusCode)
	}

	// Re-login on IAM, then test Admin Logout: destroys ONLY Admin session while IAM session stays valid!
	iamReloginRes, iamReloginRaw := doJSON(t, http.MethodPost, iamOrigin+"/v1/iam/authn/login", iamOrigin, fmt.Sprintf(
		`{"email":%q,"password":%q}`, adminBootstrapEml, adminNewPassword,
	))
	if iamReloginRes.StatusCode != http.StatusOK {
		t.Fatalf("IAM relogin status=%d body=%s", iamReloginRes.StatusCode, string(iamReloginRaw))
	}
	adminIAMCookie = cookieByName(iamReloginRes.Cookies(), "iam_session")

	// Cross-origin or non-JSON POST to /v1/admin/auth/logout is blocked by session.NewFilter (403 / 415).
	badOriginReq, _ := http.NewRequest(http.MethodPost, adminOrigin+"/v1/admin/auth/logout", strings.NewReader("{}"))
	badOriginReq.Header.Set("Content-Type", "application/json")
	badOriginReq.Header.Set("Origin", "https://evil.example")
	badOriginReq.AddCookie(adminCookie)
	badOriginRes, err := http.DefaultClient.Do(badOriginReq)
	if err != nil {
		t.Fatal(err)
	}
	_ = badOriginRes.Body.Close()
	if badOriginRes.StatusCode != http.StatusForbidden {
		t.Fatalf("cross-origin logout status = %d, want 403", badOriginRes.StatusCode)
	}

	adminLogoutRes, _ := doJSON(t, http.MethodPost, adminOrigin+"/v1/admin/auth/logout", adminOrigin, "{}", adminCookie)
	if adminLogoutRes.StatusCode != http.StatusOK {
		t.Fatalf("Admin logout status = %d", adminLogoutRes.StatusCode)
	}
	afterAdminLogout, _ := doJSON(t, http.MethodGet, adminOrigin+"/v1/admin/auth/session", adminOrigin, "", adminCookie)
	if afterAdminLogout.StatusCode != http.StatusUnauthorized {
		t.Fatalf("old Admin cookie still accepted after Logout: status=%d", afterAdminLogout.StatusCode)
	}
	iamProfStillOK, _ := doJSON(t, http.MethodGet, iamOrigin+"/v1/iam/account/profile", iamOrigin, "", adminIAMCookie)
	if iamProfStillOK.StatusCode != http.StatusOK {
		t.Fatalf("Admin logout unexpectedly revoked IAM session: status=%d", iamProfStillOK.StatusCode)
	}

	// Log back into Admin for subsequent management tests.
	cbRes2, cbCookies2 := performBrowserOIDCLogin(t, adminOrigin, iamOrigin, "/", adminIAMCookie, nil)
	if cbRes2.StatusCode != http.StatusFound {
		t.Fatalf("Admin OIDC relogin status = %d", cbRes2.StatusCode)
	}
	adminCookie = cookieByName(cbCookies2, "admin_session")

	// AC-B4: Two-step CreateUser:
	// Case 1: Default require_password_change (omitted -> UPDATE_PASSWORD required) + SMTP succeeds -> verificationEmailSent = true.
	bobInitialPass := "BobInitialPass!2026"
	createBobRes, createBobRaw := doJSON(t, http.MethodPost, adminOrigin+"/v1/admin/users", adminOrigin, fmt.Sprintf(
		`{"user":{"email":"bob@example.com","profile":{"name":"Bob Builder"}},"password":%q,"requirePasswordChange":false}`,
		bobInitialPass,
	), adminCookie)
	if createBobRes.StatusCode != http.StatusOK {
		t.Fatalf("CreateUser(bob) status=%d body=%s", createBobRes.StatusCode, string(createBobRaw))
	}
	createdBob := new(adminuserpb.CreateUserResponse)
	if err := protojson.Unmarshal(createBobRaw, createdBob); err != nil {
		t.Fatal(err)
	}
	if !createdBob.GetVerificationEmailSent() || createdBob.GetUser().GetName() == "" || len(createdBob.GetUser().GetRequiredActions()) != 0 {
		t.Fatalf("unexpected CreateUser(bob) response: %+v", createdBob)
	}
	bobName := createdBob.GetUser().GetName()
	bobUserID := createdBob.GetUser().GetUserId()
	bobVerifyToken := smtp.latestVerifyToken(t)

	// Activate Bob on IAM and confirm an ordinary IAM user is rejected at Admin OIDC callback (AC-B1).
	verifyBobRes, verifyBobRaw := doJSON(t, http.MethodPost, iamOrigin+"/v1/iam/account/verify-email", iamOrigin, fmt.Sprintf(`{"token":%q}`, bobVerifyToken))
	if verifyBobRes.StatusCode != http.StatusOK {
		t.Fatalf("IAM VerifyEmail(bob) status=%d body=%s", verifyBobRes.StatusCode, string(verifyBobRaw))
	}
	bobIAMLoginRes, _ := doJSON(t, http.MethodPost, iamOrigin+"/v1/iam/authn/login", iamOrigin, fmt.Sprintf(
		`{"email":"bob@example.com","password":%q}`, bobInitialPass,
	))
	if bobIAMLoginRes.StatusCode != http.StatusOK {
		t.Fatalf("IAM Login(bob) status = %d", bobIAMLoginRes.StatusCode)
	}
	bobIAMCookie := cookieByName(bobIAMLoginRes.Cookies(), "iam_session")

	bobCBRes, bobCBCookies := performBrowserOIDCLogin(t, adminOrigin, iamOrigin, "/", bobIAMCookie, nil)
	if bobCBRes.StatusCode != http.StatusFound || bobCBRes.Header.Get("Location") != "/auth/login?error=access_denied" {
		t.Fatalf("unauthorized user callback status=%d loc=%q, want 302 /auth/login?error=access_denied", bobCBRes.StatusCode, bobCBRes.Header.Get("Location"))
	}
	if c := cookieByName(bobCBCookies, "admin_session"); c != nil && c.Value != "" && c.MaxAge >= 0 {
		t.Fatalf("unauthorized user received non-empty admin_session cookie: %+v", c)
	}

	// Case 2: CreateUser with omitted requirePasswordChange (defaults to UPDATE_PASSWORD) + SMTP failure -> HTTP 200 & verificationEmailSent = false!
	smtp.failDelivery.Store(true)
	charliePass := "CharlieSecretPass!2026"
	createCharlieRes, createCharlieRaw := doJSON(t, http.MethodPost, adminOrigin+"/v1/admin/users", adminOrigin, fmt.Sprintf(
		`{"user":{"email":"charlie@example.com"},"password":%q}`,
		charliePass,
	), adminCookie)
	if createCharlieRes.StatusCode != http.StatusOK {
		t.Fatalf("CreateUser(charlie with SMTP down) status=%d body=%s", createCharlieRes.StatusCode, string(createCharlieRaw))
	}
	createdCharlie := new(adminuserpb.CreateUserResponse)
	if err := protojson.Unmarshal(createCharlieRaw, createdCharlie); err != nil {
		t.Fatal(err)
	}
	if createdCharlie.GetVerificationEmailSent() {
		t.Fatal("expected verificationEmailSent == false when SMTP failed")
	}
	if !slices.Contains(createdCharlie.GetUser().GetRequiredActions(), userpb.UserRequiredAction_USER_REQUIRED_ACTION_UPDATE_PASSWORD) {
		t.Fatalf("omitted requirePasswordChange did not default to UPDATE_PASSWORD: %v", createdCharlie.GetUser().GetRequiredActions())
	}
	charlieName := createdCharlie.GetUser().GetName()

	// Manual resend while SMTP is still failing -> 503 ADMIN_USER_ERROR_REASON_VERIFICATION_EMAIL_FAILED.
	resendFailRes, resendFailRaw := doJSON(t, http.MethodPost, adminOrigin+"/v1/admin/"+charlieName+":sendVerificationEmail", adminOrigin, "{}", adminCookie)
	assertErrorReason(t, resendFailRes.StatusCode, resendFailRaw, http.StatusServiceUnavailable, "ADMIN_USER_ERROR_REASON_VERIFICATION_EMAIL_FAILED")

	// Restore SMTP and resend -> 200 OK.
	smtp.failDelivery.Store(false)
	msgBefore := smtp.messageCount()
	resendOKRes, resendOKRaw := doJSON(t, http.MethodPost, adminOrigin+"/v1/admin/"+charlieName+":sendVerificationEmail", adminOrigin, "{}", adminCookie)
	if resendOKRes.StatusCode != http.StatusOK || smtp.messageCount() != msgBefore+1 {
		t.Fatalf("SendUserVerificationEmail status=%d body=%s", resendOKRes.StatusCode, string(resendOKRaw))
	}

	// Duplicate email CreateUser -> 409 ADMIN_USER_ERROR_REASON_USER_ALREADY_EXISTS and does NOT send email.
	msgBeforeDup := smtp.messageCount()
	dupRes, dupRaw := doJSON(t, http.MethodPost, adminOrigin+"/v1/admin/users", adminOrigin, fmt.Sprintf(
		`{"user":{"email":"bob@example.com"},"password":%q}`, bobInitialPass,
	), adminCookie)
	assertErrorReason(t, dupRes.StatusCode, dupRaw, http.StatusConflict, "ADMIN_USER_ERROR_REASON_USER_ALREADY_EXISTS")
	if smtp.messageCount() != msgBeforeDup {
		t.Fatal("failed CreateUser unexpectedly triggered verification email")
	}

	// AC-B3: UpdateUser 7-field profile whitelist & readonly email enforcement.
	badMaskRes, badMaskRaw := doJSON(t, http.MethodPatch, adminOrigin+"/v1/admin/"+bobName+"?updateMask=email", adminOrigin, `{"email":"hacked@example.com"}`, adminCookie)
	assertErrorReason(t, badMaskRes.StatusCode, badMaskRaw, http.StatusBadRequest, "INVALID_ARGUMENT")

	emptyMaskRes, emptyMaskRaw := doJSON(t, http.MethodPatch, adminOrigin+"/v1/admin/"+bobName, adminOrigin, `{"profile":{"name":"NoMask"}}`, adminCookie)
	assertErrorReason(t, emptyMaskRes.StatusCode, emptyMaskRaw, http.StatusBadRequest, "INVALID_ARGUMENT")

	curBobRes, curBobRaw := doJSON(t, http.MethodGet, adminOrigin+"/v1/admin/"+bobName, adminOrigin, "", adminCookie)
	if curBobRes.StatusCode != http.StatusOK {
		t.Fatalf("GetUser(bob before update) status=%d", curBobRes.StatusCode)
	}
	currentBob := new(userpb.User)
	if err := protojson.Unmarshal(curBobRaw, currentBob); err != nil {
		t.Fatal(err)
	}

	updRes, updRaw := doJSON(
		t,
		http.MethodPatch,
		adminOrigin+"/v1/admin/"+bobName+"?updateMask=profile.name,profile.nickname,profile.locale",
		adminOrigin,
		fmt.Sprintf(`{"etag":%q,"email":"ignored@example.com","profile":{"name":"Robert Builder","nickname":"Bobby","locale":"zh-CN"}}`, currentBob.GetEtag()),
		adminCookie,
	)
	if updRes.StatusCode != http.StatusOK {
		t.Fatalf("UpdateUser status=%d body=%s", updRes.StatusCode, string(updRaw))
	}
	updatedBob := new(userpb.User)
	if err := protojson.Unmarshal(updRaw, updatedBob); err != nil {
		t.Fatal(err)
	}
	if updatedBob.GetEmail() != "bob@example.com" || updatedBob.GetProfile().GetName() != "Robert Builder" || updatedBob.GetProfile().GetNickname() != "Bobby" || updatedBob.GetProfile().GetLocale() != "zh-CN" {
		t.Fatalf("unexpected updated Bob: %+v", updatedBob)
	}

	// AC-B3: RevokeUserSessions & SetUserPassword.
	revokeRes, _ := doJSON(t, http.MethodPost, adminOrigin+"/v1/admin/"+bobName+":revokeSessions", adminOrigin, "{}", adminCookie)
	if revokeRes.StatusCode != http.StatusOK {
		t.Fatalf("RevokeUserSessions status=%d", revokeRes.StatusCode)
	}
	bobProfRevoked, _ := doJSON(t, http.MethodGet, iamOrigin+"/v1/iam/account/profile", iamOrigin, "", bobIAMCookie)
	if bobProfRevoked.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected Bob IAM session to be 401 after RevokeUserSessions, got %d", bobProfRevoked.StatusCode)
	}

	bobResetPass := "BobResetPassword!2026"
	setPwdRes, setPwdRaw := doJSON(t, http.MethodPost, adminOrigin+"/v1/admin/"+bobName+":setPassword", adminOrigin, fmt.Sprintf(`{"newPassword":%q}`, bobResetPass), adminCookie)
	if setPwdRes.StatusCode != http.StatusOK {
		t.Fatalf("SetUserPassword status=%d body=%s", setPwdRes.StatusCode, string(setPwdRaw))
	}
	bobNewLogin, _ := doJSON(t, http.MethodPost, iamOrigin+"/v1/iam/authn/login", iamOrigin, fmt.Sprintf(`{"email":"bob@example.com","password":%q}`, bobResetPass))
	if bobNewLogin.StatusCode != http.StatusOK {
		t.Fatalf("Bob could not login with admin-reset password: status=%d", bobNewLogin.StatusCode)
	}

	// AC-B5: Self-protection & Fellow-admin protection BEFORE calling IAM.
	adminResourceName := "users/" + adminUserID
	selfDisRes, selfDisRaw := doJSON(t, http.MethodPost, adminOrigin+"/v1/admin/"+adminResourceName+":disable", adminOrigin, `{"etag":"any"}`, adminCookie)
	assertErrorReason(t, selfDisRes.StatusCode, selfDisRaw, http.StatusForbidden, "ADMIN_USER_ERROR_REASON_SELF_ACTION_FORBIDDEN")

	selfDelRes, selfDelRaw := doJSON(t, http.MethodDelete, adminOrigin+"/v1/admin/"+adminResourceName, adminOrigin, "", adminCookie)
	assertErrorReason(t, selfDelRes.StatusCode, selfDelRaw, http.StatusForbidden, "ADMIN_USER_ERROR_REASON_SELF_ACTION_FORBIDDEN")

	selfExpRes, selfExpRaw := doJSON(t, http.MethodPost, adminOrigin+"/v1/admin/"+adminResourceName+":expunge", adminOrigin, "{}", adminCookie)
	assertErrorReason(t, selfExpRes.StatusCode, selfExpRaw, http.StatusForbidden, "ADMIN_USER_ERROR_REASON_SELF_ACTION_FORBIDDEN")

	// Forged operator metadata/headers cannot bypass self-protection (AC-B5).
	forgedSelfReq, _ := http.NewRequest(http.MethodDelete, adminOrigin+"/v1/admin/"+adminResourceName, nil)
	forgedSelfReq.Header.Set("Content-Type", "application/json")
	forgedSelfReq.Header.Set("Origin", adminOrigin)
	forgedSelfReq.Header.Set("X-User-Id", bobUserID)
	forgedSelfReq.Header.Set("X-Actor-Id", bobUserID)
	forgedSelfReq.Header.Set("Grpc-Metadata-X-User-Id", bobUserID)
	forgedSelfReq.AddCookie(adminCookie)
	forgedSelfRes, err := http.DefaultClient.Do(forgedSelfReq)
	if err != nil {
		t.Fatal(err)
	}
	forgedSelfRaw, _ := io.ReadAll(forgedSelfRes.Body)
	_ = forgedSelfRes.Body.Close()
	assertErrorReason(t, forgedSelfRes.StatusCode, forgedSelfRaw, http.StatusForbidden, "ADMIN_USER_ERROR_REASON_SELF_ACTION_FORBIDDEN")
	// Grant Bob admin relation temporarily -> disabling/deleting/expunging Bob must return 403 TARGET_IS_ADMIN.
	if _, err := fgaSDK.Write(t.Context()).Body(fgaclient.ClientWriteRequest{
		Writes: []fgaclient.ClientTupleKey{{User: "user:" + bobUserID, Relation: "admin", Object: "admin:global"}},
	}).Execute(); err != nil {
		t.Fatal(err)
	}
	peerDisRes, peerDisRaw := doJSON(t, http.MethodPost, adminOrigin+"/v1/admin/"+bobName+":disable", adminOrigin, fmt.Sprintf(`{"etag":%q}`, updatedBob.GetEtag()), adminCookie)
	assertErrorReason(t, peerDisRes.StatusCode, peerDisRaw, http.StatusForbidden, "ADMIN_USER_ERROR_REASON_TARGET_IS_ADMIN")

	peerDelRes, peerDelRaw := doJSON(t, http.MethodDelete, adminOrigin+"/v1/admin/"+bobName, adminOrigin, "", adminCookie)
	assertErrorReason(t, peerDelRes.StatusCode, peerDelRaw, http.StatusForbidden, "ADMIN_USER_ERROR_REASON_TARGET_IS_ADMIN")

	// Remove Bob's admin tuple so normal user lifecycle operations can proceed on Bob.
	if _, err := fgaSDK.Write(t.Context()).Body(fgaclient.ClientWriteRequest{
		Deletes: []fgaclient.ClientTupleKeyWithoutCondition{{User: "user:" + bobUserID, Relation: "admin", Object: "admin:global"}},
	}).Execute(); err != nil {
		t.Fatal(err)
	}

	// AC-B3: DisableUser / EnableUser + ETag mismatch.
	etagErrRes, etagErrRaw := doJSON(t, http.MethodPost, adminOrigin+"/v1/admin/"+bobName+":disable", adminOrigin, `{"etag":"stale-etag"}`, adminCookie)
	assertErrorReason(t, etagErrRes.StatusCode, etagErrRaw, http.StatusConflict, "ADMIN_USER_ERROR_REASON_ETAG_MISMATCH")

	getBobRes, getBobRaw := doJSON(t, http.MethodGet, adminOrigin+"/v1/admin/"+bobName, adminOrigin, "", adminCookie)
	if getBobRes.StatusCode != http.StatusOK {
		t.Fatalf("GetUser(bob) status=%d", getBobRes.StatusCode)
	}
	_ = protojson.Unmarshal(getBobRaw, updatedBob)

	disBobRes, disBobRaw := doJSON(t, http.MethodPost, adminOrigin+"/v1/admin/"+bobName+":disable", adminOrigin, fmt.Sprintf(`{"etag":%q,"reason":"security audit"}`, updatedBob.GetEtag()), adminCookie)
	if disBobRes.StatusCode != http.StatusOK {
		t.Fatalf("DisableUser(bob) status=%d body=%s", disBobRes.StatusCode, string(disBobRaw))
	}
	disabledBob := new(userpb.User)
	_ = protojson.Unmarshal(disBobRaw, disabledBob)
	if disabledBob.GetStatus() != userpb.UserStatus_USER_STATUS_DISABLED {
		t.Fatalf("DisableUser status = %v, want DISABLED", disabledBob.GetStatus())
	}

	enBobRes, enBobRaw := doJSON(t, http.MethodPost, adminOrigin+"/v1/admin/"+bobName+":enable", adminOrigin, fmt.Sprintf(`{"etag":%q}`, disabledBob.GetEtag()), adminCookie)
	if enBobRes.StatusCode != http.StatusOK {
		t.Fatalf("EnableUser(bob) status=%d body=%s", enBobRes.StatusCode, string(enBobRaw))
	}
	enabledBob := new(userpb.User)
	_ = protojson.Unmarshal(enBobRaw, enabledBob)
	if enabledBob.GetStatus() != userpb.UserStatus_USER_STATUS_ACTIVE {
		t.Fatalf("EnableUser status = %v, want ACTIVE", enabledBob.GetStatus())
	}

	// AC-B3 & AC-B6: Expunging active user fails with 409 INVALID_STATE; Soft-delete -> Trash query -> Undelete -> Soft-delete -> Expunge.
	expActiveRes, expActiveRaw := doJSON(t, http.MethodPost, adminOrigin+"/v1/admin/"+bobName+":expunge", adminOrigin, "{}", adminCookie)
	assertErrorReason(t, expActiveRes.StatusCode, expActiveRaw, http.StatusConflict, "ADMIN_USER_ERROR_REASON_INVALID_STATE")

	delBobRes, delBobRaw := doJSON(t, http.MethodDelete, adminOrigin+"/v1/admin/"+bobName, adminOrigin, "", adminCookie)
	if delBobRes.StatusCode != http.StatusOK {
		t.Fatalf("DeleteUser(bob) status=%d body=%s", delBobRes.StatusCode, string(delBobRaw))
	}
	// Second soft-delete returns 404 USER_NOT_FOUND.
	delAgainRes, delAgainRaw := doJSON(t, http.MethodDelete, adminOrigin+"/v1/admin/"+bobName, adminOrigin, "", adminCookie)
	assertErrorReason(t, delAgainRes.StatusCode, delAgainRaw, http.StatusNotFound, "ADMIN_USER_ERROR_REASON_USER_NOT_FOUND")

	// Default ListUsers hides Bob; Trash query (showDeleted=true & filter=delete_time != null) includes Bob.
	activeListRes, activeListRaw := doJSON(t, http.MethodGet, adminOrigin+"/v1/admin/users?includeTotal=true", adminOrigin, "", adminCookie)
	if activeListRes.StatusCode != http.StatusOK {
		t.Fatalf("ListUsers status=%d", activeListRes.StatusCode)
	}
	activeList := new(adminuserpb.ListUsersResponse)
	_ = protojson.Unmarshal(activeListRaw, activeList)
	if activeList.TotalSize == nil || activeList.GetTotalSize() != 2 {
		t.Fatalf("ListUsers(includeTotal=true) totalSize = %v, want 2", activeList.TotalSize)
	}
	for _, u := range activeList.GetUsers() {
		if u.GetName() == bobName {
			t.Fatal("default ListUsers leaked soft-deleted Bob")
		}
	}

	trashURL := adminOrigin + "/v1/admin/users?showDeleted=true&filter=" + url.QueryEscape("delete_time != null")
	trashRes, trashRaw := doJSON(t, http.MethodGet, trashURL, adminOrigin, "", adminCookie)
	if trashRes.StatusCode != http.StatusOK {
		t.Fatalf("ListUsers(trash) status=%d body=%s", trashRes.StatusCode, string(trashRaw))
	}
	trashList := new(adminuserpb.ListUsersResponse)
	_ = protojson.Unmarshal(trashRaw, trashList)
	if len(trashList.GetUsers()) != 1 || trashList.GetUsers()[0].GetName() != bobName {
		t.Fatalf("unexpected trash list: %+v", trashList.GetUsers())
	}

	// Undelete Bob, then soft-delete again for Expunge verification.
	undelRes, undelRaw := doJSON(t, http.MethodPost, adminOrigin+"/v1/admin/"+bobName+":undelete", adminOrigin, "{}", adminCookie)
	if undelRes.StatusCode != http.StatusOK {
		t.Fatalf("UndeleteUser(bob) status=%d body=%s", undelRes.StatusCode, string(undelRaw))
	}
	delBob2Res, _ := doJSON(t, http.MethodDelete, adminOrigin+"/v1/admin/"+bobName, adminOrigin, "", adminCookie)
	if delBob2Res.StatusCode != http.StatusOK {
		t.Fatalf("second lifecycle DeleteUser(bob) status=%d", delBob2Res.StatusCode)
	}

	// AC-B6: Operator without admin:global expunge_users permission (even with valid session + forged headers) gets 403 PERMISSION_DENIED.
	if _, err := fgaSDK.Write(t.Context()).Body(fgaclient.ClientWriteRequest{
		Deletes: []fgaclient.ClientTupleKeyWithoutCondition{{User: "user:" + adminUserID, Relation: "admin", Object: "admin:global"}},
	}).Execute(); err != nil {
		t.Fatal(err)
	}
	humanDeniedReq, _ := http.NewRequest(http.MethodPost, adminOrigin+"/v1/admin/"+bobName+":expunge", strings.NewReader("{}"))
	humanDeniedReq.Header.Set("Content-Type", "application/json")
	humanDeniedReq.Header.Set("Origin", adminOrigin)
	humanDeniedReq.Header.Set("X-Permissions", "expunge_users")
	humanDeniedReq.AddCookie(adminCookie)
	humanDeniedRes, err := http.DefaultClient.Do(humanDeniedReq)
	if err != nil {
		t.Fatal(err)
	}
	humanDeniedRaw, _ := io.ReadAll(humanDeniedRes.Body)
	_ = humanDeniedRes.Body.Close()
	assertErrorReason(t, humanDeniedRes.StatusCode, humanDeniedRaw, http.StatusForbidden, "SECURITY_ERROR_REASON_PERMISSION_DENIED")
	if _, err := fgaSDK.Write(t.Context()).Body(fgaclient.ClientWriteRequest{
		Writes: []fgaclient.ClientTupleKey{{User: "user:" + adminUserID, Relation: "admin", Object: "admin:global"}},
	}).Execute(); err != nil {
		t.Fatal(err)
	}

	// AC-B6 & AC-B7: Remove service:admin expunge_users on iam:global -> IAM returns 403 PermissionDenied,
	// which Admin MUST translate to 503 DOWNSTREAM_UNAVAILABLE (never 401/403!) while keeping Admin session valid!
	if _, err := fgaSDK.Write(t.Context()).Body(fgaclient.ClientWriteRequest{
		Deletes: []fgaclient.ClientTupleKeyWithoutCondition{{User: "service:admin", Relation: "expunge_users", Object: "iam:global"}},
	}).Execute(); err != nil {
		t.Fatal(err)
	}
	svcDeniedRes, svcDeniedRaw := doJSON(t, http.MethodPost, adminOrigin+"/v1/admin/"+bobName+":expunge", adminOrigin, "{}", adminCookie)
	assertErrorReason(t, svcDeniedRes.StatusCode, svcDeniedRaw, http.StatusServiceUnavailable, "ADMIN_USER_ERROR_REASON_DOWNSTREAM_UNAVAILABLE")

	// Admin session is STILL valid!
	sessAfterSvcDenied, _ := doJSON(t, http.MethodGet, adminOrigin+"/v1/admin/auth/session", adminOrigin, "", adminCookie)
	if sessAfterSvcDenied.StatusCode != http.StatusOK {
		t.Fatalf("downstream 403 unexpectedly invalidated Admin session: status=%d", sessAfterSvcDenied.StatusCode)
	}

	// Restore service:admin expunge_users and verify ExpungeUser permanently deletes Bob!
	if _, err := fgaSDK.Write(t.Context()).Body(fgaclient.ClientWriteRequest{
		Writes: []fgaclient.ClientTupleKey{{User: "service:admin", Relation: "expunge_users", Object: "iam:global"}},
	}).Execute(); err != nil {
		t.Fatal(err)
	}
	expOKRes, expOKRaw := doJSON(t, http.MethodPost, adminOrigin+"/v1/admin/"+bobName+":expunge", adminOrigin, "{}", adminCookie)
	if expOKRes.StatusCode != http.StatusOK {
		t.Fatalf("ExpungeUser(bob) status=%d body=%s", expOKRes.StatusCode, string(expOKRaw))
	}
	getExpungedRes, getExpungedRaw := doJSON(t, http.MethodGet, adminOrigin+"/v1/admin/"+bobName, adminOrigin, "", adminCookie)
	assertErrorReason(t, getExpungedRes.StatusCode, getExpungedRaw, http.StatusNotFound, "ADMIN_USER_ERROR_REASON_USER_NOT_FOUND")

	// AC-B7: Misconfigured service client_secret -> 503 DOWNSTREAM_UNAVAILABLE, preserves Admin session, leaks zero secrets!
	badSecretVal := "wrong-client-secret-must-not-leak-32b"
	badSecretAdmin := buildAdminInstance(t, adminDB, adminDataCfg, iamOrigin, iamGRPCAddr, badSecretVal, fgaURL, storeID, modelID, nil)
	badCredRes, badCredRaw := doJSON(t, http.MethodGet, badSecretAdmin.origin+"/v1/admin/users", badSecretAdmin.origin, "", adminCookie)
	assertErrorReason(t, badCredRes.StatusCode, badCredRaw, http.StatusServiceUnavailable, "ADMIN_USER_ERROR_REASON_DOWNSTREAM_UNAVAILABLE")

	// Admin session remains valid on both instances!
	sessOnBadInst, _ := doJSON(t, http.MethodGet, badSecretAdmin.origin+"/v1/admin/auth/session", badSecretAdmin.origin, "", adminCookie)
	if sessOnBadInst.StatusCode != http.StatusOK {
		t.Fatalf("token failure invalidated Admin session: status=%d", sessOnBadInst.StatusCode)
	}

	// Verify log redaction: neither user passwords nor client/cookie secrets appear in Admin logs!
	allLogs := adminApp.logs.String() + "\n" + badSecretAdmin.logs.String()
	for _, secret := range []string{
		oidcClientSecret,
		oidcCookieSecret,
		badSecretVal,
		bobInitialPass,
		bobResetPass,
		charliePass,
	} {
		if strings.Contains(allLogs, secret) {
			t.Fatalf("sensitive credential %q leaked into Admin structured logs!", secret)
		}
	}
}
