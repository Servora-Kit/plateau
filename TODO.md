# TODO - plateau

## Cookie 会话与 HTTP 安全

- [ ] 整理 Go 后端可复用的浏览器 Cookie 会话安全接线。
  - 基于现有 `security/authn/session`，隔离业务逻辑与 Cookie 读写、安全属性、CSRF、Origin/Referer 校验及 CORS 配置。
  - 通用机制统一维护，应用只声明公开入口与保护范围；移除来源防护对 OIDC Provider 对象的直接依赖。
  - 优先评估 Go 标准库 `http.CrossOriginProtection`，明确缺失来源头、可信来源及反向代理场景的策略，避免迁移时悄然放宽防护。
  - 区分 Cookie 会话接口、Bearer/mTLS 接口与 OIDC 协议端点，不给所有微服务一刀切挂载。
  - 保留来源拒绝无业务副作用、登录 CSRF、会话生命周期等行为回归；CORS 不能替代 CSRF 防护，业务授权规则仍由业务定义。

## 可观测性

- [ ] 在 Servora 共享日志层恢复 stdout/file 日志的 `trace_id`、`span_id` 自动关联。
  - 旧 bootstrap 通过 Kratos 动态 Valuer 从请求 context 取值；`cd1aea3b`（2026-05-20）迁移到 slog/zerolog 时只保留了 `service` 字段，未保留此行为。
  - 统一从当前有效 OTel SpanContext 提取字段，覆盖默认及自定义本地 handler；无 span 时不输出虚假 ID，不在 Plateau 各服务重复手动注入。
  - 业务日志使用 `InfoContext`、`ErrorContext` 等传递请求 context；仅传 context 不会让当前 stdout/file handler 自动添加字段。
  - 验证同一请求跨服务日志的 Trace ID 一致、Span ID 对应当前操作，以及无 span、并发请求、`With`/`WithGroup` 场景；与已有 OTel 日志后端的关联能力区分。
