# 共享 Web Client

适用于修改 `web/packages/client/`，或在应用中接入该包的任务。这里定义共享浏览器通信能力；应用自身的路由、状态和业务错误文案仍由各应用规范负责。

## 开发前检查

- 接入前确认应用运行时、代理和既有 adapter，不把共享包能力视为所有应用已启用的行为。
- 读取所改协议对应的 [HTTP](http.md)、[SSE](sse.md) 或 [WebSocket](websocket.md)，并核对生成客户端的 `unary`、`serverStream`、`duplexStream` 调用形态。
- 浏览器相对地址与服务端绝对地址的约束不同；使用 `createHttpClient` 的服务端代码必须提供绝对 HTTP(S) `baseURL`。

## 质量检查

- 修改 HTTP 行为时运行 `pnpm --filter @plateau/client test` 与 `pnpm --filter @plateau/client typecheck`。
- 修改应用调用契约时，同时检查消费应用；流式协议须按真实端点进行浏览器或集成验证，HTTP 单测不替代流式端到端检查。

## 主题索引

| 主题 | 内容 |
| --- | --- |
| [架构与消费边界](architecture.md) | 包导出、装配职责和生成客户端适配 |
| [HTTP](http.md) | 请求、取消、超时、重试与 `ApiError` 契约 |
| [SSE](sse.md) | 浏览器服务端流的生命周期与失败分类 |
| [WebSocket](websocket.md) | 双向流、缓冲和控制帧约束 |
