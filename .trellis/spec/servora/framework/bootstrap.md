# 启动、配置扫描与生命周期

适用于 `core/bootstrap`。业务应用使用显式链路 `NewRuntime → Scan → rt.Run(wireApp closure)`；不要重新引入把 runtime、扫描和业务装配藏在一起的一行启动入口。公开入口和迁移要求见 [`core/bootstrap/AGENTS.md`](../../../../../servora/core/bootstrap/AGENTS.md)，实现位于 [`bootstrap.go`](../../../../../servora/core/bootstrap/bootstrap.go)、[`scan.go`](../../../../../servora/core/bootstrap/scan.go) 和 [`provider.go`](../../../../../servora/core/bootstrap/provider.go)。

- `NewRuntime` 读取配置、建立日志/trace 和 runtime 资源；`Runtime.NewApp` 注入应用 identity；`Runtime.Run` 执行业务 Wire closure 并在运行结束后关闭；`Runtime.Close` 仅关闭 runtime 自有资源。
- 业务 Wire cleanup 不登记到 runtime。`Run` 保证业务 cleanup 先于 runtime cleanup；新增 runtime 资源必须明确 LIFO 关闭顺序。
- `bootstrap.ProviderSet` 是业务 Wire 图的稳定根。不要把 `*bootstrap.Runtime` 当作 service locator 传入业务 provider。
- `Scan` 按传入顺序处理对象，拒绝空 runtime/config、空对象及指针值为空的接口。通过 Proto 描述符读取布尔 section 标记，消息短名转小写下划线段名；缩写按词分组，不裁剪后缀。无标记时扫描整份配置。
- 缺段跳过解码和 Apply，不填默认值；存在段或整份配置成功解码后调用 `Apply() error`。解码或应用失败立即返回对象位置、配置段上下文；不能吞掉存在段的类型错误。
- loader 负责配置来源的完整生命周期；返回配置的 Close 先停止监听器，再关闭可关闭的远端来源，合并错误且幂等。单独关闭 Kratos 配置不会关闭 Etcd 来源的客户端。私有配置 Scan 失败时，服务入口须关闭已经创建的 Runtime。
- 保持核心配置应用、日志和追踪初始化、私有配置扫描、业务装配的顺序，不新增全量前置校验阶段。`logger.New` 返回日志对象、关闭函数和错误，创建资源前先应用日志配置。

配置 section/default/required 的生成契约见 [Proto generation](../proto/generation.md)。改变启动或扫描语义时运行 `go test ./core/bootstrap/...`；该目录的测试覆盖 runtime identity、logger、cleanup、whole-config/section/optional scan。
