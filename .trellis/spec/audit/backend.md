# Audit 服务消费、ClickHouse 持久化与内部查询契约

## 1. 适用范围与触发条件

- 触发场景：修改 `app/audit/service` 的入站 Kafka 消费、ClickHouse 表结构/查询、`page_token` 指纹或 gRPC 服务间鉴权，或在平台微服务中新增 ClickHouse 存储表。

## 2. 核心签名

- **只读 gRPC 契约（`audit.event.v1.EventService`）**：
  - `GetEvent(GetEventRequest{name: "events/{event}"}) returns (Event)`
  - `ListEvents(ListEventsRequest) returns (ListEventsResponse)`
  - `ListServices(ListServicesRequest) returns (ListServicesResponse)`
- **ClickHouse 一表一文件 Schema（`internal/data/schema/event.go`）**：
  - `type EventRow struct { ... ch:"<column>" }`（与表全部 19 列包含 `ingested_at` 1:1 对齐）
  - `EnsureEventsTable(ctx context.Context, conn driver.Conn, retentionDays int32) error`
- **入站装配（`internal/server/kafka.go`）**：
  - `NewKafkaServer(kafkaCfg *kafkapb.Kafka, auditCfg *auditconfv1.Audit, events *service.EventService, l *slog.Logger) (*svrkafka.Server, error)`

## 3. 关键契约

- **单向终点与强依赖启动**：
  - `app/audit/service` 不配置 `audit_emitter`、不挂载 `audit.Server`，仅暴露 `10020` 内部运维 HTTP（`/healthz`、`/readyz`、`/metrics`，无业务路由与公开 Traefik 标签）与 `10021` 内部 gRPC（强制 `jwt.ServerFor[jwt.ServiceClaims]` + `openfga.Server` 校验 `audit:global#read_events`）。
  - ClickHouse 与 Kafka 在 `audit.service` 启动期均为必填强依赖，配置缺失、返回 `nil` 或网络不可达时必须终止启动，禁止降级为空跑。
- **CloudEvents 入库三项归一化**：
  1. 从 W3C `traceparent` 提取中间 32 位小写非全零 hex `trace_id`（缺失时回退读 `traceid`），确保跨服务不同 `span_id` 精确串联；
  2. 未知或缺失 `category` 存空串 `""`，查询映射为 `AUDIT_CATEGORY_UNSPECIFIED`；
  3. `detail_json` 恒为合法 JSON 对象字符串：空值存 `"{}"`，数组/标量包入 `{"value":...}`，非 JSON 二进制包入 `{"_content_type","_dataschema","_base64"}`。
- **ClickHouse 存储与查询去重分工**：
  - 默认库名 `audit`、表名 `events`，采用 `ReplacingMergeTree(ingested_at)`、`PARTITION BY toYYYYMM(occurred_at)`、`ORDER BY (occurred_at, event_id)`、`GRANULARITY 1` 布隆索引及长文本 `CODEC(ZSTD(1))`；
  - `EnsureEventsTable` 建表后查询 `system.tables.engine_full`（同时匹配 `toIntervalDay(N)` 与 `INTERVAL N DAY`），仅当 `retention_days` 实际变化时执行 `ALTER TABLE events MODIFY TTL`；
  - `ListEvents` 使用 `FINAL SETTINGS do_not_merge_across_partitions_select_final = 1` 且不隐式截断未传时间范围；`GetEvent` 使用 `WHERE event_id = ? ORDER BY ingested_at DESC LIMIT 1`（不加 `FINAL`）直接命中 `idx_event_id` 跳数索引。
  - `ListServices` 使用无 `FINAL` 的 `SELECT DISTINCT service FROM events WHERE service != '' ORDER BY service ASC` 直接扫描 `LowCardinality(String)` 分区词典元数据（重复 `event_id` 不改变服务集合），毫秒级返回已观测生产服务标识。
- **环境配置键**：
  - `jwt_authn_config` 在 `local`（`http://127.0.0.1:10000/keys`）与 `docker`（`https://iam.plateau.local/keys`）统一使用 `jwks.uri`（支持 `IAM_JWKS_URI` 覆盖），不维护静态公钥与手工 `kid`。

## 4. 校验与错误矩阵

| 条件 | 行为 / 错误码 |
| --- | --- |
| 批内单条 Kafka 帧解码失败或缺 CloudEvents 必填属性（`id`/`type`/`source`/`time`） | 记录结构化诊断日志并从待写切片剔除，严禁在同批合法事件落盘前单独提前提交位点 |
| ClickHouse `SaveBatch` 失败 / 查询失败 | `SaveBatch` 直接返回底层错误以保留整批 Kafka 位点供退避重试；`GetByID` / `List` 合并 `biz.ErrStorageUnavailable` 并映射为 `EVENT_ERROR_REASON_STORAGE_UNAVAILABLE` |
| 非法资源名、`page_size < 0` 或 `start_time > end_time` | `EVENT_ERROR_REASON_INVALID_ARGUMENT`（400） |
| 篡改任一过滤条件复用旧 `page_token` 或游标损坏 | `EVENT_ERROR_REASON_INVALID_PAGE_TOKEN`（400） |
| `GetEvent` 未命中记录 | `EVENT_ERROR_REASON_NOT_FOUND`（404） |
| 缺失令牌、人类令牌或非 `audit` 受众令牌 | gRPC `Unauthenticated` |
| 未获授 `audit:global` `read_events` 的服务令牌 | gRPC `PermissionDenied` |

## 5. 典型用例（Good / Base / Bad）

- **Good**：Admin 与 IAM 产生共享 `trace_id` 的双跳事件 → 消费端批处理去重写入 `audit.events` → `service:admin` 仅凭 `trace_id` 跨月查询返回完整两跳记录。
- **Base**：空载荷事件入库 `detail_json = "{}"`，无下一页时 `next_page_token = ""`。
- **Bad**：批内首条为损坏 JSON 帧、后两条合法 → 剔除坏帧、写入后两条成功后一次性提交整批位点；若 ClickHouse 断连则整批不提交位点。

## 6. 必测断言点

- `cmd/server/config_test.go`：`local`/`docker` 配置装载、`10020`/`10021` 端口、无公开 Traefik 标签、JWKS URI 协议约束。
- `internal/biz/event_test.go`：`traceparent` 提取、`detail_json` 对象保证、坏事件隔离、修改任一过滤项使旧 `page_token` 失效。
- `internal/data/event_test.go`：未配置/不可达启动报错、`MODIFY TTL` 按需生效、重复 `event_id` `FINAL` 与 `LIMIT 1` 去重一致。
- `internal/server/{grpc_test,kafka_test}.go`：JWT/OpenFGA 五类准入断言、`/v1/audit/events` HTTP 404、双模式解码与写失败重试。

## 7. 反例与正例

#### Wrong
```go
// 错误 1：EventRow 漏写 DEFAULT 列 ingested_at，导致 INSERT INTO events 无列名 PrepareBatch + AppendStruct 运行期列数不匹配报错。
// 错误 2：ClickHouse 未配置时返回 (nil, nil) 让审计服务空跑吞掉 Kafka 位点。
```

#### Correct
```go
// EventRow 完整包含全部 19 列（含 IngestedAt ch:"ingested_at"），统一复用于 AppendStruct 与 Select；
// NewClickHouseConn 在 conn == nil 时直接返回错误阻止服务启动。
```
