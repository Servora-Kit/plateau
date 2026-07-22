# AGENTS.md - internal/service

## Scope

Kratos application/service layer for the public User RPC contract.

## Rules

- Parse canonical resource names and parents here using generated helpers or ResourceNameMatcher.
- Convert RPC wrappers into ResourcePlan/ListPreparer values before calling biz.
- RPC request messages must not cross into biz or data.
- Apply `ToResponse`/`ToResponses` to every returned resource.
- Keep allow-missing and tombstone business branches in biz.
