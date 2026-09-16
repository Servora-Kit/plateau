# Admin 接入设计核对

核对日期：2026-09-16。本文件承接 `09-15-admin-initial` 的技术设计，不代表 Admin 后端或 OIDC RP 已经实现。早期 [current-state.md](current-state.md) 记录后端目录不存在；最新 [端口核查](port-layout-migration.md) 已发现工作树中的 IAM 复制内容，尚未完成独立 Admin 接线，Vben 登录仍是演示实现。实施以最新工作树为准并保留用户改动。

## 现状锚点与结论

- IAM 的本地公开 HTTP 是 `10000`、gRPC 是 `10001`，OIDC issuer/浏览器公开入口是 `http://localhost:10002`（[bootstrap.yaml](../../../../app/iam/service/configs/local/bootstrap.yaml):1-22、[oidc.yaml](../../../../app/iam/service/configs/local/oidc.yaml):1-20）。本地 OIDC 配置中的 `test-web` 和 `admin` client 目前是注释示例，不能当作已经启用的 Admin client；IAM 代码支持授权码、PKCE、refresh token 和 client credentials（[provider.go](../../../../app/iam/service/internal/oidc/provider.go):61-80、157-180）。
- IAM 已有浏览器会话由 `security/session.New` 接入 SCS，服务端保存数据、cookie 使用 `HttpOnly`/`Secure`/`SameSite` 等配置（[session.go](../../../../security/session/session.go):15-77）。这套封装可复用“会话管理器构造和 LoadAndSave 生命周期”，Admin 必须使用独立 Store 命名空间和独立 cookie 名称（生产 `__Host-admin_session`，开发 `admin_session`），不能读取或覆盖 `__Host-iam_session`；封装本身没有 CSRF 校验。
- 共享 session AuthN 会从应用拥有的会话身份映射可信 `security.Actor`，并明确 `WithActor` 不负责认证或授权（[authn.go](../../../../security/authn/session/authn.go):12-19、43-78、[actor.go](../../../../security/actor.go):15-46）。Admin callback 验证 OIDC `sub` 后才将其映射为 `ActorTypeHuman`，后续请求仍须走 OpenFGA PEP/PDP。
- Admin 模型已经表达“人类 Admin 资格”和“IAM 服务身份权限”是两条关系：`admin:global#admin` 接受 `user` 并派生 `manage_users`（[admin.fga](../../../../manifests/openfga/admin.fga):1-6），IAM 的 `iam:global#manage_users` 接受 `service`（[iam.fga](../../../../manifests/openfga/iam.fga):1-9）。模型和测试 tuple 不等于 Admin 运行时关系初始化或后端接线完成。

## 浏览器认证、会话与路由

### 端口和公开拓扑

本任务按用户最新要求将 Admin 规划为 HTTP `10010`、gRPC `10011`（预留）、Web `10012`；根 AGENTS 和实际配置目前仍为旧布局，待实施时更新。路由和 client 命名以主 design 为准：callback 是 `/auth/callback`，会话是 `/v1/admin/session`，用户管理是 `/v1/admin/users`，资格管理是 `/v1/admin/administrators`，OIDC clients 是 `admin-web` 与 `admin-service`。开发和生产都让浏览器只看到一个 Admin public origin：开发为 `http://localhost:10012`，Vite 将 `/auth/*`、`/v1/*` 代理到 Admin HTTP `10010`；生产由反向代理把同一路径转发给 BFF。Cookie 作用域由 host/path 决定，不由端口隔离；同源 proxy 的目的在于减少跨源请求的 CORS 和部署复杂度，并让两套应用继续使用不同 cookie 名称。IAM issuer 仍为 `http://localhost:10002`，不能误写成 IAM HTTP `10000` 或 gRPC `10001`。

建议的 Admin BFF 路由如下。所有改变状态的路由使用 POST，并经过 CSRF 中间件；GET 只读。

