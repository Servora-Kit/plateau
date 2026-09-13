# 平台安全能力规范

适用于 security 共享实现与接入边界。业务身份策略留在 IAM 或其他业务 package；这里的规则不替代具体应用授权。

| 主题 | 何时读取 |
| --- | --- |
| [Actor](actor.md) | 定义和传递可信执行身份 |
| [AuthN](authN.md) | JWT／Session 认证和路由规则 |
| [AuthZ](authZ.md) | OpenFGA 能力、目标和失败分类 |
| [CAP](capabilities.md) | PoW challenge 与一次性消费 |
| [凭据与会话工具](credentials.md) | 密码、JWT 签名/密钥和 SCS 装载 |

开发前先确认源 Proto、生成规则、具体运行时和业务 mapper 的分工；不要把旧文档中已移除的 neutral provider 或手写 errors 包当作当前接口。质量检查关注默认拒绝、匿名/可信身份区分、scope、并发隔离、provider 失败与资源清理。平台安全单测入口为 `go test ./security/...`；应用端到端验收另行记录。
