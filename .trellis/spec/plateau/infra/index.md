# 平台基础设施适配规范

适用于 `infra/` 下的 OpenFGA、ClickHouse、Kafka、Redis、通用缓存（`cache` / `cache/redis`）、Ent（`driver.go`、`tracing.go`、`crud/`、`mixin/`）与 GORM（`slog.go`、`crud/`），以及 `transport/server/kafka` 消费传输层。

Mail 仅提供平台共享配置 schema，见 [API 注解](../../api/proto/annotations.md)；发送行为由消费应用拥有，不在平台 infra 下扩张运行时职责。ClickHouse 连接适配由 Plateau 维护。

| 主题 | 何时读取 |
| --- | --- |
| [OpenFGA](openfga.md) | 配置到官方 SDK client 的映射 |
| [ClickHouse](clickhouse.md) | 可选连接、TLS 与清理责任 |
| [Kafka](kafka.md) | 可选客户端连接、SASL/TLS 与非泛型批消费 `transport.Server` |
| [Ent 软删除](../../../../infra/entgo/mixin/soft_delete.go) | tombstone 字段、默认查询过滤、删除改写与显式 bypass |
| [Redis 客户端与锁](../../../../infra/redis/redis.go) | `plateau.infra.redis.v1.Redis` 官方客户端构造、TLS 与分布式锁 |
| [Cache](../../../../infra/cache/doc.go) | 内存与 Redis Cache-aside 适配及 keyspace 隔离 |
| [Ent Driver & CRUD](../../../../infra/entgo/driver.go) | `NewDriver`（`WithDB` 借用连接池）、SQL tracing 与 [CRUD adapter](../crud/index.md) |
| [GORM 适配](../../../../infra/gorm/slog.go) | `*slog.Logger` 日志适配与 `infra/gorm/crud` 扩展位 |

开发前检查配置源与消费方，区分未配置、配置错误和运行故障。质量检查覆盖配置校验、输入/全局对象隔离、日志归属和资源关闭。入口：`go test ./infra/...`。
