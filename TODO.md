# TODO - plateau

## 可观测性

- [ ] 在 Servora 共享日志层恢复 stdout/file 日志的 `trace_id`、`span_id` 自动关联。
  - 旧 bootstrap 通过 Kratos 动态 Valuer 从请求 context 取值；`cd1aea3b`（2026-05-20）迁移到 slog/zerolog 时只保留了 `service` 字段，未保留此行为。
  - 统一从当前有效 OTel SpanContext 提取字段，覆盖默认及自定义本地 handler；无 span 时不输出虚假 ID，不在 Plateau 各服务重复手动注入。
  - 业务日志使用 `InfoContext`、`ErrorContext` 等传递请求 context；仅传 context 不会让当前 stdout/file handler 自动添加字段。
  - 验证同一请求跨服务日志的 Trace ID 一致、Span ID 对应当前操作，以及无 span、并发请求、`With`/`WithGroup` 场景；与已有 OTel 日志后端的关联能力区分。
