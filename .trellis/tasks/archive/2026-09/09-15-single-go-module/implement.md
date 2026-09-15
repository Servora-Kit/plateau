# 跨仓实施计划

## Phase 1. 建立两仓基线

- 记录 Servora 与 Plateau 的分支、HEAD、tags、remote、`git status` 和任务前 diff。
- 固定 Go 1.27.0 工具链，记录 `go version`、`GOROOT` 与两个 cwd 的 `go env GOWORK`。
- 记录 Servora 两个 module、Plateau 五个 module、各自直接依赖及已知测试状态。
- 保存本机 `/servora-kit/go.work` 原始内容，后续仅删除计划中的五个条目。

## Phase 2. Servora 合并为单 module

- 删除 `servora/api/gen/go.mod`、`go.sum`。
- 在 `GOWORK=off` 下从 Servora 根执行 `go mod tidy`，确认删除自身 `api/gen` require 且没有无关升级。
- 简化 Servora Justfile 与 Makefile 的依赖、tidy、格式、vet、测试、覆盖率和 lint 入口。
- 删除 API 子 module tag recipes，更新根与 API AGENTS。
- 修改 `.github/workflows/ci.yml`：删除两个临时 `go work init`，统一设置 `GOWORK=off`。

## Phase 3. Servora 发布前验证

使用同一 Go 工具链执行：

```bash
GOWORK=off go mod tidy
GOWORK=off go list ./...
GOWORK=off go build ./...
GOWORK=off go test -short -race ./...
GOWORK=off just lint
GOWORK=off just gen
GOWORK=off just gen-ts
just web-typecheck
just web-test
just --fmt --check
git diff --check
```

- 检查生成前后 diff，确认没有意外 Proto/生成内容变化。
- 用临时独立 consumer 或 module zip 检查证明根 module 包含 `api/gen/go/**`。
- 审查 Servora 完整 diff，确认工作区仅包含本迁移。

## Phase 4. Servora main CI 与 v0.9.7

- 创建 Servora 迁移提交并推送 `main`。
- 等待并验证该精确 SHA 的 main-push CI 成功，CI 日志不得再出现 `go work init`。
- 创建并推送根 tag `v0.9.7`，不创建 `api/gen/v0.9.7`。
- 等待 Release workflow 成功，验证远端 tag、GitHub Release 与 `GOWORK=off go mod download github.com/Servora-Kit/servora@v0.9.7`。
- 不执行本地 BSR push。

## Phase 5. Plateau 合并为单 module

- 删除 `plateau/api/gen/go.mod/go.sum`。
- 删除 Audit、Example、IAM 服务的 `go.mod/go.sum`。
- 删除 Plateau 根 `go.work/go.work.sum`，在 `.gitignore` 添加 `/go.work`、`/go.work.sum`。
- 根依赖升级到 Servora `v0.9.7`，删除 `servora/api/gen` 与 Plateau 自依赖。
- 在 `GOWORK=off` 下运行根 `go mod tidy` 并审查 `go.mod/go.sum`。
- 将根 `just lint` 改为一次根级 Go lint，移除 service Just 的 workspace 参数传递。
- 将 Wire/Ent 纳入根 `go.mod` 的 `tool` 声明并更新 leaf 生成入口；将 Plateau 生成器测试夹具的 replace 目标改为根 module。
- 同步 Plateau 根/API/App AGENTS 与相关 Trellis specs，保留并行 Admin 修改。

## Phase 6. 更新本机父 workspace

- 编辑 `/Users/horonlee/projects/go/servora-kit/go.work`，仅删除：
  - `./servora/api/gen`
  - `./plateau/api/gen`
  - `./plateau/app/audit/service`
  - `./plateau/app/example/service`
  - `./plateau/app/iam/service`
- 保留 `./servora`、`./plateau` 和所有其他相邻 module。
- 从两个仓库 cwd 验证 `go env GOWORK` 都指向父 workspace；验证 workspace 模式下基本 build/test 可用。

## Phase 7. Plateau 与跨仓验收

```bash
GOWORK=off go mod tidy
GOWORK=off go list ./...
GOWORK=off go build ./...
GOWORK=off go test -short ./...
GOWORK=off just lint
just --fmt --check
git diff --check
```

- 分别调用 Audit、Example、IAM 的 leaf build/lint，证明从服务 cwd 能向上找到根 `go.mod`。
- 对 IAM 已知配置测试保留迁移前后对照，不把同一失败归因于 module 合并。
- 从 Plateau 以 `GOWORK=off` 真实消费 Servora `v0.9.7`，证明 runtime 与生成 package 均来自同一根版本。
- 审查两个仓库与父 workspace 的最终状态，确认 Plateau 无关改动未被覆盖。

## Phase 8. 任务收尾

- 在 PRD 验收项记录实际命令、CI run、release/tag 和已知未修复基线。
- Servora 报告提交 SHA、main CI、`v0.9.7` tag、Release workflow 与干净状态。
- Plateau 报告文件变更、门禁结果和工作区剩余并行修改；未得到单独提交指令时不把这些并行修改带入提交。
