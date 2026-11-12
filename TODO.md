# TODO - plateau

## 可观测性

- [ ] 在 Servora 共享日志层恢复 stdout/file 日志的 `trace_id`、`span_id` 自动关联。
  - 旧 bootstrap 通过 Kratos 动态 Valuer 从请求 context 取值；`cd1aea3b`（2026-05-20）迁移到 slog/zerolog 时只保留了 `service` 字段，未保留此行为。
  - 统一从当前有效 OTel SpanContext 提取字段，覆盖默认及自定义本地 handler；无 span 时不输出虚假 ID，不在 Plateau 各服务重复手动注入。
  - 业务日志使用 `InfoContext`、`ErrorContext` 等传递请求 context；仅传 context 不会让当前 stdout/file handler 自动添加字段。
  - 验证同一请求跨服务日志的 Trace ID 一致、Span ID 对应当前操作，以及无 span、并发请求、`With`/`WithGroup` 场景；与已有 OTel 日志后端的关联能力区分。

## Servora CRUD 生态

- [ ] 在 Servora `core/crud/mapper` 支持嵌套 Message 的点路径字段映射（如 `Profile.Name`）。
  - 当前 `crudmapper.WithFieldMapping` 拒绝含 `.` 的 `FieldPath`（`core/crud/mapper/mapper.go`），导致 `plateau` 的 `app/iam`、`app/example` 等服务需重复手写 `WithPostToDTOHook` 与嵌套字段局部更新逻辑。
  - 支持声明式嵌套路径双向映射，明确子 Message 字段全为空时的置空与初始化语义，并与 `contrib/db/entgo/crud.ClearHelper` 的嵌套掩码清理对齐；升级后移除 `plateau` 各服务的手写转换样板。
- [ ] 让 `protoc-gen-servora-crud` 生成的 `Parse<Resource>Name` 直接返回结构化 `CRUD_ERROR_REASON_INVALID_RESOURCE_NAME`（400）错误。
  - 当前生成函数（`cmd/protoc-gen-servora-crud/generate_go.go`）在格式不合法时仅返回裸 `fmt.Errorf`，迫使各服务 `service` 层额外编写私有 `parse<Resource>Name` 调用 `plan.ParseName` 包装错误。
  - 统一由生成的 `Parse<Resource>Name` 返回（或委托 `Plan()` 产出）带 `CRUD_ERROR_REASON_INVALID_RESOURCE_NAME` 原因的结构化错误；发版并重新生成后清理 `plateau` 各服务的重复包装函数。
