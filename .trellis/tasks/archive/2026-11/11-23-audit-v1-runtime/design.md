# 底座迁移与共享审计运行时设计

需求来源：[本任务 PRD](prd.md) 与 [父任务 PRD](../11-23-audit-v1/prd.md)。

## 1. 模块职责与跨仓迁移映射

本子任务在代码层兑现「母框架 Servora 保持领域中立、Plateau 承载安全生态与专属流式基础设施」的划分：

| 原 Servora 路径 | 处置动作 | Plateau 新路径 / 归属 | 说明 |
| --- | --- | --- | --- |
| `api/protos/servora/contrib/kafka/v1/config.proto` | 迁出并从 Servora 删除 | `api/protos/plateau/infra/kafka/v1/config.proto` | package `plateau.infra.kafka.v1`；复用 `servora.conf.v1` 与 `servora.security.tls.v1.TLS` |
| `contrib/kafka/` | 迁出并从 Servora 删除 | `infra/kafka/` + `transport/server/kafka/` | `infra/kafka` 对齐 `infra/clickhouse` 专注客户端连接构造；新增与 `servora/transport/server/{http,grpc}` 同构的非泛型 `transport/server/kafka` 承载通用消费者生命周期；`servora/go.mod` 移除 `franz-go` 与 `kotel` |
| `api/protos/servora/audit/v1/annotations.proto` | 迁出并从 Servora 删除 | `api/protos/plateau/security/audit/v1/annotations.proto` | package `plateau.security.audit.v1`；保留 extension `50100` (`rule`) / `50101` (`service_default`) |
| `api/protos/servora/obs/audit/v1/config.proto` | 迁出并从 Servora 删除 | `api/protos/plateau/security/audit/v1/config.proto` | 配置消息命名为 `AuditEmitter`（section `audit_emitter`，与 `audit` 服务的 `Audit`/`audit:` 明确区分，对称于 `OIDC` vs `OIDCClient`） |
| `api/protos/servora/cloudevents/v1/cloudevents.proto` | 直接从 Servora 删除 | 不迁入 | 两仓零运行时调用；移除 `servora/go.mod` 的 `cloudevents/sdk-go/v2` |
| `cmd/protoc-gen-servora-audit/` + `cmd/internal/{ruleplan,optionmerge}` | 迁出并从 Servora 删除 | `cmd/protoc-gen-plateau-audit/` | 复用 Plateau 既有 `internal/codegen/{ruleplan,optionmerge,plugintest}`；`plugintest` 留在 Servora 仅供 `protoc-gen-servora-conf` 测试使用（移除其中对 `audit` 注解的断言） |
| `obs/audit/`（含各旧子包） | 迁出并从 Servora 删除 | `security/audit/` + `security/audit/kafka/` | 消除 `noop`/`stdout`/`log`/`multi` 碎片化子包与测试私有接口：标准库核心与内置发射器归 `security/audit/`（零 Broker 驱动依赖），Kafka 后端与协议编解码独立归 `security/audit/kafka/`（对齐 `authn/*`、`authz/*`） |

## 2. Proto 契约设计

### 2.1 `plateau.infra.kafka.v1`（`api/protos/plateau/infra/kafka/v1/config.proto`）

保持配置语义稳定，仅切换命名空间至 `plateau.infra.kafka.v1`（`go_package = ".../plateau/infra/kafka/v1;kafkapb"`）：
- `option (servora.conf.v1.section) = true;`（对应配置文件 `kafka:` 段）；
- `brokers`、`client_id`、`consumer_group`、`required_acks`、`retry_max`（默认 `3`）、`retry_backoff`、`dial_timeout`、`read_timeout`、`write_timeout`、`compression`（默认 `"none"`）、`tls`（`servora.security.tls.v1.TLS`）、`sasl`。

### 2.2 `plateau.security.audit.v1`

