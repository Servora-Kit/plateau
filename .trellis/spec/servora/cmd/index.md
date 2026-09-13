# Servora 命令与插件

适用于 `../servora/cmd`。先判断改动属于开发 CLI 还是 Proto 生成插件；两者共享 `cmd` 仓储位置，但输出契约和验证不同。

- [cli](cli.md)：`svr` 的命令边界与生成脚手架。
- [plugins](plugins.md)：`protoc-gen-*` 的输入、输出和生成检查。

质量检查从受影响命令的 `go test ./cmd/<name>` 开始。变更 plugin 后先运行 `just plugin`，并按输出选择 `just gen`、`just gen-ts`、`just web-typecheck`、`just web-build`。不要手改由 plugin 产生的文件。
