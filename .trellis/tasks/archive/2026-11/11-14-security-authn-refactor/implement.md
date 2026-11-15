# `security/authn` 职责划分与泛型扩展实施计划

需求：[PRD](prd.md)。技术合同：[设计](design.md)。

---

## 1. `security/oidc` 纯协议层补齐（零 `scs` / `authn` 依赖）

- [x] 在 `security/oidc/rp.go` 移除冗余 `VerifiedIdentity`，让 `CompleteLogin(w, r)` 从 ID Token 与 `/userinfo` 完整提取 `iam.user.v1.UserProfile` 7 个标准字段并直接返回 `(*userpb.User, string, error)`；确保 `security/oidc` 不引入 `scs/v2` 或 `security/authn/*`，并使用 `new(expr)` 与 `errors.AsType`。
- [x] 在 `security/oidc/oidc_test.go` 补充 `CompleteLogin` 最小声明、`/userinfo` 回退与 ID Token 直出完整 `UserProfile` 单测。

## 2. `security/authn/jwt` 泛型声明约束与人机 `AccessClaims`

- [x] 在 `security/authn/jwt/authn.go` 增加 `ActorClaims[T]` 指针类型约束与 `AuthenticateFor[T]`；在 `security/authn/jwt/middleware.go` 增加 `ServerFor[T]`（保留原 `Authenticate` / `Server` 兼容）。
- [x] 新增 `security/authn/jwt/claims.go`：实现可内嵌的 `AccessClaims`（校验 `token_use == "access"`、`iat`、`jti`）以及 `ServiceClaims`、`HumanClaims`（各自内嵌 `AccessClaims` 并实现 `Actor() (security.Actor, error)`）。
- [x] 新增 `security/authn/jwt/claims_test.go`：覆盖 `AuthenticateFor[ServiceClaims]`、`AuthenticateFor[HumanClaims]`、角色错配拒绝（如拿着 `HumanClaims` 进 `ServerFor[ServiceClaims]`）及自定义内嵌扩展结构体 `type CustomClaims struct { HumanClaims; ... }`。

## 3. `security/authn/session` 标准 `user.go`

- [x] 新增 `security/authn/session/user.go`：直接基于 `session.New[*userpb.User]` 导出 `SaveUser`、`NewUserAuthenticator`、`UserFrom`（内置 `RenewToken`、`proto.Marshal`/`Unmarshal`、请求内免克隆写入与 `UserFrom` 按需 `proto.Clone` 防篡改）。
- [x] 在 `security/authn/session/session_test.go` 覆盖 `SaveUser`/`NewUserAuthenticator`/`UserFrom` 的会话令牌轮换、未登录/空 `user_id`/损坏载荷拦截与克隆隔离。

## 4. `app/iam/service` 与 `app/admin/service` 迁移清理

- [x] 修改 `app/iam/service/internal/oidc/service_token.go`：`GetPrivateClaimsFromRequest` 的人类分支返回 `map[string]any{"token_use": "access", "actor_type": "human"}`（补充对应断言确认人类 OAuth Access Token 携带该声明且 ID Token 不受影响，同时 IAM `UserService` 仍严格拒绝人类令牌）。
- [x] 精简 `app/iam/service/internal/authn/jwt.go`（移除重复的 `ServiceClaims` / `NewServiceClaims` / `ServiceActor`），将 `app/iam/service/internal/server/grpc.go` 及对应测试切换为 `jwt.ServerFor[jwt.ServiceClaims]` / `jwt.AuthenticateFor[jwt.ServiceClaims]`；保留 IAM 私有的 `internal/authn/{session.go,context.go}` 不变。
- [x] 修改 `app/admin/service/api/protos/admin/auth/v1/auth.proto`：删除 `AdminPrincipal`，`GetSessionResponse` 直接使用 `iam.user.v1.User user = 1` 与 `repeated string permissions = 2`；运行 `just api`、`just openapi`、`just lint-proto`、`just api-ts-check` 刷新 `api/gen/{go,ts}/admin/...` 与 `app/admin/service/internal/assets/openapi.yaml`。
- [x] 删除整个 `app/admin/service/internal/authn/` 目录；更新 `app/admin/service/internal/service/auth.go`、`internal/server/http.go`、`cmd/server/wire.go`（重跑 `just service::admin::wire`）及 `internal/server/integration_test.go`，统一切换为 `session.SaveUser`、`session.NewUserAuthenticator`、`session.UserFrom`。

## 5. 文档同步与全量验证门禁

- [x] 同步更新 `security/AGENTS.md`、`.trellis/spec/plateau/security/authn.md`、`.trellis/spec/admin/backend.md` 及 `TODO.md` 中已完成的 `security/authn` 待办项。
- [x] 运行全量测试与检查：
  ```bash
  CC=/usr/bin/clang IAM_TEST_POSTGRES_DSN=... ADMIN_TEST_POSTGRES_DSN=... IAM_TEST_FGA_URL=... go test -count=1 ./security/... ./app/iam/service/... ./app/admin/service/...
  GOWORK=off CC=/usr/bin/clang go build ./app/iam/service/cmd/server ./app/admin/service/cmd/server
  just lint
  just api-ts-check
  ```
