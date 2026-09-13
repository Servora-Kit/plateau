# 启动、配置扫描与生命周期

适用于 `core/bootstrap`。业务应用使用显式链路 `NewRuntime → Scan → rt.Run(wireApp closure)`；不要重新引入把 runtime、扫描和业务装配藏在一起的一行启动入口。公开入口和迁移要求见 [`core/bootstrap/AGENTS.md`](../../../../../servora/core/bootstrap/AGENTS.md)，实现位于 [`bootstrap.go`](../../../../../servora/core/bootstrap/bootstrap.go)、[`scan.go`](../../../../../servora/core/bootstrap/scan.go) 和 [`provider.go`](../../../../../servora/core/bootstrap/provider.go)。

- `NewRuntime` 读取配置、建立日志/trace 和 runtime 资源；`Runtime.NewApp` 注入应用 identity；`Runtime.Run` 执行业务 Wire closure 并在运行结束后关闭；`Runtime.Close` 仅关闭 runtime 自有资源。
- 业务 Wire cleanup 不登记到 runtime。`Run` 保证业务 cleanup 先于 runtime cleanup；新增 runtime 资源必须明确 LIFO 关闭顺序。
- `bootstrap.ProviderSet` 是业务 Wire 图的稳定根。不要把 `*bootstrap.Runtime` 当作 service locator 传入业务 provider。
- `Scan` 按传入顺序处理 target，先拒绝 nil runtime/config、nil 或 typed-nil target；实现 `Section` 的 target 按非空 `SectionKey()` 扫描，其余 target 扫描整份配置。每个成功扫描的 `ConfApplier` 才调用一次 `ApplyConf()`，首个扫描或应用错误即停止并附带 target index 与 config/section 上下文。
- 只有同时实现 `OptionalSection` 且扫描错误为 Kratos `ErrNotFound` 的 section 才跳过 scan 后处理与 `ApplyConf()`；可选 section 的解码错误不应吞掉，非可选缺失同样返回错误。这个行为由 [`scan.go`](../../../../../servora/core/bootstrap/scan.go) 和 [`scan_test.go`](../../../../../servora/core/bootstrap/scan_test.go) 覆盖。

配置 section/default/required 的生成契约见 [Proto generation](../proto/generation.md)。改变启动或扫描语义时运行 `go test ./core/bootstrap/...`；该目录的测试覆盖 runtime identity、logger、cleanup、whole-config/section/optional scan。
