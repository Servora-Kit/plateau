# Admin 后端实施计划

需求：[PRD](prd.md) · 技术合同：[设计](design.md)。本文件安排获得实施批准（`task.py start`）后的分步改动与验证，规划阶段不提前修改产品代码。

---

## 1. 平台共享能力提炼与 IAM 同步回归

先将通用机制提炼到平台共享目录，并让现有 `app/iam/service` 切换后通过回归验证：

- [x] 新增 `infra/entgo/mixin/http_session.go`（`HTTPSessionMixin`）及单测；将 `app/iam/service/internal/data/schema/http_session.go` 改为嵌入 `HTTPSessionMixin`，重新执行 `just service::iam::gen-ent` 确认生成的 `sessions` 表结构零差异。
- [x] 新增 `security/session/postgres.go`（`NewPostgres(config, db)`）及测试；`app/iam/service/internal/data/data.go` 的 `NewHTTPSessionManager` 改为委托 `session.NewPostgres`。
- [x] 在 `security/authz/openfga` 新增 `DefaultSubject` 与 `NewDefault(client)` 及单测；移除 `app/iam/service/internal/authz/` 与 `app/iam/service/internal/data/openfga.go`，更新 IAM `server`/`data` Wire 绑定并执行 `just service::iam::wire`。
- [x] 新增 `api/protos/plateau/security/oidc/v1/config.proto`（`message OIDCClient`，敏感字段标注 `servora.redact.v3`），执行 `just api-go`；新增 `security/oidc` 包，实现：
  - `RelyingParty`（HKDF 密钥派生 + `zitadel/oidc` `CookieHandler` 5 分钟加密握手 Cookie + 单飞惰性 Discovery + `PKCE S256`/`state`/`nonce`/`return_to` 完整校验）；
  - `NewServiceTokenMiddleware`（共享 `ReuseTokenSource` 缓存 + 监听单次请求 `ctx.Done()` 立即中断等待 + `ErrTokenSourceUnavailable` 哨兵错误）；
  - 覆盖握手篡改/重放/过期、开放重定向拦截、并发单飞续期与请求取消中断的单元测试。

---

## 2. OpenFGA 模型与 Admin API 契约生成

- [x] 更新 `manifests/openfga/admin.fga`（在 `type admin` 下定义 `admin: [user]`、`access: admin`、`manage_users: admin`、`expunge_users: admin`）与 `manifests/openfga/admin.fga.yaml` 测试场景，运行 `just openfga-model-test`。
- [x] 建立 `app/admin/service/api/protos/admin/` 源 Proto：
  - `conf/v1/config.proto`（`Admin` 配置段：`bootstrap_admin_email`）；
  - `auth/v1/auth.proto`（`AuthService`：`GetSession`、`Logout`，挂载 `access` / `AUTHZ_NONE` 注解）；
  - `user/v1/user.proto`（`UserService`：复用 `iam/user/v1/user.proto`，声明 12 个管理 RPC、`CreateUserResponse { user, verification_email_sent }`、`AdminUserErrorReason`、密码字段 `redact` 注解，默认 `manage_users`、`ExpungeUser` 覆盖为 `expunge_users`）；
  - `app/admin/service/api/buf.openapi.gen.yaml`（输出 `app/admin/service/internal/assets/openapi.yaml`）。
- [x] 在根 `buf.yaml` 注册 `app/admin/service/api/protos`，在 `just/services.just` 注册 `admin` 服务模块，新增 `app/admin/service/justfile` 与 `.air.toml`；执行 `just api` 与 `just service::admin::openapi`，确保 `just lint-proto` 与 `just api-ts-check` 通过。

---

## 3. Admin 数据层、IAM gRPC 客户端与冷启动种子

参照 `app/example/service/internal/data/{data.go,test.go}` 风格实现 `app/admin/service/internal/data/`：

- [x] 新增 `internal/data/generate.go` 与 `internal/data/schema/http_session.go`（嵌入 `HTTPSessionMixin`），执行 `just service::admin::gen-ent`。
- [x] 实现 `internal/data/data.go`：`NewSQLDB`、`NewEntDriver`、`NewDBClient`（幂等 `Schema.Create`）、`NewHTTPSessionManager`（调用 `session.NewPostgres`）、`NewDialer`（通过 `servora/transport/client/grpc.NewDialer` + `WithMiddleware` 注入 tracing/metrics/logging 及 `security/oidc.NewServiceTokenMiddleware`，无自动重试）。
- [x] 实现 `internal/data/iam_user.go`：参照 `example` 的 `Dial(ctx, "iam.service")` + `cleanup` 模式构造 `userpb.NewUserServiceClient`，透传请求 `ctx` 调用 IAM `UserService` 各方法（不在 Repo 层重复打 `log.Error`，避免与传输层日志中间件重复记录）。
- [x] 实现 `internal/biz/seed.go`（`SeedInitializer`）与 `internal/startup/initializer.go`：通过 IAM `ListUsers` 定位 `bootstrap_admin_email` 对应未删除 `user_id`，以 Ensure-Seed 方式幂等写入 `user:<id> admin admin:global`；启动时非阻塞尝试，未完成时在登录回调惰性重试。

