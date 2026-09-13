# 平台基础设施适配规范

适用于当前 infra 下的 OpenFGA 与 ClickHouse 接入。母框架的通用 provider 与 Ent 扩展见 [Servora](../../servora/framework/index.md)。

Mail 当前仅有平台共享配置 schema，契约见 [API 注解](../../api/proto/annotations.md)；发送行为归 IAM 的 `internal/mail`，不在平台 infra 下虚构共享 Mail runtime。ClickHouse 当前由 Plateau 维护，历史 Servora contrib 路径不再是现行归属。

| 主题 | 何时读取 |
| --- | --- |
| [OpenFGA](openfga.md) | 配置到官方 SDK client 的映射 |
| [ClickHouse](clickhouse.md) | 可选连接、TLS 与清理责任 |

开发前检查实际配置源与消费方，确认未配置、配置错误和运行故障的区别。质量检查覆盖配置校验、输入/全局对象隔离、日志归属和资源关闭。入口：`go test ./infra/...`；构造测试不代替真实依赖环境验收。
