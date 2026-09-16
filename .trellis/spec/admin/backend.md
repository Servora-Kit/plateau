# Admin 后端规范

适用于规划中的 `app/admin/service`。共通分层和框架使用遵循 [共享微服务规范](../service/backend/index.md)，应用定位见 [架构](architecture.md)。

## IAM 用户管理接入

Admin 调用 IAM gRPC [UserService](../../../app/iam/service/api/protos/iam/user/v1/user.proto) 的 `CreateUser`、`GetUser`、`ListUsers`、`UpdateUser`、`DisableUser`、`EnableUser`。IAM service 层再委托给内部 `UserUsecase`；Admin 不跨服务调用内部 usecase，也不直接操作 IAM 数据表。

Admin 对操作者执行管理权限检查；调用 IAM 时使用自己的服务身份，由 IAM 再验证调用方及其 `iam.manage_users` 权限。这两个检查的主体不同，现有 IAM 接收条件与模型边界统一引用 [IAM 授权](../iam/authorization.md) 和 [OIDC 服务令牌](../iam/oidc.md)。

OAuth Client 当前由静态配置与启动协调拥有，见 [IAM 启动](../iam/startup.md)。动态管理能力应先在 IAM 建立，再由 Admin 接入，不能通过直接改库绕过静态配置的生命周期。

## 开发前与质量检查

- 用户状态和身份生命周期的规则在 IAM 实现，Admin 负责输入转换、管理授权和调用结果处理。
- 源码落地后验证操作者权限、Admin 服务身份、IAM 接收权限与实际变更结果；模型文件或测试 tuple 存在不代表运行环境已完成授权装配。
