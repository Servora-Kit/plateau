# Audit 微服务与存储重建实施计划

需求：[PRD](prd.md) · 技术契约：[设计](design.md)。

## 1. 变更范围声明

- **现状与目标差距**：当前 `app/audit/service` 仍保留历史原型（裸露无鉴权的 HTTP/Traefik 查询路由、旧 `audit.service.v1` / `audit.service.conf.v1` 契约、混在 `internal/data` 的手写消费协程、无过滤指纹的明文游标及已废弃字段的 `MergeTree` 表）。需按现行 IAM/Admin 标准推倒重建为单向消费入库（ClickHouse `audit.events`）+ 受 Service JWT 与 OpenFGA 保护的内部只读 gRPC 服务（`audit.event.v1.EventService`）。
- **改动文件清单**：
  - 删除：`app/audit/service/Makefile`、`app/audit/service/api/buf.openapi.gen.yaml`、`app/audit/service/internal/assets/openapi.yaml`、`app/audit/service/api/protos/audit/service/**`、`app/audit/service/configs/{local,docker}/{audit_consumer,broker}.yaml` 及旧 `internal/{biz,data,service}/audit*.go`、`internal/data/{batch_writer,clickhouse,consumer}*.go`；
  - 新增/重写：`app/audit/service/api/protos/audit/{conf,event}/v1/*.proto`、`manifests/openfga/{fga.mod,audit.fga,audit.fga.yaml}`、`app/audit/service/configs/{local,docker}/{bootstrap,audit,clickhouse,kafka,jwt_authn,openfga}.yaml`、`app/audit/service/cmd/server/{main,wire,wire_gen,config_test}.go`、`app/audit/service/internal/biz/{biz,event,page_token,errors,event_test}.go`、`app/audit/service/internal/data/schema/event.go`、`app/audit/service/internal/data/{data,event,event_test}.go`、`app/audit/service/internal/service/{service,event,event_test}.go`、`app/audit/service/internal/server/{server,grpc,http,kafka,grpc_test,kafka_test}.go`；
  - 规范与目录文档同步：根 `AGENTS.md`（端口段登记）、`app/AGENTS.md`、`.trellis/spec/audit/backend.md`。
- **明确不做事项**：本任务不改动 `app/iam/service`、`app/admin/service` 生产代码与 `app/admin/web` 前端页面（归子任务 3、4）。

## 2. 执行步骤

- [x] **Step 1：Proto 契约与 OpenFGA 授权模型（S1、S4）**
  - 移除旧 `app/audit/service/api/protos/audit/service/**`、`api/buf.openapi.gen.yaml`、`internal/assets/openapi.yaml` 与 `Makefile`；
  - 编写 `app/audit/service/api/protos/audit/conf/v1/config.proto`（`audit.conf.v1.Audit`）与 `app/audit/service/api/protos/audit/event/v1/event.proto`（`audit.event.v1.EventService`：`GetEvent`、`ListEvents`、`Event`、`EventErrorReason`）；
  - 新增 `manifests/openfga/audit.fga` 与 `manifests/openfga/audit.fga.yaml`，在 `manifests/openfga/fga.mod` 注册模块；
  - 运行 `GOWORK=off just api-go && GOWORK=off just api-ts` 生成新契约并自动清理 `api/gen/{go,ts}/audit/service/**`，执行 `just openfga-model-validate` 与 `just openfga-model-test`。

- [x] **Step 2：领域层与过滤指纹游标实现（`internal/biz`，S1、S3）**
  - 实现 `internal/biz/errors.go`（`ErrEventNotFound`、`ErrInvalidArgument`、`ErrInvalidPageToken`、`ErrStorageUnavailable`）；
  - 实现 `internal/biz/page_token.go`（基于 `servora/core/crud.NewUnsignedPageTokenCodec()` 编码 SHA-256 过滤指纹 + `(occurred_unix_micros, event_id)` 游标，篡改任一过滤条件立即拒绝）；
  - 实现 `internal/biz/event.go`（直接复用生成领域资源 `*auditeventpb.Event`、CloudEvents 必填字段校验、`traceparent` 32 位 `trace_id` 提取、`category` 兜底、`detail_json` JSON 对象归一化、批内坏事件剔除及查询参数校验）。

