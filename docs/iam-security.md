# IAM 认证与服务调用

IAM HTTP 提供登录、注销、账户与 OIDC 协议入口；UserService 六个管理操作只注册到 gRPC。浏览器使用 SCS Cookie，内部管理调用使用服务访问令牌和 OpenFGA 授权。

## 运行配置

本机读取 `app/iam/service/configs/local/`，容器读取 `configs/docker/`。两者使用相同配置结构，数据库、Redis、邮件和 OpenFGA 地址分别指向本机端口或 Compose 服务名。容器配置要求公开 HTTPS 入口可从调用方访问，并挂载 OIDC 签名密钥与提供方加密密钥。

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
      audiences: [iam]
```

访问令牌默认五分钟，可用 `oidc.service_access_token_ttl` 设置正时长，例如 `300s`。配置中的 Duration 遵循 Protobuf JSON 的秒格式。用户 OIDC 客户端继续配置回调、scope、`trusted: true` 及授权码/刷新流程。静态配置在部署加载时同步；换密钥保留 client_id，移除客户端停止新签发，已有服务令牌在到期前仍可验证。

Cookie 默认 Secure、HttpOnly、host-only、Path=/、SameSite=Lax。IAM 保留登录引用、真实认证时间和撤销状态；SCS 管理令牌、Cookie、闲置期限和绝对期限。数据库及官方 PostgreSQL Store 共用应用拥有的连接池，表由 Ent Schema 创建。

初始用户仅在不存在时创建，状态为 active 且邮箱已验证，初始密码只在首次创建时输出。初始化不写任何 IAM 或 Admin 授权关系。

## 显式授权

部署模型使用 `manifests/openfga/fga.mod`。IAM 关系为 `service:<client_id> manage_users iam:global`；Admin 示例中的 `user:<id> admin admin:global` 只表达 Admin 自己的用户权限。客户端注册不会产生权限。

```bash
rtk proxy just openfga-model-validate
rtk proxy just openfga-model-test
rtk proxy just openfga-model-apply
rtk proxy fga tuple write --api-url "$FGA_API_URL" --store-id "$FGA_STORE_ID" --model-id "$FGA_MODEL_ID" --on-duplicate ignore service:admin manage_users iam:global
```

Just 默认加载项目 `.env` 并覆盖同名环境变量。使用独立测试 store 时，必须同时指定 `--dotenv-path` 和 `ENV_FILE`，确保模型写入与输出文件指向同一环境：

```bash
rtk proxy bash manifests/scripts/openfga/openfga.sh init --store-name iam-security-test --env-file /tmp/iam-security-test.env
rtk proxy just --dotenv-path /tmp/iam-security-test.env ENV_FILE=/tmp/iam-security-test.env openfga-model-apply
```

开发模型直接替换，旧平台授权数据不迁移。CMS 模型由 Admin 示例替代。

已有开发库切换时，先停止 IAM，清除旧登录及其关联的用户 OAuth 会话、令牌与授权请求，再删除旧 `iam_login_sessions` 表；下次启动由 Ent 创建新表及 SCS 的 `sessions` 表。普通 `Schema.Create` 不会删除旧表中的必填列，不能代替这次格式切换。保留用户、登录标识、密码验证材料、OAuth 客户端及签名密钥；旧浏览器须重新登录。不添加版本化迁移或旧 Cookie 兼容。

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
jwt:
  issuer: https://iam.example.com
  audience: iam
```

`security/authn/jwt.New` 通过 Discovery 验证 issuer 并取得 JWKS，应用关闭时调用返回的 cleanup。`security/jwt` 复用 keyfunc/jwkset 缓存，按实际 kid 验证 RS256；已知公钥无需网络请求，未知 kid 限频刷新。初始化获取失败阻止启动，后续刷新失败保留最近成功集合。

应用在 `internal/authn` 定义 Claims，组合 `jwt.RegisteredClaims`，通过 `newClaims`、`Validate()`、`mapActor` 验证并映射扩展字段。IAM 服务令牌要求 `typ=JWT`、`token_use=access`、`actor_type=service`、`sub=client_id`，同时验证 issuer、audience、签名及时间。IAM 自身注入与 JWKS 同源的本地公钥，不请求自己的 HTTP 入口。

## 验证

复用 Plateau Compose 的 PostgreSQL、Redis 与 OpenFGA。测试通过 `IAM_TEST_POSTGRES_DSN`、`IAM_TEST_FGA_URL`、`IAM_TEST_FGA_STORE_ID`、`IAM_TEST_FGA_MODEL_ID` 指向测试环境；每个数据库测试创建并清理自身 schema，服务授权测试在指定测试 store 内准备和移除关系。未配置这些变量时，集成测试明确跳过。

入口与调用示例位于 `app/iam/service/internal/server/*_test.go`，协议测试位于 `internal/oidc/`，原子回滚和撤销并发测试位于 `internal/data/`。
