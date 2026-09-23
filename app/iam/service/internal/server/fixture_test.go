package server

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"database/sql"
	"encoding/pem"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	oidcpb "github.com/Servora-Kit/plateau/api/gen/go/iam/oidc/conf/v1"
	userpb "github.com/Servora-Kit/plateau/api/gen/go/iam/user/v1"
	cappb "github.com/Servora-Kit/plateau/api/gen/go/plateau/security/cap/v1"
	sessionpb "github.com/Servora-Kit/plateau/api/gen/go/plateau/security/session/v1"
	iamauthn "github.com/Servora-Kit/plateau/app/iam/service/internal/authn"
	iamauthz "github.com/Servora-Kit/plateau/app/iam/service/internal/authz"
	"github.com/Servora-Kit/plateau/app/iam/service/internal/biz"
	"github.com/Servora-Kit/plateau/app/iam/service/internal/data"
	entmodel "github.com/Servora-Kit/plateau/app/iam/service/internal/data/ent"
	"github.com/Servora-Kit/plateau/app/iam/service/internal/oidc"
	"github.com/Servora-Kit/plateau/app/iam/service/internal/service"
	"github.com/Servora-Kit/plateau/security/cap"
	"github.com/Servora-Kit/plateau/security/password"
	corepb "github.com/Servora-Kit/servora/api/gen/go/servora/core/v1"
	"github.com/Servora-Kit/servora/core/bootstrap"
	"github.com/alexedwards/scs/v2"
	kgrpc "github.com/go-kratos/kratos/v3/transport/grpc"
	khttp "github.com/go-kratos/kratos/v3/transport/http"
	"github.com/google/uuid"
	fgaclient "github.com/openfga/go-sdk/client"
	"github.com/redis/go-redis/v9"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/durationpb"
)

const testPassword = "correct horse battery staple"
const serviceSecret = "integration-service-secret-at-least-32-bytes"

type serverFixture struct {
	db          *sql.DB
	ent         *entmodel.Client
	manager     *scs.SessionManager
	users       biz.UserRepo
	logins      biz.SessionRepo
	resets      biz.PasswordResetTokenRepo
	user        *userpb.User
	http        *khttp.Server
	grpc        *kgrpc.Server
	web         *httptest.Server
	fga         *fgaclient.OpenFgaClient
	oidc        *oidc.OIDCStorage
	config      *oidcpb.OIDC
	initializer *oidc.OIDCInitializer
	redis       *redis.Client
	capPrefix   string
	mailer      *testMailer
}

