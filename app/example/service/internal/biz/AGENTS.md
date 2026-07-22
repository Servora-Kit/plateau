# AGENTS.md - internal/biz

## Scope

Business rules and repository ports for the User CRUD reference application.

## Rules

- Define persistence ports here as `XxxRepo`; data implements them and exposes `NewXxxRepo`.
- Let generated `XxxName` values cross service/biz/data for simple CRUD resources; add a domain identity only when it enforces real domain invariants.
- Own tenant scope, allow-missing branches, etag comparison, AIP-164 visibility, and business errors.
- Hash plaintext secrets here and clear input-only plaintext before calling a repository; repository inputs may contain only password hashes.
- Do not import generated Ent packages or SQL concerns.
