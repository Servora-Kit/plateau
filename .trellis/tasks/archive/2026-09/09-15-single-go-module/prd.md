# 将 Servora 与 Plateau 统一为单 Go module

## Goal

在一个有序任务内先将 Servora 的根代码与 `api/gen` 合并为单 Go module、修复 CI 并发布统一版本，再让 Plateau 升级到该版本并把平台根代码、生成代码与三个服务合并为单 Go module。两个仓库都不得依赖仓库内 `go.work` 才能通过干净环境门禁，本机 `/servora-kit/go.work` 只保留为可选的跨仓源码联调层。

## Background

- Servora 当前有根模块和 `api/gen` 两个 module。仓库本身不跟踪 `go.work`，但 GitHub CI 会临时执行 `go work init . ./api/gen`，根 `go.mod` 又依赖已发布的 `servora/api/gen v0.9.5`。
- Servora 历史提交 `c3d7c6d9` 表明，多 module 最初用于业务服务独立版本、未来拆仓和 Git Submodule；提交 `a2a57b49` 随后通过 CI 临时 workspace 解决当前源码跨 module 解析。这是历史设计的配套关系，不是 Buf/BSR 的要求。
- 业务服务现已归 Plateau，嵌套 Git 仓库也已清理；用户确认实际消费者会同时使用 Servora runtime 与生成 Go package，不需要 `servora/api/gen` 的独立发布周期。
- Plateau 当前有根模块、`api/gen` 生成模块以及 Audit、Example、IAM 三个服务模块；子模块通过 Plateau 伪版本互相引用，再由仓库根 `go.work` 覆盖为当前源码。
- Go 从 cwd 向父目录查找最近的 `go.work`。两个仓库移除内部 workspace 后，从其目录执行命令仍可自动使用本机 `/servora-kit/go.work`；干净 CI 没有该父文件，必须仅凭各自根 `go.mod` 工作。
- 当前 Servora `main` 工作区干净且 HEAD 为 `v0.9.6`；Plateau 工作区已有 Admin/Trellis 等并行修改，实施时必须保留。

## Requirements

### R1. Servora 单一 Go module

- Servora 只保留根 `go.mod`、`go.sum`，删除 `api/gen/go.mod`、`api/gen/go.sum`。
- 根 `go.mod` 不再依赖 `github.com/Servora-Kit/servora/api/gen`；`api/gen/go/**` 成为根 module 内普通生成 package。
- `github.com/Servora-Kit/servora/api/gen/go/...` import path 保持不变。
- 根 `go mod tidy` 统一维护 runtime、插件和生成 package 的依赖，不顺带升级无关版本。

### R2. Servora 命令、文档与 CI

- Justfile 与兼容 Makefile 从多 module 遍历改为一次根 module 操作；移除 `GO_WORKSPACE_MODULES`、`LINT_GOWORK`、`go work sync` 和独立 `tag-api` 入口。
- `.github/workflows/ci.yml` 删除两个 `go work init . ./api/gen` 步骤，并以 `GOWORK=off` 对单 module 执行 lint、build 和 test。
- 根与 `api/` AGENTS 更新为生成 package 随根 module 一起发布；历史 `api/gen/v*` tags 保留但不再创建新 tag。
- Buf module、Proto schema、生成内容和 BSR workflow 不因 Go module 合并而改变。

### R3. Servora 发布与消费证明

- 目标根版本为 `v0.9.7`，作为首个包含 `api/gen/go/**` package 的统一 Servora module 版本。
- 发布前完成 Servora 格式、lint、build、短测试、生成一致性和 `git diff --check`；提交并推送 `main` 后，必须等待该确切提交的 main CI 成功。
- main CI 成功后才创建并推送 `v0.9.7`，验证 GitHub Release workflow、远端 tag 和 Go module 消费。
- 本任务不创建 `api/gen/v0.9.7`，不手动执行 BSR push；`v0.9.7` 根 tag 仍按既有 Buf CI 自动推送未变的 Proto，并维护 BSR `main` 与 `v0.9.7` labels。

### R4. Plateau 单一 Go module

