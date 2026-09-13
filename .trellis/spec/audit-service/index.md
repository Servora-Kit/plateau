# Audit 后端规范

适用于 `app/audit/service` 的现有实现。该服务在 [app 目录说明](../../../app/AGENTS.md) 中标记为“停止更新和做参考，等待后期重构”；因此它用于理解现有审计消费和查询行为，不作为新增微服务或新 Kafka/ClickHouse 集成的推荐模板。通用分层规则仍见 [共享微服务规范](../service/backend/index.md)。

## 开发前检查

- 先确认需求是维护既有 Audit 行为还是重构设计；后者应另有任务与验收。
- Kafka 记录消费、CloudEvent 校验、批处理与提交读 [ingestion](ingestion.md)。
- 改动 API 查询还须阅读 `internal/service/audit.go`、`internal/biz/audit.go` 与 `internal/data/audit.go` 的实际契约。

## 质量检查

- 当前可见测试入口包括 [consumer_test.go](../../../app/audit/service/internal/data/consumer_test.go) 与 [batch_writer_test.go](../../../app/audit/service/internal/data/batch_writer_test.go)。
- Kafka、ClickHouse、服务端注册和关闭顺序的真实验收依赖运行基础设施；静态代码或单测不能替代该验收。

## 主题

- [维护状态](status.md)：参考边界和当前组成。
- [摄取](ingestion.md)：当前 Kafka 到 ClickHouse 的数据流和生命周期事实。