// The tests borrow Plateau Compose services and own only their schema/store.
func newServerFixture(t *testing.T) *serverFixture {
	t.Helper()
	dsn := os.Getenv("IAM_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("IAM_TEST_POSTGRES_DSN is required")
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	dbConfig := &corepb.Data{Database: &corepb.Data_Database{Driver: "pgx", Source: dsn}}
	admin, closeAdmin, err := data.NewSQLDB(dbConfig)
	check(t, err)
	schema := "server_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	_, err = admin.ExecContext(t.Context(), "CREATE SCHEMA "+schema)
	check(t, err)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_, err := admin.ExecContext(ctx, "DROP SCHEMA "+schema+" CASCADE")
		check(t, err)
		closeAdmin()
	})
	separator := "?"
	if strings.Contains(dsn, "?") {
		separator = "&"
	}
	dbConfig.Database.Source += separator + "search_path=" + schema
	db, closeDB, err := data.NewSQLDB(dbConfig)
	check(t, err)
	t.Cleanup(closeDB)
	driver, err := data.NewEntDriver(dbConfig, db)
	check(t, err)
	client, closeEnt, err := data.NewDBClient(driver)
	check(t, err)
	t.Cleanup(closeEnt)
	rdb := redis.NewClient(&redis.Options{Addr: "127.0.0.1:6379", Password: "servora_redis_pwd"})
	t.Cleanup(func() { _ = rdb.Close() })
	check(t, rdb.Ping(t.Context()).Err())
	fgaURL := os.Getenv("IAM_TEST_FGA_URL")
	if fgaURL == "" {
		t.Skip("IAM_TEST_FGA_URL is required")
	}
	fga, err := fgaclient.NewSdkClient(&fgaclient.ClientConfiguration{ApiUrl: fgaURL, StoreId: os.Getenv("IAM_TEST_FGA_STORE_ID"), AuthorizationModelId: os.Getenv("IAM_TEST_FGA_MODEL_ID")})
	check(t, err)
	d, err := data.NewData(client, rdb, fga, logger)
	check(t, err)
	users, err := data.NewUserRepository(d)
	check(t, err)
	passwords, err := data.NewCredentialRepository(d)
	check(t, err)
	logins, err := data.NewSessionRepository(d)
	check(t, err)
	resets, err := data.NewPasswordResetTokenRepository(d)
	check(t, err)
	verification, err := data.NewVerificationTokenRepo(d)
	check(t, err)
	tokens, err := data.NewOAuthRepository(d)
	check(t, err)
	sessions, err := biz.NewSessionUsecase(users, logins)
	check(t, err)
	authentication, err := biz.NewAuthenticationUsecase(users, passwords, sessions)
	check(t, err)
	manager, closeSession, err := data.NewHTTPSessionManager(&sessionpb.Session{Lifetime: durationpb.New(24 * time.Hour), IdleTimeout: durationpb.New(time.Hour), Cookie: &sessionpb.Cookie{Name: proto.String("__Host-iam_session")}}, db, client)
	check(t, err)
	t.Cleanup(closeSession)
	captcha, err := cap.New(&cappb.CAP{SigningSecret: proto.String("test-cap-signing-secret-at-least-32-bytes"), RedisKeyPrefix: proto.String(schema + ":")}, rdb)
	check(t, err)
	runtime := &bootstrap.Runtime{Bootstrap: &corepb.Bootstrap{App: &corepb.App{ExternalUrl: "http://localhost:10002"}}}
	mailer := &testMailer{}
	accountUC, err := biz.NewAccountUsecase(users, passwords, verification, resets, captcha, mailer, runtime)
	check(t, err)
	userUC, err := biz.NewUserUsecase(accountUC, users)
	check(t, err)
	authnService, err := service.NewAuthnService(authentication, manager)
	check(t, err)
	sessionService, err := service.NewSessionService(sessions, manager)
	check(t, err)
	accountService, err := service.NewAccountService(accountUC, manager)
	check(t, err)
	userService, err := service.NewUserService(userUC)
	check(t, err)
	sessionAuthn, err := iamauthn.NewSessionAuthenticator(sessions, manager)
	check(t, err)
	authorizer, err := iamauthz.NewOpenFGAAuthorizer(fga)
	check(t, err)
	web := httptest.NewUnstartedServer(nil)
	t.Cleanup(web.Close)
	config := newProtocolConfig(t, "http://"+web.Listener.Addr().String())
	storage, err := oidc.NewOIDCStorage(client, config, tokens)
	check(t, err)
	initializer, err := oidc.NewOIDCInitializer(config, storage)
	check(t, err)
	check(t, initializer.Initialize(t.Context()))
	provider, err := oidc.NewIAMProvider(config, storage, sessions, manager)
	check(t, err)
	verifier, err := oidc.NewJWTVerifier(storage)
	check(t, err)
	serviceAuthn, closeAuthn, err := iamauthn.NewServiceAuthenticator(config, verifier)
	check(t, err)
	t.Cleanup(closeAuthn)
	serverConfig := &corepb.Server{Http: &corepb.Server_HTTP{Listen: &corepb.Server_Listen{Timeout: durationpb.New(15 * time.Second)}}, Grpc: &corepb.Server_GRPC{Listen: &corepb.Server_Listen{Timeout: durationpb.New(15 * time.Second)}}}
	httpServer := NewHTTPServer(serverConfig, nil, nil, captcha, provider, manager, sessionAuthn, authorizer, authnService, sessionService, accountService, logger)
	web.Config.Handler = httpServer
	web.Start()
	grpcServer := NewGRPCServer(serverConfig, nil, nil, serviceAuthn, authorizer, userService, logger)
	email := "alice@example.com"
	hash, err := password.Hash(testPassword)
	check(t, err)
	person, err := users.Create(t.Context(), &userpb.User{UserId: uuid.NewString(), Email: &email}, hash, email)
	check(t, err)
	check(t, users.ActivateEmail(t.Context(), person.GetUserId(), time.Now()))
	person, err = users.Get(t.Context(), person.GetUserId())
	check(t, err)
	return &serverFixture{db: db, ent: client, manager: manager, users: users, logins: logins, resets: resets, user: person, http: httpServer, grpc: grpcServer, web: web, fga: fga, oidc: storage, config: config, initializer: initializer, redis: rdb, capPrefix: schema + ":", mailer: mailer}
}

func newProtocolConfig(t *testing.T, issuer string) *oidcpb.OIDC {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	check(t, err)
	dir := t.TempDir()
	signing := filepath.Join(dir, "signing.pem")
	check(t, os.WriteFile(signing, pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)}), 0600))
	crypto := filepath.Join(dir, "crypto.key")
	value := make([]byte, 32)
	_, err = rand.Read(value)
	check(t, err)
	check(t, os.WriteFile(crypto, value, 0600))
	return &oidcpb.OIDC{Issuer: proto.String(issuer), SigningKeyPath: proto.String(signing), CryptoKeyPath: proto.String(crypto), Clients: []*oidcpb.OAuthClient{{ClientId: proto.String("admin"), ClientSecret: proto.String(serviceSecret), AllowedGrantTypes: []string{"client_credentials"}, Audiences: []string{"iam"}}}}
}

type testMailer struct{ verification, reset []string }

func (m *testMailer) SendVerification(_ context.Context, _, link string, _ int) error {
	m.verification = append(m.verification, link)
	return nil
}
func (m *testMailer) SendPasswordReset(_ context.Context, _, link string, _ int) error {
	m.reset = append(m.reset, link)
	return nil
}

func check(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func (f *serverFixture) request(method, path, body string, cookie *http.Cookie) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, f.web.URL+path, strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	if cookie != nil {
		r.AddCookie(cookie)
	}
	w := httptest.NewRecorder()
	f.http.ServeHTTP(w, r)
	return w
}

func (f *serverFixture) login(t *testing.T, password string, previous *http.Cookie) *http.Cookie {
	t.Helper()
	w := f.request("POST", "/v1/iam/authn/login", `{"email":"alice@example.com","password":"`+password+`"}`, previous)
	if w.Code != http.StatusOK {
		t.Fatalf("login status=%d body=%s", w.Code, w.Body)
	}
	for _, cookie := range w.Result().Cookies() {
		if cookie.Name == f.manager.Cookie.Name {
			return cookie
		}
	}
	t.Fatal("login did not deliver session cookie")
	return nil
}
