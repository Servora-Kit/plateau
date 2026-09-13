# server 层

`server.go` 只声明 `ProviderSet`，服务实现放在协议文件。Example 的 [ProviderSet](../../../../app/example/service/internal/server/server.go) 组合 registrar、metrics、`NewGRPCServer`、`NewHTTPServer`；[HTTP](../../../../app/example/service/internal/server/http.go) 和 [gRPC](../../../../app/example/service/internal/server/grpc.go) 都构造 middleware 链并注册同一个生成 `UserService`。

新增 transport 时：

1. 在职责明确的 `http.go`、`grpc.go`、`sse.go` 或同类文件中构造 server。
2. 装配可观测性和服务端配置，再注册生成 API。
3. 将资源名、鉴权业务决定、CRUD 生命周期和存储调用交给下层。

不要创建只供 HTTP 使用的重复 Proto service，也不要在 server 中承载数据库访问或浏览器会话的登录、撤销和存储业务。HTTP server 可以装配既有 `SessionManager` 的 load/save filter 或 middleware；IAM 的 [HTTP 装配](../../../../app/iam/service/internal/server/http.go) 是当前例子。端口和配置路径遵循 [app 结构](../../../../app/AGENTS.md)，不把硬编码端口散落到 transport 构造器。
