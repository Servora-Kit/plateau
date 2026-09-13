# AGENTS.md - Example Web

## Scope

当前应用规范见 [example-web](../../../.trellis/spec/example-web/index.md)，其中 [architecture](../../../.trellis/spec/example-web/architecture.md)、[requests](../../../.trellis/spec/example-web/requests.md) 和 [forms](../../../.trellis/spec/example-web/forms.md) 分别维护应用组织、请求合同与交互要求。

Vue reference client for `example.service.v1.User`. It must exercise the public HTTP facade through generated TypeScript code and `@servora/proto-utils`; it is not a second source of API contracts.

## Rules

- Keep resource identity `example.servora.dev/User` and `tenants/{tenant}/users/{user}` aligned with the service Proto.
- Import generated API code from `@/api/generated/**`; never copy generated types by hand and never edit generated output.
- Build resource names, filters, order expressions, update masks, and pagination state with generated CRUD helpers and `@servora/proto-utils`.
- Send requests through an application-owned `ClientTransport` adapter (native fetch in this example); preserve `application/json` content negotiation with canonical ProtoJSON payloads and structured `ApiError` handling.
- Import business error reason type/value and membership guards from generated `*.errors.ts` sidecars. Keep reason-to-copy maps, safe backend-message fallback, and UI behavior application-owned.
- Vite 使用本地端口 `10032`，`/v1` 代理指向 Example 服务的 `127.0.0.1:10030`；不要为此应用启动 Audit 或数据库容器。
- Verify the request path by running `just service::example::run` in the service leaf, starting Vite with `just web::example::dev`, and exercising CRUD in a real browser; do not add a browser-test lifecycle to CI.
- Keep forms keyboard-operable, labels connected, status updates announced, and destructive actions explicit.
