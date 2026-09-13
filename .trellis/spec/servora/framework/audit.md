# Audit 运行时

`obs/audit` 是 CloudEvents 审计运行时，不是 Plateau 的 Audit 业务服务。框架侧以 `Auditor.Emit(context.Context, cloudevents.Event)` 为稳定接口，证据在 [`auditor.go`](../../../../../servora/obs/audit/auditor.go)。业务消费与维护限制另见各业务 package。

`audit.Middleware` 根据生成的 RPC 规则在 handler 返回后发送事件；[`audit_middleware.go`](../../../../../servora/obs/audit/audit_middleware.go) 明确规定发送失败只记录日志并保留原 handler 响应，不能把审计传输失败改写成业务调用失败。`NewEvent` 建立 CloudEvents 并从 sampled span 写入 `traceparent`/`tracestate`；领域字段放 event data，路由字段放 extension。

noop、stdout、log、kafka 和 multi 是已有后端。`multi.New` fanout，且在后端实现相应接口时向下传播 `Close` 和 `Flush`，见 [`multi/multi.go`](../../../../../servora/obs/audit/multi/multi.go)。加入后端时保留这一生命周期传播，不在 middleware 中实现具体传输。

审计开关的 Proto 合并与生成输出见 [annotations](../proto/annotations.md) 和 [plugins](../cmd/plugins.md)。修改运行时运行 `go test ./obs/audit/...`；不要据此宣称某个业务服务已端到端接收或存储审计事件。
