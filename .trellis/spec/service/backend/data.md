# data 层

具体仓储文件按确认顺序书写：import → 私有 `xxxRepo` 结构体 → `NewXxxRepo` → 实现 `biz.XxxRepo` 的方法。Example 的 [userRepo](../../../../app/example/service/internal/data/user.go) 和 `NewUserRepo` 是推荐命名与布局。IAM 的 [user.go](../../../../app/iam/service/internal/data/user.go) 目前使用 `userRepository`/`NewUserRepository`，这是现存差异；不因该实现改写本规范，也不在本任务中改代码。

`data.go` 只承担共享资源、`NewData`、cleanup 和 ProviderSet。Example 的 [NewData](../../../../app/example/service/internal/data/data.go) 返回 `func()` 关闭 Ent client，失败建库后立即关闭 client。Repo 用显式 Ent setter 写入、以 `mapper.ToDTO`/`ToDTOs` 映射读取结果；只有有恢复分支时才使用 Try 变体。

Ent schema 是应用持久化模型的唯一来源，负责字段的 optional/nillable、immutable、default、sensitive 与唯一约束；这些取决于领域，不能由 CRUD adapter 自动推断。Example 的 [User schema](../../../../app/example/service/internal/data/ent/schema/user.go) 把 `(tenant_id, resource_id)` 设为对活动和 tombstone 行都生效的唯一键，这是该资源的选择，不是所有服务的模板。

采用 `SoftDeleteMixin` 的 schema 默认过滤 tombstone，并把 Delete/DeleteOne 改写为带 `delete_time` 的更新；只有包含 tombstone 的业务读取或硬删除才显式使用 `entgomixin.SkipSoftDelete`，见 [mixin 实现](../../../../../servora/contrib/db/entgo/mixin/soft_delete.go) 和 [Example Repo](../../../../app/example/service/internal/data/user.go)。`show_deleted` 改变可见性时，Repo 既要显式改变查询 context，也要把可见性写入 page-token scope fingerprint；两者分别控制数据库结果和 token 重用，不能互相代替。

一个领域不变量跨多个实体或需要锁定并发状态时，事务由 data Repo 拥有。IAM 的 [inTx](../../../../app/iam/service/internal/data/transaction.go) 负责 Begin、panic rollback、错误 rollback 与 commit，OAuth/凭据变更在该边界内锁定用户；简单单实体 Example User 操作不因此自动包事务。涉及真实数据库时，测试须在所选 dialect 的实际环境验证 schema、事务与 soft-delete 行为；本轮未运行该类环境。

数据层在存储失败处记录操作和资源身份，已知持久化结果用 `errors.Join` 保留业务事实和原始错误；条件更新零行返回 `biz.ErrMutationMiss`。软删除绕过必须显式调用 `entgomixin.SkipSoftDelete`，参考 [GetUser](../../../../app/example/service/internal/data/user.go)。列表、字段清空和 mapper 构造见 [CRUD](crud.md)。
