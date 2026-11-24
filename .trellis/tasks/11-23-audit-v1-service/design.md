# Audit 微服务与存储重建技术设计

需求来源：[本任务 PRD](prd.md) · [父任务 PRD](../11-23-audit-v1/prd.md) · [运行时前置设计](../11-23-audit-v1-runtime/design.md)。

---

## 1. 服务定位、端口与目录结构

`app/audit/service` 是平台控制面专属的事后归集与只读查询微服务：入站通过 Kafka 消费 `plateau.audit.events` 并批量写入 ClickHouse，对内仅通过受 Service JWT 与 OpenFGA 保护的 gRPC 接口向获权平台服务（如 `app/admin/service`）开放检索，不直接面向浏览器暴露公开 API。

- **固定端口段（`10020–10029`）**：
  
  - HTTP `10020`：仅用于本地/容器健康检查与内部调试，移除旧 `configs/*/bootstrap.yaml` 中的 Traefik 公开路由标签（`traefik.enable` 不开启公开 `/v1/audit` 转发）；
  - gRPC `10021`：承载 `audit.event.v1.AuditEventService` 内部调用；
  - 无独立 Web 前端（端口 `10022` 留空，界面统一归 `app/admin/web`）。
- **标准四层目录布局**（彻底清理旧 `audit.service.v1` 与混放于 `data` 层的消费循环）：
  ```text
  app/audit/service/
  ├── .air.toml
  ├── justfile                              # import "../../../just/service.just"
  ├── api/
  │   ├── buf.openapi.gen.yaml              # 生成 internal/assets/openapi.yaml
  │   └── protos/audit/
  │       ├── conf/v1/config.proto          # Audit 服务消费批次与保留期配置（section: audit）
  │       └── event/v1/event.proto          # AuditEvent 资源与只读 AuditEventService
  ├── cmd/server/
  │   ├── main.go                           # bootstrap.NewRuntime + Scan + Run
  │   ├── wire.go                           # Wire 依赖装配
  │   ├── wire_gen.go
  │   └── config_test.go                    # local / docker 配置扫描校验测试
  ├── configs/
  │   ├── local/                            # bootstrap.yaml, audit.yaml, clickhouse.yaml, kafka.yaml, jwt_authn.yaml, openfga.yaml
  │   └── docker/
  └── internal/
      ├── assets/openapi.yaml
      ├── biz/
      │   ├── biz.go                        # ProviderSet
      │   ├── event.go                      # AuditEvent 领域模型、CloudEvents 校验投影、查询/入库 Usecase
      │   ├── page_token.go                 # 绑定查询过滤指纹的不透明游标编解码
      │   └── errors.go                     # 领域错误构造与映射
      ├── data/
      │   ├── data.go                       # ProviderSet + ClickHouse 连接生命周期管理
      │   ├── schema.go                     # audit_events 幂等建表与 TTL 迁移
      │   └── audit_event.go                # ClickHouse BatchInsert、GetByEventID、List 参数化查询
      ├── server/
      │   ├── server.go                     # ProviderSet
      │   ├── grpc.go                       # ServiceClaims JWT AuthN + OpenFGA AuthZ + AuditEventService
      │   ├── http.go                       # 最小内部 HTTP Server（不暴露免鉴权查询）
      │   └── kafka.go                      # Kafka 入站传输 Server（实现 Kratos transport.Server）
      ├── service/
      │   ├── service.go                    # ProviderSet
      │   └── event.go                      # Proto <-> biz 转换与 AuditEventService gRPC 实现
      └── startup/
          └── initializer.go                # BeforeStart 幂等执行 ClickHouse 表初始化
  ```

---

## 2. Proto 契约设计

### 2.1 服务配置契约（`audit/conf/v1/config.proto`）

采用 `servora.conf.v1` 标准 `Apply()` 默认值与 `buf.validate` 约束（替代旧实现手写默认值）：

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

