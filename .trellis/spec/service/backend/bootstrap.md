# 启动与装配

`cmd/server/main.go` 只处理 flag、`bootstrap.NewRuntime`、配置扫描和 `Runtime.Run`；环境配置位置见 [布局](layout.md)。

`wire.go` 按依赖方向组合 `bootstrap.ProviderSet`、data、biz、service、server 和 `newApp`。手写装配变化后通过既有生成入口刷新 Wire，不手改 `wire_gen.go`。

Provider 构造失败尽早返回并释放已获取资源；拥有连接的 provider 返回 cleanup，交给 `Runtime.Run` 的 cleanup 链。应用预检与初始化属于应用模块，不混入公共 bootstrap。
