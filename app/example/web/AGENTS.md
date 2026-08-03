# AGENTS.md - Example Web

## Scope

Vue reference client for `example.service.v1.User`. It must exercise the public HTTP facade through generated TypeScript code and `@servora/proto-utils`; it is not a second source of API contracts.

## Rules

- Keep resource identity `example.servora.dev/User` and `tenants/{tenant}/users/{user}` aligned with the service Proto.
- Import generated API code from `@/api/generated/**`; never copy generated types by hand and never edit generated output.
- Build resource names, filters, order expressions, update masks, and pagination state with generated CRUD helpers and `@servora/proto-utils`.
- Send requests through an application-owned `ClientTransport` adapter (native fetch in this example); preserve `application/json` content negotiation with canonical ProtoJSON payloads and structured `ApiError` handling.
- Import business error reason type/value and membership guards from generated `*.errors.ts` sidecars. Keep reason-to-copy maps, safe backend-message fallback, and UI behavior application-owned.
- The Vite `/v1` proxy targets the local example service on `127.0.0.1:28080`. Do not start Audit or database containers for this app.
- Verify the request path by running `just service::example::run` in the service leaf, starting Vite with `just web::example::dev`, and exercising CRUD in a real browser; do not add a browser-test lifecycle to CI.
- Keep forms keyboard-operable, labels connected, status updates announced, and destructive actions explicit.
