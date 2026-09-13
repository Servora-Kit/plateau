# 框架架构与准入

`core/` 是横切协议和平台能力聚合根。新成员必须同时满足可被多个 capability 复用、有清晰协议、不绑定业务语义；否则留在其 capability 或业务仓库中。这是 [`core/AGENTS.md`](../../../../../servora/core/AGENTS.md) 的明确准入标准。

当前 `core` 包含 `bootstrap`、`config`、`registry` 和 CRUD/mapper：CRUD 只表达资源 descriptor、资源名、列表、FieldMask、page token 与响应清理，不拥有 ORM client，也不解析认证上下文、安装权限/租户/事务策略。业务服务如何组合这些能力，见 [`service/backend/crud.md`](../../service/backend/crud.md)，不要在框架文档或代码中复制其流程。

`transport` 负责连接、协议与 middleware 装配；`obs` 负责日志、追踪、指标和 CloudEvents 审计；`contrib` 放可选第三方 client 与 capability adapter；TLS 配置只由 `security/tls` 产生。这些边界分别由 [`transport/AGENTS.md`](../../../../../servora/transport/AGENTS.md)、[`obs/AGENTS.md`](../../../../../servora/obs/AGENTS.md) 和 [`contrib/AGENTS.md`](../../../../../servora/contrib/AGENTS.md) 证明。

避免把业务 handler、领域规则、服务私有 entity、授权 scope 或页面状态提升到框架。新增 core 包时在变更说明中逐条说明准入依据，并运行受影响 package 的 Go 测试。
