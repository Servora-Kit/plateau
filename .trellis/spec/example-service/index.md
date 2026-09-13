# Example 后端规范

适用于 `app/example/service`。它是新增平台微服务的 CRUD 与分层参考，不是生产 IAM/租户授权实现。先读 [共享微服务规范](../service/backend/index.md)，再读本层 [User CRUD 实例](user-crud.md)。

## 开发前检查

- 修改 User API、生命周期、列表或 Ent 映射时同时读共享 [CRUD](../service/backend/crud.md) 和本实例。
- 不将 `UserScope` 的 path tenant 提取复制为生产认证方案；生产服务从已认证 operator context 获取身份，并独立执行授权。

## 质量检查

- 现有入口包括 [User biz 单元测试](../../../app/example/service/internal/biz/user_test.go) 和 [User service 集成测试](../../../app/example/service/internal/service/user_integration_test.go)。
- 含真实数据库/transport 的测试环境未运行时，不能以参考代码存在宣称端到端验收。
