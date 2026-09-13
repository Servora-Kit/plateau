# 平台代码生成规范

适用于 Plateau AuthN/AuthZ protoc 插件和 internal/codegen。源声明见 [API 注解](../../api/proto/annotations.md)，运行时见 [security](../security/index.md)。

| 主题 | 何时读取 |
| --- | --- |
| [命令与输出](cmd.md) | 插件入口、校验、安装、生成文件 |
| [共享规划](ruleplan.md) | 合并、分组、确定性与测试设施 |

开发前确认当前使用的插件二进制来源与生成路径。质量检查覆盖显式声明校验、整体覆盖、Proto presence/oneof、确定性排序、克隆和编译。入口：`go test ./cmd/protoc-gen-plateau-authn ./cmd/protoc-gen-plateau-authz`。改生成器后审阅其真实输出，不手改 gen 文件。
