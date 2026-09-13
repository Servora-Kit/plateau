# 分层与依赖方向

后端使用 `service -> biz <- data`：

- `server` 装配 HTTP/gRPC transport、middleware、注册和服务实现；不承载领域流程或存储语义。
- `service` 实现生成的 RPC/HTTP 接口，规范化请求、解析资源名、调用 Usecase，并转换响应。它不能承载业务逻辑，biz 也不能 import service。
- `biz` 定义 Usecase、领域错误和 `XxxRepo` port；只依赖接口，不 import data 或 Ent。
- `data` 实现 biz port，管理数据库/消息等外部系统、映射及可观测性；可以 import biz。

Example 的 [UserService](../../../../app/example/service/internal/service/user.go)、[UserUsecase](../../../../app/example/service/internal/biz/user.go) 与 [userRepo](../../../../app/example/service/internal/data/user.go) 展示完整方向。接口层不得把 RPC request 传入 biz/data；[service 层约定](../../../../app/example/service/internal/service/AGENTS.md) 明确将请求转成资源名、`ResourcePlan` 和 `ListPreparer` 值。

公共四层解决通用调用路径。OIDC 协议、会话认证和启动初始化等专有能力保留在应用 `internal`；其依赖不可绕过 biz/data 的领域与持久化边界。
