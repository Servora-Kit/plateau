# Kafka 连接与消费传输

适用于平台级 `infra/kafka` 客户端构造与 `transport/server/kafka` 非泛型入站消费 `Server`。

## Scenario: Kafka 客户端构造与批消费位点管理

### 1. Scope / Trigger

- Trigger：新增或修改 Kafka 连接参数映射、SASL/TLS 配置、生产端客户端装配，或基于消费者组构建入站批处理 `transport.Server`。

### 2. Signatures

- 配置源：[config.proto](../../../../api/protos/plateau/infra/kafka/v1/config.proto)（`package plateau.infra.kafka.v1`，`message Kafka`，`section = true` 对应 `kafka:` 段）。
- 连接构造（[infra/kafka/kafka.go](../../../../infra/kafka/kafka.go)）：
  - `BuildOpts(cfg *kafkapb.Kafka, extra ...kgo.Opt) ([]kgo.Opt, error)`
  - `NewClient(ctx context.Context, cfg *kafkapb.Kafka, extra ...kgo.Opt) (*kgo.Client, error)`
  - `NewClientOptional(ctx context.Context, cfg *kafkapb.Kafka, extra ...kgo.Opt) (*kgo.Client, error)`
  - `WithSlogLogger(l *slog.Logger) kgo.Opt`
  - `New(cfg *kafkapb.Kafka, l *slog.Logger) (*kgo.Client, func(), error)`
- 入站消费传输层（[transport/server/kafka/server.go](../../../../transport/server/kafka/server.go)）：
  - `type BatchHandler func(ctx context.Context, records []*kgo.Record) error`
  - `NewServer(cfg *kafkapb.Kafka, opts ...ServerOption) (*Server, error)`（实现 Kratos `transport.Server`）

### 3. Contracts

- `NewClientOptional` 返回三态语义（对齐 [ClickHouse](clickhouse.md)）：
  - `(nil, nil)`：`cfg == nil` 或 `brokers` 为空（未配置 Kafka，由调用方决定禁用相关特性）；
  - `(nil, err)`：已配置但参数校验、TLS/SASL 构造或 `Ping` 失败；
  - `(client, nil)`：连接成功，调用方负责 `client.Close()`。
- `New` 面向微服务 Wire 注入：当 `cfg == nil` 或 `brokers` 为空时返回 `(nil, noopCleanup, nil)`；一旦配置了非空 `brokers` 即同步执行 `Ping` 强校验并返回带 `client.Close()` 清理函数的客户端，连接不可达时启动立即失败。
- `BuildOpts` 先深克隆输入 `cfg` 再执行 `Apply()`，绝不原地修改调用方传入的 Proto 消息；SASL `password` 标注 `(servora.redact.v3.value).string = ""`。
- `transport/server/kafka.Server` 保持非泛型协议传输定位（单实例通过 `WithTopicHandler(topic, handler)` 挂载一个或多个 Topic）：
  - 构造入口末尾强制注入 `kgo.ConsumerGroup`、`kgo.ConsumeTopics` 与 `kgo.DisableAutoCommit()`；
  - 按 Topic 聚合缓冲，达到 `batch_size` 或 `flush_interval` 触发 `BatchHandler(ctx, batch)`；
  - **累积位点提交不变量**：仅当 `BatchHandler` 返回 `nil` 时才统一调用 `CommitRecords(ctx, batch...)`；返回错误时保留整批未提交记录并按 `retry_backoff` 退避重试；
  - `Stop(stopCtx)` 先取消拉取，再以独立超时保护上下文刷空尾批并提交成功批次位点，最后关闭底层客户端。

### 4. Validation & Error Matrix

| 条件 | 行为 |
| --- | --- |
| `NewClientOptional` 输入 `nil` 或空 `brokers` | 返回 `(nil, nil)` |
| `BuildOpts` 输入空 `brokers`、未知 `compression`、未知 `SASL.mechanism`、非法 `required_acks` 或无效 TLS 证书路径 | 返回配置错误 |
| `NewClient` / `NewClientOptional` `Ping` 超时或连接拒绝 | 关闭临时客户端并返回 `nil, err` |
| `NewServer` 缺少 `consumer_group`、未注册 `TopicHandler`、空 Topic、重复 Topic 或试图传入 `AutoCommitMarks` | 构造期直接返回错误 |
| `BatchHandler` 返回非 `nil` 错误 | 记录结构化错误日志，不提交位点，等待 `retry_backoff` 重试 |

### 5. Good / Base / Bad Cases

- **Good**：生产端微服务在 `data.ProviderSet` 注入 `infrakafka.New` 并在 `server.ProviderSet` 注入 `auditkafka.NewDefault`；消费端微服务在 `internal/server` 使用 `transport/server/kafka.NewServer` 注册到 `kratos.Server(...)`。
- **Base**：本地未启用 Kafka 时省略 `kafka:` 配置段，`NewClientOptional` 返回 `nil, nil`，`audit.New` 按配置回退 `Noop` / `stdout` / `log` 审计器。
- **Bad**：在业务 `data` 层手写 `PollFetches` goroutine 并在前序缓冲尚未落库时对单条坏消息单独 `CommitRecords`（导致同分区前序未刷盘偏移量被隐式确认丢失）。

### 6. Tests Required

- `go test -race -count=1 ./infra/kafka/... ./transport/server/kafka/...`：
  - 断言 `BuildOpts` 不修改原始输入消息、默认值与 SASL/压缩分支校验生效；
  - 断言 `NewClientOptional` 严格区分 `(nil, nil)`、`(nil, err)` 与 `(client, nil)`；
  - 断言 `NewServer` 强制开启 `DisableAutoCommit`、支持多 Topic 路由、`batch_size` 与 `flush_interval` 触发、失败保留未提交位点及 `Stop` 先刷盘提交再 `Close`。

### 7. Wrong vs Correct

#### Wrong

```go
// 错误：批内单条解析失败时直接提交该条位点，隐式提交了同分区前序仍在内存缓冲中的记录！
if err := decode(record); err != nil {
    _ = client.CommitRecords(ctx, record)
    return nil
}
```

#### Correct

```go
// 正确：交由 BatchHandler 整批处理（坏记录在批内按死信/日志策略显式落定），整批成功后再由 Server 统一 CommitRecords。
srv, err := kafkasrv.NewServer(kafkaCfg,
    kafkasrv.WithTopicHandler("plateau.audit.events", func(ctx context.Context, records []*kgo.Record) error {
        return uc.IngestBatch(ctx, records)
    }),
)
```
