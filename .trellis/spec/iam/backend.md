# IAM 后端

适用于 `app/iam/service`。改动同时组合 [共享微服务规范](../service/backend/index.md) 与本层主题；IAM 是主要业务实践，但其专有组织不自动成为所有新服务的模板。

## 开发前检查

- 身份资料与账号规则读 [identity](identity.md)，浏览器登录状态读 [sessions](sessions.md)。
- 改动认证端点、服务令牌或 OIDC provider 前读 [oidc](oidc.md)；改动授权执行读 [authorization](authorization.md)。
- 修改 Wire、启动初始化或静态客户端/密钥处理读 [startup](startup.md)。

## 质量检查

- 选择受影响包的现有测试：`internal/authn/session_test.go`、`internal/authz/openfga_test.go`、`internal/oidc/provider_test.go` 和各层集成测试是当前入口。
- OIDC discovery/JWKS、会话、OpenFGA 和数据库的真实联调需要相应运行配置；代码或单测存在不等于端到端已验收。

## 主题

- [身份](identity.md)、[会话](sessions.md)、[OIDC](oidc.md)、[授权](authorization.md)、[启动](startup.md)。
