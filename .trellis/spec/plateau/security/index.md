# 平台安全能力规范

适用于 security 共享实现与接入边界。身份策略由消费应用拥有；这里的规则不替代应用授权。

| 主题 | 何时读取 |
| --- | --- |
| [Actor](actor.md) | 定义和传递可信执行身份 |
| [AuthN](authN.md) | JWT／Session 认证和路由规则 |
| [AuthZ](authZ.md) | OpenFGA 能力、目标和失败分类 |
| [CAP](capabilities.md) | PoW challenge 与一次性消费 |
| [凭据与会话工具](credentials.md) | 密码、JWT 签名/密钥和 SCS 装载 |

开发前确认源 Proto、生成规则、运行时和应用 mapper 的分工。质量检查关注默认拒绝、匿名/可信身份区分、scope、并发隔离、provider 失败与资源清理。检查入口：`go test ./security/...`。
