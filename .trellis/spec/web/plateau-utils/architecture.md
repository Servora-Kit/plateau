# `@servora/plateau-utils` 架构与导出契约

`@servora/plateau-utils` 位于 [`web/packages/plateau-utils/`](../../../../web/packages/plateau-utils/)，开启 `"sideEffects": false`，通过 `tsc -p tsconfig.build.json` 构建 `dist/` 并在 `prepare` 钩子自动编译，公开四项子路径导出：`.` 与 `./client`（HTTP/SSE/WebSocket 传输客户端）、`./crud`（无状态资源名错误、FieldMask、AIP filter/order 与 pager 辅助工具）、`./error-reasons/*`（由 `buf.typescript.gen.yaml` 按 `types` 过滤生成的 `plateau/crud/v1` 与 `plateau/security/errors/v1` 错误枚举及类型守卫）。它不定义业务服务接口、页面状态或业务错误文案。

## 生成 API 的接入

应用以 `createHttpClient` 建立 client，再通过 `createTransport` 创建 transport 并注入生成客户端。装配由应用集中管理，避免页面各自创建不一致的 transport。

`createTransport` 为 unary 调用设置 `Accept: application/json`；对所有非安全方法（非 `GET`/`HEAD`，含无请求体的 `POST`/`DELETE`）始终设置 `Content-Type: application/json`，并将同一 `AbortSignal` 传给 HTTP、SSE 和 WebSocket。

选择共享 client 前确认应用的运行时、代理和既有 adapter；生成 API 和错误 sidecar 的来源规则见 [API 生成规范](../../api/proto/generation.md)，应用不手写同名契约。
