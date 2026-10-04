# 共享 Client 架构与消费边界

`@plateau/client` 是 workspace 包，入口在 [`web/packages/client/src/index.ts`](../../../../web/packages/client/src/index.ts)。它把 HTTP client 与生成 TypeScript API 所需的 transport 组合在一起，并导出 SSE、WebSocket 的流构造器；它不定义业务服务接口、页面状态或业务错误文案。

## 生成 API 的接入

应用以 `createHttpClient` 建立 client，再通过 `createTransport` 创建 transport 并注入生成客户端。装配由应用集中管理，避免页面各自创建不一致的 transport。

`createTransport` 为 unary 调用设置 `Accept: application/json`；请求体存在时设置 `Content-Type: application/json`，并把同一 `AbortSignal` 传给 HTTP、SSE 和 WebSocket。

选择共享 client 前确认应用的运行时、代理和既有 adapter；生成 API 和错误 sidecar 的来源规则见 [API 生成规范](../../api/proto/generation.md)，应用不手写同名契约。
