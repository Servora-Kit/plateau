# server 层

`server.go` 只声明 `ProviderSet`，协议文件负责构造 transport、装配 middleware，并注册生成的服务实现。

新增 transport 时：

1. 在职责明确的 `http.go`、`grpc.go`、`sse.go` 或同类文件中构造 server。
2. 装配可观测性和服务端配置，再注册生成 API。
3. 将资源名、鉴权业务决定、CRUD 生命周期和存储调用交给下层。

不要创建只供 HTTP 使用的重复 Proto service，不在 server 层承载业务流程或数据库访问。端口和配置路径遵循统一项目约定，不在 transport 构造器中硬编码。

