# `security/authn` 职责划分与泛型扩展技术设计

需求：[PRD](prd.md)。

---

## 1. 为什么这两处用 Go 泛型刚刚好（既不 YAGNI，又消除样板）

### 1.1 `security/authn/jwt`：指针接收者泛型约束 `ActorClaims[T]` + 可内嵌 `AccessClaims`

之前每加一种令牌声明，都要重复写三件套（`XxxClaims` + `NewXxxClaims()` + `XxxActor()`），调用端还要传一串回调：
`jwt.Server(serviceAuthn, authn.NewServiceClaims, authn.ServiceActor)`。

利用 Go 1.21+ 的指针类型约束推断（Constraint Type Inference）：

```go
// ActorClaims 约束指针类型 *T 同时满足 jwt.Claims 且能自映射为 security.Actor。
type ActorClaims[T any] interface {
    *T
    jwt.Claims
    Actor() (security.Actor, error)
}

// AuthenticateFor 与 ServerFor 利用编译器推断自动 new(T) 并调用 claims.Actor()：
// 调用方只需写一个类型参数：jwt.ServerFor[jwt.ServiceClaims](authn, opts...)
func AuthenticateFor[T any, P ActorClaims[T]](ctx context.Context, authenticator *Authenticator, authorization string) (security.Actor, error) {
    return Authenticate(ctx, authenticator, authorization, func() P { return new(T) }, P.Actor)
}

func ServerFor[T any, P ActorClaims[T]](authenticator *Authenticator, opts ...authn.Option) middleware.Middleware {
    return Server(authenticator, func() P { return new(T) }, P.Actor, opts...)
}
```

在此之上，`security/authn/jwt/claims.go` 提供：
1. **公共内嵌底座 `AccessClaims`**：统一承载 `jwt.RegisteredClaims` + `ClientID` + `TokenUse` + `ActorType`，实现公共 `Validate()`（强制 `token_use == "access"`、`iat != nil`、`jti != ""`）；
2. **开箱即用的 `ServiceClaims` 与 `HumanClaims`**（各仅 ~8 行，内嵌 `AccessClaims` 并实现各自的 `Actor()`）：
   - `ServiceClaims`：要求 `ActorType == ActorTypeService` 且 `Subject == ClientID != ""`，返回 `Actor{Service, ClientID}`；
   - `HumanClaims`：要求 `ActorType == ActorTypeHuman` 且 `Subject != ""`，返回 `Actor{Human, Subject}`。
   - **顺手补齐 IAM 签发端 1 行**：查核 `app/iam/service/internal/oidc/service_token.go:48` 发现，IAM 的 `GetPrivateClaimsFromRequest`（专用于签发 JWT Access Token，不影响 ID Token）目前只给 `service` 写了 `{"token_use": "access", "actor_type": "service"}`，人类分支留了空 map（尽管 `grpc_integration_test.go:138` 早已按 `actor_type: "human"` 编写测试）。本次顺手将该行补齐为 `{"token_use": "access", "actor_type": "human"}`，人机 Access Token 契约彻底闭环！
3. **未来怎么扩展自定义字段？**
   任何业务服务如果在 Token 里扩展了自定义字段，只需内嵌 `HumanClaims`（或 `AccessClaims`）：
   ```go
   type WorkspaceClaims struct {
       jwt.HumanClaims
       WorkspaceID string `json:"workspace_id"`
   }
   // 直接用：jwt.ServerFor[WorkspaceClaims](authn)
   ```

---

### 1.2 `security/authn/session`：在泛型 `session.New[T]` 之上直接提供 `user.go`（零多余抽象）

- `authn.go` 本身已具备泛型扩展点 `session.New[T](manager, resolve, mapActor, extend)`（IAM 正是通过它挂载私有的 `*identity` 查库逻辑），且浏览器 Cookie 会话只服务于人类用户（机器间调用始终走 Bearer JWT `ServiceClaims`，不存在 `session/service.go`）。
- 因此不在 `New[T]` 之上再叠一层 `Store[T]` 泛型包装，直接在 `security/authn/session/user.go` 基于 `New[*userpb.User]` 导出三个零配置函数：
  - `SaveUser(ctx, manager, *userpb.User) error`（校验已加载会话与非空 `user_id`，执行 `proto.Marshal` + `manager.RenewToken` + `manager.Put`）
  - `NewUserAuthenticator(manager) (*UserAuthenticator, error)`（每次请求反序列化全新 `*userpb.User` 并写入私有 `userContextKey{}`，不做冗余预克隆）
  - `UserFrom(ctx) (*userpb.User, bool)`（仅在业务读取时执行 `proto.Clone` 防篡改）

### 1.3 `security/oidc/rp.go`：移除冗余 `VerifiedIdentity`，`CompleteLogin` 直出 `*userpb.User`

- 全平台用户基础资料唯一权威类型为 `iam.user.v1.User`（含 7 字段 `UserProfile`）。移除中间搬运结构体 `VerifiedIdentity`，`(*RelyingParty).CompleteLogin(w, r)` 直接从 ID Token 与按需 `/userinfo` 提取字段并返回 `(*userpb.User, string, error)`，回调处一步传入 `session.SaveUser`。
- 同步使用 Go 1.27 原生 `new(expr)` 替代 `proto.String`、使用 `errors.AsType` 替代 `errors.As`。
---

## 2. 完整文件变更总览

| 路径 | 动作 | 职责说明 |
|---|---|---|
| `security/oidc/{rp.go,service_token.go}` | 精简 | 移除 `VerifiedIdentity`，`CompleteLogin` 直接返回 `*userpb.User`（含 7 字段 `UserProfile`）；换用 `new(expr)` 与 `errors.AsType` |
| `security/authn/jwt/{authn.go,middleware.go}` | 轻量增强 | 增加泛型包装 `AuthenticateFor[T]` 与 `ServerFor[T]`（原 `Authenticate` / `Server` 保留兼容） |
| `security/authn/jwt/claims.go` | **新增** | `AccessClaims` 基座 + `ServiceClaims` + `HumanClaims` |
| `security/authn/session/user.go` | **新增** | 直接基于 `session.New[*userpb.User]` 提供 `SaveUser` / `NewUserAuthenticator` / `UserFrom` |
| `app/iam/service/internal/oidc/service_token.go` | 修改（1 行） | 人类 OAuth Access Token 补写 `{"token_use": "access", "actor_type": "human"}` |
| `app/iam/service/internal/authn/jwt.go` + `server/grpc.go` | 精简 | 删除重复的 `ServiceClaims`，改用 `jwt.ServerFor[jwt.ServiceClaims]` |
| `app/admin/service/internal/authn/` | **删除目录** | 彻底移除 `session.go` 与 `session_test.go` |
| `app/admin/service/api/protos/admin/auth/v1/auth.proto` | 精简 | 删除 `AdminPrincipal`，`GetSessionResponse` 直接用 `iam.user.v1.User` |
| `app/admin/service/...` | 同步更新 | 切换至 `session.SaveUser` / `NewUserAuthenticator` / `UserFrom`，重跑 `just api` / `openapi` / `wire` |
