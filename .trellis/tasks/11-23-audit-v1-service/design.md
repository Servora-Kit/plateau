# Audit 微服务与存储重建技术设计

需求来源：[本任务 PRD](prd.md) · [父任务 PRD](../11-23-audit-v1/prd.md) · [运行时前置设计](../11-23-audit-v1-runtime/design.md)。

---

## 1. 服务定位、端口与目录结构

`app/audit/service` 是平台控制面专属的单向消费终点与只读检索微服务：入站通过 Kafka 消费 `plateau.audit.events` 并批量写入 ClickHouse `audit.events`，对内仅通过受 Service JWT 与 OpenFGA 保护的 gRPC 接口向获权平台服务（如 `app/admin/service`）开放检索；自身不反向生产审计事件（不挂载 `audit.Server`、不配置 `audit_emitter`），也不面向浏览器暴露公开 API。

- **固定端口段（`10020–10029`）**：
  - HTTP `10020`：仅由 `svrhttp.NewServer` 承载内部 `/healthz`、`/readyz` 与 `/metrics`，不注册业务路由、移除 `configs/*/bootstrap.yaml` 中的 Traefik 公开路由标签，删除空壳 `api/buf.openapi.gen.yaml`、`internal/assets/openapi.yaml` 及旧 `Makefile`；
  - gRPC `10021`：承载 `audit.event.v1.EventService` 内部调用；
  - 无独立 Web 前端（端口 `10022` 留空，界面统一归 `app/admin/web`）。
- **标准目录布局**（对齐 IAM/Admin 的 `internal/data/schema/` 规范与 `NewData` 建表生命周期，不设空壳 `internal/startup/` 包）：
  ```text
  app/audit/service/
  ├── .air.toml
  ├── justfile                              # import "../../../just/service.just"
  ├── api/
  │   └── protos/audit/
  │       ├── conf/v1/config.proto          # Audit 服务消费批次与保留期配置（section: audit）
  │       └── event/v1/event.proto          # Event 资源与只读 EventService
  ├── cmd/server/
  │   ├── main.go                           # bootstrap.NewRuntime + Scan + Run
  │   ├── wire.go                           # Wire 依赖装配
  │   ├── wire_gen.go
  │   └── config_test.go                    # local / docker 配置离线扫描校验测试
  ├── configs/
  │   ├── local/                            # bootstrap.yaml, audit.yaml, clickhouse.yaml, kafka.yaml, jwt_authn.yaml, openfga.yaml
  │   └── docker/
  └── internal/
      ├── biz/
      │   ├── biz.go                        # ProviderSet
      │   ├── event.go                      # 复用 *auditeventpb.Event 领域资源、CloudEvents 归一化投影与查询/入库 Usecase
      │   ├── page_token.go                 # 绑定查询过滤指纹的不透明游标编解码
      │   └── errors.go                     # 领域哨兵错误定义
      ├── data/
      │   ├── schema/
      │   │   └── event.go                  # 单表存储定义：表名常量、带 ch 标签的 EventRow、幂等建表与按需 MODIFY TTL
      │   ├── data.go                       # ProviderSet + ClickHouse/Kafka 非空强校验、表初始化与连接关闭
      │   └── event.go                      # 实现 biz.EventRepo（AppendStruct / Select）及 EventRow -> *auditeventpb.Event 单跳转换
      ├── server/
      │   ├── server.go                     # ProviderSet
      │   ├── grpc.go                       # ServiceClaims JWT AuthN + OpenFGA AuthZ + EventService
      │   ├── http.go                       # 最小内部运维 HTTP Server（/healthz, /readyz, /metrics）
      │   └── kafka.go                      # Kafka 入站传输 Server（实现 Kratos transport.Server）
      └── service/
          ├── service.go                    # ProviderSet
          └── event.go                      # 资源名解析、错误编码与 EventService gRPC 实现
  ```

---

## 2. Proto 契约设计

### 2.1 服务配置契约（`audit/conf/v1/config.proto`）

采用 `servora.conf.v1` 标准 `Apply()` 默认值与 `buf.validate` 约束：

```protobuf
syntax = "proto3";

package audit.conf.v1;

import "buf/validate/validate.proto";
import "google/protobuf/duration.proto";
import "servora/conf/v1/annotations.proto";

option go_package = "github.com/Servora-Kit/plateau/api/gen/go/audit/conf/v1;auditconfv1";

message Audit {
  option (servora.conf.v1.section) = true;

  optional string topic = 1 [
    (servora.conf.v1.field) = {default: "plateau.audit.events"},
    (buf.validate.field).string.min_len = 1
  ];
  optional int32 batch_size = 2 [
    (servora.conf.v1.field) = {default: "200"},
    (buf.validate.field).int32 = {gt: 0, lte: 5000}
  ];
  google.protobuf.Duration flush_interval = 3 [
    (servora.conf.v1.field) = {default: "1s"}
  ];
  optional int32 retention_days = 4 [
    (servora.conf.v1.field) = {default: "180"},
    (buf.validate.field).int32 = {gte: 1, lte: 3650}
  ];
}
```

