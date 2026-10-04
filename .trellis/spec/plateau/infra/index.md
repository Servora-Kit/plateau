# 平台基础设施适配规范

适用于 infra 下的 OpenFGA、ClickHouse 接入和 Ent 软删除 mixin。通用 provider、Ent driver 与 CRUD adapter 见 [Servora](../../servora/framework/index.md)。

Mail 仅提供平台共享配置 schema，见 [API 注解](../../api/proto/annotations.md)；发送行为由消费应用拥有，不在平台 infra 下扩张运行时职责。ClickHouse 连接适配由 Plateau 维护。

| 主题 | 何时读取 |
| --- | --- |
| [OpenFGA](openfga.md) | 配置到官方 SDK client 的映射 |
| [ClickHouse](clickhouse.md) | 可选连接、TLS 与清理责任 |
| [Ent 软删除](../../../../infra/entgo/mixin/soft_delete.go) | tombstone 字段、默认查询过滤、删除改写与显式 bypass |

开发前检查配置源与消费方，区分未配置、配置错误和运行故障。质量检查覆盖配置校验、输入/全局对象隔离、日志归属和资源关闭。入口：`go test ./infra/...`。
