# 注解声明与责任

Proto 定义注解与业务声明，生成器负责解释，运行时负责执行。三者需明确互相引用，不能把注解存在当作中间件已经装配。

## Plateau 安全规则

[AuthN](../../../../api/protos/plateau/security/authn/v1/annotations.proto) 有 UNSPECIFIED/PUBLIC/REQUIRED；[AuthZ](../../../../api/protos/plateau/security/authz/v1/annotations.proto) 有 UNSPECIFIED/NONE/REQUIRED。各自提供 method rule 与 service_default。

| 方法声明 | 有效 service default | 合并结果 |
| --- | --- | --- |
| 非零 mode | 任意 | 方法规则整体替换 |
| 缺失或零 mode | 非零 mode | 继承整个 default |
| 缺失或零 mode | 无 | 不生成该方法规则 |

AuthZ 的 REQUIRED 必须带 action、resource_type 和 oneof target 中一个非空目标：静态 resource_id，或 resource_id_field。后者使用 Proto 字段名点路径，终点支持字符串/整数，中间不能经过 repeated/map/真实 oneof。方法改 mode 后不会从 default 补齐 action 或 target。

PUBLIC 与 NONE 是不同维度：公开认证路由不自动关闭授权，NONE 不自动授予登录身份。接入路由 middleware 时缺规则会失败，源声明与装配必须一起审阅。

## 框架和标准注解

google.api 的 HTTP、resource、resource_reference、field_behavior 定义 API 形状；Servora errors、conf、crud、audit 定义各框架能力的附加合同。它们不共用 Plateau 的 optionmerge 语义，不能因名称相似就推断相同覆盖方式。

配置注解只声明 schema 与校验输入，不承担消费方实现；具体功能的必需配置由构造入口校验，共享 section 可选不代表功能可缺少配置。

检查：新增枚举是否有声明校验与运行时分支；字段路径是否匹配 input descriptor；override 是否误以为局部合并；生成与 runtime 是否同步。依据：[平台生成器](../../plateau/codegen/cmd.md)、[共享规划](../../plateau/codegen/ruleplan.md)、[AuthN](../../plateau/security/authN.md)、[AuthZ](../../plateau/security/authZ.md)。