| 路由 | 行为 | 认证/授权 |
| --- | --- | --- |
| `GET /auth/login` | 校验安全的 `return_to`，生成一次性 state、nonce、PKCE verifier，保存交易并跳转 IAM `/authorize` | 匿名可用；只允许同一 Admin public origin 回跳 |
| `GET /auth/callback` | 校验 state 一次性消费，使用 confidential client secret + PKCE 交换 code，校验 issuer、audience、签名、exp、nonce、`sub` | 回调协议校验；成功后检查 Admin 资格，普通 IAM 用户不得建 Admin 会话 |
| `GET /v1/admin/session` | 返回当前 Admin session 与资格状态 | Admin session；资格用 OpenFGA 每次请求检查 |
| `GET /auth/csrf` | 为当前 Admin session 创建/返回 CSRF token | 已登录；token 不放进 cookie 会话值 |
| `POST /auth/logout` | 清空 Admin SCS session、撤销本地会话 token 并跳回 Web 登录页 | Admin session + CSRF；不得调用 IAM `/end_session`、`/revoke` |
| `GET /v1/admin/users`、`GET /v1/admin/users/{id}` | 代理 IAM `ListUsers`/`GetUser` | Admin 资格 + IAM service token |
| `POST /v1/admin/users`、`PATCH /v1/admin/users/{id}` | 代理 IAM `CreateUser`/`UpdateUser`；PATCH 只接收七个 profile 字段及 field mask | Admin 资格 + IAM service token；邮箱只读 |
| `POST /v1/admin/users/{id}:disable`、`:enable`、`:set-password`、`:delete`、`:restore`、`:force-logout` | 代理 IAM 对应管理 RPC；新增 RPC 未在当前 proto 中实现 | Admin 资格 + CSRF + IAM service token；后端从可信 Actor 判定禁止自操作 |
| `GET /v1/admin/administrators` | 用 OpenFGA Read 读取 `admin:global` 上的直接 `admin` tuple，再按 IAM user ID 补齐展示资料 | Admin 资格 + OpenFGA read；不能以 Admin 本地表替代 |
| `POST /v1/admin/administrators`、`DELETE /v1/admin/administrators/{iamUserID}` | 对 `admin:global` 写入/删除 `user:<stable IAM user ID> --admin--> admin:global` 关系 | Admin 资格 + CSRF；禁止撤销自身；幂等返回真实写入结果 |

### OIDC code + PKCE confidential BFF

Admin 应注册 `admin-web` 授权码 client，secret 只注入 Admin 后端；另用 `admin-service` client credentials 给 IAM gRPC。两者不能共用 secret、不能把 secret 打进 Vben bundle。IAM 静态 client schema 约束 secret、精确 redirect URI、允许 scope、trusted 和 grant type（[config.proto](../../../../app/iam/service/api/protos/iam/oidc/conf/v1/config.proto):17-40）。建议启用的本地配置是：

```yaml
# IAM oidc.yaml：由部署环境注入 secret，实际值不提交仓库
- client_id: admin-web
  client_secret: "${IAM_ADMIN_WEB_CLIENT_SECRET}"
  redirect_uris:
    - "http://localhost:10012/auth/callback"
  allowed_scopes: [openid, profile, email, offline_access]
  trusted: true
  allowed_grant_types: [authorization_code, refresh_token]
- client_id: admin-service
  client_secret: "${IAM_ADMIN_SERVICE_CLIENT_SECRET}"
  allowed_grant_types: [client_credentials]
  audiences: [iam]
```

`trusted: true` 只表示 IAM 不显示 consent 页，不替 Admin 资格检查。Admin 配置应包含 `iam.issuer=http://localhost:10002`、`admin-web`/`admin-service` client ID、secret、精确 callback、`admin.public_origin=http://localhost:10012`、`iam.grpc_endpoint=127.0.0.1:10001`、OpenFGA store/model/endpoint，以及独立 session 配置。

