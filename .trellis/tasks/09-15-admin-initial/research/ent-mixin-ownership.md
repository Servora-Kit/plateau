# Ent 软删除便利层的归属与验证

日期：2026-09-16。用户提出将 Servora Ent mixin 移到 Plateau infra。本轮核对源码并执行现有测试，未迁移或删除产品代码，未修改 spec。下述迁移纳入规划，仍待整套任务实施评审。

## 事实与依赖

- Servora 的 [mixin 目录](../../../../../servora/contrib/db/entgo/mixin/) 目前只有 soft_delete.go 与 soft_delete_test.go；不是一组未分类的 Ent 工具。[实现](../../../../../servora/contrib/db/entgo/mixin/soft_delete.go):16-175 提供三个 tombstone 字段、索引、默认过滤、Delete 改写、显式 bypass 与 deleted_by context，并通过反射适配生成 Ent query/mutation。
- [单元测试](../../../../../servora/contrib/db/entgo/mixin/soft_delete_test.go):12-134 使用 fake query/mutation 验证字段、过滤、删除改写和 bypass；这些测试本身不证明真实生成 Ent 的兼容性。
- `core/crud` 的生产代码不依赖 Ent/mixin；[Ent CRUD adapter](../../../../../servora/contrib/db/entgo/crud/list_execution.go):22-120 也不 import mixin。CRUD 的生产运行时可以独立保留。
- CRUD 的 [测试 schema](../../../../../servora/contrib/db/entgo/crud/testdata/entfixture/schema/contract_row.go):13-44 使用 SoftDeleteMixin；[live contract](../../../../../servora/contrib/db/entgo/crud/live_contract_integration_test.go):599-630 验证软删除、读取 bypass、恢复及 actor。因此“CRUD 生态完全没有依赖”不准确，测试与生成夹具仍需调整。
- Plateau [Example schema](../../../../app/example/service/internal/data/ent/schema/user.go):18-45 已实际使用该 mixin；[Example repository](../../../../app/example/service/internal/data/user.go):106-143、213-252 使用 SkipSoftDelete，删除依赖 hook 改写。IAM 与 Admin 当前尚未消费该 mixin，不能以本次测试宣称 IAM 生命周期已验收。

## 本轮实际验证

在 `../servora` 使用 Go 1.27.1、`GOWORK=off` 执行：

```bash
rtk proxy env -u GOROOT GOWORK=off go test ./contrib/db/entgo/mixin ./contrib/db/entgo/crud -count=1
rtk proxy env -u GOROOT GOWORK=off SERVORA_ENT_SQLITE_DSN='file:admin_mixin_review?mode=memory&cache=shared&_fk=1' go test -count=1 -tags=integration ./contrib/db/entgo/crud -run '^TestSQLiteLiveContract$' -v
```

两个包测试均通过；SQLite live contract 的全部子测试通过，包含 `soft_delete` 与 `native_error_chain`，没有 SKIP。PostgreSQL live 未执行。该结果证明现有实现有实际验证，不证明未来 IAM 的删除事务、并发恢复或清理已完成。

## 本次归属调整

按用户提出的方向，计划将共享软删除便利能力放在 Plateau `infra/entgo/mixin`。这是共享能力所有权调整，不是以“完全没测试”为由删除，也不是在 Plateau 留一个掩盖框架问题的长期补丁。

1. 迁移现有实现与单元测试，优先保持既有 Example 行为；本次不顺带重写整套反射适配或创造通用 ORM 框架。
2. 更新 Example schema/repository 的 import，并按源 schema 重新生成 Ent；保留已有软删除、恢复、查询范围和错误语义验证。IAM 后续按主 design 消费平台便利层，领域恢复期、关联撤销、锁与 purge 事务仍由 IAM 拥有。
3. Servora 保留 Ent driver、`core/crud`、Ent List/Clear adapter。其 fixture 改为测试自用字段与显式查询范围，继续覆盖 CRUD 接收预设 scope 的合同；软删除 hook/bypass 的专属验证随能力迁到 Plateau。不能让 Servora 生产代码或测试 import Plateau，也不能仅删测试来得到绿灯。
4. 完成消费迁移、fixture/生成更新与两仓回归后，再移除 Servora 的公开 mixin 包；活跃文档与规范在实施验证后按新归属同步。历史任务证据可保留原路径并标明旧状态。记录依赖版本/提交和包路径迁移，不自动发布版本。

当前 spec 与 AGENTS 中旧归属只作为现状证据；用户本轮方向已记录在本任务，规划期间仍不改 spec。
