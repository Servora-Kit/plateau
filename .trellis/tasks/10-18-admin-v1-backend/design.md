# Admin 后端技术设计

需求：[本任务 PRD](prd.md) · [父任务 PRD](../10-04-admin-v1/prd.md) · [IAM 归档设计](../archive/2026-11/10-18-admin-v1-iam/design.md)。

---

## 1. 服务定位、端口与目录结构

`app/admin/service` 是面向平台管理员的 BFF（Backend For Frontend）兼跨服务编排后端，同时也是平台首个落地的 OAuth 2.1 / OIDC 机密客户端（对人：Authorization Code + PKCE S256；对服务间调用：`client_credentials`）。

- **固定端口**：HTTP `10010`、gRPC `10011`（本期 Admin 无对内 gRPC 消费者，仅启动 HTTP Server 监听 `10010`；`10011` 保持预留不监听）、Web `10012`（Vite 仅代理 `/v1` -> `http://127.0.0.1:10010`）。
- **目录布局**（遵循平台四层微服务结构，共享根 `go.mod`）：
  ```text
  app/admin/service/
  ├── .air.toml
  ├── justfile                           # import "../../../just/service.just"
  ├── api/
  │   ├── buf.openapi.gen.yaml           # 生成 internal/assets/openapi.yaml
  │   └── protos/admin/
  │       ├── auth/v1/auth.proto         # AuthService (GetSession, Logout)
  │       ├── conf/v1/config.proto       # Admin 业务配置 (bootstrap_admin_email)
  │       └── user/v1/user.proto         # UserService (编排 IAM UserService)
  ├── cmd/server/
  │   ├── main.go                        # NewRuntime + Scan + Run
  │   ├── wire.go                        # Wire 注入图
  │   └── wire_gen.go
  ├── configs/
  │   ├── local/                         # bootstrap.yaml, admin.yaml, oidc_client.yaml, openfga.yaml, session.yaml
  │   └── docker/
  └── internal/
      ├── assets/openapi.yaml
      ├── authn/session.go               # Admin 本地会话身份投影与 SessionAuthenticator
      ├── biz/                           # biz.go(ProviderSet), auth.go, user.go, seed.go, errors.go
      ├── data/                          # data.go(ProviderSet+NewData), iam_user.go, openfga_seed.go
      │   ├── generate.go
      │   ├── schema/http_session.go     # 嵌入共享 HTTPSessionMixin
      │   └── ent/                       # go tool ent 生成产物
      ├── server/                        # server.go(ProviderSet), http.go
      ├── service/                       # service.go(ProviderSet), auth.go, user.go
      └── startup/initializer.go         # 启动期非阻塞尝试同步 bootstrap_admin_email 种子
  ```
- **注册入口**：
  - `buf.yaml` 新增 `- path: app/admin/service/api/protos`；由根 `just gen` 统一生成至 `api/gen/go/admin/...` 与 `api/gen/ts/admin/...`（`@plateau/api`），不建服务级重复 TS 生成模板。
  - `just/services.just` 注册 `mod admin '../app/admin/service/justfile'` 并并入 `_gen`、`_build`、`lint`、`clean`、`openapi`、`wire`、`gen-ent`。
  - `docker-compose.yaml` 的 `postgres` 与 `postgres-provision` 默认 `POSTGRES_SERVICE_DATABASES` 扩充为 `plateau_iam,plateau_admin`，自动创建 `plateau_admin` 数据库。

---

## 2. 平台共享层提炼合同（IAM 与 Admin 共同消费）

在建设 Admin 前先将已验证的通用机制下沉到平台共享目录，并将 `app/iam/service` 同步切换至共享实现，杜绝跨应用复制：

### 2.1 `infra/entgo/mixin/http_session.go`（`HTTPSessionMixin`）
- 嵌入 `mixin.Schema`，统一提供 SCS `postgresstore` 所需的 `id`（`StorageKey("token")`, Postgres `text`, `Sensitive`）、`data`（`bytes`, `Sensitive`）、`expiry`（Postgres `timestamptz`）三列、`index.Fields("expiry")` 索引与 `entsql.Annotation{Table: "sessions"}`。
- `app/iam/service/internal/data/schema/http_session.go` 与 `app/admin/service/internal/data/schema/http_session.go` 均简化为仅通过 `Mixin()` 引用 `HTTPSessionMixin{}`。

