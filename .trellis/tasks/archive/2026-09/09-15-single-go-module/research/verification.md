# 实施与验收记录

记录时间：2026-09-15（Asia/Shanghai）

## Servora 发布

- 迁移提交：`1d331c0df69ccd1b53f00863b68d7e9e46ab0f66`（`refactor(go)!: 合并生成代码到根模块`）
- main push CI：run `34977423755`，精确 `head_sha` 为上述提交，结论 `success`
- 根 tag：`v0.9.7`，远端指向上述提交
- Release workflow：run `34978537980`，结论 `success`
- GitHub Release：`https://github.com/Servora-Kit/servora/releases/tag/v0.9.7`
- 未创建 `api/gen/v0.9.7`，未手动执行 BSR push。现有 tag 触发的 Buf CI run `34978537950` 自动执行并成功，生成 BSR commit `27ba9e23cd484772a40ca1db43d4d270`；`main` 与新增的 `v0.9.7` labels 指向该 commit，`v0.9.6` 保持指向旧 commit。Proto、Buf 配置及生成内容无 diff。
- BSR 自动发布口径随后同步到 Servora 根与 API 指南，文档提交为 `d1aab7dc`（本地提交，未推送）。
- `GOPROXY=direct GOSUMDB=off go mod download -json github.com/Servora-Kit/servora@v0.9.7` 成功，origin hash 与提交一致。下载 zip 只含根 `go.mod`，并包含 `api/gen/go/servora/core/v1/bootstrap.pb.go`、CRUD 等生成 package。
- 官方 `sum.golang.org` 随后返回同一 module/go.mod hash；Plateau `go.sum` 与其一致。

Servora 固定 Go 1.27.0、`GOWORK=off` 验收通过：

- `go mod tidy`（稳定）
- `go list ./...`
- `go build ./...`
- `go test -short -race -coverprofile=coverage.out ./...`
- `just lint`、`just ci-lint`、`make ci.lint`
- `just gen`、`just gen-ts`，生成内容无 diff
- `just web-typecheck`、`just web-test`、`just web-build`
- `just --fmt --check`、`git diff --check`

## Plateau 单 module

- 仓库内 `find . -name go.mod` 只返回根 `./go.mod`。
- `GOWORK=off go list -m all` 中项目相关 module 只有 Plateau 根 module与 `github.com/Servora-Kit/servora v0.9.7`，没有 Plateau 自依赖或两个历史 `api/gen` module。
- 根 `go.mod` 用 `tool` 声明固定 Wire/Ent；Audit、Example、IAM 的公开 leaf build 全部成功，三个定向 leaf lint 全部为 `0 issues`。
- Example 的 `just test-integration` 不再要求父 workspace，并显式使用 `GOWORK=off`；以进程内 SQLite 执行 `TestUserReferenceIntegration` 通过。
- `just gen` 成功，Go/TS/OpenAPI/Wire/Ent 生成内容无意外 diff。
- 根 `go mod tidy` 前后 diff SHA-256 一致，`go mod verify` 返回 `all modules verified`。
- `GOWORK=off go build ./...`、`GOWORK=off just lint`、父 workspace 模式 `go build ./...` 与 `just lint` 均通过。
- `GOWORK=off go test -short -skip '^TestDevelopmentConfigScan$' ./...` 全部通过。
- 不带 skip 的 `GOWORK=off go test -short ./...` 只剩迁移前已复现的 `app/iam/service/cmd/server`：`TestDevelopmentConfigScan/local` 与 `/docker` 在 `config_test.go:46` 失败；AuthN/AuthZ 生成器夹具等 module 回归已修复并通过。
- Example 的 `just test-integration` 已移除对父 `go.work` 的强制依赖，并在 SQLite + `GOWORK=off` 下实跑通过。Example/Audit 的旧 Makefile 因引用已删除的 `make/core.mk` 本就不可运行，其中 Example Makefile 还保留历史 workspace 变量；它们不是当前权威入口，本任务不改变其生命周期。

## 父 workspace

`/Users/horonlee/projects/go/servora-kit/go.work` 只删除已失效的五个条目，保留 Plateau、Servora、servora-example 与 servora-transport 的其他现有 modules。固定 Go 1.27.0 后：

- 从 Servora cwd 执行 `go env GOWORK` 返回该父文件，`go list ./...`、`go build ./...` 通过。
- 从 Plateau cwd 执行 `go env GOWORK` 返回该父文件，`go list ./...`、`go build ./...`、`just lint` 通过。
- 显式 `GOWORK=off` 的两仓门禁独立通过，证明父 workspace 只是本地源码联调层。
