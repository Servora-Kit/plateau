# `@servora/proto-utils`

包提供 `/transport`、`/crud`、`/errors`、`/error-reasons/*` 和 `/proto/*` subpath export。共享 API 保持小且稳定，不包含页面状态、路由、业务 store、toast/文案、Token、HTTP 重试或 React/Vue adapter。

`src/transport.ts` 导出 `TransportMeta`、`ClientTransport`、`ServerStream`、`DuplexStream` 及遵循 RFC 3986 的 `encodePathSegment`、`encodeMultiSegmentPath`，供 `protoc-gen-typescript-http` 生成产物与上层客户端按需导入。`src/crud` 提供无状态的资源名、FieldMask、AIP filter/order 和 pager helper：`makeUpdateMask` 按生成 fields 确定顺序，对象自身含键即表示选择/clear 意图（值可为 `undefined`）；`buildFilter`/`buildOrderBy` 拒绝未知字段和非法值；资源名只处理未 URL 编码的 canonical name，百分号编码由 `transport` 负责。

`ApiError` 区分 http、network、timeout、cancelled；`parseKratosErrorBody` 仅接受有效的 `{ code, reason, message, metadata? }` envelope，`isKratosReason` 精确比较 reason。不猜测业务错误或将任意响应强制转换为 Kratos 错误。

包为 ESM，`./error-reasons/*` 指向生成的 `*.errors.js` sidecar。TypeScript 使用 `moduleResolution: bundler`，非 NodeNext；生成器保持 package-relative `.js` type-only import，并由 build/typecheck 检查。HTTP 生成器将全部 64 位整数字段映射为 `string`，保持 ProtoJSON wire 契约。

`src/gen` 由 Buf 写入，禁止手改；业务仓库的 HTTP client、认证和 transport composition root 仍由各业务仓库维护。修改源码后运行 `just web-check`、`just web-typecheck` 与 `just web-test`（含 `just web-build` 与 Node 单元测试）。
