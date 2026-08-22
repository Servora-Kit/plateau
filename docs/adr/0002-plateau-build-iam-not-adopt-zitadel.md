# Plateau 自建 IAM 作为 Servora 的 dogfood 案例，而非 adopt Zitadel

> Status: accepted

Plateau IAM 选择自建身份服务，而不是部署或托管 Zitadel、Keycloak、Logto。原因是 Plateau 需要验证 Servora 能否支撑生产级身份服务，并保留对 User、认证策略和生命周期的所有权；协议正确性不重复实现，采用 `github.com/zitadel/oidc/v3` 作为 OIDC/OAuth 协议层。

## Considered Options

- **直接 adopt Zitadel / Keycloak / Logto**：拒绝。身份生命周期和核心架构会转移到外部产品，削弱 Plateau 对 Servora 的 dogfood 目标。
- **外部身份源 + 薄 IAM 适配层**：拒绝。仍然失去自建身份领域的验证价值，且外部组件边界与 Plateau 业务模型不一致。

## Consequences

- Plateau IAM 自己承担密码、认证器、session、token、恢复流程和安全审计的长期维护责任。
- IAM 复用 OIDC 协议实现，仍必须遵守 OIDC/OAuth 外部合同；业务身份事实和协议适配保持可替换边界。
- 具体数据库、前端、Redis、CAP、认证器和部署方案由 OpenSpec proposal/spec/design 记录，不在本 ADR 展开。