### 2.2 `security/session/postgres.go`（`NewPostgres`）
- 签名：`func NewPostgres(config *sessionpb.Session, db *sql.DB) (*scs.SessionManager, func(), error)`。
- 校验 `db != nil`，调用 `postgresstore.New(db)` 与 `session.New(config, store)`；若 `session.New` 校验失败立即调用 `store.StopCleanup()` 防止 goroutine 泄漏，成功则返回 `(manager, store.StopCleanup, nil)`。
- `app/iam/service/internal/data/data.go` 的 `NewHTTPSessionManager` 委托 `session.NewPostgres`（保留 `client *entmodel.Client` 形参以维持 Wire 建表先后依赖）。

### 2.3 `security/authz/openfga` 平台默认主体映射
- 新增 `DefaultSubject(actor security.Actor) (string, error)`（`ActorTypeHuman` -> `"user:" + actor.ID`，`ActorTypeService` -> `"service:" + actor.ID`，其余或无效 Actor 报错）及快捷构造 `NewDefault(client *fgaclient.OpenFgaClient) (*Authorizer, error)`。
- 删除 `app/iam/service/internal/authz/` 私有包装包与 `app/iam/service/internal/data/openfga.go` 纯转发文件，IAM 与 Admin 的 Wire 直接绑定 `infra/openfga.New` 与 `openfga.NewDefault`。

### 2.4 `security/oidc` + `api/protos/plateau/security/oidc/v1/config.proto`
共享 OIDC 客户端工具包，同时承载 **浏览器 OIDC Relying Party（Authorization Code + PKCE S256）** 与 **服务间 `client_credentials` Kratos 客户端中间件**：

```protobuf
syntax = "proto3";
package plateau.security.oidc.v1;

import "buf/validate/validate.proto";
import "google/protobuf/duration.proto";
import "servora/conf/v1/annotations.proto";
import "servora/redact/v3/redact.proto";

option go_package = "github.com/Servora-Kit/plateau/api/gen/go/plateau/security/oidc/v1;oidcv1";

// OIDCClient 配置接入上游 OIDC/OAuth 2.1 提供方的客户端（段名 oidc_client，与 IAM OP 的 oidc 段明确区分）。
message OIDCClient {
  option (servora.conf.v1.section) = true;

  optional string issuer = 1 [(servora.conf.v1.field) = {required: true}, (buf.validate.field).string.min_len = 1];
  optional string client_id = 2 [(servora.conf.v1.field) = {required: true}, (buf.validate.field).string.min_len = 1];
  optional string client_secret = 3 [
    (servora.conf.v1.field) = {required: true},
    (buf.validate.field).string.min_len = 1,
    (servora.redact.v3.value).string = ""
  ];
  // 浏览器 OIDC 登录必填；纯机器客户端可留空。
  string redirect_uri = 4;
  // 用于派生短期握手加密 Cookie 的 HMAC 与 AES-256 密钥（浏览器登录时必填，至少 32 字节）。
  string cookie_secret = 5 [(servora.redact.v3.value).string = ""];
  repeated string scopes = 6; // 缺省 ["openid", "profile", "email"]
  google.protobuf.Duration http_timeout = 7 [(servora.conf.v1.field) = {default: "5s"}];
}
```