1. **`annotations.proto`**：
   - `AuditMode`：`AUDIT_MODE_UNSPECIFIED = 0`、`AUDIT_MODE_DISABLED = 1`、`AUDIT_MODE_ENABLED = 2`。
   - `AuditCategory`（可选轻量分类枚举，便于查询侧直接按控制面视图归类而不靠方法名硬猜）：
     - `AUDIT_CATEGORY_UNSPECIFIED = 0`（默认归为 `RESOURCE_MUTATION` 或由显式事件指定）
     - `AUDIT_CATEGORY_AUTHN = 1`（认证与会话生命周期）
     - `AUDIT_CATEGORY_RESOURCE = 2`（平台资源与配置变更）
     - `AUDIT_CATEGORY_AUTHZ = 3`（授权关系变更与访问拒绝）
   - `AuditRule`：
     ```protobuf
     message AuditRule {
       AuditMode mode = 1;
       AuditCategory category = 2;
     }
     ```
     保持轻量规则表定位：绝大多数服务只需在 `service` 级声明一次 `service_default = {mode: AUDIT_MODE_ENABLED, category: AUDIT_CATEGORY_RESOURCE}`，并在只读 `Get*` / `List*` 方法上标记 `rule = {mode: AUDIT_MODE_DISABLED}`（或仅在写方法开启），无需手填资源字段。

2. **`config.proto`**：
   - 消息 `AuditEmitter`（`option (servora.conf.v1.section) = true`，对应 YAML `audit_emitter:` 段——参照 `iam` 的 `OIDC` vs `security/oidc` 的 `OIDCClient` 对称命名，将 `audit:` 段留给 `app/audit/service` 自有服务配置 `message Audit`）：
     - `bool enabled = 1;`
     - `optional string backend = 2 [(servora.conf.v1.field) = {default: "noop"}];`（支持 `noop`、`stdout`、`log`、`kafka`）
     - `optional string topic = 3 [(servora.conf.v1.field) = {default: "plateau.audit.events"}];`
   - **平台审计与业务自有审计的隔离机制**：平台服务（IAM / Admin）使用默认或显式配置的 `plateau.audit.events` 投向 `app/audit`；未来业务微服务复用同一 `security/audit` 包时，只需将自身 `audit_emitter.topic` 指向业务自有 Topic，即可天然与平台控制面审计物理隔离。

### 2.3 `transport/server/kafka`（非泛型协议传输 Server）

与 `servora/transport/server/{http,grpc}` 保持严格同构，**不将 `Server` 设计为泛型类型**：

1. **为什么保持非泛型 `*kafka.Server`**：
   - **协议传输层面向协议原生帧**：正如 `http.Server` 处理 HTTP 请求、`grpc.Server` 处理 gRPC 调用，Kafka 传输层操作的是协议原生帧 `*kgo.Record`（未来若扩展 `sse`、`ws`、`nats` 等其他入站协议，同样保持各自非泛型 `*Server` 具体类型，不引入跨协议泛型传染）。
   - **单实例支持多 Topic 与异构消息类型**：通过 `WithTopicHandler(topic string, handler BatchHandler)`（其中 `type BatchHandler func(ctx context.Context, records []*kgo.Record) error`）注册消费处理器，同一个 `*kafka.Server` 实例可同时订阅多个不同载荷格式的 Topic，且对 `google/wire` 依赖注入与 `kratos.Server(...)` 零额外类型噪音。
2. **核心接口与消费位点安全不变量**：
   - `NewServer(cfg *kafkapb.Kafka, opts ...ServerOption) (*Server, error)`：内部调用 `infrakafka.BuildOpts` 并强制追加 `kgo.ConsumerGroup(...)`、`kgo.ConsumeTopics(...)` 与 **`kgo.DisableAutoCommit()`**，从构造入口杜绝调用方遗漏禁用自动提交导致消息未落库即丢位点；内部通过未导出 `consumerClient` 接口隔离 `PollFetches` / `CommitRecords` / `Close`，便于无外部 Broker 的确定性单测。
   - **累积位点原子提交（防前序缓冲丢失）**：Kafka 同分区位点是累积水位线（提交 offset `N` 即隐式确认 `<= N` 的全部记录）。`Server` 按 Topic 聚合达到 `batch_size` 或 `flush_interval` 后调用 `BatchHandler(ctx, records)`；**当且仅当 `BatchHandler` 返回 `nil` 时**，才对该批记录统一执行 `CommitRecords`。绝不在前序有效记录尚未落库时提前单独提交批内某条记录的位点；`BatchHandler` 返回非 `nil` 错误时记录日志并退避重试，保留整批未提交位点。
   - **优雅停机**：`Stop(stopCtx)` 停止拉取新记录，使用独立超时保护的上下文将内存缓冲尾批交由 `BatchHandler` 刷盘并完成最后一次 `CommitRecords`，随后关闭底层客户端。

