# IAM 规范

适用于 IAM 应用：Go service 负责身份、认证会话、CAP 与 OIDC Provider 协议；Next Web 是同源 Provider UI。应用定位和跨端契约见 [架构](architecture.md)。

## 开发前检查

- 改前端页面、路由、请求或状态前读 [前端](frontend.md)；认证、账户和一次性链接流程再读 [认证流程](auth-flows.md)。
- 改 Go service 前读 [后端](backend.md)，再按变更主题读 [身份](identity.md)、[会话](sessions.md)、[OIDC](oidc.md)、[授权](authorization.md) 或 [启动](startup.md)。
- 改公开入口、同源转发、浏览器会话或 OIDC 路径前，先读 [架构](architecture.md)，并同时审查涉及的前后端实现。

## 质量检查

- 前端按 [前端检查入口](frontend.md#质量检查) 执行静态检查、构建与必要的页面验证。
- 后端按 [后端检查入口](backend.md#质量检查) 选择受影响测试；涉及 OIDC、OpenFGA、数据库或公开入口时，另行核验真实环境。

## 主题

| 主题 | 内容 |
| --- | --- |
| [架构](architecture.md) | 应用定位、Provider UI 与 Go backend 的跨端边界、同源公开入口 |
| [前端](frontend.md) | App Router、CSR 页面、组件、hook、状态、生成 API 与前端检查 |
| [后端](backend.md) | Go service 后端入口、专题选择与后端检查 |
| [认证流程](auth-flows.md) | 登录、注册、账户、重置和验证的前端生命周期 |
| [身份](identity.md) | 用户和服务身份、账号与密码规则 |
| [会话](sessions.md) | 浏览器会话、认证投影、当前注销能力 |
| [OIDC](oidc.md) | Provider 协议与服务令牌 |
| [授权](authorization.md) | IAM OpenFGA PDP 适配与业务 PEP/PAP 边界 |
| [启动](startup.md) | IAM 特有启动与 Wire 组合边界 |
