# Audit 后端规范

适用于 `app/audit/service` 的现有实现。该服务在 [app 目录说明](../../../app/AGENTS.md) 中标记为“停止更新和做参考，等待后期重构”；因此它用于理解现有审计消费和查询行为，不作为新增微服务或新 Kafka/ClickHouse 集成的推荐模板。通用分层规则仍见 [共享微服务规范](../service/backend/index.md)。

## 开发前检查

- 先确认需求是维护既有 Audit 行为还是重构设计；后者应另有任务与验收。
- Kafka 记录消费、CloudEvent 校验、批处理与提交遵循本文件的摄取链；改动 API 查询还须阅读 `internal/service/audit.go`、`internal/biz/audit.go` 与 `internal/data/audit.go` 的实际契约。

## 质量检查

- 当前可见测试入口包括 [consumer_test.go](../../../app/audit/service/internal/data/consumer_test.go) 与 [batch_writer_test.go](../../../app/audit/service/internal/data/batch_writer_test.go)。
- Kafka、ClickHouse、服务端注册和关闭顺序的真实验收依赖运行基础设施；静态代码或单测不能替代该验收。

## 维护状态与适用范围

Audit 是 Kafka 消费审计 CloudEvent、存入 ClickHouse 并提供查询 API 的现有服务。README 将它称为“停止维护，待后期重构”，[app 目录说明](../../../app/AGENTS.md) 进一步规定“不再更新和做参考”。本规范记录当前基线，不把它提升为新服务样板。

当前 Wire 图在 [cmd/server/wire.go](../../../app/audit/service/cmd/server/wire.go)：bootstrap、可选 Kafka client、data、biz、service、server 共同装配。`newKafkaClient` 用 `NewClientOptional` 取得 client；[Consumer.Start](../../../app/audit/service/internal/data/consumer.go) 在 client 为 nil 时只记录 consumer disabled 并结束，这说明代码具备可选配置路径，不说明 Kafka/ClickHouse 已在某环境验收。

重构前，修复必须保留该服务的维护限制、明确依赖的 CloudEvent/Kafka/ClickHouse 版本契约，并独立验证消费提交、批量失败、关闭 flush 和查询分页。不要从该服务复制命名、data 结构或后台 goroutine 管理到新增业务服务。

## 现有摄取链

当前链为 `Kafka record -> DecodeRecord -> CloudEvent 校验 -> BatchWriter -> ClickHouse -> CommitRecords`。入口是 [Consumer](../../../app/audit/service/internal/data/consumer.go)：解码失败或缺少 id/type/source/time 的记录会记录 warning 并提交该条记录；通过校验的记录按事件类型路由，当前已知 RPC 类型和未知类型都会进入通用 writer。

Consumer 使用 franz-go 的 `kgo.Record`；框架 [DecodeRecord](../../../../servora/obs/audit/kafka/kafka.go) 支持 structured JSON 与 binary CloudEvents，当前 [consumer 测试](../../../app/audit/service/internal/data/consumer_test.go) 覆盖两种输入。未知 CE type 保留通用写入路径；payload 是 JSON 时原样存为 detail，已注册的 protobuf payload 转为 ProtoJSON，无法识别的 payload 包成含 content type、schema 和 base64 的 envelope，见 [BatchWriter](../../../app/audit/service/internal/data/batch_writer.go)。

`BatchWriter` 在 [batch_writer.go](../../../app/audit/service/internal/data/batch_writer.go) 以容量或 interval 触发 flush，默认值为 100 条和一秒，可由服务 AuditConsumerConfig 覆盖。配置了 ClickHouse 时，batch 成功 `Send` 后才调用 `commitAll`；PrepareBatch、Append 或 Send 失败会记录 warning 而不在该分支提交 Kafka records。解码/属性校验失败则在 consumer 边界提交该条坏记录；`CommitRecords` 自身失败只记录 warning。`Data.ClickHouse()` 为 nil 时，当前实现跳过写入并直接提交该 batch，这是可选依赖路径的静态事实，不能视为审计持久化成功。停止时使用独立的 10 秒 context 完成最后一次 flush，避免已取消的 Start context 直接丢掉末尾 batch。修改该顺序必须配套测试成功、无效事件、写入失败和停机行为。

事件投影将 CloudEvent 的 id/type/time/source/subject 与扩展字段写成审计列，原始 payload 保留为 detail。当前 `serviceFromSource` 支持新 `//app-name` 格式并对旧 RPC path 回退；这是兼容现有记录的实现事实，字段语义调整要同时审查 [audit 查询 Repo](../../../app/audit/service/internal/data/audit.go) 的过滤和 page token 逻辑。现有 DDL 使用按 `occurred_at` 的日分区、`(service, event_type, occurred_at, event_id)` 排序及配置 retention 的 TTL（缺省 90 天），见 [clickhouse.go](../../../app/audit/service/internal/data/clickhouse.go)；这是现有服务的维护事实，不是新服务的存储模板。

## 查询维护边界

[AuditService](../../../app/audit/service/internal/service/audit.go) 提供 `ListAuditEvents`／`CountAuditEvents`，经 biz 转交 [auditRepo](../../../app/audit/service/internal/data/audit.go)。当前过滤为起止时间、event types、actor ID 与 service；值通过参数绑定。列表按 `occurred_at, event_id` 升序，页大小默认 50、上限 200，多取一条判断续页；token 为这两个 cursor 字段的 base64 JSON。它不是通用 CRUD 的查询指纹 token，不能假定它会绑定 filter、资源 scope 或授权条件。

当前错误从 Repo 经 biz/service 返回，没有统一补成通用 CRUD reason；ClickHouse 为 nil 时返回空列表或计数 0。这些是停止维护实现的现状，不推荐复制其分层或错误方式。查询变更应检查当前 Proto、过滤、cursor 和实际 ClickHouse 行为；本轮只核对源码，未运行真实查询验收。
