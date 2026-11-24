# audit-v1：Audit 微服务与存储重建

## 目标

按现行 IAM / Admin 工程标准彻底推倒重建平台控制面审计服务 `app/audit/service`，落地全新 AIP 查询契约、ClickHouse `plateau_audit` 去重存储与 180 天 TTL、Kafka 批量可靠消费，以及基于 Service JWT + OpenFGA 的内部服务间鉴权。

## 需求来源与范围

来源为 [audit-v1 主 PRD](../11-23-audit-v1/prd.md) §3、§4。涉及 `app/audit/service/**`、`manifests/openfga/{fga.mod,audit.fga,audit.fga.yaml}`、`docker-compose.yaml`、根 `AGENTS.md` 端口表与 `.trellis/spec/audit/`。前置依赖 [底座迁移与共享审计运行时](../11-23-audit-v1-runtime/prd.md)。

| ID | 父需求 | 本任务交付 |
| --- | --- | --- |
| S1 | §4 | 重建 `app/audit/service/api/protos/audit/{event,conf}/v1/*.proto`：定义只读 `AuditEventService`（`GetAuditEvent`、`ListAuditEvents`）、AIP 资源名 `auditEvents/{event}`、多维过滤参数、绑定过滤指纹的不透明 `page_token`、领域错误枚举与 AuthN/AuthZ 规则注解。 |
| S2 | §3 | 重建 ClickHouse 表初始化与数据访问层：库名统一为 `plateau_audit`，采用支持 `event_id` 幂等去重的 `ReplacingMergeTree` 表引擎，按月分区（`toYYYYMM(occurred_at)`），默认 TTL 180 天（可由 `audit.conf.v1` 覆盖），所有查询强制参数绑定。 |
| S3 | §3 | 基于平台通用 `transport/server/kafka` 重建审计入站消费装配（`internal/server/kafka.go`）：兼容 structured JSON 与 binary CloudEvents，批内坏消息记录诊断日志并剔除、不阻塞同批合法事件，仅在批次入库成功后统一提交累积位点，停机时刷空尾批。 |
| S4 | §4 | 对齐 `app/iam/service` 标准分层（`server` / `service` / `biz` / `data`）与端口登记（`10020` HTTP、`10021` gRPC）：gRPC 挂载 `jwt.ServerFor[jwt.ServiceClaims]` + `openfga.Server`；新增 `manifests/openfga/audit.fga` 模块并通过模型校验与单测；移除公开 Traefik 路由。 |

## 验收标准

- [ ] AC-S1（S1）：旧 `audit.service.v1` / `audit.service.conf.v1` 已彻底移除并替换为 `audit.event.v1` / `audit.conf.v1`；`just gen-check`、`just fmt-proto-check`、`just api-ts-check` 通过。
- [ ] AC-S2（S2、S3）：消费端单测与 ClickHouse 集成测试覆盖 structured/binary 双模式解码、坏消息跳过不阻塞、写失败不提交位点、停机优雅 flush、相同 `event_id` 重复写入去重查询一致，以及 180 天默认 TTL 配置生效。
- [ ] AC-S3（S1、S2）：`GetAuditEvent` 与 `ListAuditEvents` 覆盖时间区间、分类、服务、`actor_type`、`actor_id`、目标资源、`success`、`trace_id` 过滤与稳定游标续页；修改任一过滤条件复用旧 `page_token` 时返回明确的无效游标错误。
- [ ] AC-S4（S4）：`just openfga-model-validate` 与 `just openfga-model-test` 通过；集成测试验证缺失凭据、人类令牌、错误 audience 及未授予 `audit:global` 读取权限的服务调用均被拒绝，获权服务调用正常返回。

## 依赖与交付协作

- 前置依赖：`11-23-audit-v1-runtime`。
- 向 `11-23-audit-v1-producers-bff` 提供稳定的 `audit.event.v1.AuditEventService` gRPC 契约与 OpenFGA 授权模型。