## 3. CloudEvents 标准信封与扩展属性契约

这是连接生产端（`security/audit`）与消费端（`app/audit/service`）的核心协议契约。所有自动与显式审计事件统一遵循 CloudEvents v1.0（扩展属性名按 CE 规范仅使用小写字母数字 `a-z0-9`）：

| 位置 | 属性名 | 来源与填充规则 |
| --- | --- | --- |
| CE 标准属性 | `specversion` | `"1.0"` |
| CE 标准属性 | `id` | UUIDv7（时间有序且全局唯一，供 ClickHouse 按主键有序写入与幂等去重） |
| CE 标准属性 | `time` | Producer 事件产生时刻（UTC `time.Now().UTC()`） |
| CE 标准属性 | `source` | `"//" + appName`（从 Kratos `AppInfo` 提取，如 `//iam.service`、`//admin.service`） |
| CE 标准属性 | `type` | RPC 中间件自动事件默认 `"plateau.audit.rpc.v1"`；业务显式事件使用领域事件名（如 `"plateau.iam.authn.login.v1"`、`"plateau.security.authz.denied.v1"`） |
| CE 标准属性 | `subject` | RPC `operation`（如 `"/admin.user.v1.UserService/DisableUser"`）或显式动作标识 |
| CE 扩展属性 | `category` | `"authn"` / `"resource"` / `"authz"` |
| CE 扩展属性 | `actortype` | 从 `security.ActorFrom(ctx)` 提取：`"human"` / `"service"` / `"anonymous"`（未注入 Actor 时兜底 `"anonymous"`） |
| CE 扩展属性 | `actorid` | `actor.ID`（`anonymous` 时为空或留空；免鉴权登录失败等场景的目标标识放入 `resourcename` 或 `data`，绝不伪造已认证 `actorid`） |
| CE 扩展属性 | `resourcetype` | 从请求/响应消息的 `google.api.resource` / `google.api.resource_reference` 提取（如 `"iam.plateau.dev/User"`），或由显式 `EventOption` 指定 |
| CE 扩展属性 | `resourcename` | 从标注了 `IDENTIFIER` 或 `resource_reference` 的字段自动提取规范资源名（如 `"users/019..."`；`CreateUser` 请求无 ID 时自动回退从成功响应 `User.name` 提取！） |
| CE 扩展属性 | `outcome` | `"success"` 或 `"failure"` |
| CE 扩展属性 | `errorcode` | 失败时提取 Kratos `*errors.Error` 的 `Reason`（无结构化 Reason 时取 gRPC/HTTP Code 名称） |
| CE 扩展属性 | `errormessage` | 失败时提取对外安全错误摘要（不透出内部原始数据库/系统错误堆栈） |
| CE 扩展属性 | `traceparent` / `tracestate` | 从当前 OTel `SpanFromContext(ctx)` 提取（**注意**：只要 `sc.IsValid()` 即注入 `traceparent` 以保证同一调用链 `trace_id` 100% 可关联，不再局限于 `IsSampled()`，避免采样率 < 1.0 时丢失跨服务审计串联！） |
| CE 扩展属性 | `requestid` | 从入站 Header / Metadata（`X-Request-Id`）提取 |
| CE 扩展属性 | `clientip` / `useragent` | 从 HTTP Request 或 gRPC Peer/Metadata 提取调用端地址与 User-Agent |
| CE `data` | JSON / Protobuf | 经 `servora.redact.v3` 脱敏克隆后的请求关键参数摘要或业务显式挂载的强类型 Proto / JSON 载荷 |

