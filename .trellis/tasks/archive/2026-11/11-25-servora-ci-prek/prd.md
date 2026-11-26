# 阶段一：Servora 工具链与工程化门禁统一

## 目标

在不破坏现有对外契约（零破坏性变更、无需下游 `plateau` 同步改码）的前提下，将 `../servora` 的开发工具链、代码生成器版本固定、本地 Git 钩子、GitHub Actions 门禁及依赖自动更新机制全面对齐至 `plateau` 标准，并彻底清理遗留双轨构建入口与失效文件，为阶段二 `servora` 模块瘦身提供可靠的自动化校验保障。

## 背景与现状事实

来源为父任务 [11-25-servora-kit-engineering/prd.md](../11-25-servora-kit-engineering/prd.md) §阶段一（R1.1–R1.3）与 [design.md](../11-25-servora-kit-engineering/design.md) §4.3、§5。经源码核查确认以下现状：

1. **遗留文件与冗余 CLI 依赖**：
   - `../servora/Makefile:1-217` 与 `../servora/justfile:1-192` 双轨并存，且均通过 `@latest` 执行 `go install`（`Makefile:24-35,61-86`、`justfile:15-27,43-69`），本地与 CI 生成结果无法严格复现。
   - `../servora/cmd/svr/`（含 `cmd/svr/internal/cmd/{new,gen}`）面向旧单体目录生成骨架，且在 `../servora/go.mod:12-14,36` 中单独引入 `charmbracelet/huh`、`charmbracelet/lipgloss`、`glebarez/sqlite` 与 `spf13/cobra`（全仓其他包零引用）。
   - 已跟踪的 `../servora/docker-compose.yaml:1-280` 引用不存在的 `./manifests/{otel,loki,prometheus,grafana}`、已迁出的 `openfga` / `clickhouse` / `kafka` 及多处 `:latest` 镜像，仓内零脚本或测试消费；`../servora/CLAUDE.md` 为 0 字节空跟踪文件。
2. **工具链与前端包管理器版本分散**：
   - `../servora` 缺少 `mise.toml`；`../servora/go.mod:5` 仅将 `ent` 列入 `tool`，`buf.go.gen.yaml:9-53` 与 `buf.typescript.gen.yaml:11-25` 仍依赖宿主全局 PATH 中的 `protoc-gen-*` 二进制。
   - `../servora/web/package.json:3` 与 `../servora/.github/workflows/publish-proto-utils.yml:22` 硬编码 `pnpm@10.33.0`，与 `plateau/mise.toml:8-9`（`node = "24.22.3"`、`pnpm = "12.21.0"`）及 `pnpm-workspace.yaml` 1 天发布冷却策略不一致。
3. **本地钩子、CI 与安全/依赖治理缺失**：
   - `../servora` 无 `prek.toml`、无 `renovate.json5`、无 `.github/workflows/security.yml`（`govulncheck`）。
   - `../servora/.github/workflows/ci.yml:20-57` 仅运行 `just --fmt --check`、`golangci-lint` 与 `go test -short`，缺少路径分流、Proto lint/format/breaking 校验、`gen-check` 生成漂移校验、`web` 构建单测门禁以及唯一分支保护聚合检查 `ci-ok`。
   - 既有 `../servora/.github/workflows/release.yml:36-48` 依赖查询 `ci.yml` 在 `main` 分支 `push` 事件上的成功运行记录，重构 `ci.yml` 时须保持该文件名与触发条件兼容。

## 需求清单

