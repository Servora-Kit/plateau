# Implement：阶段一 Servora 工具链与工程化门禁统一

关联文档：[prd.md](prd.md) · [design.md](design.md) · 父任务 [11-25-servora-kit-engineering](../11-25-servora-kit-engineering/prd.md)

## 前置条件

- [x] `../servora` 当前位于 `main` 分支且工作区干净（`git -C ../servora status --short` 为空）
- [x] 本子任务规划产物已通过用户审阅并执行 `python3 ./.trellis/scripts/task.py start 11-25-servora-ci-prek`

## 实施步骤

1. **清理遗留构建入口、废弃 CLI 与死文件（R1）**
   - [x] 在 `../servora` 删除 `Makefile`、`cmd/svr/`、`docker-compose.yaml`、`CLAUDE.md`
   - [x] 执行 `GOWORK=off go -C ../servora mod tidy`，确认 `charmbracelet/huh`、`charmbracelet/lipgloss`、`glebarez/sqlite`、`spf13/cobra` 已从 `go.mod` 移除
   - [x] 更新 `../servora/cmd/AGENTS.md`（移除 `svr/` 说明）与 `../servora/README.md`（将残留 `make` 示例替换为 `just` 或对应说明）

2. **固定工具版本与 `go.mod tool` 生成器（R2）**
   - [x] 新增 `../servora/mise.toml`（固定 `buf`、`golangci-lint`、`just`、`node`、`pnpm`、`prek`，对齐 `plateau/mise.toml` 版本）
   - [x] 在 `../servora` 执行 `mise install`，确认 `mise ls --current` 生效
   - [x] 更新 `../servora/go.mod`：补充 `toolchain go1.27.4`，通过 `GOWORK=off go get -tool` 注册当前所需的全部外部与本仓 `protoc` 插件（`protoc-gen-go`、`protoc-gen-go-grpc`、`protoc-gen-go-http/v3`、`protoc-gen-validate`、`protoc-gen-go-errors`、`protoc-gen-redact`、`protoc-gen-servora-conf`、`protoc-gen-servora-crud`、`protoc-gen-typescript-http` 及既有 `ent`），执行 `GOWORK=off go mod tidy`
   - [x] 将 `../servora/buf.go.gen.yaml` 与 `../servora/buf.typescript.gen.yaml` 中所有插件调用改为 `local: ["go", "tool", "protoc-gen-..."]`

3. **对齐前端 pnpm 工作区与重构 `justfile`（R2）**
   - [x] 移除 `../servora/web/package.json` 中的 `"packageManager": "pnpm@10.33.0"`；在 `../servora/web/pnpm-workspace.yaml` 增加 `minimumReleaseAge: 1440`，在 `../servora/web` 使用 `pnpm 12.21.0` 更新 `pnpm-lock.yaml`
   - [x] 重构 `../servora/justfile`：移除 `@latest` 变量、`plugin`、`cli`；按 `design.md` 实现 `init`、`gen-go`、`gen-ts`、`gen`、`gen-fresh`、`gen-check`（`[unix]`/`[windows]`）、`test`、`test-all`、`lint-go`、`lint-proto`、`fmt-proto`、`fmt-proto-check`、`breaking`、`lint`、`web-install`、`web-check`、`web-typecheck`、`web-build`、`web-test`
   - [x] 运行 `just --fmt --unstable --justfile ../servora/justfile` 规范格式，并在 `../servora` 执行 `GOWORK=off just gen-fresh` 同步插件版本固定后的生成物

4. **落地 `prek.toml` 与全仓存量卫生对齐（R3）**
   - [x] 新增 `../servora/prek.toml`（含 `pre-commit`、`pre-push`、`commit-msg` 三阶段与生成目录排除项）；确认 `.gitignore` 忽略 `coverage.out`
   - [x] 在 `../servora` 执行 `prek install` 与 `prek run --all-files --stage pre-commit`，一次性修复存量文件格式或尾随空白差异
   - [x] 用合法与非法样例测试 `prek run --hook-stage commit-msg` 的拦截与放行行为

