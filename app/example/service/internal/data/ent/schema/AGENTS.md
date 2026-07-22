# AGENTS.md - internal/data/ent/schema

## Scope

Hand-written Ent schema for the User CRUD reference application.

## Rules

- `SoftDeleteMixin` exclusively owns `delete_time`, `purge_time`, and `deleted_by`.
- `tenant_id` and `resource_id` form the canonical resource identity; generated numeric `id` is the hidden stable cursor key.
- `password_hash` is private and sensitive. No plaintext password field is allowed.
- Generated Ent output under the parent `ent` directory must not receive hand-written AGENTS files.