- Plateau 只保留根 `go.mod`、`go.sum`，删除 `api/gen` 及 Audit、Example、IAM 三个服务的 `go.mod/go.sum`。
- 删除 Plateau 仓库提交的 `go.work/go.work.sum`，并在根 `.gitignore` 忽略 `/go.work`、`/go.work.sum`。
- 根 `go.mod` 升级为只依赖 `github.com/Servora-Kit/servora v0.9.7`；删除 Plateau 自依赖、Plateau API 伪版本和独立 `servora/api/gen` 依赖。
- `api/gen/go` 与各服务现有 import path 保持不变；服务继续作为独立二进制和部署单元。

### R5. Plateau Just、文档与规范

- 根 `just lint` 从 Plateau 根目录执行一次完整 Go lint，同时保留 API TypeScript 与 Buf lint。
- 删除多 module 专用的 `LINT_GOWORK` 参数传递；服务级 `dev`、`run`、`build`、生成和定向 lint 入口保持可用。
- Wire 与 Ent 作为根 `go.mod` 的 Go tools 固定版本，避免删除服务级 `go.sum` 后 leaf 生成入口缺失 CLI 传递依赖校验和。
- 根及 `api/`、`app/` AGENTS，以及项目结构、开发入口、API 生成和服务布局 Trellis specs 同步为单 module 事实。
- pnpm workspace、Buf workspace、Proto/BSR 发布和 Web 项目行为保持不变。

### R6. 本地父 workspace

- 更新 `/Users/horonlee/projects/go/servora-kit/go.work`：保留 `./servora` 与 `./plateau`，删除 `./servora/api/gen`、`./plateau/api/gen` 和三个 Plateau 服务条目；其他相邻仓库 module 条目不改动。
- 从 Plateau cwd 与 Servora cwd 执行 `go env GOWORK` 都应发现该父文件。
- 两仓所有仓库级门禁还必须在 `GOWORK=off` 下通过，证明父 workspace 不是构建前置条件。

## Acceptance Criteria

- [x] Servora 只剩根 `go.mod/go.sum`，根模块不再 require 自身 `api/gen`，`GOWORK=off go list ./...` 包含 `api/gen/go/**`。
- [x] Servora Justfile、Makefile 和 `.github/workflows/ci.yml` 不再创建、同步或遍历 Go workspace；CI 明确以 `GOWORK=off` 执行单 module 门禁。
- [x] Servora 的 `GOWORK=off` 格式、lint、build、短测试、生成一致性与 `git diff --check` 通过。
- [x] Servora 迁移提交已进入远端 `main`，对应 main-push CI 成功后才发布 `v0.9.7`；Release workflow、远端 tag 与 Go module 下载可验证。
- [x] 不存在 `api/gen/v0.9.7` tag，且本任务未手动执行 BSR push；既有 tag-triggered Buf CI 自动推送成功，BSR `main`／`v0.9.7` labels 指向新 commit，Buf 配置与生成 Proto 内容没有无关变化。
- [x] Plateau 只剩根 `go.mod/go.sum`，不再跟踪 `go.work/go.work.sum`，也不再 require Plateau 自模块或 `servora/api/gen`。
- [x] Plateau `GOWORK=off go mod tidy` 稳定，`go list ./...`、`go build ./...`、`just lint` 通过并覆盖根共享包、生成 package 与三个服务。
- [x] Plateau 三个服务的 leaf build/lint 入口仍能从服务目录解析根 module；服务运行与部署边界不变。
- [x] Plateau `GOWORK=off go test -short ./...` 不出现 module/workspace 回归；IAM `TestDevelopmentConfigScan` 因 OIDC clients 被注释而存在的基线失败单独记录。
- [x] 从两个仓库 cwd 都能自动发现更新后的 `/servora-kit/go.work`，显式 `GOWORK=off` 时又都能独立完成门禁。
- [x] 两仓相关 AGENTS/Trellis 规范与结构一致，Just 格式及所有改动的 `git diff --check` 通过，Plateau 的既有 Admin/Trellis 修改未被覆盖。

## Out of Scope

- 修改 Proto schema、Go/TypeScript 生成内容、Buf module、Buf/BSR workflow，手动改写 BSR 数据或发布 `@servora/proto-utils`。
- 合并 `servora-example` 及其他相邻仓库的内部 Go modules。
- 修复 Plateau IAM 配置中已注释 OIDC clients 导致的既有测试失败。
- 修改 Plateau 当前并行 Admin 初始化任务的无关代码和规范内容。