- **浏览器 RP 封装（`security/oidc.RelyingParty`）**：
  1. **密钥派生**：校验 `len(cookie_secret) >= 32`，使用 Go 标准库 `crypto/hkdf`（SHA-256）派生独立的 32 字节 `hashKey` 与 32 字节 `encryptKey`，构造 `zitadel/oidc/v3/pkg/http.CookieHandler`（`MaxAge=300s`、`HttpOnly`、`SameSite=Lax`；当 `redirect_uri` 为 `https://` 时启用 `Secure` 与 `__Host-` Cookie 前缀，本地 `http://localhost` 时自动使用 `WithUnsecure()` 及无 `__Host-` 前缀的宿主机 Cookie 名，如 `admin_oidc_handshake`）。
  2. **单飞惰性 Discovery**：构造期仅做本地配置与 URL 格式校验（不发起远程网络请求，防止容器并行启动时因 IAM 慢启动而崩溃）；首次调用登录或回调时通过 `singleflight` + 带 `http_timeout` 的 `*http.Client` 执行 `rp.NewRelyingPartyOIDC`（配置 `rp.WithAuthStyle(oauth2.AuthStyleInHeader)`、`rp.WithSigningAlgsFromDiscovery()`、`rp.WithVerifierOpts(rp.WithNonce(nonceFromContext))`），成功后原子缓存复用。
  3. **完整四元组握手**：
     - `BeginLogin(w, r, defaultReturnTo)`：校验 `r.URL.Query().Get("return_to")`（仅接受单 `/` 开头且非 `//`、`/\` 的站内相对路径，非法则回退 `defaultReturnTo`）；生成 32 字节随机 `state`、`nonce`、`code_verifier`，算 `S256` `code_challenge`，将 JSON `{State, Nonce, CodeVerifier, ReturnTo}` 编码进单一短期加密 Cookie，发出 `302` 跳转至 IAM `/authorize`。
     - `CompleteLogin(w, r)`：读取并**立即删除**握手 Cookie；若上游带 `error` 参数或 Cookie 缺失/过期/`state` 不匹配则返回分类错误 `ErrHandshakeInvalid`；否则将 `Nonce` 注入 `ctx`，携带 `code_verifier` 执行 `rp.CodeExchange`（内部同时完成 `/oauth/token` Basic Auth 交换、`id_token` RS256 签名/`iss`/`aud`/`exp`/`nonce` 校验及 `at_hash` 校验），返回验证通过的 `VerifiedIdentity{Subject, Email, EmailVerified, Name, Picture, AuthTime}` 与 `ReturnTo`。
- **服务令牌客户端中间件（`security/oidc.NewServiceTokenMiddleware`）**：
  - 基于同一 `*oidcv1.OIDCClient` 配置（`issuer + "/oauth/token"`、`client_id`、`client_secret`、`AuthStyleInHeader`）构造进程内共享的 `oauth2.ReuseTokenSource`（绑定服务级后台生命周期 `appCtx`，关闭时 `cancel`）。
  - 请求拦截逻辑：先检查 `reqCtx.Err()`；在通过 `singleflight`/异步通道调用 `source.Token()` 时用 `select { case <-reqCtx.Done(): return nil, reqCtx.Err() ... }` 监听当前请求取消信号（解决 `x/oauth2` `ReuseTokenSource` 不感知单次请求取消的问题）；拿到令牌后写入 `transport.FromClientContext(ctx).RequestHeader().Set("Authorization", "Bearer "+tok.AccessToken)`。
  - 若取令牌失败，包装返回可识别的哨兵错误 `oidc.ErrTokenSourceUnavailable`（不透出 `client_secret` 或原始响应体），供业务层统一映射为 `503 DOWNSTREAM_UNAVAILABLE`。

---

## 3. OpenFGA 模型、冷启动种子与保护规则

### 3.1 `manifests/openfga/admin.fga` 与测试更新
```fga
module admin

type admin
  relations
    define admin: [user]
    define access: admin
    define manage_users: admin
    define expunge_users: admin
