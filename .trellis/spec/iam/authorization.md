# IAM 服务授权边界

IAM 将已验证的 `security.Actor` 映射为 OpenFGA subject：human 为 `user:{id}`，service 为 `service:{id}`，见 [NewOpenFGAAuthorizer](../../../app/iam/service/internal/authz/openfga.go)。无效 Actor 或未知类型在映射处失败。

该模块是 IAM 内的 OpenFGA PDP 适配与 subject 规范化。资源、关系 tuple 的管理（PAP）和 receiving service 的 PEP 由各业务服务的领域与入口承担；服务令牌、OIDC discovery/JWKS 或 OpenFGA client 成功构造都不是用户已获业务权限的证据。业务服务须在自身受保护操作处调用相应 authorizer 并处理结果。

当前模型区分 service 主体的 `iam.manage_users` 与 user 主体的 Admin 管理关系，具体模型和部署边界由 [平台 OpenFGA 规范](../plateau/infra/openfga.md) 统一维护。IAM 用户目录变更不自动建立任意业务关系；业务资源权限及其 tuple 生命周期归对应业务领域，测试夹具中的 tuple 不能当作运行数据。服务 token 的认证条件见 [OIDC](oidc.md)。

当前验证入口为 [openfga_test.go](../../../app/iam/service/internal/authz/openfga_test.go)。涉及 model 或 tuple 语义时，还要遵循根项目的 `just openfga-model-validate`、`just openfga-model-test` 和 model apply 流程；未执行真实 OpenFGA 环境检查时不能标为运行验收。