登录交易的 server-side 记录至少包括 state hash、PKCE verifier、nonce、redirect URI、return_to、创建时间和已消费标记；state 必须绑定当前 Admin 浏览器会话并在 token exchange 前删除，防重放。callback 要使用发现文档/JWKS 验证 ID token，且只以 `sub` 作为稳定 IAM user ID；email、显示名及浏览器提交的 user ID 都不能用于身份绑定。IAM 发现文档明确列出 `sub`、`iss`、`aud`、`exp`、`nonce` 和 profile claims，且只支持 S256 PKCE（[provider.go](../../../../app/iam/service/internal/oidc/provider.go):157-180）。协议实现可在实施时选用与当前 IAM 版本相容的 `github.com/zitadel/oidc/v3/pkg/client/rp` 或 `x/oauth2` 配合验证器；这属于新增 Admin RP 代码和依赖，不是当前已支持能力。Context7 查阅的官方材料：ZITADEL RP 的 authorization code/PKCE/nonce 示例、`x/oauth2` 的 verifier 与 client credentials、SCS 的 server-side cookie 会话。

成功 callback 的顺序：验证交易和 token → 保存由 `admin-web` 发行的 access/refresh token 到服务端 SCS session → 用 `sub` 查询/确认 IAM 身份状态（是否需要邮箱验证/首次改密的后续流程按 IAM 返回结果处理）→ 用 OpenFGA `HIGHER_CONSISTENCY` Check 检查 `user:<sub>` 对 `admin:global` 的 `manage_users` → `manager.RenewToken` → 写入 Admin session 的 IAM user ID、issuer/client 标识和必要审计字段 → 重定向白名单内的 `return_to`。没有 Admin 资格时不创建工作会话，返回明确的 forbidden 页面。Admin session 不保存可当作永久授权的 role/资格快照。

Vben 当前 `apps/web-antd` 是独立 workspace 和 lockfile；它的演示 `/auth/login` 不能当作 IAM/OIDC 参考。实施时保留该独立边界，在 `apps/web-antd` 以相对路径消费平台生成包：`../../../../../api/gen`（`@plateau/api`）和 `../../../../../web/packages/client`（`@plateau/client`），不要并入根 workspace、复制生成 API 或把 IAM secret 放进前端。前端只调用同源 `/auth`、`/v1`，登录状态以 BFF 的 HttpOnly cookie 为准。

### Cookie、CSRF 与退出

Admin 复用 `security/session.New` 的 cookie 校验、SCS Store、HashTokenInStore 和 `LoadAndSave`，但用 Admin 自己的 Redis/Store namespace 与 cookie 名称。生产使用 `__Host-admin_session`、Path `/`、无 Domain、Secure、HttpOnly、SameSite=Lax；本地明确定义普通 `admin_session`、`Secure=false`、HttpOnly、SameSite=Lax，以适配 `http://localhost:10012`，不能把 `__Host-` 前缀与非 Secure cookie 混用。Cookie 按 host/path 发送，端口不是隔离边界；同源 proxy 仍用于减少跨源 CORS 与部署复杂度。回调完成身份提权前先 `RenewToken`，避免 session fixation。

现有 session 封装不提供 CSRF，因此新增 `security` 或 Admin 内的中间件：登录后在 SCS 保存 CSRF token 的 hash，`GET /auth/csrf` 返回一次性/可轮换 token，所有 POST/PATCH/DELETE 要求 `X-CSRF-Token`、常量时间比较和允许的 `Origin`/`Referer`；Admin API 不开放跨源 credential CORS。OIDC callback 另由一次性 state + nonce + PKCE 保护，不用普通 CSRF token 代替。

Admin logout 只销毁 Admin session 和其中的 access/refresh token，不请求 IAM `/end_session`、不撤销 IAM OAuth token、不改变 IAM Web 的 `__Host-iam_session`。因此重新访问 Admin 会再次走 IAM authorize，IAM SSO 仍可使其无密码返回，但每次仍要通过 Admin qualification Check。当前 IAM 的 `end_session` 仅是 IAM 自身协议入口（[provider.go](../../../../app/iam/service/internal/oidc/provider.go):22-32、84-94）；仓库没有跨应用注销传播证据。Admin 每个受保护请求都必须使用 `admin-web` Basic client authentication 调 IAM `/oauth/introspect`，确认 access token `active=true`、`sub` 与 session UID 相同、`client_id=admin-web` 且未过期，再执行 OpenFGA Check；introspection 非 2xx、返回 inactive 或主体不匹配均清理本地 Admin session 并返回 401。access token 过期时，后端用同一 `admin-web` Basic credentials 和 server-side refresh token 调 `/oauth/token`，验证轮换后的 token 后再 introspect；不能改成丢弃 token、仅保留 UID cookie 的方案。

