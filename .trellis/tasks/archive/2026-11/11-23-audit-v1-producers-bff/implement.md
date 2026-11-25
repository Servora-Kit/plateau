# IAM 与 Admin 审计接入及查询编排实施计划

需求：[PRD](prd.md) · 技术契约：[设计](design.md)。

## 1. 变更范围声明

- **现状与目标差距**：`security/audit`、`infra/kafka` 与 `app/audit/service` 已在子任务 1、2 落地，但 `app/iam/service` 与 `app/admin/service` 尚未声明审计规则注解、尚未装配审计发送器与中间件，原生 OIDC HTTP 入口缺少无侵入审计包装，`admin.fga` 尚无 `read_audit` 权限，且 Admin 尚未提供 `admin.audit.v1.AuditService` 查询编排接口与 TS 客户端。
- **改动文件清单**：
  - 共享运行时轻量扩展：`infra/kafka/{kafka,kafka_test}.go`、`security/audit/{context,http,http_test}.go`、`security/audit/kafka/{kafka,kafka_test}.go`；
  - OpenFGA 与文档：`manifests/openfga/{admin.fga,admin.fga.yaml}`、`docs/iam-security.md`、`.trellis/spec/{plateau/infra/kafka.md,plateau/security/audit.md,admin/backend.md}`；
  - IAM 审计生产：`app/iam/service/api/protos/iam/{user,authn,account,session}/v1/*.proto`、`app/iam/service/internal/biz/{account,account_test}.go`、`app/iam/service/internal/service/{authn,account,session}.go`、`app/iam/service/internal/oidc/{provider,op_auth_storage,provider_test}.go`、`app/iam/service/internal/data/data.go`、`app/iam/service/internal/server/{server,http,grpc}.go`、`app/iam/service/cmd/server/{main,wire,wire_gen,config_test}.go`、`app/iam/service/configs/{local,docker}/{kafka,audit_emitter,oidc}.yaml`；
  - Admin 审计生产与查询 BFF：`app/admin/service/api/buf.openapi.gen.yaml`、`app/admin/service/api/protos/admin/{user,auth,audit}/v1/*.proto`、`app/admin/service/internal/assets/openapi.yaml`、`api/gen/{go,ts}/**`、`app/admin/service/internal/biz/{biz,auth,audit,audit_test}.go`、`app/admin/service/internal/data/{data,audit_event}.go`、`app/admin/service/internal/service/{service,auth,audit,audit_test}.go`、`app/admin/service/internal/server/{server,http,integration_test}.go`、`app/admin/service/cmd/server/{main,wire,wire_gen,config_test}.go`、`app/admin/service/configs/{local,docker}/{bootstrap,kafka,audit_emitter}.yaml`。
- **明确不做事项**：不改动任何现有业务 `.proto` 请求/响应消息字段，不在 `openfga.sh` 或根 `justfile` 硬编码特定服务元组，不触碰 `app/admin/web` 视图层页面（归子任务 4）。

## 2. 执行步骤

- [x] **Step 1：共享运行时 Wire 构造器与原生 HTTP 审计包装器（`infra/kafka`、`security/audit`）**
  - 在 `infra/kafka` 新增 `New(cfg *kafkapb.Kafka, l *slog.Logger) (*kgo.Client, func(), error)`（配置非空时同步 `Ping` 强校验）；
  - 在 `security/audit/kafka` 新增 `NewDefault(cfg *auditpb.AuditEmitter, client *kgo.Client, l *slog.Logger) (audit.Auditor, error)`；
  - 在 `security/audit` 扩展 `requestSlot` 被动富化辅助（`SetDetail`、`SetError`、`SetEventOverride`、`Skip`）与 `HTTPHandler` 原生 HTTP 审计包装器，补充单测验证。

- [x] **Step 2：Proto 契约、OpenFGA `read_audit` 权限与生成物更新（P1、P2、P3）**
  - 为 IAM `user.proto`、`authn.proto`、`account.proto`、`session.proto` 与 Admin `user.proto`、`auth.proto` 添加 `plateau.security.audit.v1` 规则注解；
  - 新增 `app/admin/service/api/protos/admin/audit/v1/audit.proto` 并在 `app/admin/service/api/buf.openapi.gen.yaml` 登记；
  - 更新 `manifests/openfga/{admin.fga,admin.fga.yaml}` 新增 `read_audit: admin`；
  - 执行 `GOWORK=off just gen`、`just openfga-model-validate`、`just openfga-model-test`，确认 `api/gen/go`、`api/gen/ts` 与 `app/admin/service/internal/assets/openapi.yaml` 生成干净。

