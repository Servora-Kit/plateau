# 日志、追踪与指标

`obs` 从 Bootstrap Proto 装配 `slog`、OpenTelemetry trace/metric 和 Prometheus handler，见 [`obs/AGENTS.md`](../../../../../servora/obs/AGENTS.md)。配置结构属于 `servora.core.v1.Bootstrap`，不要为单个 backend 发明绕过 Bootstrap 的独立配置。

- `logger.New` 返回 `*slog.Logger` 和必须在关闭时执行的 closer，定义于 [`logger/logger.go`](../../../../../servora/obs/logger/logger.go)。bootstrap 负责绑定 Kratos v3 默认 logger。
- `tracing.InitTracerProvider` 在 endpoint 为空时返回 noop cleanup，实现在 [`tracing/tracing.go`](../../../../../servora/obs/tracing/tracing.go)。
- `metrics.New` 建立私有 Prometheus registry 和 OTel provider；业务自定义指标经 `Metrics.Meter(name)` 创建。服务名是 Resource 属性，不是 Meter name，见 [`metrics/metrics.go`](../../../../../servora/obs/metrics/metrics.go)。

调用方必须保存并执行 logger/OTel cleanup；不要把原生 Prometheus 默认 registry 当作 Servora `/metrics` 的扩展点。TLS/CA 解析复用 [TLS 构造](tls.md)。修改任一子包时运行 `go test ./obs/...`，再按影响检查 Bootstrap schema 与启动装配。
