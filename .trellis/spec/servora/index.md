# Servora 母框架规范

适用于独立仓库 `../servora` 的框架开发。它是 Plateau 的依赖和规范来源，不是本仓联调、发布或生成物写入的授权边界。

开发前先按改动位置读取对应层的 `index.md` 和正文；不要从 Plateau 的业务服务规范反推框架内部实现。

| 分组 | 适用内容 |
| --- | --- |
| [framework](framework/index.md) | Go runtime、CRUD 内部契约、transport、可观测性、provider、TLS |
| [proto](proto/index.md) | 框架公共 Proto 注解与生成物归属 |
| [cmd](cmd/index.md) | `svr` 开发 CLI 与 `protoc` 插件 |
| [web](web/index.md) | `@servora/proto-utils` 的共享 TypeScript 契约 |

框架的常规检查入口由 [`../servora/AGENTS.md`](../../../../servora/AGENTS.md) 定义：`just test`、`just test-all`、`just lint-proto`、`just web-typecheck` 和 `just web-build` 按影响面选择执行。生成或发布命令只是框架维护入口；本任务未运行它们。