严格遵循 AIP 资源模型与平台安全注解，**仅暴露 `GetAuditEvent` 与 `ListAuditEvents` 两个只读 RPC**：

```protobuf
syntax = "proto3";

package audit.event.v1;

import "buf/validate/validate.proto";
import "google/api/client.proto";
import "google/api/field_behavior.proto";
import "google/api/resource.proto";
import "google/protobuf/timestamp.proto";
import "plateau/security/authn/v1/annotations.proto";
import "plateau/security/audit/v1/annotations.proto";
import "plateau/security/authz/v1/annotations.proto";
import "servora/errors/v1/errors.proto";

option go_package = "github.com/Servora-Kit/plateau/api/gen/go/audit/event/v1;auditeventpb";

service AuditEventService {
  option (plateau.security.authn.v1.service_default) = {mode: AUTHN_MODE_REQUIRED};
  option (plateau.security.authz.v1.service_default) = {
    mode: AUTHZ_MODE_REQUIRED
    action: "read_events"
    resource_type: "audit"
    resource_id: "global"
  };
  // 查询审计自身不重复产生平台审计事件，防递归放大。
  option (plateau.security.audit.v1.service_default) = {mode: AUDIT_MODE_DISABLED};

  rpc GetAuditEvent(GetAuditEventRequest) returns (AuditEvent) {
    option (google.api.method_signature) = "name";
  }

  rpc ListAuditEvents(ListAuditEventsRequest) returns (ListAuditEventsResponse) {
    option (google.api.method_signature) = "page_size,page_token";
  }
}

enum AuditErrorReason {
  option (servora.errors.v1.default_code) = 500;

  AUDIT_ERROR_REASON_UNSPECIFIED = 0;
  AUDIT_ERROR_REASON_NOT_FOUND = 1 [(servora.errors.v1.code) = 404];
  AUDIT_ERROR_REASON_INVALID_ARGUMENT = 2 [(servora.errors.v1.code) = 400];
  AUDIT_ERROR_REASON_INVALID_PAGE_TOKEN = 3 [(servora.errors.v1.code) = 400];
  AUDIT_ERROR_REASON_STORAGE_UNAVAILABLE = 4 [(servora.errors.v1.code) = 503];
}

message AuditEvent {
  option (google.api.resource) = {
    type: "audit.plateau.dev/AuditEvent"
    pattern: "auditEvents/{event}"
    singular: "auditEvent"
    plural: "auditEvents"
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

  // 规范化的 JSON 对象字符串（空载荷统一为 "{}"），避免深层动态嵌套对 Proto/TS 造成的损耗。
  string detail_json = 19 [(google.api.field_behavior) = OUTPUT_ONLY];
}
```

`ListAuditEventsRequest` 提供类型明确的过滤字段（避免在 ClickHouse 上手写易出错的任意表达式解析器）：
- `google.protobuf.Timestamp start_time = 1` / `end_time = 2`（若均提供且 `start_time > end_time` 返回 `AUDIT_ERROR_REASON_INVALID_ARGUMENT`）；
- `plateau.security.audit.v1.AuditCategory category = 3`（非 `UNSPECIFIED` 时精确匹配）；
- `string service = 4`、`string event_type = 5`、`string operation = 6`；
- `string actor_type = 7`、`string actor_id = 8`；
- `string resource_type = 9`、`string resource_name = 10`；
- `optional bool success = 11`（使用 `optional` 严格区分“不限成败”、“仅成功 `true`”与“仅失败 `false`”）；
- `string trace_id = 12`（供详情抽屉一键拉取同一调用链上下游关联事件）；
- `int32 page_size = 13`（默认 `50`，最大钳制或校验 `200`）、`string page_token = 14`。

---

## 3. ClickHouse 存储模型与去重策略（`plateau_audit`）

