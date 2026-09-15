# 实施基线

记录时间：2026-09-15（Asia/Shanghai）

## Servora

- 路径：`/Users/horonlee/projects/go/servora-kit/servora`
- 分支：`main`，与 `origin/main` 一致，工作区干净
- HEAD：`98d3c55e2497891769fcbb4a3eeccf6fdd292887`
- HEAD tag：`v0.9.6`
- Go modules：根 module、`api/gen` module
- 仓库 cwd 自动发现的 workspace：`/Users/horonlee/projects/go/servora-kit/go.work`

## Plateau

- 路径：`/Users/horonlee/projects/go/servora-kit/plateau`
- 分支：`main`，相对 `origin/main` ahead 2
- HEAD：`cf4d742b88a5e5529cd5802796e4b4c5546ca40b`
- Go modules：根 module、`api/gen` module、Audit/Example/IAM 三个 service modules
- 仓库 cwd 自动发现的 workspace：`/Users/horonlee/projects/go/servora-kit/plateau/go.work`
- 工作区已有 Admin/Trellis 规范整理等并行修改；本任务不得回退或覆盖。

## 工具链

- shell 默认 `go` 二进制为 `/opt/homebrew/bin/go`（Go 1.27.1），但默认 `GOROOT` 指向 mise Go 1.27.0，存在混用风险。
- 本任务门禁固定使用 `/Users/horonlee/.local/share/mise/installs/go/1.27.0/bin/go`，同时设置对应 `GOROOT` 与 `GOTOOLCHAIN=local`。

## 已知失败

迁移前使用固定 Go 1.27.0、从 `app/iam/service` 执行：

```bash
go test ./cmd/server -run '^TestDevelopmentConfigScan$' -count=1
```

`local` 与 `docker` 两个子测试均在 `config_test.go:46` 失败，消息为 `service configuration did not load`。该失败来自当前 OIDC clients 配置被注释，是迁移前基线，不属于本任务。

## 父 workspace 原始条目

迁移前 `/Users/horonlee/projects/go/servora-kit/go.work` 同时包含 Servora/Plateau 根 module、两者的 `api/gen` module 与 Plateau 三个 service modules；实施只删除设计中列出的五个失效条目，保留其他仓库条目。
