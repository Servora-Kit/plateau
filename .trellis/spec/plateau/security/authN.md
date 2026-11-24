# AuthN：认证实现与路由规则

适用于 `security/authn` 及微服务的认证适配。共享包按认证机制提供具体 `jwt` 与 `session` 实现及轻量泛型扩展点，底层协议包（`security/oidc`、`security/jwt`、`security/session`）绝不反向依赖 `security/authn/*`。

## 规则与默认拒绝

启用路由中间件时，通过 `security.WithRulesFuncs`（定义于 [security/rules.go](../../../../security/rules.go)）传入生成的 `AuthnRules`。规则按 operation 查找，provider 按注册顺序合并，后者覆盖同名 operation；nil provider 跳过，nil Option 导致 panic。不要依赖偶然重复注册覆盖安全策略。

- `PUBLIC`：直接写入 anonymous Actor，不顺带尝试把无效或已有凭据升级为用户。
- `REQUIRED`：验证当前实现的凭据，成功后写入可信 Actor。
- transport 缺失、operation 没有规则或 mode 不支持均返回错误，不自动放行。
- Proto 声明的 service default／method override 见 [注解](../../api/proto/annotations.md)。安装中间件和生成完整规则是两个都需要完成的步骤。

## JWT profile

[JWT Authenticator](../../../../security/authn/jwt/authn.go) 固定一个 issuer/audience。构造时注入 verifier、静态 verification_keys、显式 JWKS 只能选择一种；没有显式来源时走 issuer discovery。构造返回的 cleanup 用于自有刷新资源，交给应用启动装配。

每次认证必须从工厂得到一个已初始化、非 nil 的 claims 指针，不能并发复用 claims。流程为解析 Bearer → 验签 → `typ=JWT` → issuer/audience/exp/iat 及 `ClaimsValidator` 校验 → 映射非匿名 Actor。底层 [JWT verifier](credentials.md) 只负责签名，不能代替 AuthN 的 claims policy。

- 声明结构体实现 `ActorClaims[T]`（`*T` 同时满足 `jwt.Claims` 与 `Actor() (security.Actor, error)`）后，直接通过 `jwt.AuthenticateFor[T]` / `jwt.ServerFor[T]` 装配，无需重复编写工厂与映射回调。
- [claims.go](../../../../security/authn/jwt/claims.go) 提供公共内嵌底座 `AccessClaims`（强制 `token_use=access`、`iat != nil`、`jti != ""`）以及开箱即用的 `ServiceClaims`（`actor_type=service` 且 `sub == client_id != ""`）与 `HumanClaims`（`actor_type=human` 且 `sub != ""`）；扩展业务私有声明时直接内嵌 `HumanClaims`、`ServiceClaims` 或 `AccessClaims`。

## Session profile

先在 HTTP 边界装配 `security/session.LoadAndSave(manager)`（或 `security/session.NewFilter`），再由同一个 manager 构造 Session Authenticator。Resolver 读取应用身份，ActorMapper 转换身份；可选 ContextExtender 只附加可信状态。缺失加载标记与已加载的匿名会话是不同情况，不能拿另一 manager 的 context 充数，也不能假装这套 HTTP session 生命周期自动覆盖 gRPC。

- 常规浏览器服务直接使用 [user.go](../../../../security/authn/session/user.go) 导出的 `SaveUser`、`NewUserAuthenticator`、`UserFrom`（承载 `*iam.user.v1.User`，由 `security/oidc.RelyingParty.CompleteLogin` 直接返回）：`SaveUser` 校验已加载会话与非空 `user_id` 后执行 `proto.Marshal` + `manager.RenewToken` + `manager.Put`；`NewUserAuthenticator` 将反序列化后的快照写入请求 `context.Context`，`UserFrom` 按需返回 `proto.Clone` 防篡改副本。无需自建私有 `internal/authn` 包；仅需服务端实时查表吊销的 IdP 会话（如 IAM）或自定义会话结构才直接使用底层泛型 `session.New[T]`。

## 错误与检查

取消／超时保持原错误。凭据缺失、失效与 Actor 映射失败映射为公开 Unauthenticated；Session 依赖不可用映射 Unavailable；其他内部错误保留 cause 并隐藏内部说明。具体分类以各实现的 `apiError` 为准。

检查缺规则、匿名、有效凭据、错误 profile、claims 隔离和 manager 归属。入口：`go test ./security/authn/...`。
