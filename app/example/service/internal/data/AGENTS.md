# AGENTS.md - internal/data

## Scope

Ent-backed persistence for the public User reference flow.

## Rules

- Repository methods accept generated resource names, biz-owned scope, and normalized resource/WriteMask values; they do not infer authorization or tenancy.
- Do not add resource-specific Params/Command wrappers unless they enforce a real persistence invariant.
- Map filters and ordering only through the immutable User ListFields contract.
- Keep Ent and driver errors wrapped with `%w`; business error translation belongs to biz.
- Use explicit generated setters for create/update/undelete.
- Tombstone bypass must be explicit through `mixin.SkipSoftDelete`.
