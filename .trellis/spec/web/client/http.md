# HTTP 请求

`createHttpClient` 的实现位于 [`http-client.ts`](../../../../web/packages/client/src/http-client.ts)，底层使用 `ofetch`，失败向调用方归一为 `@servora/proto-utils/errors` 的 `ApiError`。业务页面应保留自身的错误分支和文案，不应只把异常字符串直接展示。

## 地址与凭据

浏览器默认 `baseURL` 为 `/`，相对路径按当前站点 origin 解析；Node 环境拒绝非绝对的 HTTP(S) `baseURL`。默认 `credentials` 是 `same-origin`，因此依赖同源 cookie 的应用应维持代理或 rewrite 边界，而不是在页面里拼后端 host。

IAM 的 Next rewrite 将 `/v1`、`/cap`、OIDC 等同源路径转发至 `IAM_BACKEND_ORIGIN`，见 [`next.config.ts`](../../../../app/iam/web/next.config.ts)。`createIamApi` 因而不传 `baseURL`。这条做法只适用于 IAM 当前拓扑。

`createTransport` 为生成 unary 调用固定协商 `application/json`，并在 body 存在时设置同一 Content-Type，见 [`index.ts`](../../../../web/packages/client/src/index.ts)。通用 `createHttpClient` 只传递调用方 headers；当前源码没有建立 `application/protojson` 的 client 兼容承诺。ProtoJSON 与普通 JSON 的服务端协议分流属于 Servora transport 约束，不能在此把历史要求写成当前 client 已支持的能力。

## 取消、超时和重试

- 默认超时为 10 秒；调用方 `AbortSignal` 与内部超时控制器独立，取消结果为 `ApiError.kind === "cancelled"`，超时为 `"timeout"`。
- 默认只对 GET 重试一次；网络错误和 `408`、`429`、`500`、`502`、`503`、`504` 可重试。写请求默认不重试，避免重复提交。
- `operation` 可覆盖 `service`、`method` 元数据，便于 `ApiError` 归因。生成 transport 会将生成器提供的元数据传入。

这些行为由 [`test/http.test.ts`](../../../../web/packages/client/test/http.test.ts) 覆盖读取重试、写入不重试、响应体超时、取消和相对路径解析。新增行为应扩展这些针对性测试，不以构建通过替代协议验证。

## 调用方责任

将生命周期对应的 `AbortSignal` 传入 transport 或请求。例如 IAM 的 [`useProfile`](../../../../app/iam/web/hooks/use-profile.ts) 在 effect 清理时取消请求，[`useSingleFlight`](../../../../app/iam/web/hooks/use-single-flight.ts) 防止重复提交并在组件卸载时取消活动请求。`ApiError` 的 transport message 只说明网络、HTTP、取消或超时事实；页面仍须根据 kind、HTTP 状态和业务 reason 决定安全、应用自有的可见反馈。
