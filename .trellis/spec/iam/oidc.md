# OIDC Provider 与服务令牌

- Provider 构造时校验 issuer、签名密钥与配置；静态 client 和密钥元数据在服务可用前协调，见 [启动](startup.md)。
- 授权请求限定 authorization-code、query response mode、PKCE S256 与 `openid` scope；能力变更同步 discovery 元数据和存储契约。
- 授权回调先确认 SCS 已加载并解析活跃身份与登录会话，不绕过 [会话校验](sessions.md)。
- 授权请求完成、code 保存、人类令牌签发/刷新与 userinfo 投影均须校验非软删除 `ACTIVE` 身份、主邮箱已验证且 `required_actions` 为空；code 与 refresh 发行在事务中锁定身份，code 条件消费，refresh 轮换消费旧 token 并建立后继，重放已消费 refresh 时撤销对应 OAuth 会话并返回 invalid grant。
- 服务 token 仅发给允许 `client_credentials` 且已注册 audience 的 client，不提供 refresh token，access-token TTL 必须为正。
- 服务认证要求 `token_use=access`、`actor_type=service`、`sub=client_id`、签发时间与 token ID，并校验 issuer 和接收服务声明的 audience。
- token 校验仅产生身份，不产生额外管理权限；接收服务独立执行 [授权](authorization.md)。
