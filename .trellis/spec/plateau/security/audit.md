# 平台审计运行时

适用于 `security/audit`、`security/audit/kafka`、审计 Proto 契约与服务端中间件装配。

## Scenario: 控制面 RPC 自动审计、鉴权拒绝捕获与显式富化

### 1. Scope / Trigger

- Trigger：为平台微服务声明审计规则注解、挂载 `audit.Server` 中间件、在免鉴权认证流程中补充审计主体/目标，或编解码 CloudEvents Kafka 记录。

### 2. Signatures

- Proto 契约：
  - [annotations.proto](../../../../api/protos/plateau/security/audit/v1/annotations.proto)：`AuditMode`（`UNSPECIFIED`/`DISABLED`/`ENABLED`）、`AuditCategory`（`UNSPECIFIED`/`AUTHN`/`RESOURCE`/`AUTHZ`）、`AuditRule`（extension `50100` `rule` / `50101` `service_default`）。
  - [config.proto](../../../../api/protos/plateau/security/audit/v1/config.proto)：`message AuditEmitter`（`section = true` 对应 `audit_emitter:` 段，默认 `backend = "noop"`、`topic = "plateau.audit.events"`）。
- 生成器：[cmd/protoc-gen-plateau-audit](../../../../cmd/protoc-gen-plateau-audit/main.go) 输出 `audit_rules.gen.go` 与防御性克隆的 `AuditRules()`。
- 运行时核心接口（[security/audit](../../../../security/audit/auditor.go)，仅依赖标准库与 CloudEvents/Kratos）：
  - `type Auditor interface { Emit(ctx context.Context, event cloudevents.Event) error }`
  - `New(cfg *auditpb.AuditEmitter, logger *slog.Logger, opts ...Option) (Auditor, error)`（内置 `Noop`、`NewStdout`、`NewLog`、`Multi`；外部后端通过 `WithBackend` 注册）
  - `Server(auditor Auditor, opts ...security.RulesOption[*auditpb.AuditRule]) middleware.Middleware`
  - `HTTPHandler(auditor Auditor, eventType, category string, opts ...HTTPOption) func(http.Handler) http.Handler`（支持 `WithHTTPOnlyErrors()`、`WithHTTPPostHook(...)`）
  - `NewEvent(ctx context.Context, opts ...EventOption) cloudevents.Event` 与 `SetJSONProtoData(e *cloudevents.Event, msg proto.Message) error`
  - `SetActor(ctx context.Context, actor security.Actor)` / `SetResource(ctx context.Context, resourceType, resourceName string)`（RPC 与 `HTTPHandler` 共用）；`SetDetail(ctx context.Context, detail map[string]any)` / `SetError(ctx context.Context, code, message string)` / `SetEventOverride(ctx context.Context, eventType, category string)` / `Skip(ctx context.Context)`（由 `HTTPHandler` 消费）
  - `ExtractResource(msg proto.Message) (resourceType, resourceName string)` / `RedactClone(msg proto.Message) proto.Message`
- Kafka 审计后端与协议绑定（[security/audit/kafka](../../../../security/audit/kafka/kafka.go)）：
  - `NewDefault(cfg *auditpb.AuditEmitter, client *kgo.Client, l *slog.Logger) (audit.Auditor, error)` / `Backend(client *kgo.Client) audit.Option` / `New(client *kgo.Client, cfg Config) (audit.Auditor, error)`（直接借用 `*kgo.Client`，连接生命周期归属 `infra/kafka` 调用方）
  - `EncodeRecord(topic string, event cloudevents.Event, mode Mode) (*kgo.Record, error)` / `DecodeRecord(record *kgo.Record) (*cloudevents.Event, error)`

### 3. Contracts

- **CloudEvents v1.0 标准与扩展属性**（扩展名全小写字母数字）：
  - 标准属性：`specversion = "1.0"`、`id = <UUIDv7>`、`time = <UTC>`、`source = "//" + appName`、`type`（默认 `"plateau.audit.rpc.v1"`，403 拒绝为 `"plateau.security.authz.denied.v1"`）、`subject = <RPC operation>`。
  - 扩展属性：`category`（`authn`/`resource`/`authz`）、`actortype`（`human`/`service`/`anonymous`）、`actorid`（`anonymous` 时严禁伪造填充）、`resourcetype`、`resourcename`、`outcome`（`success`/`failure`）、`errorcode`（优先取 Kratos `*errors.Error.Reason`，否则取标准大写状态码名）、`errormessage`（仅对 `<500` 的结构化业务错误保留消息，5xx 或未包装内部错误统一脱敏为 `"internal server error"`）、`traceparent` / `tracestate`（只要 `SpanContext.IsValid()` 即写入，不依赖 `IsSampled()`）、`requestid`、`clientip`、`useragent`、`partitionkey`。
  - `data`：对请求消息执行 `RedactClone`（递归调用 `protoc-gen-redact` 生成的 `RedactFields()`）后再序列化为规范 ProtoJSON，绝不修改调用方原始请求对象。
