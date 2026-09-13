# OIDC 与服务令牌

`internal/oidc` 是 IAM 的协议专有模块。`IAMProvider` 只注册 discovery、authorize、token、userinfo、revoke、end-session 和 JWKS 路由；其构造校验 issuer、密钥和 provider 配置，见 [provider.go](../../../app/iam/service/internal/oidc/provider.go)。当前 authorization request 只接受 authorization-code、query response mode、PKCE S256 和 `openid` scope；这些是当前实现约束，修改前必须同步评估 discovery 元数据、存储和 provider 测试。

授权回调先确认 SCS 已加载，再从登录引用解析活跃用户/会话，之后才完成授权请求。不要让 OIDC callback 绕过 `SessionUsecase.Resolve`。静态 OAuth client 与签名密钥元数据由 [OIDCInitializer](../../../app/iam/service/internal/oidc/initializer.go) 在服务可用前校验和协调；这不是请求期的惰性初始化。

用户 OAuth code/refresh 发行在 [OAuthRepository.Issue](../../../app/iam/service/internal/data/oauth.go) 的事务中完成，并以 `lockUser(...).ForUpdate()` 串行化身份变化、登录状态和令牌发行。authorization code 以未消费条件写入消费时间；refresh token 轮换时消费旧 token、建立后继 token。发现已消费 refresh token 的重放时，当前实现撤销其 OAuth token session 并返回无效 grant。不要把这些并发/重放分支拆成独立的非事务读写。

服务 token 只来自允许 `client_credentials` 且已注册 audience 的 client；它没有 refresh token。`OIDCStorage` 为该请求写入 `token_use=access` 与 `actor_type=service`，并按配置使用正的 service access-token TTL（默认五分钟），见 [service_token.go](../../../app/iam/service/internal/oidc/service_token.go) 与 [storage.go](../../../app/iam/service/internal/oidc/storage.go)。服务 API 的 `ServiceClaims` 还要求 `sub=client_id`、签发时间和 token ID；认证器以 IAM OIDC issuer 与 audience `iam` 验证 token，见 [authn/jwt.go](../../../app/iam/service/internal/authn/jwt.go)。通过这些校验只得到服务身份，业务权限仍需 receiving service 的策略决定和执行。

操作权限、管理关系和接收服务执行检查的职责统一引用 [IAM 授权规范](authorization.md)，不从 token claims 推导额外管理员权限。
