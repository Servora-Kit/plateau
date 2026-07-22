# AGENTS.md - app/example/service

## Purpose

`app/example/service` is the single public CRUD reference service for `example.servora.dev/User`.

## Boundaries

- `internal/service` owns RPC wrappers, resource-name parsing, request preparation, and response cleanup.
- `internal/biz` owns business semantics, tenant scope, allow-missing, etag, and AIP-164 decisions.
- `internal/data` owns Ent predicates, explicit mutations, mapping, and persistence errors.
- Public Proto contracts come from `servora/api/protos/servora/example/v1`; this module does not define a second User API.

## Rules

- Use the generated typed User name and field helpers; do not hand-build field-path strings.
- Writes use explicit generated Ent setters and mutations. Do not reflect request values into Ent writes.
- SoftDeleteMixin is storage capability only. Get/List tombstone visibility and Undelete remain explicit application behavior.
- Never persist `temporary_password`; convert it to private `password_hash`.
- Keep CI database-free. Live SQLite/PostgreSQL checks must be explicitly invoked.
- Run the full reference contract explicitly with `SERVORA_EXAMPLE_SQLITE_DSN='<sqlite DSN>' make test.integration`; default CI tests must not compile or execute it.
