# Servora 代码生成插件

适用于 `../servora/cmd` 下的 `protoc-gen-*` 代码生成插件（废弃脚手架 `cmd/svr` 已移除）。

- [plugins](plugins.md)：`protoc-gen-*` 的输入、输出契约与生成检查。

所有插件均在 `../servora/go.mod` 的 `tool` 块注册，由 `buf.go.gen.yaml` 与 `buf.typescript.gen.yaml` 通过 `local: ["go", "tool", "protoc-gen-*"]` 直接调用，不再使用 `just plugin` 或全局 `go install`。

质量检查从受影响插件的 `GOWORK=off go test -race ./cmd/<name>/...` 开始；变更插件后运行 `just gen`（同步刷新 Go 与 TypeScript 生成物）及 `GOWORK=off just gen-check`，涉及 TypeScript 输出时继续运行 `just web-check`、`just web-typecheck`、`just web-test`。不要手改由插件生成的文件。
