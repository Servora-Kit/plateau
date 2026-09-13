# WebSocket 双向流

`createDuplexStream` 在 [`websocket.ts`](../../../../web/packages/client/src/websocket.ts) 中实现，供生成 transport 的 `duplexStream` 调用。它在浏览器中把 HTTP(S) 地址转换为 WS(S)，并以 JSON 文本帧传输数据。

## 使用约束

- 浏览器 WebSocket 不支持自定义握手请求头；传入 `headers` 会使流以错误结束。认证方案必须与浏览器握手能力和服务端约定一致。
- `send` 在连接建立前排队，默认累计上限为 1 MiB（含 native `bufferedAmount`）；超过上限会抛出 `RangeError`。需要不同上限时显式传入正安全整数 `maxBufferedBytes`。
- `closeSend()` 发送记录分隔符开头的 `end` 控制帧；远端可发送 `end` 或 `error:` 控制帧。未知控制帧、非文本帧、非 JSON 数据帧和异常 close 都会结束并报错。
- 组件或页面离开时调用 `close()` 或取消 signal；实现同样监听 `pagehide`，但调用方不应依赖浏览器卸载时机处理业务清理。

当前仓库没有业务端对该能力的消费或端到端测试。接入前需要同时确认生成 API 是否声明双向流、服务端控制帧约定和浏览器可用的认证方式。