```
同步在 `manifests/openfga/admin.fga.yaml` 补充 `access`、`manage_users`、`expunge_users` 的正向与负向（如 `service:admin` 不具备 `admin:global` 人类权限、普通用户 `bob` 三项均为 `false`）断言，并通过 `just openfga-model-test`。

### 3.2 `admin.conf.v1.Admin` 与幂等种子协调（Ensure Seed）
- `app/admin/service/api/protos/admin/conf/v1/config.proto`：
  ```protobuf
  message Admin {
    option (servora.conf.v1.section) = true;
    optional string bootstrap_admin_email = 1 [
      (servora.conf.v1.field) = {required: true},
      (buf.validate.field).string.email = true
    ];
  }
  ```
- **同步逻辑（`biz.SeedInitializer`）**：
  1. 维护原子布尔标记 `seeded atomic.Bool` 与 `singleflight.Group`。
  2. 调用 IAM `UserService.ListUsers`（`filter: email == "<normalized_email>"`, `show_deleted: true`, `page_size: 2`）精确定位唯一匹配的 `user_id`；若用户尚未创建或处于已删除状态，记录 `Warn` 日志并保持 `seeded = false`（不报错崩溃）。
  3. 查到 `user_id` 后，先执行 OpenFGA `Check(user:<user_id>, "admin", "admin", "global")`；若为 `false`，调用 OpenFGA SDK `Write` 写入 `ClientTupleKey{User: "user:" + userID, Relation: "admin", Object: "admin:global"}`（若并发写入报 duplicate tuple 则视为成功），成功后置 `seeded = true`。**绝不删除 OpenFGA 中已有的其他管理员或角色元组**。
  4. **触发时机**：`startup.Initializer` 在 `kratos.BeforeStart` 中非阻塞尝试一次（设 3s 超时，IAM 未就绪仅打 `Warn` 日志不阻断启动）；在 `GET /v1/admin/auth/callback` 校验权限前，若 `!seeded.Load()` 则以请求 `ctx` 再尝试一次同步，确保容器乱序拉起时首次登录即可自动完成种子赋权。

### 3.3 自我保护与同级管理员保护（PRD B4 / AC-B5）
在 `UserUsecase` 的 `DisableUser` 与 `DeleteUser`（以及 `ExpungeUser` 兜底检查）中，于调用 IAM gRPC **之前**执行两步保护：
1. **防自操作**：从可信认证上下文读取 `actor`（`security.ActorFrom(ctx)`），若 `actor.ID == targetUserID`，立即返回 `biz.ErrSelfActionForbidden`（映射为 `403 ADMIN_USER_ERROR_REASON_SELF_ACTION_FORBIDDEN`）。
2. **防操作其他管理员**：调用 OpenFGA `authorizer.Check(ctx, security.Actor{Type: security.ActorTypeHuman, ID: targetUserID}, "admin", "admin", "global")`；若返回 `true`，立即返回 `biz.ErrTargetIsAdmin`（映射为 `403 ADMIN_USER_ERROR_REASON_TARGET_IS_ADMIN`）；若 OpenFGA 不可用则返回依赖错误（503），失败关闭（fail-closed），绝不在未确认时放行下游变更。

---

## 4. OIDC BFF 登录、回调、本地会话与退出链路

所有 Admin 后端 HTTP 入口均位于单一前缀 **`/v1/admin/*`** 下，由 `security/session.NewFilter`（`PublicOrigin: app.GetExternalUrl()`, `JSONPrefixes: []string{"/v1/admin/"}`）统一保护：

| 路径与方法 | 类型 | 认证 / 授权 | 行为与状态流转 |
| --- | --- | --- | --- |
| `GET /v1/admin/auth/login` | 原生 HTTP Handler | 公开（`GET` 安全方法放行） | 调用 `oidcRP.BeginLogin(w, r, "/")` 写短期加密握手 Cookie 并 `302` 跳转 IAM `/authorize`。 |
| `GET /v1/admin/auth/callback` | 原生 HTTP Handler | 公开（已装载 SCS） | 1. 限制仅 `GET`（否则 `405`）；<br>2. `oidcRP.CompleteLogin(w, r)` 校验 `state`/`PKCE`/`nonce` 并换取 `VerifiedIdentity`（握手失败则销毁已有本地会话并 `302` 跳 `/auth/login?error=login_failed`）；<br>3. 若 `!seed.Seeded()` 则尝试同步一次种子；<br>4. 调 OpenFGA `Check(user:<sub>, "access", "admin", "global")`（若为 `false`，销毁已有本地会话并 `302` 跳 `/auth/login?error=access_denied`；若 OpenFGA 故障跳 `/auth/login?error=service_unavailable`）；<br>5. 允许访问：`manager.RenewToken(ctx)`，在 SCS 写入结构化快照（`admin_session` JSON：`user_id`、`email`、`name`、`picture`、`auth_time`），`302` 跳转 `returnTo`。 |
| `GET /v1/admin/auth/session` | Proto HTTP (`AuthService.GetSession`) | `AUTHN_REQUIRED` + `access` (`admin:global`) | 从认证上下文读取本地会话身份快照，调用 `authorizer.BatchCheck` 一次性查询 `["access", "manage_users", "expunge_users"]`，返回管理员资料与当前拥有的权限码数组。 |
| `POST /v1/admin/auth/logout` | Proto HTTP (`AuthService.Logout`) | `AUTHN_REQUIRED` + `AUTHZ_NONE` | 调用 `manager.Destroy(ctx)` 销毁 Admin 本地 SCS 会话（不调 IAM `/end_session`，不影响 IAM 会话）；非 JSON 或跨源请求在装载会话前由 `session.NewFilter` 拒绝（415/403）。 |

- **本地会话独立性（满足 `AC-B2`、`AC-B7`）**：
  `internal/authn/session.go` 的 `SessionAuthenticator` 仅解析 Admin 自身 SCS PostgreSQL Store 中的会话快照构造 `security.Actor{Type: ActorTypeHuman, ID: snap.UserID}`，请求期不回调 IAM 验证 IAM 登录态；权限变更通过每跳 `openfga.Server` 中间件实时生效。

---

## 5. Proto API 契约与错误映射矩阵

### 5.1 `admin.auth.v1`（`app/admin/service/api/protos/admin/auth/v1/auth.proto`）
- 服务默认：`AUTHN_MODE_REQUIRED`，`AUTHZ_MODE_REQUIRED`（`action: "access", resource_type: "admin", resource_id: "global"`）。
- `rpc GetSession(GetSessionRequest) returns (GetSessionResponse)` -> `GET /v1/admin/auth/session`
  - `GetSessionResponse`：`AdminPrincipal user = 1`（`user_id`, `email`, `name`, `picture`, `google.protobuf.Timestamp auth_time`）、`repeated string permissions = 2`。
- `rpc Logout(LogoutRequest) returns (LogoutResponse)` -> `POST /v1/admin/auth/logout` `body: "*"`（方法级覆盖 `AUTHZ_MODE_NONE`）。

### 5.2 `admin.user.v1`（`app/admin/service/api/protos/admin/user/v1/user.proto`）
直接 `import "iam/user/v1/user.proto"`，所有资源名在路径中以 `{user_id}` 绑定（或规范资源名 `users/{user_id}`——对齐 IAM `user.proto` 的 `name` 模式 `users/{user}`，HTTP 路径采用 `/v1/admin/{name=users/*}`，与 `iam.user.v1` 资源名格式 100% 同构，前端无需手动拼接/拆解 ID！）：

- 服务默认：`AUTHN_MODE_REQUIRED`，`AUTHZ_MODE_REQUIRED`（`action: "manage_users", resource_type: "admin", resource_id: "global"`）。
- RPC 清单：
  1. `ListUsers(ListUsersRequest) returns (ListUsersResponse)`：`GET /v1/admin/users`（透传 `page_size`、`page_token`、`filter`、`order_by`、`show_deleted`；返回 `repeated iam.user.v1.User users`、`next_page_token`、`total_size`）。
  2. `GetUser(GetUserRequest) returns (iam.user.v1.User)`：`GET /v1/admin/{name=users/*}`。
  3. `CreateUser(CreateUserRequest) returns (CreateUserResponse)`：`POST /v1/admin/users` `body: "*"`
     - 请求字段：`string user_id = 1`（可选）、`iam.user.v1.User user = 2`（必填，含 `email` 与可选 `profile`）、`string password = 3`（必填，`redact` 脱敏，`min_len=8, max_len=128`）、`optional bool require_password_change = 4`（完整保留 `nil`/`true`/`false` 三态透传给 IAM）。
     - 响应字段：`CreateUserResponse { iam.user.v1.User user = 1; bool verification_email_sent = 2; }`。
     - 编排规则：先调 IAM `CreateUser`；若失败直接映射错误返回，不调发信。若成功，保留返回的 `createdUser`，继续调 IAM `SendUserVerificationEmail(createdUser.Name)`；发信成功返回 `{user: createdUser, verification_email_sent: true}`；发信若返回任何错误（含 SMTP 503 或超时），记录结构化 `WarnContext` 日志（含 `user_name`、下游 reason，不含密码），仍返回 HTTP `200` `{user: createdUser, verification_email_sent: false}`。
  4. `SendUserVerificationEmail(SendUserVerificationEmailRequest) returns (SendUserVerificationEmailResponse)`：`POST /v1/admin/{name=users/*}:sendVerificationEmail` `body: "*"`。
  5. `UpdateUser(UpdateUserRequest) returns (iam.user.v1.User)`：`PATCH /v1/admin/{user.name=users/*}` `body: "user"`（携带 `update_mask` 作为 query 参数；Admin `biz` 层校验 `update_mask` 仅包含允许编辑的 `profile.{name,given_name,family_name,nickname,preferred_username,picture,locale}` 子集且非空，若包含 `email` 或其他不可写路径直接返回 `400 INVALID_ARGUMENT`，并在请求体中强制清空 `user.Email = nil` 后再转发 IAM）。
  6. `SetUserPassword(SetUserPasswordRequest) returns (SetUserPasswordResponse)`：`POST /v1/admin/{name=users/*}:setPassword` `body: "*"`（`new_password` 标注 `redact`）。
  7. `RevokeUserSessions(RevokeUserSessionsRequest) returns (RevokeUserSessionsResponse)`：`POST /v1/admin/{name=users/*}:revokeSessions` `body: "*"`。
  8. `DisableUser(DisableUserRequest) returns (iam.user.v1.User)`：`POST /v1/admin/{name=users/*}:disable` `body: "*"`（先执行自我保护与同级管理员保护，透传 `etag` 与可选 `reason`）。
  9. `EnableUser(EnableUserRequest) returns (iam.user.v1.User)`：`POST /v1/admin/{name=users/*}:enable` `body: "*"`（透传 `etag`）。
  10. `DeleteUser(DeleteUserRequest) returns (iam.user.v1.User)`：`DELETE /v1/admin/{name=users/*}`（仅含 `name`，无 `etag`，先执行自我保护与同级管理员保护）。
  11. `UndeleteUser(UndeleteUserRequest) returns (iam.user.v1.User)`：`POST /v1/admin/{name=users/*}:undelete` `body: "*"`（仅含 `name`）。
  12. `ExpungeUser(ExpungeUserRequest) returns (google.protobuf.Empty)`：`POST /v1/admin/{name=users/*}:expunge` `body: "*"`（方法级覆盖注解：`action: "expunge_users", resource_type: "admin", resource_id: "global"`；执行自我保护与同级管理员保护后调用 IAM `ExpungeUser`）。

### 5.3 错误原因与下游 gRPC 错误转换矩阵（满足 `AC-B7`）

`AdminUserErrorReason` 定义与转换规则（**核心铁律：下游 IAM 返回的 `401/403` 服务凭据或服务权限故障，在 Admin 对浏览器返回时必须转换为 `503 DOWNSTREAM_UNAVAILABLE`，绝不向浏览器透传 `401`/`403` 以免混淆为当前管理员登录失效或管理员个人无权限**）：

| 触发来源与条件 | Admin HTTP 状态码 | Error Reason | 说明 |
| --- | --- | --- | --- |
| 管理员浏览器未登录 / 本地会话失效 | `401` | `SECURITY_ERROR_REASON_UNAUTHENTICATED` | 由 `security/authn/session` 统一产生 |
| 管理员在 `admin:global` 缺少 `access`/`manage_users`/`expunge_users` | `403` | `SECURITY_ERROR_REASON_PERMISSION_DENIED` | 由 `security/authz/openfga` 统一产生 |
| 管理员尝试封禁/删除/永久删除自身 | `403` | `ADMIN_USER_ERROR_REASON_SELF_ACTION_FORBIDDEN` | Admin `biz` 自我保护拦截 |
| 管理员尝试封禁/删除/永久删除具备 `admin` 资格的用户 | `403` | `ADMIN_USER_ERROR_REASON_TARGET_IS_ADMIN` | Admin `biz` OpenFGA 检查拦截 |
| 下游 IAM 返回 `USER_ERROR_REASON_NOT_FOUND` | `404` | `ADMIN_USER_ERROR_REASON_USER_NOT_FOUND` | 用户不存在或未处于预期删除态 |
| 下游 IAM 返回 `USER_ERROR_REASON_ALREADY_EXISTS` | `409` | `ADMIN_USER_ERROR_REASON_USER_ALREADY_EXISTS` | 邮箱已被占用（含软删除占用）或恢复已存活用户 |
| 下游 IAM 返回 `USER_ERROR_REASON_INVALID_STATE` | `409` | `ADMIN_USER_ERROR_REASON_INVALID_STATE` | 如直接永久删除未软删除用户、向已激活用户发验证信 |
| 下游 IAM 返回 `USER_ERROR_REASON_ETAG_MISMATCH` | `409` | `ADMIN_USER_ERROR_REASON_ETAG_MISMATCH` | 并发版本冲突 |
| 手动补发邮件时 IAM 返回 `USER_ERROR_REASON_VERIFICATION_EMAIL_FAILED` | `503` | `ADMIN_USER_ERROR_REASON_VERIFICATION_EMAIL_FAILED` | 邮件投递失败 |
| 下游 IAM 返回 `CRUD_ERROR_REASON_*` 或 `400 INVALID_ARGUMENT`（如密码不合规、非法 filter/page_token） | `400` | 原样保留 `CRUD_ERROR_REASON_*` 或 `INVALID_ARGUMENT` | 参数校验失败（不透出内部堆栈） |
| 请求 `ctx` 超时或客户端取消（`DeadlineExceeded` / `Canceled`） | `504` / `499` | `GATEWAY_TIMEOUT` / `CLIENT_CLOSED` | 原样反映超时/取消，不吞掉取消状态 |
| 服务令牌获取失败（`oidc.ErrTokenSourceUnavailable`）、IAM 返回 `SECURITY_ERROR_REASON_*`（401/403 服务鉴权故障）或网络不可达（`Unavailable`） | `503` | `ADMIN_USER_ERROR_REASON_DOWNSTREAM_UNAVAILABLE` | 服务端记录含下游 RPC 与原因的 `ErrorContext` 日志，向前端仅返回安全提示，保持本地会话有效 |

---

## 6. IAM 客户端装配、配置结构与运维联调

### 6.1 `servora/transport/client/grpc.Dialer` 装配（满足 `AC-B8`）
- `app/admin/service/internal/data/data.go`：
  - 使用 `clientmw.NewChainBuilder(l).WithTrace(obs.GetTrace()).WithMetrics(mtc).WithoutCircuitBreaker().Build()`（或保留默认熔断，不配置任何自动重试中间件），追加 `security/oidc.NewServiceTokenMiddleware(oidcCfg)`，传入 `grpcclient.NewDialer(...)`。
  - `NewIAMUserRepo` 调用 `dialer.Dial(context.Background(), "iam.service")` 构造 `userpb.NewUserServiceClient(conn)`，随 Wire cleanup 关闭 `conn`。
- 日志不重复记录：遵循 `.trellis/spec/service/backend/coding.md`，传输层已有 Kratos server/client logging middleware，业务层仅在两步创建中的非致命发信降级或后台种子同步处打带 `Context` 的结构化日志，不在普通请求返回路径重复 `log.Error`。

### 6.2 单一机密客户端配置（D5 选项 1）
1. **IAM 配置同步（`app/iam/service/configs/{local,docker}/oidc.yaml`）**：
   取消注释并正式启用 `admin` 客户端配置：
   ```yaml
   clients:
     - client_id: admin
       client_secret: "${IAM_ADMIN_CLIENT_SECRET:dev-admin-oidc-client-secret-32bytes}"
       redirect_uris:
         - "${ADMIN_OIDC_REDIRECT_URI:http://localhost:10012/v1/admin/auth/callback}"
       allowed_scopes: [openid, profile, email]
       trusted: true
       allowed_grant_types: [authorization_code, client_credentials]
       audiences: [iam]
   ```
2. **Admin 本地运行配置（`app/admin/service/configs/local/`）**：
   - `bootstrap.yaml`：`app.name: admin.service`、`app.external_url: "${ADMIN_PUBLIC_ORIGIN:http://localhost:10012}"`、`server.http.listen.addr: "0.0.0.0:10010"`、`data.database` 指向 `plateau_admin`、`data.client.services` 配置 `iam.service` gRPC endpoint（本地直连默认 `${IAM_GRPC_ENDPOINT:127.0.0.1:10001}` 或 Consul discovery）。
   - `admin.yaml`：`bootstrap_admin_email: "${IAM_BOOTSTRAP_USER_EMAIL:admin@plateau.local}"`。
   - `oidc_client.yaml`：`issuer: "${IAM_PUBLIC_ORIGIN:http://localhost:10002}"`、`client_id: admin`、`client_secret`、`redirect_uri: "${ADMIN_PUBLIC_ORIGIN:http://localhost:10012}/v1/admin/auth/callback"`、`cookie_secret`。
   - `session.yaml`：独立 Cookie 名（本地 HTTP 为 `admin_session` + `secure: false` 或生产 `__Host-admin_session` + `secure: true`）。
   - `openfga.yaml`：共用平台 `FGA_API_URL`、`FGA_STORE_ID`、`FGA_MODEL_ID`。
3. **OpenFGA 服务权限预置**：
   联调与部署时确保写入两条服务关系元组（`service:admin manage_users iam:global` 与 `service:admin expunge_users iam:global`），人类管理员元组 `user:<id> admin admin:global` 由 Admin `SeedInitializer` 自动幂等写入。