### 2.2 领域查询契约（`audit/event/v1/event.proto`）

对齐 `iam.user.v1` 命名规范（去除 `audit` 命名空间内的重复 `Audit` 前缀），**仅暴露 `GetEvent` 与 `ListEvents` 两个只读 RPC**：

```protobuf
syntax = "proto3";

package audit.event.v1;

import "buf/validate/validate.proto";
import "google/api/client.proto";
import "google/api/field_behavior.proto";
import "google/api/resource.proto";
import "google/protobuf/timestamp.proto";
import "plateau/security/audit/v1/annotations.proto";
import "plateau/security/authn/v1/annotations.proto";
import "plateau/security/authz/v1/annotations.proto";
import "servora/errors/v1/errors.proto";

option go_package = "github.com/Servora-Kit/plateau/api/gen/go/audit/event/v1;auditeventpb";

service EventService {
  option (plateau.security.authn.v1.service_default) = {mode: AUTHN_MODE_REQUIRED};
  option (plateau.security.authz.v1.service_default) = {
    mode: AUTHZ_MODE_REQUIRED
    action: "read_events"
    resource_type: "audit"
    resource_id: "global"
  };
  option (plateau.security.audit.v1.service_default) = {mode: AUDIT_MODE_DISABLED};

  rpc GetEvent(GetEventRequest) returns (Event) {
    option (google.api.method_signature) = "name";
  }

  rpc ListEvents(ListEventsRequest) returns (ListEventsResponse) {
    option (google.api.method_signature) = "page_size,page_token";
  }
}

enum EventErrorReason {
  option (servora.errors.v1.default_code) = 500;

  EVENT_ERROR_REASON_UNSPECIFIED = 0;
  EVENT_ERROR_REASON_NOT_FOUND = 1 [(servora.errors.v1.code) = 404];
  EVENT_ERROR_REASON_INVALID_ARGUMENT = 2 [(servora.errors.v1.code) = 400];
  EVENT_ERROR_REASON_INVALID_PAGE_TOKEN = 3 [(servora.errors.v1.code) = 400];
  EVENT_ERROR_REASON_STORAGE_UNAVAILABLE = 4 [(servora.errors.v1.code) = 503];
}

message Event {
  option (google.api.resource) = {
    type: "audit.plateau.dev/Event"
    pattern: "events/{event}"
    singular: "event"
    plural: "events"
  };

  string name = 1 [(google.api.field_behavior) = IDENTIFIER];
  string event_id = 2 [(google.api.field_behavior) = OUTPUT_ONLY];
  string event_type = 3 [(google.api.field_behavior) = OUTPUT_ONLY];
  plateau.security.audit.v1.AuditCategory category = 4 [(google.api.field_behavior) = OUTPUT_ONLY];
  google.protobuf.Timestamp occurred_time = 5 [(google.api.field_behavior) = OUTPUT_ONLY];

  string service = 6 [(google.api.field_behavior) = OUTPUT_ONLY];
  string operation = 7 [(google.api.field_behavior) = OUTPUT_ONLY];

  string actor_type = 8 [(google.api.field_behavior) = OUTPUT_ONLY];
  string actor_id = 9 [(google.api.field_behavior) = OUTPUT_ONLY];

  string resource_type = 10 [(google.api.field_behavior) = OUTPUT_ONLY];
  string resource_name = 11 [(google.api.field_behavior) = OUTPUT_ONLY];

  bool success = 12 [(google.api.field_behavior) = OUTPUT_ONLY];
  string error_code = 13 [(google.api.field_behavior) = OUTPUT_ONLY];
  string error_message = 14 [(google.api.field_behavior) = OUTPUT_ONLY];

  string trace_id = 15 [(google.api.field_behavior) = OUTPUT_ONLY];
  string request_id = 16 [(google.api.field_behavior) = OUTPUT_ONLY];
  string client_ip = 17 [(google.api.field_behavior) = OUTPUT_ONLY];
  string user_agent = 18 [(google.api.field_behavior) = OUTPUT_ONLY];

  // 规范化 JSON 对象字符串（空载荷存 "{}"，数组/标量包入 {"value":...}，二进制包入信封对象）。
  string detail_json = 19 [(google.api.field_behavior) = OUTPUT_ONLY];
}
```

