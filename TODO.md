# TODO - plateau

## 可观测性

- [ ] 在 Servora 共享日志层恢复 stdout/file 日志的 `trace_id`、`span_id` 自动关联。
  - 旧 bootstrap 通过 Kratos 动态 Valuer 从请求 context 取值；`cd1aea3b`（2026-05-20）迁移到 slog/zerolog 时只保留了 `service` 字段，未保留此行为。
  - 统一从当前有效 OTel SpanContext 提取字段，覆盖默认及自定义本地 handler；无 span 时不输出虚假 ID，不在 Plateau 各服务重复手动注入。
  - 业务日志使用 `InfoContext`、`ErrorContext` 等传递请求 context；仅传 context 不会让当前 stdout/file handler 自动添加字段。
  - 验证同一请求跨服务日志的 Trace ID 一致、Span ID 对应当前操作，以及无 span、并发请求、`With`/`WithGroup` 场景；与已有 OTel 日志后端的关联能力区分。

## 平台安全认证（`security/authn`）

- [ ] 讨论并提炼 `security/authn/jwt` 与 `security/authn/session` 的平台公共身份解析组件（IAM 专属查表会话认证仍留在 `app/iam/service/internal/authn`）：
  - **服务令牌声明共享（`security/authn/jwt`）**：将 `app/iam/service/internal/authn/jwt.go` 中的 `ServiceClaims`、`NewServiceClaims`、`ServiceActor` 下沉至 `security/authn/jwt`（具体文件划分待讨论确认），供所有接收内部服务调用的微服务统一复用；IAM 仅保留注入本地公钥的 `NewServiceAuthenticator`。
  - **BFF 本地会话快照认证共享（`security/authn/session`）**：将 OIDC 客户端服务（如 Admin 及后续业务 Web 后端）通用的本地 SCS 会话身份快照读写、`Authenticator` 构造及 `context` 读取函数提炼至 `security/authn/session`（具体文件与类型划分待讨论确认），消除各 BFF 服务的私有 `internal/authn` 样板。
