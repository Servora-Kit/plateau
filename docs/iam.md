# IAM 认证与服务调用

IAM HTTP 提供登录、注销、账户与 OIDC 协议入口；UserService 管理操作只注册到 gRPC。浏览器使用 SCS Cookie，内部管理调用使用服务访问令牌和 OpenFGA 授权。

## 运行配置

本机读取 `app/iam/service/configs/local/`（默认通过本地 Traefik 暴露 RFC 6761 回环域名 `http://iam.plateau.localhost:8080`，免改 `/etc/hosts`），容器读取 `configs/docker/`。两者使用相同配置结构，数据库、Redis、邮件和 OpenFGA 地址分别指向本机端口或 Compose 服务名。非回环生产环境要求公开 HTTPS 入口可从调用方访问，并挂载 OIDC 签名密钥与提供方加密密钥。

```yaml
iam:
  bootstrap_user_email: initial@example.com
session:
  lifetime: 2592000s # 30 天
  idle_timeout: 604800s # 7 天
  cookie:
    name: __Host-iam_session
oidc:
  issuer: https://iam.example.com
  signing_key_path: /run/secrets/oidc-signing.pem
  crypto_key_path: /run/secrets/oidc-crypto.key
  clients:
    - client_id: admin
      client_secret: ${IAM_ADMIN_CLIENT_SECRET}
      allowed_grant_types: [client_credentials]
      audiences: [iam, audit]
```

访问令牌默认五分钟，可用 `oidc.service_access_token_ttl` 设置正时长，例如 `300s`。配置中的 Duration 遵循 Protobuf JSON 的秒格式。

### 双源 OAuth 客户端（`STATIC` 与 `DYNAMIC`）

IAM 同时支持配置文件引导客户端（`source=static`）与管理台在线注册客户端（`source=dynamic`）：

- **静态客户端对账**：`oidc.yaml` 允许配置 `0..N` 个引导客户端（如 `admin`）。启动期 `OIDCInitializer` 仅同步 `source=static` 记录：移除配置项时级联撤销该静态客户端关联的令牌会话后物理删除，保留全部 `source=dynamic` 客户端不变；若静态 `client_id` 与已有动态客户端冲突则拒绝启动。静态客户端在管理台只读展示，禁止在线修改、轮换密钥或删除。
- **动态客户端与多密钥轮换**：通过 IAM 内部 gRPC `plateau.iam.client.v1.OAuthClientService`（由 Admin BFF `/v1/admin/oauth-clients*` 代理）管理，支持 `WEB`（机密或公共）、`USER_AGENT`（SPA 强制 `none` + PKCE S256）与 `NATIVE`（允许自定义 Scheme 及 RFC 8252 本地回环动态端口回调）三类应用。机密客户端密钥采用 `pcs_` 前缀高熵随机串，库内仅存 SHA-256 摘要与前缀，单客户端最多并存 5 把密钥以支持零停机平滑轮换，且删除有效密钥时强制保留至少 1 把未过期密钥。
- **第一方免审与第三方授权同意**：`trusted: true` 的第一方应用登录后直接签发授权码；`trusted: false` 的第三方应用在首次授权、请求新增 Scope 或携带 `prompt=consent` 时重定向至 `/consent/?request_id=<id>`，由用户确认后持久化 `oauth_consents` 并支持在个人中心 `/account/applications/` 查看与一键撤销（原子销毁授权同意及该客户端下全部活跃令牌会话）。

Cookie 默认 Secure、HttpOnly、host-only、Path=/、SameSite=Lax。IAM 保留登录引用、真实认证时间和撤销状态；SCS 管理令牌、Cookie、闲置期限和绝对期限。数据库及官方 PostgreSQL Store 共用应用拥有的连接池，表由 Ent Schema 创建。

初始用户仅在不存在时创建，状态为 active 且邮箱已验证，初始密码只在首次创建时输出。初始化不写任何 IAM 或 Admin 授权关系。

### OIDC 密钥生成与轮换（`cmd/keygen`）

`app/iam/service/cmd/keygen` 复用 `internal/oidc` 的密钥解析规则，用于生成与校验 RSA-2048 PKCS#8 PEM 签名私钥和 32 字节加密密钥（目录权限 `0700`、文件权限 `0600`，临时文件回读校验后原子替换）：

- **本地初始化**：执行 `just service::iam::keygen`（默认读取 `./configs/local/` 中的 `oidc.yaml`）。目标密钥文件已存在且合法时幂等跳过，保护当前登录态与未过期令牌；若文件损坏且未传 `-force` 则直接报错退出，拒绝静默覆盖；需要轮换密钥时传入 `just service::iam::keygen -force`。
- **容器与多副本生产环境**：`just service::iam::build` 会在 `./bin/` 下同时编译产出 `server` 与 `keygen`。多副本部署严禁让每个 `iam.service` 副本各自在容器临时盘执行生成（否则各副本持有不同私钥与加密密钥会导致跨实例验签失败）；应由 CI/CD 或一次性初始化 Job 执行 `./bin/keygen -signing-key <path> -crypto-key <path>` 将密钥写入外部共享 Secret 后再挂载给各只读副本。

## 显式授权

部署模型使用 `manifests/openfga/fga.mod`。IAM 关系包含 `service:<client_id> manage_users iam:global`、独立的永久删除关系 `service:<client_id> expunge_users iam:global`（不从 `manage_users` 派生）以及 OAuth 客户端管理关系 `service:<client_id> manage_clients iam:global`；Audit 关系包含 `service:<client_id> read_events audit:global`；Admin 示例中的 `user:<id> admin admin:global` 派生 `access`、`manage_users`、`expunge_users`、`manage_clients` 与 `read_audit`。客户端注册不会产生权限。

