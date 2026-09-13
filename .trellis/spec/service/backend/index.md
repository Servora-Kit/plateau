# 微服务后端规范

适用于 `app/*/service` 的手写后端代码。公共结构以 [app 服务结构](../../../../app/AGENTS.md) 为准；`app/example/service` 是 CRUD 和分层的推荐参考，IAM 的领域专有模块只在其业务规范中定义。

## 开发前检查

- 新增或移动文件先读 [布局](layout.md)；改变依赖关系读 [分层](layers.md)。
- 改动 `internal/server`、`service`、`biz`、`data` 时分别读对应正文，并始终同时读 [共同编码](coding.md)。
- 使用资源 CRUD 时读 [CRUD](crud.md)；启动、Wire 或资源清理读 [启动装配](bootstrap.md)。

## 质量检查

- 服务级命令以该服务的 `justfile` 为准；根目录提供 `just lint` 和 `just gen`，见 [app 常用命令](../../../../app/AGENTS.md)。生成、OpenFGA apply、部署不因编辑手写业务代码而自动执行。
- 为改动的分层运行相邻单元测试；涉及 HTTP/gRPC、数据库、消息中间件或生成契约时，补充相应集成检查，见 [测试](testing.md)。

## 主题

- [布局](layout.md)：服务根目录、生成物和专有 internal 模块的位置。
- [分层](layers.md)：`service -> biz <- data` 与 transport 边界。
- [共同编码](coding.md)：依赖注入、context、错误和日志边界。
- [CRUD](crud.md)：Servora CRUD 的消费与跨层流程。
- [server](server.md)、[service](service.md)、[biz](biz.md)、[data](data.md)：各层的具体写法。
- [启动装配](bootstrap.md)、[测试](testing.md)：运行时和验证边界。
