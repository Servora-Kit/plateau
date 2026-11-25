# audit-v1：Audit 微服务与存储重建

## 目标

按现行 IAM / Admin 工程标准彻底推倒重建平台控制面审计服务 `app/audit/service`，落地去重命名的 `audit.event.v1` AIP 只读契约、ClickHouse `audit.events` 表（180 天默认 TTL + 按需动态同步）、Kafka 批量可靠消费，以及基于 Service JWT + OpenFGA 的内部服务间鉴权。

## 需求来源与范围

来源为 [audit-v1 主 PRD](../11-23-audit-v1/prd.md) §3、§4。涉及 `app/audit/service/**`、`manifests/openfga/{fga.mod,audit.fga,audit.fga.yaml}`、根 `AGENTS.md`、`app/AGENTS.md` 与 `.trellis/spec/audit/`。前置依赖 [底座迁移与共享审计运行时](../11-23-audit-v1-runtime/prd.md)。

| ID | 父需求 | 本任务交付 |
| --- | --- | --- |
| S1 | §4 | 重建 `app/audit/service/api/protos/audit/{event,conf}/v1/*.proto`：定义只读 `EventService`（`GetEvent`、`ListEvents`）、AIP 资源 `Event`（`events/{event}`）、严格精确匹配的多维过滤参数、绑定过滤指纹的不透明 `page_token`、`EventErrorReason` 与 AuthN/AuthZ/Audit 规则注解；删除空壳 OpenAPI 模板与旧 `Makefile`。 |
| S2 | §3 | 重建 ClickHouse 表定义与数据访问层：默认库名统一对齐 `audit`，按“一表一文件”在 `internal/data/schema/event.go` 维护带 `ch` 标签的 `EventRow` 与 19 列 `events` 表（`ReplacingMergeTree(ingested_at)`、按月分区 `toYYYYMM(occurred_at)`、`ZSTD(1)` 压缩、`GRANULARITY 1` 布隆跳数索引、默认 180 天 TTL 及配置变更时的按需 `MODIFY TTL`）；启动期强制校验 ClickHouse 与 Kafka 非空可达。 |
| S3 | §3 | 基于平台共享 `transport/server/kafka` 重建审计入站消费装配（`internal/server/kafka.go`）：兼容 structured JSON 与 binary CloudEvents，执行 `traceparent` 拆解、未知类别兜底与 `detail_json` 对象包封；批内坏消息记日志剔除且不提前单独提交位点，仅在批次入库成功后统一提交累积位点，停机时刷空尾批。 |
| S4 | §4 | 对齐 `app/iam/service` 标准分层与端口登记（`10020` 仅承载内部 `/healthz`、`/readyz`、`/metrics`，移除 Traefik 公开路由；`10021` gRPC）：gRPC 挂载 `jwt.ServerFor[jwt.ServiceClaims]` + `openfga.Server`，自身保持纯消费终点（移除 `audit_emitter`、不挂载 `audit.Server`）；Local 与 Docker 统一采用 `jwks.uri` 验签；新增 `manifests/openfga/audit.fga` 并通过模型校验与单测。 |

## 验收标准

- [x] AC-S1（S1）：旧 `audit.service.v1` / `audit.service.conf.v1`、空壳 OpenAPI 配置/产物与旧 `Makefile` 已彻底移除并替换为 `audit.event.v1` / `audit.conf.v1`；`just gen-check`、`just fmt-proto-check`、`just api-ts-check` 通过。
- [x] AC-S2（S2、S3）：消费端单测与 ClickHouse 集成测试覆盖 structured/binary 双模式解码、`traceparent`/`detail_json` 归一化、坏消息跳过不阻塞、写失败不提交位点、停机优雅 flush、相同 `event_id` 重复写入去重一致，以及 180 天默认 TTL 与配置变更 `MODIFY TTL` 生效。
- [x] AC-S3（S1、S2）：`GetEvent`（跳数索引 `LIMIT 1`）与 `ListEvents`（`FINAL` + 无隐式时间截断）覆盖时间区间、分类、服务、`event_type`、`operation`、`actor_type`、`actor_id`、目标资源、`success`、`trace_id` 精确过滤与稳定游标续页；修改任一过滤条件复用旧 `page_token` 时返回 `EVENT_ERROR_REASON_INVALID_PAGE_TOKEN`。
- [x] AC-S4（S4）：`just openfga-model-validate` 与 `just openfga-model-test` 通过；集成测试验证缺失凭据、人类令牌、错误 audience 及未授予 `audit:global` 读取权限的服务调用均被拒绝，获权服务调用正常返回。

## 依赖与交付协作

- 前置依赖：`11-23-audit-v1-runtime`。
- 向 `11-23-audit-v1-producers-bff` 提供稳定的 `audit.event.v1.EventService` gRPC 契约与 OpenFGA `audit.fga` 授权模型。
