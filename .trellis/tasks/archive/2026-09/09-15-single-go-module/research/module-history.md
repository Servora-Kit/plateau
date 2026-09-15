# Go module 与 workspace 调研证据

## Servora

- 仓库：`/Users/horonlee/projects/go/servora-kit/servora`
- 调研时状态：`main`、工作区干净、HEAD 位于 `v0.9.6`。
- 当前 module：
  - `github.com/Servora-Kit/servora`
  - `github.com/Servora-Kit/servora/api/gen`
- Servora 仓库不跟踪 `go.work`；根 `.gitignore` 已忽略 `go.work`、`go.work.sum`。
- 根 `go.mod` require `github.com/Servora-Kit/servora/api/gen v0.9.5`。
- `.github/workflows/ci.yml` 的 lint 与 test jobs 都执行 `go work init . ./api/gen`。
- Justfile 与 Makefile 都通过 `GO_WORKSPACE_MODULES := . api/gen` 遍历 module，并维护 `tag-api`/`tag.api`。
- `buf.yaml` 的 BSR module 是 `buf.build/servora/servora`；`buf push` 与 Go module/workspace 没有依赖关系。

## 历史原因

- `c3d7c6d9`（2026-03-06，`refactor: migrate to Buf v2 workspace and Go multi-module structure`）的归档 OpenSpec 明确写明：拆 module 是为框架和业务服务独立发布、服务独立依赖、未来 Git Submodule/独立仓库做准备。
- 同一设计中，`buf generate clean: true` 只解释了为什么独立 module 的 `go.mod` 放在 `api/gen/` 而不是 `api/gen/go/`，并不要求生成代码必须成为独立 module。
- `a2a57b49`（2026-05-19，`ci: CI 生成 go.work 解决多模块版本解析问题`）说明 CI workspace 是 module 拆分后的版本解析补丁。
- 当前业务服务已经迁出 Servora；用户确认实际消费者同时使用 runtime 与生成 package，因此独立生成 module 的原始前提不再成立。

## Plateau

- 仓库：`/Users/horonlee/projects/go/servora-kit/plateau`
- 当前 module 共五个：根、`api/gen`、Audit service、Example service、IAM service。
- 根与服务 module 通过 Plateau 伪版本互相引用；根 `go.work` 还引用相邻 Servora modules。
- 根直接依赖已升级为 Servora `v0.9.6` 与 Servora API `v0.9.5`。
- 三个服务及生成 module 的主要直接依赖版本一致；根 module 已包含大部分共享/IAM 依赖。
- Plateau 工作区存在 `09-15-admin-initial` 等并行改动；迁移必须逐文件保留。
- IAM `TestDevelopmentConfigScan` 在迁移前已因 local/docker 的 OIDC clients 被注释而失败，失败位置为 `app/iam/service/cmd/server/config_test.go:46`。

## 本地父 workspace

- `/Users/horonlee/projects/go/servora-kit/go.work` 是本机跨仓 workspace。
- 从 Servora 或 Plateau cwd，Go 都会向父目录查找并发现它；当前 Plateau 根自己的 `go.work` 会优先遮住父文件。
- 两仓合并后父 workspace 只需保留 `./servora` 与 `./plateau`，其他相邻仓库条目保持不变。

## 验证环境

- 本机出现过 Go 1.27.0 标准库缓存与 Go 1.27.1 tool binary 混用导致的 `compile: version ... does not match go tool version ...`。
- 正式验证须统一 `GOROOT`、`PATH`、`GOTOOLCHAIN=local`，必要时使用任务专用 `GOCACHE`。
- 可移植性门禁统一显式使用 `GOWORK=off`；父 workspace 模式另作本地联调验证。