1. **数据库重命名**：更新 `docker-compose.yaml` 与本地/容器 `clickhouse.yaml`，将历史残留 `servora_audit` / `servora` 统一收敛为 `plateau_audit` / `plateau`。
2. **表引擎与主键设计**：
   ```sql
   CREATE TABLE IF NOT EXISTS audit_events (
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
       error_message  String,
   
       trace_id       String,
       request_id     String,
       client_ip      String,
       user_agent     String,
   
       detail_json    String,
   
       INDEX idx_event_id event_id TYPE bloom_filter(0.01) GRANULARITY 4,
       INDEX idx_trace_id trace_id TYPE bloom_filter(0.01) GRANULARITY 4,
       INDEX idx_actor_id actor_id TYPE bloom_filter(0.01) GRANULARITY 4,
       INDEX idx_resource_name resource_name TYPE bloom_filter(0.01) GRANULARITY 4
   ) ENGINE = ReplacingMergeTree(ingested_at)
   PARTITION BY toYYYYMM(occurred_at)
   ORDER BY (occurred_at, event_id)
   TTL toDateTime(occurred_at) + INTERVAL 180 DAY
   SETTINGS index_granularity = 8192;
   ```
   - **为什么排序键定为 `(occurred_at, event_id)`**：
     - `ReplacingMergeTree` 仅对**排序键完全相同**的行执行去重；同一条 CloudEvent 在 Kafka at-least-once 重试时，其 Producer 产生的 `occurred_at` 与 `event_id` 恒定不变，天然命中同一排序键；
     - 管理台列表按 `ORDER BY occurred_at DESC, event_id DESC` 倒序浏览，与物理主键顺序完全逆序对齐，ClickHouse 可直接利用主键索引裁剪时间窗口并避免全量重排；
     - 按月分区 `toYYYYMM(occurred_at)`（比按天分区减少 30 倍分区 part 数量，180 天仅维护约 6–7 个活跃分区），同时在 `event_id`、`trace_id`、`actor_id`、`resource_name` 上建立 `bloom_filter` 跳数索引，保障 `GetAuditEvent` 单点查询与 `trace_id` 关联查询的毫秒级响应；
     - 查询语句附带 `FINAL SETTINGS do_not_merge_across_partitions_select_final = 1`，确保后台异步 Merge 尚未触发前读取结果也严格无重复行。

---

## 4. 绑定过滤指纹的 `page_token` 契约

遵循 `.trellis/spec/audit/backend.md` 要求，杜绝旧实现明文 JSON 游标被跨条件混用：
- 复用 `servora/core/crud.NewUnsignedPageTokenCodec()`（确定性 Proto 二进制 + Base64URL 编码）或专用二进制载荷，内部包含：
  1. `version = 1`；
  2. `filter_sha256`：对除 `page_size` 与 `page_token` 外的全部规范化过滤条件计算确定性 SHA-256 指纹；
  3. 游标锚点：`last_occurred_unix_micros`（int64）+ `last_event_id`（string）。
- `ListAuditEvents` 收到非空 `page_token` 时：解码失败、版本不匹配或 `filter_sha256` 与当前请求不一致，立即返回 `AUDIT_ERROR_REASON_INVALID_PAGE_TOKEN`（HTTP 400 / gRPC `InvalidArgument`）；校验通过时注入参数化谓词 `(occurred_at < ? OR (occurred_at = ? AND event_id < ?))`，多取 `page_size + 1` 行判断是否存在下一页。

---

## 5. Kafka 入站薄装配与四层职责分工（`internal/server/kafka.go`）

依托子任务 1 提供的非泛型 `plateau/transport/server/kafka`（`*svrkafka.Server`），`app/audit/service/internal/server/kafka.go` 保持与 `http.go`、`grpc.go` 完全对称的 ~25 行薄装配形态，交由 `rt.NewApp(kratos.Server(gs, hs, ks))` 统一托管生命周期。

