# 服务分层与主题边界

适用于 `app/iam/service`，组合 [共享微服务规范](../service/backend/index.md) 与本目录专题；应用专有组织不自动成为其他服务模板。

- 身份与凭据变更读 [身份](identity.md)，登录状态与撤销读 [会话](sessions.md)。
- 协议与服务令牌读 [OIDC](oidc.md)，受保护操作读 [授权](authorization.md)。
- 初始化、静态客户端或密钥处理读 [启动](startup.md)，不将启动副作用放入请求路径。
