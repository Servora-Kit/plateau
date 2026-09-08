# AGENTS.md - app/example/service/internal/server/

<!-- Parent: ../../AGENTS.md -->
<!-- Updated: 2026-07-30 -->

## Scope

Transport assembly for the CRUD reference service.

## Rules

- Register the generated `UserService` contract on both internal gRPC and Kratos HTTP; do not define a duplicate HTTP-only Proto service.
- Keep transport setup free of CRUD business semantics; service/biz own name, scope, lifecycle, and AIP-164 decisions.
- 本地 HTTP/gRPC 分别监听 `127.0.0.1:10030`、`127.0.0.1:10031`；不要在此处管理数据库或浏览器生命周期。
