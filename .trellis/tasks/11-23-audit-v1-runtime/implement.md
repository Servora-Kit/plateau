# 底座迁移与共享审计运行时实施计划

需求：[PRD](prd.md)。技术契约：[设计](design.md)。

## 1. Servora 母框架净身与独立验证（`../servora`）

- [x] 删除 `../servora` 中的旧审计与 Kafka Proto：
  - `api/protos/servora/audit/v1/annotations.proto`
  - `api/protos/servora/obs/audit/v1/config.proto`
  - `api/protos/servora/cloudevents/v1/cloudevents.proto`
  - `api/protos/servora/contrib/kafka/v1/config.proto`
- [x] 移除 `../servora/api/protos/servora/example/v1/example.proto` 对 `servora/audit/v1/annotations.proto` 的导入与方法/服务注解。
- [x] 删除 `../servora` 中的旧生成器、运行时与客户端封装：
  - `cmd/protoc-gen-servora-audit/`
  - `cmd/internal/ruleplan/`、`cmd/internal/optionmerge/`
  - `obs/audit/`（含 `noop`、`stdout`、`log`、`kafka`、`multi`）
  - `contrib/kafka/`
- [x] 同步更新 `../servora` 构建、测试与仓库文档：
  - `buf.go.gen.yaml`、`Makefile`、`justfile`、`.gitignore` 移除 `protoc-gen-servora-audit`；
  - `cmd/internal/plugintest/` 移除对 `servora/audit/v1/annotations.proto` 的编译断言，仅保留 `protoc-gen-servora-conf` 所需测试辅助；
  - `core/bootstrap/scan_test.go` 将配置扫描单测中的 `audit_contract` 替换为仍在用的框架配置类型；
  - 更新 `../servora/README.md`、`AGENTS.md`、`api/protos/{AGENTS,README}.md`、`cmd/AGENTS.md`、`obs/AGENTS.md`、`contrib/AGENTS.md`。
- [x] 在 `../servora` 执行 `just gen-fresh`、`just gen-ts`、`just tidy`，确认 `go.mod` 已移除 `github.com/cloudevents/sdk-go/v2`、`github.com/twmb/franz-go`、`github.com/twmb/franz-go/plugin/kotel`，并在 `GOWORK=off` 下跑通全套门禁：
  ```bash
  GOWORK=off just gen-fresh
  GOWORK=off just gen-ts
  GOWORK=off just lint-proto
  GOWORK=off just test
  just web-typecheck
  GOWORK=off just tidy
  ```

## 2. Plateau 契约与代码生成器落地（`api/`、`cmd/`、`buf.go.gen.yaml`）

- [x] 新增 `api/protos/plateau/infra/kafka/v1/config.proto`（package `plateau.infra.kafka.v1`，`section = true`）。
- [x] 新增 `api/protos/plateau/security/audit/v1/annotations.proto`（`AuditMode`、`AuditCategory`、`AuditRule`，extension `50100` / `50101`）与 `api/protos/plateau/security/audit/v1/config.proto`（`AuditEmitter` 配置消息，`section = true` 对应 `audit_emitter:` 段，默认 `backend = "noop"`、`topic = "plateau.audit.events"`）。
- [x] 新增 `cmd/protoc-gen-plateau-audit/main.go` 与 `main_test.go`：
  - 对齐 `cmd/protoc-gen-plateau-authn` 结构，基于 `internal/codegen/ruleplan` 实现 `service_default` 与方法级 `rule` 合并校验；
  - 输出不可变 `_auditRules` 与带 `proto.Clone` 防御拷贝的 `AuditRules() map[string]*auditpb.AuditRule`。
- [x] 更新根 `go.mod` `tool` 声明、`justfile`（或插件构建入口）与 `buf.go.gen.yaml`：将 `protoc-gen-servora-audit` 替换为 `protoc-gen-plateau-audit`，生成 `api/gen/go` 与 `api/gen/ts`。

## 3. `infra/kafka`、`transport/server/kafka` 与 `security/audit` 运行时实现

- [x] 新增 `infra/kafka/kafka.go` 与 `kafka_test.go`：
  - 对齐 `infra/clickhouse` 构造风格：克隆输入配置并调用 `Apply()`，提供 `BuildOpts`、`NewClient`、`NewClientOptional`（未配置返回 `nil, nil`；配置非法或 Ping 失败返回 `nil, err`）与 `WithSlogLogger`；
  - 复用 `servora/security/tls` 构造客户端 TLS，支持 `PLAIN` / `SCRAM-SHA-256` / `SCRAM-SHA-512` SASL 与 `kotel` 追踪插件。