当前 IAM 已能支持该“RP introspect 自己发行的 access token”路径，但有明确约束和待补接线：introspection endpoint 强制 client authentication（ZITADEL `server_http.go`，模块缓存 `/Users/horonlee/go/pkg/mod/github.com/zitadel/oidc/v3@v3.49.2/pkg/op/server_http.go:412-436`，要求 Basic/private client credentials），Plateau storage 还要求 introspecting client ID 等于 token 的发行 client ID（[op_op_storage.go](../../../../app/iam/service/internal/oidc/op_op_storage.go):68-79）；因此必须用 `admin-web` 的同一 secret，不能用 `admin-service` introspect 用户 token。IAM 的 `activeAccessToken` 检查 access token 未过期、未撤销及 token session 未撤销（[claims.go](../../../../app/iam/service/internal/oidc/claims.go):18-49），人类 token 的 introspection 还通过 `populateUserinfo` 要求 IAM user 为 active（同文件:52-65、[op_op_storage.go](../../../../app/iam/service/internal/oidc/op_op_storage.go):92-112）。refresh grant 同样需要 client secret，且配置必须同时允许 authorization code 与 refresh token（ZITADEL `token_refresh.go`，模块缓存 `/Users/horonlee/go/pkg/mod/github.com/zitadel/oidc/v3@v3.49.2/pkg/op/token_refresh.go:115-150`；[initializer.go](../../../../app/iam/service/internal/oidc/initializer.go):151-177；现有测试以 Basic auth 调 token/introspection，[provider_test.go](../../../../app/iam/service/internal/oidc/provider_test.go):526-581）。需补的仅是 Admin RP 的 token server-side 保存、每请求 introspection/过期刷新、`admin-web` 静态 client 启用和端到端失效验收；IAM 现有实现不提供 Admin RP 的这些接线。

## 两层授权与 IAM gRPC 服务身份

Admin HTTP middleware 先用共享 session AuthN 将会话身份映射为 `Actor{Type: human, ID: IAM user ID}`，然后对所有受保护 Admin route 以 `user:<Actor.ID>` 检查 `manage_users` / `admin:global`。UI 菜单只能改善体验，不能成为权限边界。用户 ID 只能来自已验证 OIDC `sub` 和服务端 session，不能来自请求 body、query 或任意 header。

Admin 调 IAM gRPC 时使用独立服务身份。当前 IAM gRPC server 已安装 JWT service AuthN 和 OpenFGA middleware，并只注册 `UserService`（[grpc.go](../../../../app/iam/service/internal/server/grpc.go):21-44）。服务令牌必须含 service actor 所需的 `token_use=access`、`actor_type=service`、`client_id`/`sub`，IAM 会将其映射为 `ActorTypeService`（[jwt.go](../../../../app/iam/service/internal/authn/jwt.go):16-47）；`iam:global#manage_users` tuple 决定是否可调用 UserService。Admin 后端通过 client credentials 获取服务 token，并经目标 IAM client 的凭据扩展发送。IAM 已有 HTTP+gRPC 接收测试（[grpc_integration_test.go](../../../../app/iam/service/internal/server/grpc_integration_test.go):29-42、74-115），其中直接使用原生 gRPC per-RPC credentials，尚不能证明 Servora Dialer 已有同等接线；下节记录本轮核查。人类 OIDC token 不应转发到 IAM gRPC，Admin 服务 token 也不能赋予人类 Admin 资格。

建议在 Admin 后端拆出三个显式接口，避免把 transport、身份和领域调用混在 handler：

