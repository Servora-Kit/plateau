# 启动与装配

`cmd/server/main.go` 只处理 flag、`bootstrap.NewRuntime`、配置扫描和 `Runtime.Run`。Example 的 [main.go](../../../../app/example/service/cmd/server/main.go) 使用 `-conf` 选择 `configs/local`；容器环境由 `configs/docker` 及应用编排提供。

`wire.go` 按依赖方向组合 `bootstrap.ProviderSet`、data、biz、service、server 和 `newApp`，见 [Example wire.go](../../../../app/example/service/cmd/server/wire.go)。Wire 生成物由生成流程维护；手写装配变化后使用既有生成入口刷新，不手改 `wire_gen.go`。

Provider 构造失败应尽早返回；拥有连接的 provider 返回 cleanup，并交给 `Runtime.Run` 的返回 cleanup 链。应用启动特有的预检/初始化（如 IAM OIDC 和初始用户）放在该服务的 `internal/startup`，并由应用规范定义，不混入公共 bootstrap 规则。