`ListEventsRequest` 提供类型明确的精确过滤字段（所有字符串过滤项统一 `strings.TrimSpace` 后执行严格精确匹配 `=`）：
- `google.protobuf.Timestamp start_time = 1` / `end_time = 2`（若均提供且 `start_time > end_time` 返回 `EVENT_ERROR_REASON_INVALID_ARGUMENT`；未传时不在服务端隐式截断时间窗，确保仅按 `trace_id` 检索时能跨月返回完整调用链）；
- `plateau.security.audit.v1.AuditCategory category = 3`（非 `UNSPECIFIED` 时精确匹配）；
- `string service = 4`、`string event_type = 5`、`string operation = 6`；
- `string actor_type = 7`、`string actor_id = 8`；
- `string resource_type = 9`、`string resource_name = 10`；
- `optional bool success = 11`（区分不限成败、仅成功 `true` 与仅失败 `false`）；
- `string trace_id = 12`；
- `int32 page_size = 13`（默认 `50`，最大钳制为 `200`；负值返回 `EVENT_ERROR_REASON_INVALID_ARGUMENT`）、`string page_token = 14`。

---

## 3. ClickHouse 存储模型、结构体标签与读写策略（`audit.events`）

1. **数据库与强依赖校验**：
   - 默认库名对齐 `docker-compose.yaml` 现行 `CLICKHOUSE_DB: audit`；
   - `app/audit/service` 启动构造期强制要求 `infra/clickhouse.NewConnOptional` 与 `infra/kafka.NewClientOptional` 返回的客户端均非 `nil`（未配置或不可达立即报错退出，禁止降级为空跑）。
2. **一表一文件 Schema 与 `ch` 标签行模型（`internal/data/schema/event.go`）**：
   - 使用官方 `clickhouse-go/v2` 原生 `ch:"..."` 结构体标签承载 `batch.AppendStruct` 与 `conn.Select` 映射，不引入第三方 ClickHouse ORM：
   ```sql
   CREATE TABLE IF NOT EXISTS events (
       event_id       String,
       event_type     LowCardinality(String),
       category       LowCardinality(String),
       occurred_at    DateTime64(6, 'UTC'),
       ingested_at    DateTime64(6, 'UTC') DEFAULT now64(6, 'UTC'),

       service        LowCardinality(String),
       operation      LowCardinality(String),

       actor_type     LowCardinality(String),
       actor_id       String,

       resource_type  LowCardinality(String),
       resource_name  String,

       success        Bool,
       error_code     LowCardinality(String),
       error_message  String CODEC(ZSTD(1)),

       trace_id       String,
       request_id     String,
       client_ip      String,
       user_agent     String CODEC(ZSTD(1)),

       detail_json    String CODEC(ZSTD(1)),

       INDEX idx_event_id event_id TYPE bloom_filter(0.01) GRANULARITY 1,
       INDEX idx_trace_id trace_id TYPE bloom_filter(0.01) GRANULARITY 1,
       INDEX idx_actor_id actor_id TYPE bloom_filter(0.01) GRANULARITY 1,
       INDEX idx_resource_name resource_name TYPE bloom_filter(0.01) GRANULARITY 1
   ) ENGINE = ReplacingMergeTree(ingested_at)
   PARTITION BY toYYYYMM(occurred_at)
   ORDER BY (occurred_at, event_id)
   TTL toDateTime(occurred_at) + INTERVAL 180 DAY
   SETTINGS index_granularity = 8192;
   ```
   - **按需 TTL 同步**：`EnsureEventsTable` 在执行 `CREATE TABLE IF NOT EXISTS` 后，查询 `SELECT engine_full FROM system.tables WHERE database = currentDatabase() AND name = 'events'` 提取当前 `INTERVAL N DAY`；仅当 `N != retentionDays` 时执行 `ALTER TABLE events MODIFY TTL toDateTime(occurred_at) + INTERVAL %d DAY`。
   - **查询去重分工**：
     - `ListEvents`：按 `ORDER BY occurred_at DESC, event_id DESC` 扫描，附带 `FINAL SETTINGS do_not_merge_across_partitions_select_final = 1`；
     - `GetEvent`：按 `WHERE event_id = ? ORDER BY ingested_at DESC LIMIT 1`（不附带 `FINAL`）直接利用 `idx_event_id` 跳数索引单点返回最新行。

---

## 4. 绑定过滤指纹的 `page_token` 契约

复用 `servora/core/crud.NewUnsignedPageTokenCodec()` 与 `crudpb.PageTokenPayload`（确定性 Proto 二进制 + 无填充 Base64URL）：
1. `Version = crud.CurrentPageTokenVersion`（`1`）；
2. `ContextFingerprint`：对除 `page_size` 与 `page_token` 外的全部规范化过滤条件计算确定性 SHA-256（32 字节）；
3. `Cursor`：`[Int64Value(lastOccurredAt.UTC().UnixMicro()), StringValue(lastEventID)]`。
- `ListEvents` 收到非空 `page_token` 时：解码失败、版本不匹配、游标长度异常或 `ContextFingerprint` 不一致，立即返回 `EVENT_ERROR_REASON_INVALID_PAGE_TOKEN`（HTTP 400 / gRPC `InvalidArgument`）；校验通过时注入参数化谓词 `(occurred_at < ? OR (occurred_at = ? AND event_id < ?))`，多取 `page_size + 1` 行判断是否存在下一页。

