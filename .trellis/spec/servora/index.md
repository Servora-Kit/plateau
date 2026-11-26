# Servora 母框架规范

适用于独立仓库 `../servora` 的框架开发。它是 Plateau 的上游依赖与规范来源；跨仓修改、发版或生成物更新须在 `../servora` 自身门禁下独立验证，不替代消费方仓库检查。

开发前先按改动位置读取对应层的 `index.md` 和正文；不要从 Plateau 的业务服务规范反推框架内部实现。

| 分组 | 适用内容 |
| --- | --- |
| [framework](framework/index.md) | Go 微内核 runtime、transport、可观测性、`contrib/k8s`、TLS |
| [proto](proto/index.md) | 框架公共 Proto 注解、生成流程与产物归属 |
| [cmd](cmd/index.md) | `protoc` 代码生成插件契约与验证 |
| [web](web/index.md) | `@servora/proto-utils` 的共享 TypeScript 契约 |

## 工具链与工程门禁

- **版本单一来源**：Go 最低版本与安全补丁工具链由 `../servora/go.mod` 的 `go` + `toolchain` 指令声明；全部 4 款微内核 `protoc` 插件在 `go.mod` 的 `tool (...)` 块固定并通过 `go tool` 调用；`buf`、`golangci-lint`、`just`、`node`、`pnpm`、`prek` 由 `../servora/mise.toml` 固定；前端依赖冷却期由 `../servora/web/pnpm-workspace.yaml`（`minimumReleaseAge: 1440`）声明，并由 `renovate.json5` 自动同步跟踪。
- **本地钩子（`prek.toml`）**：首次准备执行 `just init`（串联 `mise install`、`prek install`、`just web-install`）。`pre-commit` 执行文件卫生、`gofmt`、`buf format`、`just --fmt --check --unstable` 与 `biome check`；`pre-push` 在 `GOWORK=off` 下执行 `just lint-go`、`just lint-proto`、`just breaking`、`just gen-check`；`commit-msg` 校验 Conventional Commits。
- **CI 门禁与跨平台脚本**：仓库门禁统一在 `GOWORK=off` 下运行，`ci.yml` 按变更路径分流 `prek`、`go`、`proto`、`gen`、`web` 并汇总至唯一必过检查 `ci-ok`（同时供 `release.yml` 校验 `main` 分支 `push` 通过记录），其中生成目录（`api/gen/go/**`、`web/packages/proto-utils/src/gen/**`）须触发 `gen`（`just gen-check`）；`security.yml` 定期及随 `go.mod`/`go.sum` 变更执行 `govulncheck ./...`。`justfile` 的 Windows PowerShell `[script(...)]` 配方因 `just` 以临时 `.ps1` 文件传参，须声明 `[script("powershell.exe", "-NoLogo", "-NoProfile", "-ExecutionPolicy", "Bypass", "-File")]`（禁用 `-Command`），并在 `$ErrorActionPreference = "Stop"` 下用 `[Console]::Error.WriteLine(...)` 配合显式 `exit` 传递退出码。

框架日常命令入口见 [`../servora/AGENTS.md`](../../../../servora/AGENTS.md)。
