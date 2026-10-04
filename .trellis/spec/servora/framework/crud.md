# CRUD 框架内部契约

适用于 `core/crud`、`core/crud/mapper` 和 `contrib/db/entgo/crud`。应用层组合方式见 [服务 CRUD 规范](../../service/backend/crud.md)，这里仅定义框架能力。

`ResourcePlan` 是 AIP 资源的不可变运行时描述，负责 canonical name 校验、生命周期分区和 `ToResponse` 的 clone/INPUT_ONLY 清理。`ListPreparer` 预先校验配置并将列表输入解析为 `ListQuery`；`ResourceNameMatcher` 只接受 canonical、未转义的相对资源名。core 不处理 URL 编码或 repository 查询编排。

`MustBuildResourcePlan` 只接受与泛型资源类型一致、带 `google.api.resource`、具有非空 type 和至少一个 pattern 的 descriptor；资源必须恰有一个 singular string `IDENTIFIER`。保留全部 pattern，由 matcher 判别，不能任选 multi-pattern 的首项。框架 CRUD reason 只表达资源名、page token、filter、order、mask、字段值和内部不变量错误；存储事实与业务语义由应用错误表达。

`NormalizeWriteMask` 的显式 mask 只接受已声明路径，去重并按 canonical path 排序，拒绝 `*` 与其他路径并存及祖先/子路径重叠。省略 mask 时按 Proto presence 选择顶层字段：list/map 必须非空，具有 presence 的字段使用 `Has`，其他 scalar 只在非零值时选入；省略 mask 不能表示 scalar clear。`PrepareUpdate` 排除 system 字段，将 mutable leaf 写入 mask、immutable 字段转为比较意图。

`ListQuery` 保存 collection、page size/token、skip、filter、order 和 include-total。`PrepareList` 校验页大小/skip、token 解码及 filter/order 的语法与资源限制，不决定查询 scope 或最终排序。adapter 在数据库访问前以 resource type、collection、filter、`FinalOrder` 和 opaque scope fingerprint 构造 context fingerprint；`ValidatePageTokenPayload` 校验 token version、query fingerprint、cursor 数量和类型，禁止跨查询、排序或 scope 复用。

`core/crud/mapper` 是 ORM 无关的 PO→资源 PB 单向读投影。`NewResourceMapper` 验证并冻结同名/显式字段映射、converter、canonical name formatter 和 post hook；`TryToDTO` 返回错误，`ToDTO`/`ToDTOs` 遇映射错误 panic。写入 setter、mutation、clear 与业务校验由 repository 负责。

Ent adapter 将已解析 `ListQuery` 绑定到调用方建立的 Ent builder；`ListFields` 只开放 repository 显式声明的字段，禁止扫描 schema 自动暴露字段。adapter 不开启、提交或传播事务，不计算授权或 scope。`ClearHelper.Apply` 在 Ent `Save` 前消费规范化 mutable leaf mask：仅 singular、`HasPresence()==true` 且 `Has(field)==false` 时执行同名 `ClearField` 或 override，无 presence 字段报错。nested clear 必须显式配置；present 值和 list/map empty replacement 留给 repository setter。

`NewListFields` 和 `NewClearHelper` 在启动期验证并冻结配置。SQL 必须经 Ent selector/dialect builder 生成，不拼接客户端 filter/order 文本。检查入口：`go test ./core/crud/...`、`go test ./contrib/db/entgo/crud/...`；真实数据库检查使用专用测试环境。

跨 Go/TS 的语义使用共享 [string_matches.json](../../../../../servora/conformance/crud/string_matches.json) 和 [resource_names.json](../../../../../servora/conformance/crud/resource_names.json) 一致性向量；变更语义或向量时检查两侧消费者，不能只更新一侧预期。
