# SSE 服务端流

`createServerStream` 在 [`sse.ts`](../../../../web/packages/client/src/sse.ts) 中实现，供生成 transport 的 `serverStream` 调用。它只能在浏览器运行：创建时检查 `window` 与 `document`，并在 `pagehide`、显式 `close()` 或传入 signal 取消时释放连接。

## 协议和失败处理

- 建连固定使用 GET、client 的 credentials 和 `text/event-stream` Accept；成功响应必须带该 Content-Type 且有 body。
- 仅接收空事件、`message` 或无 event 名的 JSON 数据；事件名为 `error`、无效 JSON、无效内容类型和非暂时 HTTP 响应都会终止流并触发 `onError`。
- `408`、`429`、`500`、`502`、`503`、`504` 与建连超时会重试。服务端 `retry` 值受 30 秒上限约束，否则采用抖动指数退避。

调用方必须保留 `onEvent` 返回的取消函数，并在页面、订阅或 route 生命周期结束时调用 `close()` 或取消 signal；不得让流继续写入已离开的页面。

当前仓库未发现业务前端调用 `createServerStream`、`serverStream` 或 `@plateau/client` 的流接口。这里记录的是已实现的共享能力，未宣称任何业务 SSE 端点已联调验收。
