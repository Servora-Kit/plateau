## 本地端口

- 从 `10000` 起，每个应用固定分配 10 个端口：`+0` HTTP、`+1` gRPC、`+2` Web，`+3～+9` 预留；IAM、Admin 使用前两个段，Example/Test 固定使用 `10080`/`10090` 段。新增基础服务从 `10020–10079` 中尚未占用的段登记，不因 Example/Test 编号较大而从 `10100` 继续分配，也不重排已有编号。
- 此约定用于本地监听和 Docker 宿主机映射；容器内部端口、共享中间件端口和生产公开端口不受影响。同一应用的原生运行与容器映射不可同时占用相同端口。
- Web 的 dev 与 preview/start 默认共用 Web 端口，不同时启动。改动 IAM 公开入口时，同步 `IAM_PUBLIC_ORIGIN`、Web 端口和后端代理地址。

| 应用 | 端口段 | HTTP | gRPC | Web |
|---|---|---|---|---|
| IAM | 10000–10009 | 10000 | 10001 | 10002 |
| Admin | 10010–10019 | 10010（预留） | 10011（预留） | 10012 |
| Example | 10080–10089 | 10080 | 10081 | 10082 |
| Test | 10090–10099 | 10090（预留） | 10091 | 10092 |

Audit/CMS 暂不列入上表；现有 Audit 的本地监听和宿主映射占用 `10020/10021`，因此 `10020–10029` 段不可重复分配。Admin 后端尚未建立，表中的 HTTP/gRPC 仅预留，不表示已有服务监听。

## 目录结构

- `api/` 平台领域 Proto 与生成产物（详见 [api/AGENTS.md](api/AGENTS.md)）
  - `protos/` 领域 Proto 定义（`api/protos/plateau/**`）
  - `gen/go/` 所有微服务的 Go Proto 生成输出目录
  - `gen/ts/` 所有微服务的 TypeScript Proto 生成输出目录
  - `gen/package.json` 管理共享 TS 包依赖与 exports
- `app/` 平台微服务，均在 `app/{ServiceName}/` 下（服务结构见 [app/AGENTS.md](app/AGENTS.md)）
- `cmd/` 平台级命令工具
  - `protoc-gen-plateau-authz/`、`protoc-gen-plateau-authn/` AuthN/AuthZ 代码生成插件；插件从当前 checkout 的 `cmd/` 本地安装
- `internal/codegen/` 共享代码生成实现
- `security/` 共享安全生态：`actor.go`、`authn/<implementation>`、`authz/<engine>`、`cap/`、`password/`、`jwt/`、`session/`；共享错误源在 `api/protos/plateau/security/errors/v1/`
- `infra/` 共享基础设施：`openfga/`、`clickhouse/`、`entgo/mixin/` 软删除便利层；通用 Ent driver 与 CRUD adapter 归 Servora 的 `contrib/db/entgo/`
- `web/packages/client/` 平台共享前端通信能力（见 [web/client 规范](.trellis/spec/web/client/index.md)）
- `just/` 平台共享 Just settings、registry 与 service 实现
- `manifests/` 部署资源文件（`scripts/`、`openfga/`、`grafana/`、`prometheus/`、`otel/`、`traefik/`、`loki/`）
- `docs/adr/` 架构决策记录
- `justfile` 项目级 Just 命令入口
- `pnpm-workspace.yaml` 统一纳管 `api/gen`、平台原生 Web 与 `web/packages/*`；`app/admin/web` 保留 Vben 自己的 workspace 和 lockfile
- `pnpm-lock.yaml` Platform workspace 共享依赖锁文件
- `buf.yaml` buf 总配置，依赖以及 lint 规则
- `buf.go.gen.yaml` 项目级统一 Go 生成配置
- `buf.typescript.gen.yaml` 项目级统一 TypeScript HTTP、error reason 与 CRUD helper 生成配置
- `buf.es.gen.yaml` 已停用并全部注释，仅保留作 Protobuf-ES 配置参考
- `go.mod`、`go.sum` 统一管理平台根代码、`api/gen/go` 与四个 Go 后端的依赖
- 本机共同父目录的 `../go.work` 纳入 `./plateau`、`./servora`，用于跨仓源码联调与 LSP；仓库自身不跟踪 `go.work`，独立构建门禁使用 `GOWORK=off`
- `docker-compose.yaml` 本地基础设施编排；`docker-compose.apps.yaml` 应用容器编排

## 命令

```bash
just init
just gen
just wire
just lint
just api-ts-check
just openfga-model-validate
just openfga-model-test
just openfga-model-apply
just web::example::dev
just web::iam::dev
just web::test::dev
just web::admin::dev
just web::build
just web::lint
```

`api/gen`、平台原生 Web 与 `web/packages/*` 共用根 pnpm workspace 和 lockfile；Vben Admin 在 `app/admin/web` 中独立安装依赖。新增平台服务参考 `app/example`。

