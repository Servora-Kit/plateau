# Platform JWT primitive 与 JWT AuthN 边界

> Status: accepted

Platform 将 JWT 分为 `security/jwt` primitive 与 `security/authn/jwt` 具体认证实现：前者只负责 RS256、KID、Signer、Verifier 和 claims-neutral 验签，后者负责 Bearer 提取、issuer/audience/expiry 校验、claims 到 Actor 的映射和认证错误。验证后的 claims 默认只在认证调用内存活，不进入 shared Actor 或 primitive context；JWT AuthN verifier config 只拥有 issuer、audience 和 public KID key set，private signing key、access/refresh TTL 和业务 claims policy 留给未来 IAM/infra key-source。Verifier 第一阶段构造后不可变，密钥轮换通过替换整个 Verifier，避免 primitive 引入并发可变 key registry。

## Considered Options

- **把所有 JWT 放入 infra/security/jwt**：拒绝。RS256/KID/验签是安全协议 primitive，不是部署接线；只有 key loader、KMS/HSM 和轮换接线属于 infra。
- **把完整 claims 放入 shared security/jwt context**：拒绝。会把业务 tenant、role、scope 和 provider claims 重新提升为共享主体上下文。
- **在第一阶段支持运行时增删 KID**：延后。当前没有真实轮换消费者，构造期 immutable key set 更简单、更容易保证并发安全。
