# 可选 Provider 与 Capability Adapter

ClickHouse 配置与连接适配归 [Plateau infra](../../plateau/infra/clickhouse.md)，不在框架恢复重复适配。

适用于 `contrib/`。基础 provider 负责官方 client 构造、Proto 配置映射、生命周期及显式 provider-native logging/tracing/health 接线；跨能力 adapter 留在所属能力路径，不塞入通用 base 包。

Redis provider 只接受 Proto 配置；Kafka provider 只接受 Proto 配置和 `kgo.Opt`，需要原生日志时显式传 `kafka.WithSlogLogger`，consumer/producer 角色选项留在调用方。Provider 错误使用稳定类型并通过 `errors.Is`/`errors.As` 分类。

Ent `NewDriver` 区分 `database/sql` 与 Ent dialect；`WithDB` 只借用外部 pool，不取得关闭所有权。软删除、授权 scope、业务查询和具体 entity 由 repository 决定；adapter 约束见 [CRUD 内部契约](crud.md)。

公共 Proto config 变化时按影响运行 `just lint-proto`、`just gen-fresh`/`just gen` 和相关 package 测试；删除或重命名 Proto 才使用 fresh 生成，见 [Proto generation](../proto/generation.md)。