- [x] 新增非泛型 `transport/server/kafka/`（`server.go`、`options.go`、`server_test.go`）：
  - 实现 Kratos `transport.Server`（`Start` / `Stop`），通过 `WithTopicHandler(topic string, handler BatchHandler)` 注册多 Topic 批处理器；
  - 构造入口调用 `infrakafka.BuildOpts` 并强制注入 `kgo.ConsumerGroup`、`kgo.ConsumeTopics` 与 `kgo.DisableAutoCommit()`；
  - 支持 `batch_size` 与 `flush_interval` 双触发，仅在 `BatchHandler` 返回 `nil` 时统一执行 `CommitRecords`，失败保留未提交位点并退避重试；`Stop` 使用超时保护上下文刷空内存尾批并提交位点后关闭客户端。
- [x] 新增 `security/rules.go`（泛型规则表聚合）、核心运行时 `security/audit/` 与独立外部后端子包 `security/audit/kafka/`：
  - `security/audit/auditor.go`：`Auditor` 接口、`New(cfg, logger, opts...)` + `WithBackend` 外部后端注册点，以及标准库内置 `Noop`、`NewStdout`、`NewLog`、`Multi`（零 `franz-go` 依赖）；
  - `security/audit/kafka/kafka.go`：`Backend(client)` / `New(client, cfg)` Kafka 后端（直接借用 `*kgo.Client`，无测试专用私有接口，不越权 `Close` 调用方客户端）及 CloudEvents v1.0 Kafka Binary / Structured JSON 协议编解码（`EncodeRecord` / `DecodeRecord`）；
  - `event.go`：`NewEvent`（UUIDv7 `id`、UTC `time`、`//app-name` `source`、只要 `sc.IsValid()` 即注入 `traceparent`/`tracestate`、`security.ActorFrom(ctx)` 自动注入 `actortype`/`actorid`）、`WithHTTPRequest`、`SetJSONProtoData` 及基于 `kerrors.FromError` / Kratos `transport` 的元数据提取；
  - `context.go`：并发安全的请求级上下文富化辅助（`SetActor`、`SetResource`），供免鉴权入口完成身份核验后补充主体与资源目标；
  - `resource.go`：基于 `protoreflect` 自动从请求或成功响应消息提取 `google.api.resource` / `google.api.resource_reference`，并通过递归调用 `protoc-gen-redact` 生成的 `RedactFields()` 完成敏感字段克隆脱敏；
  - `middleware.go`：`Server(auditor, ...)`，在 `DISABLED` 只读路由上 200 OK 零分配直返，无条件拦截 403 `SECURITY_ERROR_REASON_PERMISSION_DENIED` 发出 `authz` 拒绝事件，发送失败仅记录 `slog.ErrorContext` 不阻断业务返回。
- [x] 对暂未重建的 `app/audit/service` 执行最小 import 与配置段平替（指向 `plateau/infra/kafka` 与 `plateau/security/audit`），保证根仓库编译与既有单测不中断（完整服务重建归子任务 2）。

## 4. 规范更新、跨仓版本升级与门禁检查

- [x] 同步更新 `.trellis/spec/`、`.github/workflows/ci.yml`（如有新增顶层目录 `transport/**` 过滤规则）及 `AGENTS.md`：
  - 移除 `.trellis/spec/servora/framework/audit.md`，更新 `.trellis/spec/servora/{framework/index.md,framework/providers.md,cmd/plugins.md,proto/annotations.md}`；
  - 新增 `.trellis/spec/plateau/infra/kafka.md`、`transport/server/kafka` 规范说明与 `.trellis/spec/plateau/security/audit.md`，并在各自 `index.md`、`.trellis/spec/api/proto/annotations.md`、`.trellis/spec/plateau/codegen/{cmd,index}.md`、根 `AGENTS.md` 注册。
- [x] 完成 `../servora` 新版本 `v0.11.0` 发布，升级 Plateau `go.mod` 中的 `github.com/Servora-Kit/servora` 至 `v0.11.0` 与 `buf.lock`，并在 `GOWORK=off` 下跑通完整门禁：
  ```bash
  GOWORK=off go test -race -count=1 ./cmd/protoc-gen-plateau-audit/... ./infra/kafka/... ./transport/server/kafka/... ./security/audit/...
  GOWORK=off just gen-check
  GOWORK=off just fmt-proto-check
  GOWORK=off just api-ts-check
  GOWORK=off just lint
  GOWORK=off just test
  ```

## 5. 风险点与回退策略

- **跨仓版本衔接风险**：Plateau 升级 `servora` 前须确保 `../servora` 已通过 `GOWORK=off` 测试并生成明确版本引用；切勿仅凭父目录 `go.work` 联调通过便视为完成。
- **回退点**：子任务 1 不涉及数据库持久化表变更或线上协议切换；若发现规则合并或中间件提取缺陷，可直接修正生成器与运行时并重跑 `just gen`。