5. **升级 GitHub Actions 工作流与新增 Renovate 配置（R4、R5）**
   - [x] 重构 `../servora/.github/workflows/ci.yml`（`changes` → `prek` / `go` / `proto` / `gen` / `web` → `ci-ok`）
   - [x] 新增 `../servora/.github/workflows/security.yml`（`govulncheck ./...`）
   - [x] 更新 `../servora/.github/workflows/publish-proto-utils.yml`（接入 `jdx/mise-action@v5` 复用 `mise.toml` 版本，发包前执行 `pnpm run test`），检查 `release.yml` 与 `buf-ci.yml` 兼容性
   - [x] 新增 `../servora/renovate.json5`

6. **更新 `AGENTS.md` 与 `.trellis/spec/servora/` 规范（R1、R5）**
   - [x] 更新 `../servora/AGENTS.md` 常用命令段（补充 `just init`、`just gen`、`just gen-check`、`just fmt-proto-check`、`just breaking`，移除 `just plugin`）
   - [x] 更新本仓 `.trellis/spec/servora/` 规范（`index.md`、`cmd/{index.md,plugins.md}` 并移除失效 `cmd/cli.md`、`proto/{index.md,generation.md}`、`web/{index.md,proto-utils.md}`），记录 `mise.toml`、`go.mod tool`、`prek.toml`、`gen-check` 与 `ci-ok` 门禁约定

7. **全量本地验证与远端 CI 实跑验收（AC-R1～AC-R5）**
   - [x] 按下方验证矩阵在 `../servora` 逐项跑通本地检查
   - [x] 按逻辑提交 `../servora` 变更并推送远端，验证 GitHub Actions `CI`（含 `ci-ok`）与 `Security`（`govulncheck`）运行全绿；在本仓提交 `.trellis/spec/servora/` 与任务记录

## 验证命令矩阵

| 检查项 | 执行命令（在 `../servora` 下，`GOWORK=off`） | 预期结果 |
| --- | --- | --- |
| 工具版本对齐 | `mise install && mise ls --current` | 输出版本与 `mise.toml` 完全一致 |
| Go 模块与依赖净身 | `GOWORK=off go mod tidy && git diff --exit-code go.mod go.sum` | 无漂移，且 `huh`/`lipgloss`/`glebarez`/`cobra` 已清退 |
| 生成幂等校验 | 连续执行两次 `GOWORK=off just gen-check` | 两次均 0 退出码，无修改或未跟踪文件 |
| Go 构建、Lint 与竞态测试 | `GOWORK=off go build ./... && GOWORK=off just lint-go && GOWORK=off just test` | 全部通过，0 lint issue |
| Proto 检查 | `just lint-proto && just fmt-proto-check && BUF_BREAKING_AGAINST=.git#ref=HEAD just breaking` | 全部通过 |
| 前端包检查 | `just web-install && just web-check && just web-typecheck && just web-test` | Biome 检查、类型检查、构建与单测全部通过 |
| 本地钩子全阶段 | `prek run --all-files --stage pre-commit && BUF_BREAKING_AGAINST=.git#ref=HEAD prek run --all-files --stage pre-push` | 全部通过 |
| 漏洞扫描 | `GOWORK=off go run golang.org/x/vuln/cmd/govulncheck@latest ./...` | 0 已知可触发漏洞 |
| Workflow 与 Renovate 语法 | `mise x actionlint@latest -- actionlint`、`npx --yes -p renovate renovate-config-validator --strict renovate.json5` | 语法与严格模式校验均 0 错误 |

## 风险文件与回滚切分点

- **提交 1（遗留清理）**：删除 `Makefile`、`cmd/svr/`、`docker-compose.yaml`、`CLAUDE.md` 及 `go.mod` 清理，独立可回退。
- **提交 2（工具链与代码生成固定）**：`mise.toml`、`go.mod tool`、`buf.*.gen.yaml`、`justfile`、`web/` 锁文件及一次性生成头刷新。
- **提交 3（钩子、CI 与 Renovate）**：`prek.toml`（含存量格式修复）、`.github/workflows/*`、`renovate.json5`、`AGENTS.md` / `README.md`。

## 实施后人工/仓库设置事项

- 在 GitHub `Servora-Kit/servora` 分支保护规则中将 `ci-ok` 设为 `main` 唯一必过状态检查。
- 在 Mend Developer Portal 确认 `Servora-Kit/servora` 已开启 Renovate Interactive 模式（并确保仓库开启 Issues 以承载 Dependency Dashboard）。