- [x] **Step 3：ClickHouse Schema 与数据层实现（`internal/data`，S2）**
  - 实现 `internal/data/schema/event.go`（`TableEvents = "events"`、带 `ch:"..."` 标签的 `EventRow`、19 列 `ReplacingMergeTree(ingested_at)` 建表 DDL 及比对 `system.tables.engine_full` 按需执行的 `MODIFY TTL`）；
  - 实现 `internal/data/data.go`（ClickHouse 与 Kafka 客户端强依赖非空校验、`NewData` 内执行 `schema.EnsureEventsTable` 及连接关闭清理）；
  - 实现 `internal/data/event.go`（`SaveBatch` 使用 `PrepareBatch` + `AppendStruct`；`GetByID` 使用 `WHERE event_id = ? ORDER BY ingested_at DESC LIMIT 1`；`List` 使用参数化条件 + `FINAL SETTINGS do_not_merge_across_partitions_select_final = 1` + `conn.Select`）。

- [x] **Step 4：接口适配、三传输 Server、配置与 Wire 装配（`internal/service`、`internal/server`、`cmd`、`configs`，S3、S4）**
  - 实现 `internal/service/event.go`（`events/{event}` 资源名校验、Proto 映射、`IngestEvents` 转发与 `eventError` 统一错误编码）；
  - 实现 `internal/server/{http,grpc,kafka,server}.go`（`10020` 最小运维 HTTP Server；`10021` gRPC 挂载 `jwt.ServerFor[jwt.ServiceClaims]` + `openfga.Server`；Kafka Server 注册批次解码与累积位点安全提交）；
  - 重建 `configs/{local,docker}/{bootstrap,audit,clickhouse,kafka,jwt_authn,openfga}.yaml` 与 `cmd/server/{main,wire}.go`，执行 `GOWORK=off just wire` 生成 `wire_gen.go`。

- [x] **Step 5：分层单测、真实存储/鉴权集成测试与全仓门禁（AC-S1 ~ AC-S4）**
  - 编写 `cmd/server/config_test.go`、`internal/biz/event_test.go`、`internal/data/event_test.go`（覆盖真实 ClickHouse 建表、TTL 动态变更、重复 `event_id` 幂等去重、多维过滤与游标续页/防篡改）、`internal/service/event_test.go`、`internal/server/{kafka_test,grpc_test}.go`（覆盖双模式解码、坏帧隔离、写失败保位点、停机 flush 及 JWT + OpenFGA 全拒绝/放行分支）；
  - 同步更新 `AGENTS.md`、`app/AGENTS.md` 与 `.trellis/spec/audit/backend.md`；
  - 在 `GOWORK=off` 下跑通：
    ```bash
    GOWORK=off just gen-check
    GOWORK=off just fmt-proto-check
    GOWORK=off just api-ts-check
    just openfga-model-validate
    just openfga-model-test
    GOWORK=off just lint
    GOWORK=off just test
    ```

## 3. 风险点与验证保障

- **ClickHouse `FINAL` 与时间精度一致性**：游标锚点使用 `DateTime64(6, 'UTC')` 微秒时间戳 `UnixMicro()` 与 `time.UnixMicro(us).UTC()` 往返无损对齐，避免毫秒截断造成翻页漏行或重行。
- **Kafka 批内坏记录位点安全**：`server/kafka.go` 与 `biz/event.go` 剔除坏记录时绝不单独提前提交位点，统一交由 `svrkafka.Server` 在同批合法事件写入 ClickHouse 成功后一次性提交。
