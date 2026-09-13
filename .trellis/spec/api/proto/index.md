# Proto 与生成规范

适用于平台和业务源 Proto、注解声明以及共享生成产物的归属。框架注解内部设计见 [servora/proto](../../servora/proto/index.md)。

| 主题 | 何时读取 |
| --- | --- |
| [API 契约](contracts.md) | 资源、查询、字段、错误和兼容性 |
| [注解](annotations.md) | 安全声明、覆盖语义与解释责任 |
| [生成](generation.md) | Buf、插件、Go/TS/OpenAPI/Wire/Ent 输出 |

开发前确认改动源定义和真实消费者，不编辑生成文件。质量检查包括 `just lint-proto`、`just api-ts-check`、生成 diff 以及受影响 Go/前端契约；对 breaking 比较明确真实基线。记录已运行检查，未运行的生成或运行验证不能标为通过。
