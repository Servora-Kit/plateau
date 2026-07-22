# AGENTS.md - app/example/service/internal/server/

<!-- Parent: ../../AGENTS.md -->
<!-- Updated: 2026-07-21 -->

## Scope

Transport assembly for the CRUD reference service.

## Rules

- Register the generated `UserService` on internal gRPC and the generated `UserHTTPService` on Kratos HTTP.
- Keep transport setup free of CRUD business semantics; service/biz own name, scope, lifecycle, and AIP-164 decisions.
- HTTP listens on `127.0.0.1:28080` for local use; do not add database or browser lifecycle management here.
