## 编码规范

- 永远不能因为单元测试需要而定义没必要的私有方法和变量

## 本地端口

- 从 `10000` 起，每个应用固定分配 10 个端口：`+0` HTTP、`+1` gRPC、`+2` Web，`+3～+9` 预留；IAM、Admin、Audit 依次使用前三个段。新增服务从 `10030` 起按 10 端口段顺序登记，不重排已有编号。
- 此约定用于本地监听和 Docker 宿主机映射；容器内部端口、共享中间件端口和生产公开端口不受影响。同一应用的原生运行与容器映射不可同时占用相同端口。
- 本地全栈开发默认经 Docker Traefik（`:8080`）使用 RFC 6761 回环域名 `http://iam.plateau.localhost:8080` 与 `http://admin.plateau.localhost:8080`（免改 `/etc/hosts`），各应用自身监听端口仍保持 `10000–10029` 不变。Web 的 dev 与 preview/start 默认共用 Web 端口，不同时启动。改动 IAM 或 Admin 公开入口时，同步 `IAM_PUBLIC_ORIGIN` / `ADMIN_PUBLIC_ORIGIN`、Web 端口和网关/后端代理地址。

| 应用 | 端口段 | HTTP | gRPC | Web |
| --- | --- | --- | --- | --- |
| IAM | 10000–10009 | 10000 | 10001 | 10002 |
| Admin | 10010–10019 | 10010 | 10011（预留） | 10012 |
| Audit | 10020–10029 | 10020（内部运维） | 10021 | 10022（预留） |

## 目录结构

- `api/` 平台领域 Proto 与生成产物（详见 [api/AGENTS.md](api/AGENTS.md)）
  - `protos/` 全平台集中式 Proto 定义（单一 BSR 模块 `buf.build/servora/plateau`，全部 29 个公开 `.proto` 统一位于 `api/protos/plateau/{security,crud,infra,iam,admin,audit}/**/v1`，包名 `plateau.*.*.v1`）
  - `gen/go/` 所有微服务的 Go Proto 生成输出目录
  - `gen/ts/` 所有微服务的 TypeScript Proto 生成输出目录
  - `gen/package.json` 管理共享 TS 包依赖与 exports
- `app/` 平台微服务，均在 `app/{ServiceName}/` 下（服务结构见 [app/AGENTS.md](app/AGENTS.md)）
- `cmd/` 平台级命令工具
  - `protoc-gen-plateau-authz/`、`protoc-gen-plateau-authn/`、`protoc-gen-plateau-audit/`、`protoc-gen-plateau-crud/` 安全与 CRUD 代码生成插件；与其余 protoc 插件统一在根 `go.mod` `tool` 声明，由 buf 模板经 `go tool` 调用
