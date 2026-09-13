# Example 请求与生成 API

Example 的生成 API 从 `@/api/generated/**` 导入。`userApi.ts` 以 [`transport.ts`](../../../app/example/web/src/api/transport.ts) 创建 `createUserServiceClient`；资源名、字段表、更新 mask、filter、order 和分页必须使用生成的 `user.crud.ts` 以及 `@servora/proto-utils/crud` helper，具体用法见 [`stores/users.ts`](../../../app/example/web/src/stores/users.ts)。

## 应用自有 transport

该 adapter 是原生 `fetch` 实现，默认请求地址由 `VITE_API_BASE_URL` 或同源相对路径决定；Vite 在开发和预览时将 `/v1` 代理至 `127.0.0.1:10030`，配置见 [`vite.config.ts`](../../../app/example/web/vite.config.ts)。它设置 `Accept: application/json`，仅在 body 非空时设置 `Content-Type: application/json`，并直接传递已经序列化的 ProtoJSON body。

适配器将超时转为 `ApiError.kind === "timeout"`，原生 fetch `TypeError` 转为 `"network"`，非成功 HTTP 响应保留 status、响应体和生成调用提供的 service/method 元数据。服务端流和双向流会明确抛出“不支持”，因为当前 User reference API 没有这些调用。

## 业务错误与 CRUD

`failureMessage` 先处理网络和超时，再以 `parseKratosError` 解出 reason。已知的 `UserErrorReason` 映射为应用文案；未知但可读的后端 message 可展示；机器 reason 不作为兜底文案。错误 enum 和 guard 必须来自生成 sidecar，不手写同名枚举。

列表使用 `buildFilter`、`buildOrderBy`、`applyPager` 和 `advancePager`；更新使用 `makeUpdateMask`、当前 `etag` 和资源名 helper。创建结果会核验生成的 `UserName.format`，避免把异常响应当成正确资源。请求 adapter 的 HTTP 契约由 [`transport.test.ts`](../../../app/example/web/src/api/transport.test.ts) 覆盖，reason 文案由 [`users.test.ts`](../../../app/example/web/src/stores/users.test.ts) 覆盖。
