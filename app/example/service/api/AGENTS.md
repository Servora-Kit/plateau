# AGENTS.md - app/example/service/api

## Scope

Source API and per-service generation templates for the runnable User CRUD reference service.

## Rules

- Keep this API behaviorally aligned with the public framework example while using the platform `api/gen` go_package.
- Preserve `example.servora.dev/User` and `tenants/{tenant}/users/{user}` across all references.
- Generate Go, HTTP, CRUD helpers, TypeScript, and OpenAPI through the repository templates; never edit generated output.
