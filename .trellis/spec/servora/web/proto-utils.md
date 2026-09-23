# `@servora/proto-utils`

该包以一个 npm package 提供 `/crud`、`/errors` 和 `/proto/*` subpath export。导出 API 是多个业务仓库的共享契约，因此保持小且稳定；不要放页面状态、路由、业务 store、toast/文案、Token、HTTP 重试或 React/Vue adapter。完整包边界在 [`web/AGENTS.md`](../../../../../servora/web/AGENTS.md)。

`src/crud` 只提供无状态的资源名、FieldMask、AIP filter/order 和 pager helper。`makeUpdateMask` 以生成的 fields 确定字段顺序，并把对象自身含有该键（值可以是 `undefined`）视为选择/clear 意图；`buildFilter`/`buildOrderBy` 拒绝未知字段和非法值，证据在 [`src/crud/index.ts`](../../../../../servora/web/packages/proto-utils/src/crud/index.ts) 与 [`test/crud.test.mjs`](../../../../../servora/web/packages/proto-utils/test/crud.test.mjs)。资源名只处理未 URL 编码 canonical name；百分号编码属于 transport。

`ApiError` 区分 http、network、timeout、cancelled；`parseKratosErrorBody` 仅接受 `{ code, reason, message, metadata? }` 的有效运行时 envelope，`isKratosReason` 精确比较 reason，见 [`src/errors.ts`](../../../../../servora/web/packages/proto-utils/src/errors.ts) 和 [`test/errors.test.mjs`](../../../../../servora/web/packages/proto-utils/test/errors.test.mjs)。不要在 helper 中猜测业务错误或把任意响应强制转换为 Kratos 错误。

包为 ESM，`./error-reasons/*` export 指向生成的 `*.errors.js` sidecar，见 [`package.json`](../../../../../servora/web/packages/proto-utils/package.json)。当前 TypeScript 配置使用 `moduleResolution: bundler`，不是 NodeNext；因此保持生成器的 package-relative `.js` type-only import，并以当前 build/typecheck 验证。TypeScript HTTP 生成器将全部 64 位整数字段映射为 `string` 以保持 ProtoJSON wire 合同，证据在 [`type.go`](../../../../../servora/cmd/protoc-gen-typescript-http/internal/plugin/type.go) 和 [`generate_test.go`](../../../../../servora/cmd/protoc-gen-typescript-http/internal/plugin/generate_test.go)。

`src/gen` 由 Buf 写入，禁止手改；业务仓库的 HTTP client、认证和 transport composition root 仍由各业务仓库维护。修改源码运行 `just web-typecheck`、`just web-build`；改动导出或序列化行为还应运行对应 Node 测试。
