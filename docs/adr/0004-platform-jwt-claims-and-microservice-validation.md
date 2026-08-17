# ADR 0004：Platform JWT claims 与微服务认证传播边界

- 状态：accepted
- 日期：2026-08-16
- 范围：`servora-platform` 安全基础与未来 IAM/Resource Server 接入

## Context

旧 IAM 在 `app/iam/service/internal/biz/authn.go` 中定义了 `UserClaims`，用于 IAM 签发和验证 JWT。GoWind 的 `UserTokenPayload` 则是验证后的本地 auth context payload。两者都不是跨服务共享 `security.Actor` 的替代物。

微服务有三种常见请求认证路径：网关预检后由 Resource Server 本地验证 JWT；Resource Server 调用 token issuer 的 introspection 查询 token 当前状态；或完全信任受认证的网关 assertion。把所有 claims 枚举到 `security/authn/jwt` 会把不同 issuer 的业务语义错误地绑在一个实现包中。

## Decision

1. 每个 token issuer 拥有自己签发 token 的 claims schema。IAM 可以定义 `UserClaims`，CMS、Mall 或其他服务也可以定义自己的 claims 扩展；这些类型不由 `security/authn/jwt` 统一枚举。
2. `security/authn/jwt` 只接收调用方提供的 claims 类型与 Actor 映射策略。Resource Server 可以为自己消费的 token 定义本地最小 decoder；它必须遵守 issuer 发布的 JWT wire semantics，不能把未验证或未加载的业务字段当作授权事实。
3. 跨服务不共享 IAM claims Proto 或 `api/gen` claims Go type。共享的是最小必要的 JWT wire semantics；Resource Server 只定义自身需要消费的本地 claims decoder。Profile 至少明确 issuer、audience/resource、签名算法、KID、token type、有效期、subject，以及 human/service Actor 映射规则。
4. CMS、Audit、Admin 等业务服务通常只消费已验证 Actor 和本地业务事实，不消费 IAM token 的完整业务 claims。确需读取时，只读取并验证标准 claims 与最小 issuer-specific 字段。
5. 默认认证链采用“网关预检 + Resource Server 本地验证 JWT”。网关不得通过未保护的 `X-User-ID`、`X-Role` 等明文 header 传递可信身份。网关唯一认证边界必须使用 mTLS/受信服务认证和完整性受保护 assertion。
6. Opaque/reference token 或即时撤销要求使用受保护的 introspection；这不是 JWT 默认路径。introspection 故障时受保护请求 fail closed，并由服务按策略使用有界缓存。
7. 当前 `Actor` 只表示单一执行主体。服务代表 human 调用下游时，delegation/impersonation、RFC 8693 token exchange 和双主体 context 留给后续 IAM change。
8. JWT 配置拆分：`platform/security/authn/jwt/v1` 的 `JwtAuthnConfig` 独占 section key `jwt`；`platform/security/jwt/v1` 只提供可嵌套 reusable key/config message，不独立扫描。未来若独立扫描基础 key config，使用 `jwt_keys`，不得复用 `jwt`。

## Consequences

- Resource Server 不需要导入 IAM service 或 `security/authn/jwt` 的共享业务 claims package，减少跨服务编译耦合。
- JWT 本地验证避免 token issuer 成为每个业务请求的延迟和可用性依赖，但要求各服务正确执行 issuer/audience/KID/time/token-type 校验并处理 key rotation。
- 不共享 Go struct 不等于不共享协议语义；issuer 仍需发布稳定、可版本化的 wire profile。
- 需要即时撤销、opaque token 或跨服务 delegation 时，必须另立 IAM 设计，不在本 change 偷加全局 claims/context 抽象。

## References

- RFC 6750：Bearer token 传输与 Resource Server 验证边界。
- RFC 7662：OAuth 2.0 Token Introspection。
- RFC 8693：OAuth 2.0 Token Exchange 与 delegation/impersonation。
- RFC 9068：JWT Profile for OAuth 2.0 Access Tokens。
