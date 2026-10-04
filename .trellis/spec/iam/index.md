# 身份与认证规范

适用于 `app/iam/service` 与 `app/iam/web`：服务端拥有身份、认证会话和 OIDC Provider，Web 提供同源交互界面，不拥有业务资源授权。

| 层/职责 | 规范 |
| --- | --- |
| 跨端与公开入口 | [架构](architecture.md) |
| 页面、请求与状态 | [前端](frontend.md) |
| 服务分层 | [后端](backend.md) |
| 认证交互生命周期 | [认证流程](auth-flows.md) |
| 身份与凭据 | [身份](identity.md) |
| 会话与撤销 | [会话](sessions.md) |
| Provider 与服务令牌 | [OIDC](oidc.md) |
| 策略与执行边界 | [授权](authorization.md) |
| 初始化所有权 | [启动](startup.md) |

按变更职责组合读取；公开入口、会话或协议变更须同时审查前后端边界。
