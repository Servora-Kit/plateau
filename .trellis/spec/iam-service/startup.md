# IAM 启动边界

`internal/startup` 承担 IAM 特有启动逻辑，不应移入通用 `data` 或 `server`。`Initializer` 与 provider 组合定义于 [providers.go](../../../app/iam/service/internal/startup/providers.go)，具体初始化顺序见 [initializer.go](../../../app/iam/service/internal/startup/initializer.go)。OIDC 静态 client/密钥协调归 `internal/oidc` 的 initializer。

`cmd/server/main.go` 仍只负责 bootstrap runtime、配置扫描和运行；Wire 在 [cmd/server/wire.go](../../../app/iam/service/cmd/server/wire.go) 明确组合 startup、authn、authz、oidc 和通用四层。新增初始化步骤必须有幂等定义、错误传播和 shutdown 所有权，不能在请求路径首次触发或手改 `wire_gen.go`。

启动配置可被静态读取或单测覆盖；外部 PostgreSQL、OpenFGA、密钥文件和 OIDC public origin 未启动时，不应把启动链说明成部署验收。
