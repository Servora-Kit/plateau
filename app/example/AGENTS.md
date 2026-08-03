# AGENTS.md - app/example/

<!-- Parent: ../AGENTS.md -->
<!-- Updated: 2026-07-21 -->

## Scope

`app/example/` is the runnable golden path for the single `example.servora.dev/User` CRUD resource.

## Structure

- `service/`: Kratos internal gRPC + HTTP facade, biz/data/Ent implementation, local SQLite runtime.
- `web/`: Vue request console consuming generated TypeScript clients and `@servora/proto-utils`.

## Rules

- Keep resource identity and request fields aligned between source Proto, generated Go/TypeScript, service, and web.
- The service owns the API contract; the web app never duplicates generated message or CRUD helper code.
- Keep CI database-free. Local verification explicitly runs `just service::example::run`, starts Vite, and exercises the real HTTP facade.
- Generated API, Ent, Wire, OpenAPI, and build output must not receive hand-written `AGENTS.md` files.
