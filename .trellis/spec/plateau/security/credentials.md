# 密码、JWT 与 HTTP 会话工具

这些工具提供独立能力，身份解析、登录状态和授权策略由消费方拥有。

## 密码

[password.Hash/Compare](../../../../security/password/password.go) 使用 Argon2id PHC 字符串；Hash 每次生成独立随机盐。当前参数是 memory=64×1024、iterations=3、parallelism=1、saltLength=16、keyLength=32。调用 Compare 必须分别处理 match、needsRehash、err；只有密码匹配成功后才考虑重哈希，不把 needsRehash 当作登录成功。

不记录明文密码、哈希、签名私钥或会话 token。修改参数时检查独立盐、错误编码和升级信号，已有哈希的升级由应用负责。

## JWT 工具与认证分离

[Signer/Verifier](../../../../security/jwt/jwt.go) 支持 RSA 签名／RS256 验签、KID 和公钥隔离；签名器接受有效的 RSA PKCS1/PKCS8 输入。Verifier 要求已知字符串 KID 和 RSA 公钥，但 `VerifySignature` 不拥有 issuer、audience、过期或业务 claims policy。应用 AuthN 须按 [authN](authN.md) 完成后续校验。

[JWKS](../../../../security/jwt/jwks.go) 构造返回 verifier 与 cleanup；初始加载失败不能当作可用实例。密钥刷新生命周期由创建它的应用管理，不能忽略 cleanup；共享密钥工具不接管应用 OIDC token 生命周期。

## Session 工具

[session.New](../../../../security/session/session.go) 接收显式配置和应用注入的 SCS Store，返回独立 manager；Store 命名空间与关闭由应用负责。配置克隆后校验 lifetime、idle timeout、cookie path、SameSite 及 cookie 前缀约束，默认启用 token hash 存储。

`LoadAndSave` 调用 SCS 生命周期并记录 manager 专属装载标记；保存失败中断成功响应正文。应用不能绕过装载检查把缺少接线当作普通未登录，OP/RP 会话职责由消费应用拥有。

[session.NewFilter](../../../../security/session/filter.go) 校验公开入口裸 origin，按应用声明的 `JSONPrefixes`、`SameOriginPaths` 与 `CrossOriginPaths` 分流防护和 `LoadAndSave`：JSON 路由前缀在装载会话前拦截非 `application/json` 写请求（415）与跨源请求（403）；应用自有同源路径在装载会话前拦截非安全跨源请求（403）；标准跨站协议路径仅装载会话；其余路径透传不触碰会话存储。


PostgreSQL 会话持久化统一使用 [HTTPSessionMixin](../../../../infra/entgo/mixin/http_session.go) 声明 `sessions` 表结构，并由 [session.NewPostgres](../../../../security/session/postgres.go) 绑定 `postgresstore`（配置校验失败时立即停止后台清理协程）。邮箱比较统一调用 [security.NormalizeEmail](../../../../security/identity.go)（trim + Unicode NFC + Unicode case-fold）。

## OIDC 客户端工具

[security/oidc](../../../../security/oidc/rp.go) 基于 `plateau.security.oidc.v1.OIDCClient`（`oidc_client` 配置段）提供两类共享能力：
- `NewRelyingParty`：使用 HKDF（SHA-256）从不少于 32 字节的 `cookie_secret` 派生独立签名与加密密钥，维护 5 分钟单次加密握手 Cookie（`{state, nonce, code_verifier, return_to}`），单飞惰性执行 OIDC Discovery，完成 PKCE S256、`nonce`、签名与站内相对 `return_to` 校验，并在需要时经 `/userinfo` 补齐邮箱与资料声明。
- `NewServiceTokenMiddleware`：维护进程内共享的 `client_credentials` 令牌源，通过 `singleflight` 合并冷启动/续期请求并监听当前请求 `ctx.Done()` 立即中断等待；取令牌失败统一包装为脱敏的 `ErrTokenSourceUnavailable`。

检查入口：`go test ./security/...`。
