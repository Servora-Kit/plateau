# 维护状态与适用范围

Audit 是 Kafka 消费审计 CloudEvent、存入 ClickHouse 并提供查询 API 的现有服务。README 将它称为“停止维护，待后期重构”，[app 目录说明](../../../app/AGENTS.md) 进一步规定“不再更新和做参考”。本规范记录当前基线，不把它提升为新服务样板。

当前 Wire 图在 [cmd/server/wire.go](../../../app/audit/service/cmd/server/wire.go)：bootstrap、可选 Kafka client、data、biz、service、server 共同装配。`newKafkaClient` 用 `NewClientOptional` 取得 client；[Consumer.Start](../../../app/audit/service/internal/data/consumer.go) 在 client 为 nil 时只记录 consumer disabled 并结束，这说明代码具备可选配置路径，不说明 Kafka/ClickHouse 已在某环境验收。

重构前，修复必须保留该服务的维护限制、明确依赖的 CloudEvent/Kafka/ClickHouse 版本契约，并独立验证消费提交、批量失败、关闭 flush 和查询分页。不要从该服务复制命名、data 结构或后台 goroutine 管理到新增业务服务。