- [x] **Step 3：IAM 控制面审计接入与 OAuth 受众扩展（`app/iam/service`，P1）**
  - 调整 `AccountUsecase.ConfirmPasswordReset` 返回 `(userID string, err error)`（`ChangeInitialPassword` 保持 `(*LoginSession, error)`），在 `service/{authn,account,session}.go` 补充 `SetActor` / `SetResource`；
  - 在 `oidc.RegisterHTTPServer` 为 `/authorize/callback`、`/end_session`、`/oauth/token`（含 `64 KiB` 请求体上限与字段长度截断）外包 `audit.HTTPHandler`，在 `provider.go` 与 `op_auth_storage.go` 通过被动 `ctx` 调用补充身份、授权客户端/Scopes、注销信息、`302 ?error=...` 失败判定及未登录 `302` 跳过（保持 `IAMProvider` 与 `OIDCStorage` 构造签名不变）；
  - 在 `server/{http,grpc}.go` 按 `AuthN → Audit → AuthZ` 挂载 `audit.Server`，接入 `infrakafka.New` + `auditkafka.NewDefault`，新增 `configs/{local,docker}/{kafka,audit_emitter}.yaml`，将 `oidc.yaml` 中 `admin` 客户端 `audiences` 扩为 `[iam, audit]` 并重新生成 `wire_gen.go`。

- [x] **Step 4：Admin 审计接入与 `admin.audit.v1` 查询 BFF 编排（`app/admin/service`，P2、P3）**
  - 在 `biz/auth.go` 新增 `PermReadAudit = "read_audit"` 纳入 `GetSessionResponse.permissions`；
  - 在 `service/auth.go` `Logout` 补充 `SetResource`，在 `RegisterOIDCHTTP` 为 `/v1/admin/auth/callback` 外包 `audit.HTTPHandler` 并在 `handleCallback` 覆盖全部 4 类退出口；
  - 实现 `data/audit_event.go`（复用 `data.dialer` 拨号 `audit.service`）、`biz/audit.go` 与 `service/audit.go`（零重复结构体透传 `*auditeventpb.Event`，`auditError` 将下游鉴权/连接/存储异常统一转换为 `503 ADMIN_AUDIT_ERROR_REASON_DOWNSTREAM_UNAVAILABLE`）；
  - 在 `server/http.go` 注册 `AuditServiceHTTPServer` 并挂载 `audit.Server`，更新 `configs/{local,docker}/{bootstrap,kafka,audit_emitter}.yaml` 并重新生成 `wire_gen.go`。

- [x] **Step 5：分层单测、跨服务真实端到端集成测试、规范同步与全仓门禁（AC-P1 ~ AC-P3）**
  - 补充 IAM / Admin 配置加载单测、OIDC 无侵入审计单测、`service/audit_test.go` 错误映射单测；
  - 在真实 PostgreSQL + Redis + OpenFGA + Kafka + ClickHouse 环境下扩展跨服务集成测试，串联 `IAM + Admin + Audit` 验证双跳同 `trace_id` 关联、敏感字段零泄漏、未登录/非管理员 401/403 拒绝及下游故障 `503` 会话保护；
  - 同步更新 `docs/iam-security.md` 与 `.trellis/spec/` 相关规范，在 `GOWORK=off` 下跑通 `just gen` 幂等检查、`just fmt-proto-check`、`just api-ts-check`、`just openfga-model-validate`、`just openfga-model-test`、`just lint`、`just test` 及 GitNexus 索引同步。

## 3. 风险点与验证保障

- **原生 OIDC 敏感参数防泄漏**：`/oauth/token` 的 `WithHTTPPostHook` 仅允许从 `r.BasicAuth()` 用户名与 `r.PostForm` 读取 `client_id`、`grant_type`，单测必须显式提交 `client_secret`、`code`、`refresh_token` 并断言落盘事件中零残留。
- **下游异常与管理员本地会话隔离**：单测与集成测试均须模拟移除 `service:admin read_events audit:global` 元组或关闭 `audit.service`，验证 `/v1/admin/audit/events` 返回 `503` 且紧随其后的 `/v1/admin/auth/session` 仍保持 `200 OK`。