- **中间件顺序契约**：
  - 必须位于 `AuthN` 之后、`AuthZ`（`openfga.Server`）之前：`AuthN → Audit → AuthZ → Handler`。
  - 当下游返回 `SECURITY_ERROR_REASON_PERMISSION_DENIED`（403）时，即使方法规则为 `AUDIT_MODE_DISABLED` 也会自动发出 `authz` 拒绝事件；常规 `200 OK` 只读请求在 `DISABLED` 下零分配直接返回。
- **AIP 资源提取回退**：优先从请求消息提取 `google.api.resource` 或 `google.api.resource_reference`；若请求无资源 ID（如 `CreateUserRequest`）且调用成功，自动回退从响应消息提取已分配的规范资源名。
- **原生 HTTP 协议无侵入审计**：针对非 Kratos RPC 的协议路由（如 IAM OIDC `/authorize/callback`、`/end_session`、`/oauth/token` 与 Admin `/v1/admin/auth/callback`），在路由注册层使用 `audit.HTTPHandler` 包装（`subject = "<METHOD> <PATH>"`，内置 `defer` 保证外层会话落库 `panic(responseAborted{})` 或未捕获异常中止时仍按 `500` 记录失败审计），协议处理函数仅通过 `ctx` 被动调用 `SetActor`、`SetResource`、`SetDetail`、`SetError`、`SetEventOverride`、`Skip`；严禁读取或记录 `client_secret`、授权 `code`、`refresh_token`、`code_verifier` 或会话 Cookie。

### 4. Validation & Error Matrix

| 条件 | 行为 |
| --- | --- |
| `audit.Server(nil, ...)` | 构造期直接 `panic("audit: auditor is nil")` |
| `audit.New(nil, ...)` 或 `enabled == false` | 返回无操作 `Noop()` 审计器 |
| `ctx` 未注入有效 `security.Actor` | `actortype = "anonymous"`，不设置 `actorid` |
| `SetActor` 传入无效 `Actor`（如 `human` 空 ID 或 `anonymous` 非空 ID） | 忽略该调用，保持原上下文身份 |
| Handler 返回原始数据库/系统错误（非 `<500` Kratos 结构化错误） | `errorcode = "INTERNAL"`，`errormessage = "internal server error"`，屏蔽内部敏感错误详情 |
| `Auditor.Emit` 发送失败 | `slog.ErrorContext` 记录错误，原样返回业务 `(resp, err)`，绝不阻断主请求 |

### 5. Good / Base / Bad Cases

- **Good**：服务通过 `audit.New(emitterCfg, logger, auditkafka.Backend(kafkaClient))` 构造 `Auditor`，声明 `service_default = {mode: AUDIT_MODE_ENABLED, category: AUDIT_CATEGORY_RESOURCE}` 并将只读 `Get*` / `List*` 覆盖为 `AUDIT_MODE_DISABLED`；`Login` 等 `PUBLIC` 接口核验通过后调用 `audit.SetActor(ctx, actor)` 与 `audit.SetResource(ctx, "iam.plateau.dev/User", user.Name)`。
- **Base**：开发环境 `audit_emitter.backend: stdout` 或 `log`，生产环境 `audit_emitter.backend: kafka` 统一投向 `plateau.audit.events`，平台核心服务与后续业务微服务的管控变更及鉴权拒绝事件共享同一审计存储与全局管理检索。
- **Bad**：将 `audit.Server` 放在 `openfga.Server` 之后（导致 403 鉴权拒绝绕过审计中间件），或仅在 `sc.IsSampled()` 为真时写入 `traceparent`（导致低采样率下跨服务审计记录无法通过 `trace_id` 关联）。

### 6. Tests Required

- `go test -race -count=1 ./cmd/protoc-gen-plateau-audit/... ./security/audit/...`：
  - 覆盖 `human` / `service` / `anonymous` 主体映射、未采样 Span 的 `traceparent` 注入、HTTP/gRPC 与原生 `*http.Request` 元数据提取；
  - 覆盖 `Create*` 响应资源名回退、`protoc-gen-redact` 敏感字段递归脱敏且不污染原请求、Kratos `Reason` 提取与内部错误脱敏、`DISABLED` 路由上的 403 拒绝审计、`Emit` 失败不阻断业务返回；
  - 覆盖 `audit.New` + `WithBackend` 配置构造、`security/audit/kafka` 真实 `*kgo.Client` 线协议发送及 `BinaryMode` / `StructuredJSONMode` 往返编解码。

### 7. Wrong vs Correct

#### Wrong

```go
// 错误 1：将 Audit 放在 AuthZ 之后，导致 403 鉴权拒绝无法被审计捕获
ms = append(ms, session.Server(...), openfga.Server(...), audit.Server(auditor, ...))

// 错误 2：仅在采样命中时写入 traceparent，低采样率下丢失跨服务关联
if sc.IsValid() && sc.IsSampled() { ... }
```

#### Correct

```go
// 正确：位于 AuthN 之后、AuthZ 之前；NewEvent 只要 SpanContext.IsValid() 即写入 traceparent
ms = append(
    ms,
    session.Server(authn, security.WithRulesFuncs(pb.AuthnRules)),
    audit.Server(auditor, security.WithRulesFuncs(pb.AuditRules)),
    openfga.Server(authorizer, security.WithRulesFuncs(pb.AuthzRules)),
)
```
