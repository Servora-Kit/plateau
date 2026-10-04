# 框架架构与准入

`core/` 是横切协议和平台能力聚合根。新成员必须同时满足多 capability 复用、清晰协议、不绑定业务语义；否则留在所属 capability 或应用。

core CRUD 只表达资源 descriptor、资源名、列表、FieldMask、page token 与响应清理，不拥有 ORM client，不解析认证上下文或安装权限/租户/事务策略。应用组合方式见 [服务 CRUD 规范](../../service/backend/crud.md)，不在框架重复其流程。

`transport` 负责连接、协议与 middleware 装配；`obs` 负责日志、追踪、指标和 CloudEvents 审计；`contrib` 放可选第三方 client 与 capability adapter；TLS 配置由 `security/tls` 统一构造。

避免把业务 handler、领域规则、服务私有 entity、授权 scope 或页面状态提升到框架。新增 core 包时在变更说明中逐条说明准入依据，并运行受影响 package 的 Go 测试。