- **R1（对应父 R1.1）清理遗留入口与失效跟踪文件**：删除 `../servora/Makefile`、`../servora/cmd/svr/`、`../servora/docker-compose.yaml` 与空文件 `../servora/CLAUDE.md`；清理 `go.mod` 中仅由 `cmd/svr` 引入的依赖，并同步更新 `README.md`、`AGENTS.md`、`cmd/AGENTS.md` 中的命令说明。
- **R2（对应父 R1.2）单一来源工具链与生成器固定**：新增 `../servora/mise.toml`（对齐 `plateau` 固定 `buf`、`golangci-lint`、`just`、`node`、`pnpm`、`prek` 版本）；在 `../servora/go.mod` 通过 `tool` 块固定当前全部 `protoc` 插件（含外部插件与本仓插件）并按需配置 `toolchain` 补丁版本；将 `buf.go.gen.yaml`、`buf.typescript.gen.yaml` 切换为 `local: ["go", "tool", ...]`；重构 `justfile`（移除 `@latest`、`plugin`、`cli`，补齐 `init`、`gen`、`gen-fresh`、`gen-check`、`fmt-proto-check`、`breaking` 及跨平台支持）；对齐 `web/` 的 pnpm 版本与 `minimumReleaseAge` 冷却配置。
- **R3（对应父 R1.3）落地三阶段 `prek.toml` 本地钩子**：新增 `../servora/prek.toml`，覆盖 `pre-commit`（内置文件卫生、`gofmt`、`buf format`、`just --fmt`、`biome check`）、`pre-push`（`GOWORK=off` 下的 `lint-go`、`lint-proto`、`breaking`、`gen-check`）与 `commit-msg`（Conventional Commits 校验），全局排除生成目录与锁文件，并对存量文件完成一次性格式对齐。
- **R4（对应父 R1.3）重构 GitHub Actions 门禁与兼容衔接**：升级 `../servora/.github/workflows/ci.yml` 为 `changes` 路径分流 + `prek` / `go` / `proto` / `gen` / `web` + `ci-ok` 唯一必过聚合门禁；新增 `security.yml`（`govulncheck ./...`）；更新 `publish-proto-utils.yml` 复用 `mise.toml` 工具版本，保持与 `buf-ci.yml`、`release.yml` 完全兼容。
- **R5（对应父 R1.3）新增 Renovate 配置与规范文档同步**：新增 `../servora/renovate.json5`（对齐 `plateau` 分组、时间表与 `minimumReleaseAgePackageManagerSync`）；同步更新本仓 `.trellis/spec/servora/` 对应工程与生成规范。

## 验收标准

- [x] **AC-R1（R1）**：`../servora` 中 `Makefile`、`cmd/svr/`、`docker-compose.yaml`、`CLAUDE.md` 已移除；`GOWORK=off go mod tidy` 后 `go.mod` 不再包含 `charmbracelet/huh`、`charmbracelet/lipgloss`、`glebarez/sqlite`、`spf13/cobra`，且仓内文档无残留的 `make <target>` 或 `cmd/svr` 用法指引。
- [x] **AC-R2（R2）**：`mise install` 后工具版本与 `../servora/mise.toml` 一致；`buf.go.gen.yaml` 与 `buf.typescript.gen.yaml` 全部经 `go tool` 调用插件；在干净工作区且 `GOWORK=off` 下连续两次执行 `just gen-check` 均零差异通过（同时覆盖 `api/gen/go` 与 `web/packages/proto-utils/src/gen`）。
- [x] **AC-R3（R3）**：`prek run --all-files --stage pre-commit` 与 `prek run --all-files --stage pre-push` 全部通过；`commit-msg` 钩子准确放行合规 Conventional Commits（含中文描述、`Merge`、`Revert`、`fixup!`、`squash!`）并拦截非法提交说明。
- [x] **AC-R4（R4）**：`actionlint` 校验 `../servora/.github/workflows/*.yml` 零告警；`GOWORK=off` 下本地等价执行 `go`（build + lint + race test）、`proto`（lint + format check + breaking）、`gen`（`gen-check`）、`web`（`pnpm install --frozen-lockfile` + `check` + `typecheck` + `test`）及 `govulncheck ./...` 全部通过；推送远端后 `ci.yml`（含 `ci-ok`）与 `security.yml` 实跑全绿。
- [x] **AC-R5（R5）**：`renovate.json5` 通过 Renovate 配置严格校验（无废弃字段或语法警告）；`.trellis/spec/servora/` 已移除失效 `svr` CLI 规范并更新工具链、`prek` 与 `gen-check` 约定。

## 范围外事项

- **剥离 CRUD 与数据库/缓存 `contrib`**：保留在阶段二子任务实施，本子任务不删除任何对外 Go 运行时包、Proto 定义或 `@servora/proto-utils` 导出项，也不发布破坏性新版本。
- **改动 `plateau` 业务与构建代码**：本子任务仅更新本仓 `.trellis/spec/servora/` 规范文档，不修改 `plateau` 的 Go/TS 代码或依赖版本。

## 跨阶段顺序与协作约定

- 本子任务是父任务 [11-25-servora-kit-engineering](../11-25-servora-kit-engineering/prd.md) 的首个独立交付单元，无前置阻塞任务。
- 本子任务合入 `servora/main` 并开启 `ci-ok` 分支保护后，阶段二（剥离 CRUD 与非核心 `contrib`）将直接在新建成的门禁保护下执行。
