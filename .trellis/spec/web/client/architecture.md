# 共享 Client 架构与消费边界

`@plateau/client` 是 workspace 包，入口在 [`web/packages/client/src/index.ts`](../../../../web/packages/client/src/index.ts)。它把 HTTP client 与生成 TypeScript API 所需的 transport 组合在一起，并导出 SSE、WebSocket 的流构造器；它不定义业务服务接口、页面状态或业务错误文案。

## 生成 API 的接入

生成客户端需要 transport。IAM 的 [`createIamApi`](../../../../app/iam/web/lib/iam-api.ts) 先以 `createHttpClient({ service: "iam" })` 建立 client，再以 `createTransport` 创建 transport，并传给 `createAccountServiceClient`、`createAuthnServiceClient` 和 `createSessionServiceClient`。新增 IAM API 时沿用此单一装配入口，避免页面各自创建不一致的 transport。

`createTransport` 为 unary 调用设置 `Accept: application/json`；请求体存在时设置 `Content-Type: application/json`，并把同一 `AbortSignal` 传给 HTTP、SSE 和 WebSocket。实现见 [`index.ts`](../../../../web/packages/client/src/index.ts)。生成 API 和错误 sidecar 的来源规则由 `api/proto` 规范定义，应用不手写同名契约。

## 当前应用差异

- IAM 已声明并实际导入 workspace `@plateau/client`，其 Next 配置也将该包加入 `transpilePackages`，证据见 [`package.json`](../../../../app/iam/web/package.json)、[`next.config.ts`](../../../../app/iam/web/next.config.ts) 与 [`lib/iam-api.ts`](../../../../app/iam/web/lib/iam-api.ts)。
- Example 明确使用应用自有的原生 `fetch` `ClientTransport`，证据见 [`AGENTS.md`](../../../../app/example/web/AGENTS.md) 与 [`src/api/transport.ts`](../../../../app/example/web/src/api/transport.ts)。它不是共享 client 的退化实现，也不应被擅自迁移。
- Test 只在页面中导入 `@plateau/api` 的生成 client、资源名 helper 与错误枚举；`package.json` 未依赖 `@plateau/client`，当前没有请求适配器。证据见 [`app/page.tsx`](../../../../app/test/web/app/page.tsx) 与 [`package.json`](../../../../app/test/web/package.json)。

因此，选择共享 client 前先确认应用的运行时、代理和既有 adapter；不要把共享包的流式能力假定为每个应用已经启用。