```go
type AdminSessionResolver interface {
    Resolve(ctx context.Context) (iamUserID string, ok bool, err error)
}

type AdminQualificationStore interface {
    Check(ctx context.Context, iamUserID string) (bool, error)
    List(ctx context.Context, continuationToken string) (userIDs []string, nextToken string, err error)
    Grant(ctx context.Context, iamUserID string) error
    Revoke(ctx context.Context, iamUserID string) error
}

type IAMUserClient interface {
    Get(ctx context.Context, userID string) (...)
    List(ctx context.Context, pageToken string, filter string) (...)
    Create(ctx context.Context, request ...) (...)
    Update(ctx context.Context, request ...) (...)
}
```

具体类型应包住 generated `userpb.UserServiceClient` 和 OAuth2 token source；接口签名以最终 proto 为准，以上仅规定责任边界。HTTP handler 不直接访问 IAM Ent/数据库、不解析服务 token、不接受客户端指定的操作者 ID。

## Servora gRPC client 接入核查

2026-09-16 补充，支持 PRD R21/AC21。以下为源码证据，不代表 Admin 联调已经通过。

- Servora [Dialer](../../../../../servora/transport/client/grpc/dialer.go):23-54 已提供 `WithMiddleware`，并在同文件 117-128 将其接入底层 gRPC client。可先在专用于 IAM 的 Dialer 上注入请求级服务凭据 middleware，token 获取、缓存和刷新留在 Plateau。当前未直接暴露通用 `grpc.DialOption`/`PerRPCCredentials`；这属于现状，不直接判为框架缺陷。
- Dialer 接收构造连接时的 context（同文件 86-94）；请求级 deadline/cancel 必须在 generated client 的每次 RPC 中继续传入，不能用 Dial 的 context 作为传播已经完成的证据。[客户端链](../../../../../servora/transport/client/middleware/chain.go):32-36 有可选 tracing；真实 Admin → IAM 的取消、超时和 trace 尚待 AC21 验证。
- [IAM gRPC 集成测试](../../../../app/iam/service/internal/server/grpc_integration_test.go):35-38、80-86 使用原生 gRPC credentials，96-115 覆盖服务授权 tuple 缺失、添加、删除后的拒绝/放行；需新增经过 Servora client 的消费证据，不能只沿用原生 Dial 测试作为框架验收。本轮未执行这些测试。
- Dialer 配置读取 endpoint、timeout、TLS（[dialer.go](../../../../../servora/transport/client/grpc/dialer.go):154-182），当前未消费通用 endpoint options；没有合同和复现之前，不据此新增 credentials 配置协议或扩大重构。
- 后续按既有扩展点接线并验证，若暴露出通用 credentials/metadata、上下文或错误处理缺口，再在 Servora 补齐。重试须结合实际运行配置检查；当前未证明完整链路中的重试和错误语义，也未验证 streaming。当前 UserService 为 unary，本任务不据此增加 streaming 能力建设。

## OpenFGA 单一事实源、撤权和一致性

### 关系写入与列表

管理员资格的当前状态只存在 OpenFGA tuple：

```text
user:6f...  --admin-->  admin:global
```

`manage_users` 由模型从 `admin` 派生。Admin qualification API 的 grant/revoke 直接对该 tuple 做幂等 Write/Delete；不要再建一个 `admins` 表作为当前资格，也不要把 Vben 菜单、IAM 用户字段或 bootstrap 文件当事实源。可以记录不可用于授权的审计事件，但审计投影不能回写或覆盖关系。

首次 bootstrap 是初始化协调，而不是第二个资格事实源：IAM 与 seed 用户在 IAM 侧原子创建并持久化 stable binding；Admin 使用自己的 service credentials 调新增的 `GetBootstrapUser` gRPC（复用 `iam:global#manage_users`）读取真实 seed UID，再写入 `user:<seed-uid> --admin--> admin:global`，读回并验证 tuple 后才把 Admin bootstrap 状态从 `pending` 标为 `completed`。绑定完成后启动重试永不按邮箱 `ListUsers` 猜测或重新授予；bootstrap 状态记录只防止重复初始化，后续资格仍以 OpenFGA tuple 为唯一事实。该 RPC 和协调状态目前都尚未实现，必须纳入 proto、并发/重启和失败回滚验收。

