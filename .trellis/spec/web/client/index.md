# 共享 Web Client

适用于修改 `web/packages/client/`，或在应用中接入该包的任务。这里定义共享浏览器通信能力；应用自身的路由、状态和业务错误文案仍由各应用规范负责。

## 开发前检查

- 确认调用方是否已经使用共享 client。当前 IAM 使用它；Example 使用自己的 `ClientTransport`；Test 只导入生成 API 做构建验证。不得据此把某一种实现推广到全部前端。
- 读取所改协议对应的 [HTTP](http.md)、[SSE](sse.md) 或 [WebSocket](websocket.md)，并核对生成客户端的 `unary`、`serverStream`、`duplexStream` 调用形态。
- 浏览器相对地址与服务端绝对地址的约束不同；使用 `createHttpClient` 的服务端代码必须提供绝对 HTTP(S) `baseURL`。

## 质量检查

- 修改 HTTP 行为时运行 `pnpm --filter @plateau/client test` 与 `pnpm --filter @plateau/client typecheck`。
- 修改已接入的 IAM 请求时，再运行 `just web::iam::lint` 与 `pnpm --filter @plateau/iam-web run typecheck`。
- 流式协议当前只有实现证据；接入某个应用后，应按真实服务端点补充可重复的浏览器或集成验证，不能把包内 HTTP 测试表述为流式端到端验收。

## 主题索引

| 主题 | 内容 |
| --- | --- |
| [架构与消费边界](architecture.md) | 包导出、应用接入现状和生成客户端适配 |
| [HTTP](http.md) | 请求、取消、超时、重试与 `ApiError` 契约 |
| [SSE](sse.md) | 浏览器服务端流的生命周期与失败分类 |
| [WebSocket](websocket.md) | 双向流、缓冲和控制帧约束 |

来源：[`web/packages/client/src/index.ts`](../../../../web/packages/client/src/index.ts)、[`web/packages/client/package.json`](../../../../web/packages/client/package.json)。
