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

## 本地端口

- 从 `10000` 起，每个应用固定分配 10 个端口：`+0` HTTP、`+1` gRPC、`+2` Web，`+3～+9` 预留；新增应用登记下一个空闲段，不重排已有编号。
- 此约定用于本地监听和 Docker 宿主机映射；容器内部端口、共享中间件端口和生产公开端口不受影响。同一应用的原生运行与容器映射不可同时占用相同端口。
- Web 的 dev 与 preview/start 默认共用 Web 端口，不同时启动。改动 IAM 公开入口时，同步 `IAM_PUBLIC_ORIGIN`、Web 端口和后端代理地址。

| 应用 | 端口段 | HTTP | gRPC | Web |
|---|---|---|---|---|
| IAM | 10000–10009 | 10000 | 10001 | 10002 |
| Audit | 10010–10019 | 10010 | 10011 | 10012（预留） |
| CMS（占位） | 10020–10029 | 10020（预留） | 10021（预留） | 10022（预留） |
| Example | 10030–10039 | 10030 | 10031 | 10032 |
| Test | 10040–10049 | 10040（预留） | 10041（预留） | 10042 |
| Admin（占位） | 10050–10059 | 10050（预留） | 10051（预留） | 10052（预留） |

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
- `infra/` 共享基础设施：`openfga/`、`clickhouse/`；Ent 适配归 Servora 的 `contrib/db/entgo/`
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
- `go.mod`、`go.sum` 统一管理平台根代码、`api/gen/go` 与三个 Go 后端的依赖
- 本机父级 `/servora-kit/go.work` 仅用于可选的跨仓源码联调；仓库自身不跟踪 `go.work`
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