> **关键改进点**：
> 1. **修复 `IsSampled()` 陷阱**：原 `servora/obs/audit/event_helpers.go:59` 仅在 `sc.IsSampled()` 为真时才写 `traceparent`；生产环境一旦 OTel Trace 采样率调低（如 10%），90% 的跨服务调用将丢失 `trace_id` 关联。迁入 `security/audit` 后改为只要 `sc.TraceID().IsValid()` 即写入 `trace_id` / `traceparent`。
> 2. **解决 `Create*` 新建资源无请求 ID 问题**：中间件在 `handler(ctx, req)` 返回后，优先从请求消息提取 `resource_reference` 字段；若请求未携带（如 `CreateUserRequest`）且响应成功返回资源实体（带 `google.api.resource` + `IDENTIFIER` 字段 `name`，或嵌套子字段 `user.name`），自动从响应消息补齐 `resourcetype` 与 `resourcename`。
> 3. **自动脱敏防泄漏**：序列化请求摘要进 `data` 前，若消息实现 `redact` 接口，先克隆并执行脱敏，确保 `password` / `new_password` 等字段永不进入 Kafka。

## 4. 中间件装配位置与执行顺序

在 Kratos 服务端中间件链中（对比 `app/admin/service/internal/server/http.go:45-53` 与 `app/iam/service/internal/server/grpc.go:23-30`）：

```text
Recovery → Tracing → Logging → RateLimit → Validate → Metrics
  → AuthN (session.Server / jwt.ServerFor)
  → Audit (audit.Server)          <-- 位于 AuthN 之后、AuthZ 之前！
  → AuthZ (openfga.Server)
  → Handler
```

**为什么 `audit.Server` 放在 `AuthN` 之后、`AuthZ` 之前，以及两个高杠杆设计点**：
1. **自动捕获全接口 403 授权拒绝（含只读接口）**：
   - 位于 `AuthN` 之后，请求认证通过时 `ctx` 已携带真实 `security.Actor`（`human` 或 `service`）；
   - 位于 `AuthZ`（`openfga.Server`）之前，中间件始终先执行 `reply, err := handler(ctx, req)`：当 `err` 为 `SECURITY_ERROR_REASON_PERMISSION_DENIED`（见 `security/authz/openfga/middleware.go:74`）时，**即使该方法的 `AuditRule.mode` 为 `AUDIT_MODE_DISABLED`（如 `ListUsers`、`ListAuditEvents`）**，也自动发出一条 `category = "authz"`、`type = "plateau.security.authz.denied.v1"`、`outcome = "failure"`、`errorcode = "SECURITY_ERROR_REASON_PERMISSION_DENIED"` 的审计事件；而常规 `200 OK` 只读请求在 `AUDIT_MODE_DISABLED` 下直接零分配返回。
2. **请求级上下文富化辅助（`audit.SetActor` / `audit.SetResource`）**：
   - 对于 `AUTHN_MODE_PUBLIC` 入口但在业务执行中完成身份核验的 RPC（如 `Login`、`ChangeInitialPassword`、`VerifyEmail`、`ConfirmPasswordReset`），`audit.Server` 在进入 `handler` 前向 `ctx` 注入轻量请求元数据槽位；业务层核验成功后只需一行 `audit.SetActor(ctx, actor)` 或 `audit.SetResource(ctx, resType, resName)`，返回时即由中间件统一组装发出完整审计事件，无需在每个 Service 方法重复手写 CloudEvents 样板代码。
3. **统一泛型规则聚合入口（`security/rules.go`）**：
   - 将 `AuthN`、`AuthZ` 与 `Audit` 三处完全同构的生成规则表聚合逻辑提取至根包 `security/rules.go`（`RulesOption[R proto.Message]`、`WithRulesFuncs[R]`、`NewRules[R]`），删除 `security/authn/rules.go` 与 `security/authz/rules.go` 冗余包，服务端统一经 `security.WithRulesFuncs(...)` 装配。

## 5. 与旧 `app/audit/service` 的过渡衔接策略

子任务 1 实施期间，仓库内旧 `app/audit/service` 尚引用 `servora/obs/audit` 与 `servora/contrib/kafka`：
- 当子任务 1 删除 Servora 中的上述包并升级 Plateau `go.mod` 时，若旧 `app/audit/service` 不改动会导致根 `go test ./...` 编译报错；
- 处理方式：在子任务 1 中对 `app/audit/service` 仅做 import 路径平替（指向新迁入的 `plateau/infra/kafka` 与 `plateau/security/audit`），保证根仓库在子任务 1 完成后 `GOWORK=off` 全绿可独立提交；随后在子任务 2 中将 `app/audit/service` 整体推倒重写。
