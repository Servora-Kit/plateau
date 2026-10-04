# 分层与依赖方向

后端使用 `service -> biz <- data`：

- `server` 装配 HTTP/gRPC transport、middleware、注册和服务实现；不承载领域流程或存储语义。
- `service` 实现生成接口，规范化请求、解析资源名、调用 Usecase 并转换响应；不承载领域逻辑。
- `biz` 定义 Usecase、领域错误和 Repo port；只依赖接口，不 import service、data 或 Ent。
- `data` 实现 biz port，管理数据库/消息等外部系统、映射及可观测性；可以 import biz。

应用专有能力可独立放在 `internal/`，但不能绕过领域与持久化边界。原始 RPC request 和 transport 类型不进入 biz/data。
