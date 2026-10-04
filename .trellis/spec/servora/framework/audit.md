# 审计运行时

`obs/audit` 是 CloudEvents 审计运行时，稳定接口为 `Auditor.Emit(context.Context, cloudevents.Event)`；业务消费与存储由应用拥有。

`audit.Middleware` 按生成的 RPC 规则在 handler 返回后发送事件；发送失败只记录日志并保留 handler 响应，不改写业务调用结果。`NewEvent` 从 sampled span 写入 `traceparent`/`tracestate`；领域字段放 event data，路由字段放 extension。

审计后端在实现相应接口时由 `multi.New` 向下传播 `Close` 和 `Flush`。新增后端须保留生命周期传播，不在 middleware 实现具体传输。

审计开关的 Proto 合并与输出见 [annotations](../proto/annotations.md)、[plugins](../cmd/plugins.md)。检查入口：`go test ./obs/audit/...`。