| 层与文件 | 职责内容 | 严禁越层行为 |
| --- | --- | --- |
| `transport/server/kafka`（平台共享） | 非泛型 `*svrkafka.Server`：强制 `DisableAutoCommit`、`PollFetches` 拉取、按 `batch_size` / `flush_interval` 攒批、失败退避、仅在 `BatchHandler` 返回 `nil` 时执行 `CommitRecords`、优雅 `Stop` 尾批刷盘 | 零 CloudEvents 依赖、零审计领域耦合 |
| `internal/server/kafka.go` | **本服务协议薄装配**：注册 `auditCfg.GetTopic()` 的 `BatchHandler`，遍历 `[]*kgo.Record` 调用 `auditkafka.DecodeRecord` 解码为 `[]cloudevents.Event`，转发给 `eventSvc.IngestEvents` | 不将 `*kgo.Record` 或 Kafka 位点概念下传至 `service`/`biz`/`data`；不拼装 SQL |
| `internal/service/event.go` | **入站适配**：除实现 `AuditEventService` gRPC 查询外，提供内部入站方法将 `[]cloudevents.Event` 转发给 `biz.AuditEventUsecase.IngestEvents` | 不持有数据库或 Kafka 客户端 |
| `internal/biz/event.go` | **领域校验与投影**：校验 CloudEvents 必填属性（`id`、`type`、`source`、`time`），投影生成领域实体 `biz.AuditEvent`（缺失必填属性的坏事件在此记诊断日志并剔除，不阻塞同批合法事件），调用 `AuditEventRepo.SaveBatch` | 不感知 Kafka 分区/位点，不 import ClickHouse 驱动 |
| `internal/data/audit_event.go` | **ClickHouse 持久化**：仅实现 `biz.AuditEventRepo`，执行 `PrepareBatch` / `Send` 与参数化 `SELECT ... FINAL` | 不做 CloudEvents 解码或坏消息丢弃裁决 |

- **累积位点安全与毒丸隔离不变量**：
  - Kafka 同分区 offset 为累积水位线：批内若夹杂协议帧损坏记录（`auditkafka.DecodeRecord` 失败），`internal/server/kafka.go` 记录含 `topic/partition/offset` 的错误日志并将其从待写切片剔除，**绝不在同批前序合法记录写入 ClickHouse 前提前单独提交该坏记录位点**；
  - 若批内存在合法事件，待 `eventSvc.IngestEvents` 写入 ClickHouse 返回 `nil` 后，由 `svrkafka.Server` 一次性提交整批（含已剔除坏帧与已持久化合法事件）的位点；若整批全为坏帧（过滤后切片为空），直接返回 `nil` 推进位点防死锁；若 ClickHouse 写入报错，返回 `err` 保留整批位点供退避重试。
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
   - 新增 `manifests/openfga/audit.fga.yaml` 模型单测：验证 `service:admin` 被授予 `read_events` on `audit:global` 时允许访问，未授权 `service:other` 及任意 `user:*` 均被拒绝。
2. **JWT 认证配置（`configs/*/jwt_authn.yaml`）**：
   - `audit` gRPC 服务装配 `jwt.ServerFor[jwt.ServiceClaims](jwtAuthn, security.WithRulesFuncs(auditeventpb.AuthnRules))` 与 `openfga.Server(authorizer, security.WithRulesFuncs(auditeventpb.AuthzRules))`。
   - `audience` 固定设为 `"audit"`（与 IAM 的 `"iam"` 严格隔离）。
   - **源码约束对齐（见 `security/jwt/jwks.go:24-31`）**：
     - `configs/local/jwt_authn.yaml`：本地回环地址允许 HTTP JWKS，配置 `issuer: "${IAM_PUBLIC_ORIGIN:http://localhost:10002}"` 与 `jwks.uri: "http://127.0.0.1:10000/keys"`（启动时需 IAM 先就绪）；
     - `configs/docker/jwt_authn.yaml`：容器网络内主机名为 `iam`（非 loopback，`NewJWKS` 强制要求 HTTPS），因此 Docker 开发编排采用挂载只读签名公钥或 `verification_keys` 配置验签（生产环境走内部/外部 HTTPS JWKS），避免容器 HTTP 主机名被 `NewJWKS` 拒绝。