---

## 5. CloudEvents 入库归一化与四层职责分工

| 层与文件 | 职责内容 | 禁止越层行为 |
| --- | --- | --- |
| `internal/server/kafka.go` | 注册 `auditCfg.GetTopic()` 的 `BatchHandler`，遍历 `[]*kgo.Record` 调用 `auditkafka.DecodeRecord` 解码为 `[]cloudevents.Event`，转发给 `eventSvc.IngestEvents` | 不将 `*kgo.Record` 或 Kafka 位点概念下传至 `service`/`biz`/`data` |
| `internal/service/event.go` | 实现 `EventService` gRPC 查询与内部 `IngestEvents` 转发，统一在 `eventError` 中将 `biz` 哨兵错误映射为 `EventErrorReason`（直接返回 `*auditeventpb.Event` 无需二次拷贝） | 不持有数据库或 Kafka 客户端 |
| `internal/biz/event.go` | 校验必填属性（`id`、`type`、`source`、`time`）并归一化生成领域资源 `*auditeventpb.Event`（坏事件记日志剔除），调用 `EventRepo.SaveBatch` | 不含 `ch` 标签，不反向 import `data` 或透传 RPC 请求类型 |
| `internal/data/schema/event.go` + `internal/data/event.go` | 单表 `EventRow`、建表/TTL 同步、`PrepareBatch` + `AppendStruct` 写入及参数化 `Select` 查询 | 不做 CloudEvents 解码或坏消息丢弃裁决 |

- **入库三项归一化规则（`internal/biz/event.go`）**：
  1. **`trace_id` 提取**：优先解析 W3C `traceparent`（`00-<32hex>-<16hex>-<2hex>`）提取中间全小写 32 位非全零 hex `trace_id`；若无合法 `traceparent` 则回退读取 `traceid` 扩展；
  2. **`category` 归一化**：仅接受已知小枚举值（`authn`、`resource`、`authz`），缺失或未知值统一存为空串 `""`，查询时映射为 `AUDIT_CATEGORY_UNSPECIFIED`；
  3. **`detail_json` 对象保证**：空载荷存 `"{}"`；合法 JSON 对象紧凑化后存入；合法 JSON 数组/标量包入 `{"value": <compact_json>}`；非 JSON 原始字节包入 `{"_content_type": ..., "_dataschema": ..., "_base64": ...}`。
- **累积位点安全**：批内协议帧损坏或缺失必填 CloudEvents 属性的记录仅记结构化诊断日志并从待写切片剔除，绝不提前单独提交位点；待同批合法记录写入 ClickHouse 返回 `nil` 后由 `svrkafka.Server` 一次性提交整批位点（整批均为空/坏记录时直接返回 `nil` 推进位点防死锁；ClickHouse 写失败返回 `err` 保留整批位点供退避重试）。

---

## 6. 服务间认证与 OpenFGA 授权模型

1. **OpenFGA 模型扩展（`manifests/openfga/`）**：
   - 新增 `manifests/openfga/audit.fga` 并注册进 `manifests/openfga/fga.mod`：
     ```fga
     module audit

     type audit
       relations
         define read_events: [service]
     ```
   - 新增 `manifests/openfga/audit.fga.yaml` 模型单测：覆盖 `service:admin` 获授 `read_events` on `audit:global` 时放行，未授权 `service:other` 及任意人类主体拒绝。
2. **JWT 认证配置（`configs/*/jwt_authn.yaml`）**：
   - `audit` gRPC 服务装配 `jwt.ServerFor[jwt.ServiceClaims](jwtAuthn, security.WithRulesFuncs(auditeventpb.AuthnRules))` 与 `openfga.Server(authorizer, security.WithRulesFuncs(auditeventpb.AuthzRules))`。
   - Local 与 Docker 统一采用 `jwks.uri` 模式（不引入静态公钥与手工 `kid`）：
     - `configs/local/jwt_authn.yaml`：`issuer: "${IAM_PUBLIC_ORIGIN:http://localhost:10002}"`、`audience: "audit"`、`jwks.uri: "${IAM_JWKS_URI:http://127.0.0.1:10000/keys}"`；
     - `configs/docker/jwt_authn.yaml`：`issuer: "${IAM_PUBLIC_ORIGIN:https://iam.plateau.local}"`、`audience: "audit"`、`jwks.uri: "${IAM_JWKS_URI:https://iam.plateau.local/keys}"`。