OpenFGA 采用三层分离管理：Store（`init`）、授权模型（`init` / `apply`）与平台服务静态关系元组（`sync-tuples`，读取 `manifests/openfga/tuples/platform-services.yaml`）。`openfga-init` 与 `openfga-model-apply` 保持零预设元组以保障集成测试隔离：

```bash
# 本地一键完成 Store 创建、模型校验写入与平台服务元组同步
just openfga-setup

# 生产或指定环境分步执行
just openfga-model-validate
just openfga-model-test
just openfga-model-apply
just openfga-tuple-sync
```

Just 默认加载项目 `.env` 并覆盖同名环境变量。使用独立测试 store 时，必须同时指定 `--dotenv-path` 和 `ENV_FILE`，确保模型写入与输出文件指向同一环境：

```bash
bash manifests/scripts/openfga/openfga.sh init --store-name iam-test --env-file /tmp/iam-test.env
just --dotenv-path /tmp/iam-test.env ENV_FILE=/tmp/iam-test.env openfga-model-apply
```

开发模型直接替换，旧平台授权数据不迁移。CMS 模型由 Admin 示例替代。

开发库遇到不兼容的会话或协议表结构变更时，先停止 IAM，就地同步表/列重命名或直接删除并重建短期会话表（`login_sessions`、`auth_flows`、`oidc_authorization_requests`、`oauth_authorization_codes`、`oauth_token_sessions`、`oauth_access_tokens`、`oauth_refresh_tokens`），下次启动由 Ent `Schema.Create` 自动建表（含 SCS 的 `sessions` 表）。若旧本地开发库的 `oauth_clients` 表仍残留已移除的非空 `secret_hash` 列，升级多密钥模型时一并删除旧 `oauth_clients` 表后重启 IAM，由 `Schema.Create` 与 `OIDCInitializer` 自动重建 `oauth_clients`、`oauth_client_secrets` 与 `oauth_consents`。`Schema.Create` 仅执行幂等建表与补列，不自动删除已废弃的必填列，也不维护版本化迁移脚本；常规重建短期会话表时保留用户、登录标识、密码验证材料、OAuth 客户端及签名密钥，浏览器重新登录即可。

## 调用方与接收方

调用方自行将以下应用配置映射到 Go OAuth2 的 `clientcredentials.Config`；`oauth2` 是示例应用配置块，共享框架不另建配置或令牌管理器。

```yaml
oauth2:
  token_url: https://iam.example.com/oauth/token
  client_id: admin
  client_secret: ${IAM_ADMIN_CLIENT_SECRET}
```

应用启动时创建一次 TokenSource，复用到两个传输客户端。context 随应用结束取消，取令牌 HTTP Client 单独设置超时；gRPC 使用已有 TLS 配置。

```go
tokenContext := context.WithValue(appContext, oauth2.HTTPClient, &http.Client{Timeout: 5 * time.Second})
config := clientcredentials.Config{
    ClientID: clientID, ClientSecret: clientSecret, TokenURL: tokenURL,
    AuthStyle: oauth2.AuthStyleInHeader,
}
source := config.TokenSource(tokenContext)
httpClient := oauth2.NewClient(tokenContext, source)
connection, err := grpc.NewClient(target,
    grpc.WithTransportCredentials(credentials.NewTLS(tlsConfig)),
    grpc.WithPerRPCCredentials(grpcoauth.TokenSource{TokenSource: source}),
)
```

其中 `grpcoauth` 为 gRPC-Go 的 OAuth 适配包别名。有效令牌直接从进程缓存读取；进入 SDK 的默认十秒到期余量后，并发请求复用一次成功续取。取令牌失败则当前业务请求失败；HTTP 401/403 和 gRPC Unauthenticated/PermissionDenied 不触发业务重放。

接收方通常只需：

```yaml
jwt_authn_config:
  issuer: https://iam.example.com
  audience: iam
```

`security/authn/jwt.New` 通过 Discovery 验证 issuer 并取得 JWKS，应用关闭时调用返回的 cleanup。`security/jwt` 复用 keyfunc/jwkset 缓存，按实际 kid 验证 RS256；已知公钥无需网络请求，未知 kid 限频刷新。初始化获取失败阻止启动，后续刷新失败保留最近成功集合。

应用在 `internal/authn` 定义 Claims，组合 `jwt.RegisteredClaims`，通过 `newClaims`、`Validate()`、`mapActor` 验证并映射扩展字段。IAM 服务令牌要求 `typ=JWT`、`token_use=access`、`actor_type=service`、`sub=client_id`，同时验证 issuer、audience、签名及时间。IAM 自身注入与 JWKS 同源的本地公钥，不请求自己的 HTTP 入口。

## 验证

复用 Plateau Compose 的 PostgreSQL、Redis 与 OpenFGA。测试通过 `IAM_TEST_POSTGRES_DSN`、`IAM_TEST_FGA_URL`、`IAM_TEST_FGA_STORE_ID`、`IAM_TEST_FGA_MODEL_ID` 指向测试环境；每个数据库测试创建并清理自身 schema，服务授权测试在指定测试 store 内准备和移除关系。未配置这些变量时，集成测试明确跳过。

入口与调用示例位于 `app/iam/service/internal/server/*_test.go`，协议测试位于 `internal/oidc/`，原子回滚和撤销并发测试位于 `internal/data/`。
