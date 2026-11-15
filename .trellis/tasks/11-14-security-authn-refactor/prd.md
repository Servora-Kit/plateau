# 重构 security/authn 职责划分与平台身份接入

## 目标

基于“IAM 负责 OIDC IdP、`security` 作为各业务服务接入 IAM 认证与鉴权的便利工具包”的平台定位，用轻量 Go 泛型同时实现**开箱即用**与**低成本横向扩展**：
1. **保持底层协议包零反向耦合**：`security/oidc`、`security/jwt`、`security/session` 绝不反向依赖 `security/authn/*`；
2. **`security/authn/jwt` 泛型声明约束 + 统一 `AccessClaims`**：通过 `ActorClaims[T]` 指针接收者泛型约束消除散装的 `NewXxxClaims` / `XxxActor` 样板函数，并基于公共 `AccessClaims` 同时提供 `ServiceClaims` 与 `HumanClaims`（同步补齐 IAM 人类 Access Token 的 `token_use` / `actor_type` 签发字段）；
3. **`security/authn/session` 标准 `user.go`**：直接在底层泛型 `session.New[*userpb.User]` 之上提供全平台默认的 `SaveUser` / `NewUserAuthenticator` / `UserFrom`（避免多余泛型包装层与无谓拷贝）；
4. **统一用户基础资料契约**：移除 `admin.auth.v1.AdminPrincipal`，全平台统一以 `iam.user.v1.User`（含 `UserProfile`）表达用户基础身份与展示资料。

## 需求范围

- **R1（`security/authn/jwt` 泛型声明扩展与人机令牌统一）**：
  - 定义 `ActorClaims[T]` 泛型约束及配套的 `AuthenticateFor[T]` / `ServerFor[T]`，让任意实现 `jwt.Claims` + `Actor() (security.Actor, error)` 的声明结构体无需额外编写工厂函数与映射回调；
  - 在 `security/authn/jwt/claims.go` 提供可内嵌的 `AccessClaims` 基座，以及开箱即用的 `ServiceClaims` 与 `HumanClaims`；
  - 在 `app/iam/service/internal/oidc/service_token.go` 补齐人类 OAuth Access Token 的 `{"token_use": "access", "actor_type": "human"}` 私有声明（ID Token 保持不变）；IAM gRPC 服务切换使用 `jwt.ServerFor[jwt.ServiceClaims]`。
- **R2（`security/oidc/rp.go` 保持纯协议层并直接产出 `*userpb.User`）**：
  - 移除冗余中间结构体 `VerifiedIdentity`，`CompleteLogin(w, r)` 完整提取与 `iam.user.v1.UserProfile` 对齐的 7 个 OIDC 标准资料字段并直接返回 `(*userpb.User, string, error)`；零 `scs/v2` 与零 `security/authn/*` 依赖；全面采用 Go 1.27 `new(expr)` 与 `errors.AsType`。
- **R3（`security/authn/session/user.go` 标准用户会话接入）**：
  - 新增 `security/authn/session/user.go`：直接基于 `session.New[*userpb.User]` 导出平台默认的 `SaveUser`、`NewUserAuthenticator`、`UserFrom`（`UserFrom` 按需执行 `proto.Clone` 防篡改，常规请求零额外克隆开销）；
  - IAM 继续使用 `authn.go` 底层的 `session.New[*identity]` 维护其私有的 IdP 查表逻辑。
- **R4（清理 Admin 重复消息与私有 authn 目录）**：
  - 删除 `app/admin/service/internal/authn/` 整个目录，切换使用 `session.SaveUser` / `session.NewUserAuthenticator` / `session.UserFrom`；
  - 删除 `admin/auth/v1/auth.proto` 中的 `AdminPrincipal`，`GetSessionResponse` 直接返回 `iam.user.v1.User user = 1` 与 `repeated string permissions = 2`，重新生成 Go/TS/OpenAPI 产物。
- **R5（文档与 TODO 同步）**：更新 `.trellis/spec/plateau/security/`、`.trellis/spec/admin/`，清理 `TODO.md` 对应条目。

## 验收标准

- [x] AC-1：`security/oidc` 不依赖 `security/authn/*` 或 `scs/v2`；`security/authn/jwt` 与 `security/authn/session` 各文件职责单一清晰。
- [x] AC-2：`jwt.ServerFor[jwt.ServiceClaims]`、`jwt.ServerFor[jwt.HumanClaims]` 及自定义内嵌扩展 Claims 均有单测覆盖；IAM 签发的人类 Access Token 包含 `token_use=access` 与 `actor_type=human`（且 IAM 仅接受 `ServiceClaims` 的管理接口仍严格拒绝 `HumanClaims`）。
- [x] AC-3：`session.SaveUser` / `NewUserAuthenticator` / `UserFrom` 单测覆盖会话令牌轮换、未认证/空 `user_id`/损坏字节拦截与按需克隆防篡改。
- [x] AC-4：`admin/auth/v1/auth.proto` 已移除 `AdminPrincipal`，`GetSessionResponse` 使用 `iam.user.v1.User`，生成产物通过 `just lint-proto` 与 `just api-ts-check`；`app/admin/service/internal/authn/` 目录已删除。
- [x] AC-5：`./security/...`、`./app/iam/service/...`、`./app/admin/service/...` 全量测试与 `just lint` 全部通过。