- `crud/` 后端中立的 AIP CRUD 生命周期、FieldMask、List 预处理、分页令牌、`crud/mapper/` 读投影及 `crud/testdata/` 跨语言测试向量（详见 [`docs/crud.md`](docs/crud.md)）
- `internal/codegen/` 共享代码生成实现（`optionmerge`、`ruleplan`、`plugintest`、`protoreach`）
- `security/` 共享安全生态：`actor.go`、`identity.go`、`loopback.go`、`rules.go`、`authn/<implementation>`、`authz/<engine>`、`audit/`（含 `audit/kafka/`）、`oidc/`、`cap/`、`password/`、`jwt/`、`session/`；共享错误源在 `api/protos/plateau/security/errors/v1/`
- `infra/` 共享基础设施：`openfga/`、`clickhouse/`、`kafka/`、`redis/`（客户端与分布式锁）、`cache/`（内存与 `cache/redis/`）、`entgo/`（通用 SQL driver、tracing、`crud/` adapter 与 `mixin/` 软删除便利层）、`gorm/`（slog 适配与 `crud/` 扩展位）
- `transport/` 平台传输扩展：`server/kafka/` 非泛型 Kafka 入站批消费 `transport.Server`
- `web/packages/plateau-utils/` 平台统一前端工具包 `@servora/plateau-utils`（导出 `.` / `./client`、`./crud`、`./error-reasons/*`，见 [plateau-utils 规范](.trellis/spec/web/plateau-utils/index.md)）
- `just/` 平台共享 Just settings、registry 与 service 实现
- `manifests/` 部署资源文件（`scripts/`、`openfga/`、`grafana/`、`prometheus/`、`otel/`、`traefik/`、`loki/`）
- `docs/adr/` 架构决策记录
- `justfile` 项目级 Just 命令入口
- `mise.toml` 固定 CLI 工具版本（buf、golangci-lint、just、fga、process-compose、prek、node、pnpm、air）；Go 版本以 `go.mod` 为准
- `prek.toml` 根 Git 钩子配置（`pre-commit` / `pre-push` / `commit-msg`）；`app/admin/web/prek.toml` 为 Vben 子项目钩子配置
- `pnpm-workspace.yaml` 统一纳管 `api/gen`、平台原生 Web 与 `web/packages/*`；`app/admin/web` 保留 Vben 独立 workspace 和 lockfile，并通过 `link:` 引用 `@plateau/api`、`@servora/plateau-utils`
- `pnpm-lock.yaml` Platform workspace 共享依赖锁文件
- `buf.yaml` 单一模块（`buf.build/servora/plateau`）、依赖以及 lint / breaking 规则（`0.x` 阶段关闭 `prek.toml`、`ci.yml`、`buf-ci.yml` 三处自动 breaking 拦截，保留 `FILE` 规则供手动执行 `just breaking`，待 `v1.0.0` 发布前恢复）
- `buf.go.gen.yaml` 项目级统一 Go 生成配置
- `buf.typescript.gen.yaml` 项目级统一 TypeScript 生成配置（输出 `@plateau/api` 的 `api/gen/ts` 及 `@servora/plateau-utils` 的 `web/packages/plateau-utils/src/gen`）
- `buf.openapi.gen.yaml` 仓库根级唯一 OpenAPI 生成模板（输出至各服务 `internal/assets/openapi.yaml`，由 `just/service.just` 按 `api/protos/plateau/{ServiceName}` 过滤调用）
- `buf.es.gen.yaml` 已停用并全部注释，仅保留作 Protobuf-ES 配置参考
- `go.mod`、`go.sum` 统一管理平台根代码、`api/gen/go` 与各服务 Go 后端的依赖
- 生产构建统一执行 `CGO_ENABLED=0` 门禁（三处一致：`just/service.just` `_build` 默认值、`prek.toml` `pre-push` `go-build-nocgo`、`.github/workflows/ci.yml` `go` 任务）；`just test` 保持默认 CGO 环境以支持 `-race` 与测试专用 `mattn/go-sqlite3`
- 跨仓源码联调（如与 Servora）可在本地共同父目录自建 `go.work` / `pnpm-workspace.yaml`，不提交到仓库；父级 pnpm workspace 需排除 `plateau/app/admin/web`。仓库门禁以 `GOWORK=off` 的独立检查为准，联调通过不代表门禁通过
- `docker-compose.yaml` 本地基础设施编排
- `process-compose.yaml` 本地全栈进程编排（REST `:18090`，内置 SSE MCP `:18091`）
- `.mcp.json` 项目级 MCP 服务声明（含 `process-compose` SSE 接入）

## 命令

```bash
just init              # mise install + prek install + 根 workspace 与 Admin Web 依赖安装
just dev               # process-compose TUI 全栈启动（含 compose-up、keygen、openfga-setup 与 5 个应用）
just dev-up            # 后台守护模式启动全栈开发环境
just dev-down          # 停止本地应用进程（保留 Docker 中间件容器）
just gen
just gen-check         # GOWORK=off 下校验生成产物无漂移
just wire
just test              # go test -race -count=1 ./...
just lint
just fmt-proto-check
just breaking          # 手动对比 BUF_BREAKING_AGAINST（默认 .git#ref=origin/main；0.x 关闭自动拦截，v1.0.0 恢复）
just api-ts-check
just service::iam::keygen
just openfga-model-validate
just openfga-model-test
just openfga-model-apply
just openfga-tuple-sync
just openfga-setup     # openfga-init + openfga-tuple-sync
just web::iam::dev
just web::admin::dev
just web::build
just web::lint
just web::test
```

`api/gen`、平台原生 Web 与 `web/packages/*` 共用根 pnpm workspace 和 lockfile；Vben Admin 在 `app/admin/web` 中独立安装依赖。新增平台服务参考 `app/iam`、`app/admin` 与 [app/AGENTS.md](app/AGENTS.md)。

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
