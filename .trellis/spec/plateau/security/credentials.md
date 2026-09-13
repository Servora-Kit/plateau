# 密码、JWT 与 HTTP 会话工具

这些工具提供独立能力，身份解析、登录状态和授权策略由消费方拥有。

## 密码

[password.Hash/Compare](../../../../security/password/password.go) 使用 Argon2id PHC 字符串；Hash 每次生成独立随机盐。当前参数是 memory=64×1024、iterations=3、parallelism=1、saltLength=16、keyLength=32。调用 Compare 必须分别处理 match、needsRehash、err；只有密码匹配成功后才考虑重哈希，不把 needsRehash 当作登录成功。

不记录明文密码、哈希、签名私钥或会话 token。修改参数时用 [password tests](../../../../security/password/password_test.go) 检查独立盐、错误编码和升级信号，已有哈希的升级由应用负责。

## JWT 工具与认证分离

[Signer/Verifier](../../../../security/jwt/jwt.go) 支持 RSA 签名／RS256 验签、KID 和公钥隔离；签名器接受有效的 RSA PKCS1/PKCS8 输入。Verifier 要求已知字符串 KID 和 RSA 公钥，但 `VerifySignature` 不拥有 issuer、audience、过期或业务 claims policy。应用 AuthN 须按 [authN](authN.md) 完成后续校验。

[JWKS](../../../../security/jwt/jwks.go) 构造返回 verifier 与 cleanup；初始加载失败不能当作可用实例。密钥刷新生命周期由创建它的应用管理，不能忽略 cleanup。不要用共享密钥工具接管 IAM OIDC token 生命周期。

## Session 工具

[session.New](../../../../security/session/session.go) 接收显式配置和应用注入的 SCS Store，返回独立 manager；Store 命名空间与关闭由应用负责。配置克隆后校验 lifetime、idle timeout、cookie path、SameSite 及 cookie 前缀约束，默认启用 token hash 存储。

`LoadAndSave` 调用 SCS 生命周期并记录 manager 专属装载标记；保存失败中断成功响应正文。应用不能绕过装载检查把缺少接线当作普通未登录。IAM 的 OP/RP 会话职责由 [IAM sessions](../../iam-service/sessions.md) 定义。

检查入口：`go test ./security/password ./security/jwt ./security/session`。依据：[JWT tests](../../../../security/jwt/jwt_test.go)、[JWKS tests](../../../../security/jwt/jwks_test.go)、[Session tests](../../../../security/session/session_test.go)。
