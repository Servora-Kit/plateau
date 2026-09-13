# Plateau 开发规范索引

这里面向开发者与 AI，按职责组织当前项目的开发规范。任务需要显式组合相关正文；索引链接不会自动注入全部内容。

业务包名中的 `-service` / `-web` 已说明前后端，主题和 `index.md` 直接放在包根。共享包继续按实际职责分组，例如 `service/backend`、`plateau/security`、`web/client`；只有分组具有独立含义时才增加目录层级。

| Package | 入口与职责 |
| --- | --- |
| plateau | [project](plateau/project/index.md)、[security](plateau/security/index.md)、[infra](plateau/infra/index.md)、[codegen](plateau/codegen/index.md) |
| api | [Proto 契约与生成](api/proto/index.md) |
| web | [共享 client](web/client/index.md) |
| service | [共通微服务](service/backend/index.md) |
| servora | [framework](servora/framework/index.md)、[proto](servora/proto/index.md)、[cmd](servora/cmd/index.md)、[web](servora/web/index.md) |
| iam-service | [IAM 后端](iam-service/index.md) |
| iam-web | [IAM 前端](iam-web/index.md) |
| example-service | [Example 后端](example-service/index.md) |
| example-web | [Example 前端](example-web/index.md) |
| audit-service | [Audit 现状与限制](audit-service/index.md) |
| test-web | [测试前端](test-web/index.md) |

[通用思考指南](guides/index.md) 提供跨层检查与复用思路，具体规则以各 package 的权威主题为准。新功能按实际能力扩展，目录和 topic 不必复制默认模板；新增/调整规范时同步索引和真实任务上下文引用。
