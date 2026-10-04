# 微服务后端规范

适用于 `app/*/service` 的手写后端代码。目录职责见 [app 服务结构](../../../../app/AGENTS.md)。

## 开发前检查

- 新增或移动文件读 [布局](layout.md)；改变依赖关系读 [分层](layers.md)。
- 改动任一层时读对应正文和 [共同编码](coding.md)。
- CRUD、启动装配和测试分别读对应主题；质量命令以服务 `justfile` 和 [app 常用命令](../../../../app/AGENTS.md) 为准。

## 主题

- [布局](layout.md)、[分层](layers.md)：目录职责与依赖边界。
- [共同编码](coding.md)：注入、context、错误和日志。
- [CRUD](crud.md)：公共 CRUD 机制的应用消费。
- [transport](server.md)、[接口适配](service.md)、[领域](biz.md)、[持久化](data.md)：各层规范。
- [启动装配](bootstrap.md)、[测试](testing.md)：生命周期与验证边界。
