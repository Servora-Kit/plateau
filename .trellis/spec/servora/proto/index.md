# Servora Proto

适用于 `../servora/api/protos`、根 Buf 配置和生成物归属。开发前先确认契约是否为跨业务仓库复用的框架能力；业务 service Proto 不放入 Servora。

- [annotations](annotations.md)：公共 annotation、命名空间和合并语义。
- [generation](generation.md)：Buf 输入、插件输出、生成物和检查。

质量检查：变更源 Proto 后运行 `just lint-proto` 与 `just gen`；删除、重命名 Proto 或移除插件时使用 `just gen-fresh`。生成器或 TypeScript 输出受影响时再读取 [cmd/plugins](../cmd/plugins.md) 和 [web/proto-utils](../web/proto-utils.md)。生成目录不可手改。
