# Servora framework

适用于修改 `../servora/core/`、`transport/`、`obs/`、`contrib/` 或 `security/tls/` 的框架运行时代码。

开发前检查：确认改动是多个 capability 复用、具有清晰协议且不绑定业务语义；这与 [`core/AGENTS.md`](../../../../../servora/core/AGENTS.md) 的准入要求一致。单一 capability 的工具留在该 capability，不创建 `util`、`helper` 或 `common` 聚合包。

| 主题 | 读取时机 |
| --- | --- |
| [architecture](architecture.md) | 调整 core 边界、依赖或共享协议 |
| [bootstrap](bootstrap.md) | 启动、扫描、Wire、配置和关闭顺序 |
| [crud](crud.md) | 框架 CRUD plan、列表、映射或 Ent adapter |
| [transport](transport.md) | HTTP/gRPC client/server、endpoint 或 middleware |
| [audit](audit.md) | 审计事件、规则或 auditor 后端 |
| [observability](observability.md) | 日志、trace、metric 运行时 |
| [providers](providers.md) | `contrib` provider、生命周期或 adapter |
| [tls](tls.md) | TLS Proto 到 `crypto/tls.Config` 的构造 |

质量检查按改动包运行 `go test ./<package>/...`；影响 core 时还要考虑 `go test -short ./...`。改动 public Proto、生成器或 web 合同时，再读取相应分组的检查入口。不要把这些命令的存在写成已经完成的验收。
