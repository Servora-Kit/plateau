# HTTP 请求

`createHttpClient` 的实现位于 [`http-client.ts`](../../../../web/packages/client/src/http-client.ts)，底层使用 `ofetch`，失败向调用方归一为 `@servora/proto-utils/errors` 的 `ApiError`。业务页面应保留自身的错误分支和文案，不应只把异常字符串直接展示。

## 地址与凭据

浏览器默认 `baseURL` 为 `/`，相对路径按当前站点 origin 解析；Node 环境拒绝非绝对的 HTTP(S) `baseURL`。默认 `credentials` 是 `same-origin`，因此依赖同源 cookie 的应用应维持代理或 rewrite 边界，而不是在页面里拼后端 host。

`createTransport` 的 unary JSON 协商见 [架构](architecture.md)；通用 `createHttpClient` 只传递调用方 headers，不据此承诺 `application/protojson` 兼容。服务端协议分流见 [Servora transport](../../servora/framework/transport.md)。

## 取消、超时和重试

- 默认超时为 10 秒；调用方 `AbortSignal` 与内部超时控制器独立，取消结果为 `ApiError.kind === "cancelled"`，超时为 `"timeout"`。
- 默认只对 GET 重试一次；网络错误和 `408`、`429`、`500`、`502`、`503`、`504` 可重试。写请求默认不重试，避免重复提交。
- `operation` 可覆盖 `service`、`method` 元数据，便于 `ApiError` 归因。生成 transport 会将生成器提供的元数据传入。

新增行为须覆盖读取重试、写入不重试、响应体超时、取消和相对路径解析，不以构建通过替代协议检查。

## 调用方责任

将生命周期对应的 `AbortSignal` 传入 transport 或请求，并在生命周期结束时取消。`ApiError` 的 transport message 只说明网络、HTTP、取消或超时事实；页面须根据 kind、HTTP 状态和业务 reason 决定安全的可见反馈。