当前 `security/authz/openfga.Authorizer.Check` 没有一致性参数，且只有以 `ListObjects` 为基础的 `ListAllowed`（[authz.go](../../../../security/authz/openfga/authz.go):45-67、127-169）。管理员名单不使用 `ListUsers` 推导权限集合：OpenFGA Go SDK `v0.8.2` 的 `ClientListUsersOptions` 没有 continuation token（SDK `client.go`:2995-3006），而 `Read` API 支持按直接 tuple 过滤、`page_size`、`continuation_token` 和 consistency（SDK `client.go`:1389-1402、[model_read_request.go](https://github.com/openfga/go-sdk/blob/v0.8.2/model_read_request.go):21-26）。实施时给共享 adapter 增加 `CheckWithConsistency`/选项式 Check，以及 `ReadAdminTuples(ctx, continuationToken)` 封装，调用 `Read` 的 `object="admin:global"`、`relation="admin"`、`HIGHER_CONSISTENCY`，解析并仅接受 `user:<stable IAM ID>` 直接 tuple，返回 OpenFGA continuation token。若部署的 OpenFGA 版本/API 不支持该 Read 或一致性选项，接口应明确报 unavailable，不能退回本地名单。拿到 user IDs 后再通过 IAM `GetUser`/分页查询补齐展示资料，IAM 缺失的 ID 作为待清理关系显示，不能因资料查询失败而猜测或自动改授权。

grant/revoke 成功响应必须在 OpenFGA 写入/删除成功后返回；重复 grant/revoke 可视为幂等成功，但必须以实际读取验证结果为准。列表结果是展示数据，不能代替 protected route 的 Check。由于 OpenFGA 的一致性偏好会增加延迟并可能降低可用性，所有 Admin qualification Check、直接 tuple Read 和 grant/revoke 后的 read-after-write 验证都使用 SDK 的 `HIGHER_CONSISTENCY`（或当前服务端等价的 fully-consistent 配置）；OpenFGA 不可用时 fail closed，不能使用过期缓存放行。

### 既有会话撤权

每个受保护 Admin 请求都执行：session LoadAndSave → 解析可信 Actor → OpenFGA `Check(user="user:<Actor.ID>", relation="manage_users", object="admin:global", consistency=HIGHER_CONSISTENCY)` → 通过后才进入 handler。Admin session 只保存身份，不保存“已是 Admin”的永久结论。因此删除 tuple 后，原 cookie 的下一次受保护请求必须得到 403；可选的短缓存也必须有严格失效通知，否则不能满足 R11/AC10。自我保护检查使用同一个可信 Actor ID，拒绝对自己的 disable/delete/revoke；不读取请求体中的 operator ID。IAM UserService 的服务 tuple 仍单独检查，撤销人类资格不撤销 `service:admin`，也不修改 IAM 密码、状态或会话。

OpenFGA read/write 的一致性并不替代数据库事务：并发 grant/revoke 按 tuple 操作结果和最后一次强一致 Check 判定，Admin 记录操作审计的 request ID/idempotency key。实施需实测“撤销响应返回后同一 session 下一请求立即 403”以及“写入成功但列表短暂滞后”的行为；若底层只提供最终一致列表，页面显示应标为重新读取/重试，授权 Check 仍必须强一致。

## 与现有 IAM 用户模型的接口边界

当前 UserService 已要求 service AuthN 和 `iam:global` `manage_users` AuthZ，并提供 Create/Get/List/Update/Disable/Enable（[user.proto](../../../../app/iam/service/api/protos/iam/user/v1/user.proto):18-57）。Admin 的资料表单只映射 `UserProfile` 七字段 `name`、`given_name`、`family_name`、`nickname`、`preferred_username`、`picture`、`locale`（同文件:78-87）；email、status、email_verified、时间和 etag 按 IAM 的 immutable/output-only 语义处理（同文件:89-114）。创建请求中的 password 只传给 IAM，必须遵守 redaction 和日志禁写约束（同文件:117-129）；邮件验证、首次改密、删除恢复、token/session 撤销由 IAM 实现，Admin 只呈现明确的远端结果。

当前 IAM HTTP 装配已将 SCS、OIDC、Account、Session/Authn 和 OpenFGA middleware 按路由注册（[http.go](../../../../app/iam/service/internal/server/http.go):29-75），但这是 IAM provider 的接线，不是 Admin RP 的现成实现。Admin 应建立自己的 HTTP session middleware、OIDC RP callback 与 OpenFGA PEP，使用生成的 UserService gRPC client，避免把 IAM 内部 session key `iam_login_id`（[session.go](../../../../app/iam/service/internal/authn/session.go):14-34）当作跨应用共享凭据。

## 必要验证清单

1. **OIDC 协议**：issuer discovery 为 `http://localhost:10002`；未登记 redirect、state/nonce/PKCE 错误、callback 重放、签名/issuer/audience/exp 不符都拒绝；正确 code + S256 verifier 建立 Admin session；无 `admin:global#admin` 的普通 IAM 用户得到 forbidden；首次改密/邮箱验证要求不能由 Admin callback 绕过。
2. **会话与 CSRF**：登录前后 session ID 轮换；cookie 具备独立名称、HttpOnly、Secure、Path/Domain/SameSite 约束；缺失/错误 CSRF 或错误 Origin 的 POST/PATCH/DELETE 拒绝；GET 只读；`POST /auth/logout` 后旧 Admin cookie 不能访问管理 API，但 IAM `10002` 的会话仍可继续访问 IAM Web，且没有请求 `/end_session`。
3. **两层 AuthN/AuthZ**：Admin 人类请求只产生 human Actor；IAM gRPC 请求只使用 service client credentials；错误 secret、错误 issuer/audience、过期 service token、缺少 `iam:global` tuple 均被拒绝；不能以人类 Admin tuple 替换 `service:admin` tuple。
4. **资格单事实源**：grant/list/revoke 均读写 `admin:global` 的 OpenFGA relation；不建本地权威名单；列表补齐 IAM 用户资料；grant/revoke 响应后使用高一致性读取验证；撤销后一条已有 session 的下一请求返回 403；自撤销及对自身生命周期操作被后端拒绝。
5. **远端管理链路**：Vben `10012` → Admin HTTP `10010` → IAM gRPC `10001` 的成功、未登录、无 Admin 资格、IAM 拒绝、etag 冲突和依赖不可用均显示真实结果；Profile 七字段 field mask 正确，email 不可变；password 不出日志/响应；不连接 IAM 数据库。
6. **端口和依赖**：Admin dev proxy、IAM `10000/10001/10002`、OpenFGA endpoint/store/model 的本地配置逐项可启动并验证；Vben 继续使用独立 workspace/lockfile，以相对 `../../../../../api/gen` 与 `../../../../../web/packages/client` 消费平台包，不修改根 workspace 纳管规则。需要运行时才能证明的项目（client bootstrap 是否已启用、OpenFGA 当前 datastore 的强一致语义、IAM 禁用对 Admin session 的即时观察）在实现验收前不得写成“已支持”。

## 参考资料

- [Admin PRD](../prd.md)
- [当前实现与参考项目边界](current-state.md)
- [Admin 后端规范](../../../spec/admin/backend.md)
- [Admin 前端规范](../../../spec/admin/frontend.md)
- [ZITADEL OIDC RP 文档（Context7 读取的官方仓库文档）](https://github.com/zitadel/oidc/tree/main/_autodocs)
- [Go OAuth2 官方包文档](https://pkg.go.dev/golang.org/x/oauth2)
- [SCS 官方 README](https://github.com/alexedwards/scs/blob/master/README.md)
- [OpenFGA Go SDK 官方仓库](https://github.com/openfga/go-sdk)
