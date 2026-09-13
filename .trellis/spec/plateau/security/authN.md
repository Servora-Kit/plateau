# AuthN：认证实现与路由规则

适用于 `security/authn` 及微服务的认证适配。共享包提供具体 JWT／Session Authenticator，不另建一个包揽所有认证形态的通用 provider。业务身份如何映射 [Actor](actor.md) 由消费应用决定。

## 规则与默认拒绝

启用路由中间件时，通过 `authn.WithRulesFuncs` 传入生成的 `AuthnRules`。规则按 operation 查找，provider 按注册顺序合并，后者覆盖同名 operation；nil provider 跳过，nil Option 导致 panic。不要依赖偶然重复注册覆盖安全策略。

- `PUBLIC`：直接写入 anonymous Actor，不顺带尝试把无效或已有凭据升级为用户。
- `REQUIRED`：验证当前实现的凭据，成功后写入可信 Actor。
- transport 缺失、operation 没有规则或 mode 不支持均返回错误，不自动放行。
- Proto 声明的 service default／method override 见 [注解](../../api/proto/annotations.md)。安装中间件和生成完整规则是两个都需要完成的步骤。

## JWT profile

[JWT Authenticator](../../../../security/authn/jwt/authn.go) 固定一个 issuer/audience。构造时注入 verifier、静态 verification_keys、显式 JWKS 只能选择一种；没有显式来源时走 issuer discovery。构造返回的 cleanup 用于自有刷新资源，交给应用启动装配。

每次认证必须从工厂得到一个已初始化、非 nil 的 claims 指针，不能并发复用 claims。流程为解析 Bearer → 验签 → `typ=JWT` → issuer/audience/exp/iat 校验 → 映射非匿名 Actor。底层 [JWT verifier](credentials.md) 只负责签名，不能代替 AuthN 的 claims policy。

## Session profile

先在 HTTP 边界装配 `security/session.LoadAndSave(manager)`，再由同一个 manager 构造 Session Authenticator。Resolver 读取应用身份，ActorMapper 转换身份；可选 ContextExtender 只附加可信状态。缺失加载标记与已加载的匿名会话是不同情况，不能拿另一 manager 的 context 充数，也不能假装这套 HTTP session 生命周期自动覆盖 gRPC。

## 错误与检查

取消／超时保持原错误。凭据缺失、失效与 Actor 映射失败映射为公开 Unauthenticated；Session 依赖不可用映射 Unavailable；其他内部错误保留 cause 并隐藏内部说明。具体分类以各实现的 `apiError` 为准。

修改后检查缺规则、匿名、有效凭据、错误 profile、claims 隔离和 manager 归属：`go test ./security/authn/...`。这是验证入口，不代表每次读规范已经运行。

依据：[规则聚合](../../../../security/authn/rules.go)、[JWT middleware](../../../../security/authn/jwt/middleware.go)、[JWT tests](../../../../security/authn/jwt/authn_test.go)、[Session middleware](../../../../security/authn/session/middleware.go)、[Session tests](../../../../security/authn/session/authn_test.go)。
