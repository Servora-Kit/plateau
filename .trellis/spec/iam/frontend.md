# 认证页面、请求与状态

适用于 `app/iam/web/`，跨端边界见 [架构](architecture.md)，交互生命周期见 [认证流程](auth-flows.md)，HTTP 规则见 [共享 Web client](../web/client/index.md)。

- Next.js App Router 的页面负责提交与导航，组件负责布局和可访问字段，hook 负责复用状态，lib 负责请求装配、校验与错误映射；不把 transport 或敏感值处理复制进页面。
- 浏览器 API、state 和事件处理使用 `"use client"`；认证流程保持 CSR，不混入未经设计的 SSR 数据获取。
- 页面调用统一装配的生成 API 与 workspace `@plateau/client`，不直接实现 `fetch` 或另造 client。
- 提交采用单飞控制与 AbortController，卸载取消活动请求；敏感输入在提交结束后显式清除，卸载清理不能替代提交后的清理。