---

## 4. Admin 认证、业务编排、自我保护与 HTTP 服务装配

- [x] 实现 `internal/authn/session.go`：读写 Admin 本地 SCS 会话中的结构化管理员快照，构造 `security.Actor{Type: ActorTypeHuman, ID: userID}` 与 `session.Authenticator`。
- [x] 严格对齐 `app/iam/service` 与 `app/example/service` 统一四层风格，建立独立 `ProviderSet` 入口文件（`biz/biz.go`、`service/service.go`、`server/server.go`）及业务实现：
  - `internal/biz/{auth.go,user.go,errors.go}`：
    - `AuthUsecase`：调用 `openfga.Authorizer.BatchCheck` 计算当前管理员的 `["access", "manage_users", "expunge_users"]` 权限码列表；
    - `UserUsecase`：`CreateUser` 两步编排（先 `CreateUser`，成功后再调 `SendUserVerificationEmail`；发信失败记录 `WarnContext` 并返回 `{user, verification_email_sent: false}`，保留 `require_password_change` 三态语义）；`UpdateUser` 白名单校验 7 个 `profile.*` 字段并清空 `Email`；`DisableUser`、`DeleteUser`、`ExpungeUser` 在调 IAM 前执行自我保护与同级管理员保护；
  - `internal/service/{auth.go,user.go}`：嵌入 `Unimplemented*Server`，遵循设计 §5.3 错误矩阵转换 `AdminUserErrorReason`（下游鉴权/网络故障统一转 `503 DOWNSTREAM_UNAVAILABLE`）；
  - `internal/server/http.go` 与 `cmd/server/{main.go,wire.go}`：注入窄配置（不向 `internal/...` 传 `*bootstrap.Runtime`），`main.go` 在 `Scan` 失败时合并 `rt.Close` 清理资源，挂载 `session.NewFilter`、OIDC 登录/回调 handler、`session.Server`、`openfga.Server` 与生成 HTTP 服务，执行 `just wire`。

---

## 5. 本地/容器配置同步与全链路验证

- [x] 更新 `docker-compose.yaml`（`POSTGRES_SERVICE_DATABASES` 默认值加入 `plateau_admin`）及 `app/iam/service/configs/{local,docker}/oidc.yaml`（启用单一机密客户端 `client_id: admin`，开放 `authorization_code` 与 `client_credentials`，`audiences: [iam]`，回调 `${ADMIN_PUBLIC_ORIGIN:http://localhost:10012}/v1/admin/auth/callback`）。
- [x] 编写 `app/admin/service/configs/{local,docker}/{bootstrap,admin,oidc_client,openfga,session}.yaml`，确保 `cmd/server/config_test.go` 扫描校验通过。
- [x] 编写端到端集成测试（`app/admin/service/internal/server/integration_test.go`），同时拉起真实 IAM（HTTP OIDC + gRPC `UserService`）、真实 Admin HTTP Server、真实 PostgreSQL（独立测试 schema）与临时 OpenFGA Store，逐条验证 `AC-B1`～`AC-B9`：
  1. 完整 OIDC 登录跳转 + PKCE S256 + `nonce` 回调建会话、`GetSession` 返回身份与权限码、独立 `Logout` 不注销 IAM、IAM 注销不影响有效 Admin 会话；
  2. 普通已登录 IAM 用户回调被拒（跳 `/auth/login?error=access_denied`、无本地会话）与缺少 `expunge_users` 时永久删除返回 `403`；
  3. 两步创建成功与 SMTP 失败降级（返回已创建用户且 `verification_email_sent=false`）及手动补发邮件；
  4. 资料白名单编辑（禁改邮箱）、设密、强退、封禁/解禁、软删除、回收站查询、无限期恢复、显式永久删除；
  5. 自封禁/自删除（`SELF_ACTION_FORBIDDEN`）与操作其他管理员（`TARGET_IS_ADMIN`）拦截无 IAM 副作用；
  6. 服务凭据错误 / audience 不匹配 / 下游超时取消：返回 `503` 或对应超时取消且保留有效 Admin 会话，日志无明文密码/密钥泄漏。

### 验证门禁命令

```bash
just openfga-model-test
just gen
CC=/usr/bin/clang go test -count=1 ./infra/... ./security/... ./app/iam/service/... ./app/admin/service/...
GOWORK=off CC=/usr/bin/clang go build ./app/admin/service/cmd/server ./app/iam/service/cmd/server
just lint
```

---

## 6. 规范同步与风险控制

- **高危回滚与保护**：新增 `plateau_admin` 库与 `app/admin/service` 不改动 IAM 既有表结构；对 `app/iam` 的共享层切换（`HTTPSessionMixin`、`session.NewPostgres`、`openfga.NewDefault`）均保持行为等价并以 IAM 既有全量单元/集成测试作回归保障。
- **规范文档更新**：实施完成后同步更新 `.trellis/spec/admin/backend.md`、`.trellis/spec/plateau/security/{authZ.md,credentials.md}` 与 `.trellis/spec/api/proto/generation.md`（清理已过时的服务级重复 TS 生成描述）。
