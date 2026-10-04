# 持久化层

仓储文件顺序：import → 私有 `xxxRepo` → `NewXxxRepo` → 实现 `biz.XxxRepo` 的方法。

`data.go` 只承担共享资源、`NewData`、cleanup 和 ProviderSet；资源清理见 [启动装配](bootstrap.md)。Repo 用显式 Ent setter 写入，以 mapper 映射读取结果；仅在有恢复分支时使用 Try 变体。

Ent schema 是应用持久化模型的唯一来源，明确 optional/nillable、immutable、default、sensitive 与唯一约束；CRUD adapter 不代替领域选择。

共享软删除使用 Plateau `infra/entgo/mixin`：`SoftDeleteMixin` 提供 tombstone 字段、默认查询过滤和 Delete/DeleteOne 改写；`SkipSoftDelete` 用于显式 tombstone 读取与硬删除。`show_deleted` 对应的可见性须写入 page-token scope fingerprint。

跨实体不变量或并发锁定由 data Repo 的事务保护，确保失败或 panic 回滚、成功提交；不自动为简单单实体操作增加事务。存储语义应在所选 dialect 的真实环境验证，见 [测试](testing.md)。

条件更新零行须返回可检查的并发失败结果。错误和日志边界见 [共同编码](coding.md)；列表、映射和字段清空见 [CRUD](crud.md)。