<!-- TRELLIS:START -->
# Trellis Instructions

These instructions are for AI assistants working in this project.

This project is managed by Trellis. The working knowledge you need lives under `.trellis/`:

- `.trellis/workflow.md` — development phases, when to create tasks, skill routing
- `.trellis/spec/` — package- and layer-scoped coding guidelines (read before writing code in a given layer)
- `.trellis/workspace/` — per-developer journals and session traces
- `.trellis/tasks/` — active and archived tasks (PRDs, research, jsonl context)

If a Trellis command is available on your platform (e.g. `/trellis:finish-work`, `/trellis:continue`), prefer it over manual steps. Not every platform exposes every command.

If you're using Codex or another agent-capable tool, additional project-scoped helpers may live in:
- `.agents/skills/` — reusable Trellis skills
- `.codex/agents/` — optional custom subagents

Managed by Trellis. Edits outside this block are preserved; edits inside may be overwritten by a future `trellis update`.

<!-- TRELLIS:END -->

<!-- gitnexus:start -->
# GitNexus — Code Intelligence

This project is indexed by GitNexus as **plateau** (27625 symbols, 63558 relationships, 332 execution flows).

> Index stale? Run `node .gitnexus/run.cjs analyze --index-only` from the project root — it auto-selects an available runner. No `.gitnexus/run.cjs` yet? Bootstrap with `npx`, `bunx`, or `pnpm dlx` — e.g. `bunx gitnexus@latest analyze` (npm 11 npx crash; #1939).

## Always Do

- **MUST run impact before editing.** Use `impact({target: "symbolName", direction: "upstream"})` or `node .gitnexus/run.cjs impact "symbolName" --direction upstream --repo .`; report callers, processes, and risk. Never substitute grep for graph analysis.
- **MUST analyze graph changes before committing.** Use `detect_changes({scope: "all"})` (MCP) or `node .gitnexus/run.cjs detect-changes --scope all --repo .` (CLI fallback). `partial: true` or `truncated: true` is not a clean check — a zero means unseen, not unaffected; re-run it. For regression review: `detect_changes({scope: "compare", base_ref: "main"})` or `node .gitnexus/run.cjs detect-changes --scope compare --base-ref "main" --repo .`.
- MUST warn on HIGH/CRITICAL `risk` pre-edit; never use `riskSharedAxes` to waive a HIGH/CRITICAL `risk` warning. Compare File/symbol: MCP File omits axes; Graph-RAG expands File.
- **MUST treat `risk: UNKNOWN` as unresolved, not as low.** An empty caller set is not evidence the symbol is unused — it can also mean the callers are not resolvable by the index (plain-object property access, dynamic dispatch, cross-language calls). `impact` pairs `UNKNOWN` with a `riskNote` saying so. Confirm with a text search before treating the symbol as safe to change or delete; do not proceed on the strength of a zero.
- **MUST use `query({search_query: "concept"})` for concepts/flows, `context({name: "symbolName"})` for a named symbol, or `impact` for blast radius, on read-only callers, dependencies, imports, or execution flow.** Graph first; text search only for empty/`UNKNOWN`/literals.
- For security review, `explain({target: "fileOrSymbol"})` lists taint findings (source→sink flows; needs `analyze --pdg`).

## Never Do

- NEVER edit a function, class, or method before MCP/CLI impact analysis.
- NEVER ignore HIGH or CRITICAL risk warnings from impact analysis, and never read `UNKNOWN` as an all-clear — it means the walk could not answer, which is the one verdict that requires confirming by other means.
- NEVER rename symbols with find-and-replace — use `rename` which understands the call graph.
- NEVER commit before MCP/CLI graph change analysis.

## Resources

| Resource | Use for |
| --- | --- |
| `gitnexus://repo/plateau/context` | Codebase overview, check index freshness |
| `gitnexus://repo/plateau/clusters` | All functional areas |
| `gitnexus://repo/plateau/processes` | All execution flows |
| `gitnexus://repo/plateau/process/{name}` | Step-by-step execution trace |

## CLI

| Task | Read this skill file |
| --- | --- |
| Understand architecture / "How does X work?" | `.claude/skills/gitnexus-exploring/SKILL.md` |
| Blast radius / "What breaks if I change X?" | `.claude/skills/gitnexus-impact-analysis/SKILL.md` |
| Trace bugs / "Why is X failing?" | `.claude/skills/gitnexus-debugging/SKILL.md` |
| Rename / extract / split / refactor | `.claude/skills/gitnexus-refactoring/SKILL.md` |
| Tools, resources, schema reference | `.claude/skills/gitnexus-guide/SKILL.md` |
| Index, status, clean, wiki CLI commands | `.claude/skills/gitnexus-cli/SKILL.md` |

<!-- gitnexus:end -->
