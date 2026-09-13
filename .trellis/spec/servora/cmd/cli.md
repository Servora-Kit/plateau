# `svr` 开发 CLI

`cmd/svr` 是仓库内统一开发 CLI；入口 [`main.go`](../../../../../servora/cmd/svr/main.go) 只执行 `root.Execute()`，错误以退出码 1 结束。命令从 Servora 项目根执行，详见 [`cmd/svr/AGENTS.md`](../../../../../servora/cmd/svr/AGENTS.md)。

`svr new api <name> <server_name>` 校验小写 snake_case（可点分层级）和现有 `app/<server_name>/service`，只生成服务 Proto 骨架和文档 Proto；它不生成 HTTP 专用 Proto 或最终 Go 代码。`svr gen gorm` 支持多服务、无参数交互选择和 `--dry-run`；发现/校验逻辑属于 `internal/discovery`，批量失败最终汇总。

保持命令解析、发现、生成和 UX 输出的现有内部边界，不让 CLI 直接承担业务服务生成配置。变更后运行相关 `go test ./cmd/svr/...`；生成的 Proto 还需由调用方按 [Proto generation](../proto/generation.md) 执行并审查生成结果。
