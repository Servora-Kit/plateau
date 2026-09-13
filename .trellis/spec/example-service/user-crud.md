# User CRUD 实例

Example 的 `User` 资源通过生成 `UserCRUDDescriptor()` 驱动 service 的 `ResourcePlan`，完整 service/biz/data 路径见 [UserService](../../../app/example/service/internal/service/user.go)、[UserUsecase](../../../app/example/service/internal/biz/user.go)、[userRepo](../../../app/example/service/internal/data/user.go)。构造和每个组件的推荐用法由共享 [CRUD](../service/backend/crud.md) 定义。

本资源的业务特有选择包括：parent `tenants/{tenant}` 转为 `UserScope`；scope fingerprint 绑定 tenant 与删除可见性；临时密码在 biz 哈希后清除；更新以 etag 和零行条件 mutation 解决并发；软删除可显式查询并支持 undelete。这些是当前 User 例子的需要，不能推广为所有资源都要有 tenant path、密码或软删除。

`NewUserRepo` 中的 `ListFields` 允许 display name、email、tenant plan、nickname 和时间字段的不同过滤/排序能力；`ClearHelper` 的 display-name 清空和 temporary-password rename 均是 User 字段语义。新增字段时按共享 CRUD 清单审查 descriptor、bind、mapper、clear、setter 和测试。

User Ent schema 的 `(tenant_id, resource_id)` 唯一索引同时约束活动和 tombstone 行，见 [schema/user.go](../../../app/example/service/internal/data/ent/schema/user.go)。这与本资源的 canonical name 和 undelete 语义配套；新资源应由自己的 schema 决定唯一性、软删除和恢复规则，不能照搬该索引。

## 验证分工

[biz 单元测试](../../../app/example/service/internal/biz/user_test.go) 替代 Repo，检查 scope、密码哈希传递、错误与并发删除／恢复分支；[TestUserReferenceIntegration](../../../app/example/service/internal/service/user_integration_test.go) 则实际装配 service→biz→data→Ent→SQLite，覆盖资源名、INPUT_ONLY/OUTPUT_ONLY、filter/order、续页与 total、FieldMask、etag、软删除和 undelete。它使用 `integration` build tag，并要求显式 `SERVORA_EXAMPLE_SQLITE_DSN`；未配置会失败，不是自动跳过。

在服务目录按专用测试数据库配置运行 `go test -tags=integration ./internal/service -run '^TestUserReferenceIntegration$'`。该测试直接调用 service 方法，不经过 HTTP/gRPC 网络或浏览器；协议、生成前端与真实 Web CRUD 仍需各自验收。本任务没有运行该集成测试或建立数据库。
