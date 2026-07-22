# AGENTS.md - internal/data

## Scope

Ent-backed persistence for the public User reference flow.

## Rules

- Repository methods accept generated resource names, biz-owned scope, and normalized resource/WriteMask values; they do not infer authorization or tenancy.
- Do not add resource-specific Params/Command wrappers unless they enforce a real persistence invariant.
- Map filters and ordering only through the immutable User ListFields contract.
- Log ORM/driver operation failures at the data source with operation and resource identity. Return unknown backend errors unchanged; map known outcomes with `errors.Join(biz.ErrXxx, err)` so both the generic persistence fact and original ORM cause remain inspectable.
- A zero-row conditional mutation returns `biz.ErrMutationMiss`; ETag, allow-missing, tombstone, and idempotency semantics are resolved by biz.
- Data never constructs public Proto/Kratos errors. Biz translates generic persistence facts to generated application errors and does not duplicate the data-layer log for propagated repository failures.
- Standard read projection calls `ResourceMapper.ToDTO`/`ToDTOs` directly. Use Try variants only with an explicit recovery branch; do not add pass-through `mapXxx` helpers.
- Use explicit generated setters for create/update/undelete.
- Tombstone bypass must be explicit through `mixin.SkipSoftDelete`.
