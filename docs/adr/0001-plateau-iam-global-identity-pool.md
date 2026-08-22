# Plateau IAM 是业务无关的全局身份池

> Status: accepted

Plateau IAM 是平台自用的全局身份池：它管理全局唯一的 `User` 与 `Credential`，处理登录会话与 token 签发，但不提供身份域隔离（不做 Logto tenant / Keycloak realm）。`tenant` 与 `membership` 是业务概念，归业务服务。每个 `User` 拥有稳定、不可复用的 `iam_user_id`；email 仅是登录与联系标识，在有效 User 范围内全局唯一，User 资料字段仅为 OIDC 标准 claim。

## Considered Options

- **多租户 IdP（Logto tenant / Keycloak realm 隔离）+「同 email 在不同租户是不同 user」**：拒绝。IAM 只有 plateau 一个自用客户，无多客户隔离诉求；而「同自然人在不同租户是两个账号」要求 IAM 感知业务 tenant 边界，恰恰是本决策排除的耦合。
- **User 资料含标准字段 + attributes/metadata 扩展**：拒绝。YAGNI，OIDC 标准字段已覆盖身份载荷；额外的属性块会把业务字段吸引进 IAM。
- **IAM 统一保存 business membership**：拒绝。各业务服务的成员关系语义不同，统一保存会把业务耦合进身份层。

## Consequences

- 同自然人在多个业务 tenant 出现 = 一个 IAM `User` + 多条业务 membership（各业务服务引用 `iam_user_id`）。
- Mall、CMS 等业务服务各自维护本地 User projection；projection 通过 `iam_user_id` 显式引用 IAM User，但不复制 IAM 凭据或把业务字段写入 IAM。
- 业务服务各自维护 tenant 与 membership 表；IAM 不承担业务资源隔离。
- OpenFGA 业务 module 使用全局唯一的业务 User type，并以显式 identity relation 连接 IAM `user`；module 名不提供 type namespace。
- IAM 导出的是稳定身份契约（`iam_user_id` 与 OIDC 标准 profile），不随业务字段演进；email 变更不改变 `iam_user_id`。