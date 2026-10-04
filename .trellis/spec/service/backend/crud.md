# 公共 CRUD 消费机制

适用于使用生成 CRUD descriptor 的服务。descriptor、资源名、字段行为、FieldMask、错误、过滤、排序和分页契约以 [Servora CRUD 框架规范](../../servora/framework/crud.md) 为准；这里只定义应用消费边界。

## 一次构造，逐请求使用

- service 在构造期创建并复用 `ResourcePlan`、`ListPreparer` 和 `ResourceNameMatcher`；构造错误阻止启动，不在请求内重建合同。
- 解析资源名或 parent 后，由 biz 建立领域作用域。`ListPreparer.PrepareList` 将 `ListInput` 转成后端无关的 `ListQuery`，再交给 Usecase。
- `ResourcePlan.PrepareCreate`/`PrepareUpdate` 规范化资源和 FieldMask；biz 决定 allow-missing、etag、软删除与敏感输入语义。单资源与列表响应分别经过 `ToResponse`/`ToResponses`，不绕过响应清理。

## 查询与映射

- Repo 在构造期保存 `ListFields`、`ResourceMapper` 和 `ClearHelper`，每个合同的构造失败均返回错误。
- `Columns` 提供 Ent 列白名单；`Bind` 显式声明公共字段到列的映射及过滤、排序、nullable 能力，默认排序和 cursor key 也须显式配置。客户端字段名不直接拼入 SQL。
- `ResourceMapper` 用 `WithResourceName` 从持久化身份生成 canonical name；读取统一用 `ToDTO`/`ToDTOs`。
- `ClearHelper` 显式映射可清空字段到 mutation，按存储语义选择 `ClearToValue`/`RenameClear`，更新时执行 `Apply`。biz 向 Repo 传递 `PreparedUpdate.WriteMask` 的规范化结果，不直接下传原始 `update_mask`。
- data 先把领域作用域写入查询 builder，再调用 `entcrud.List`。scope fingerprint 仅绑定 page token 的查询上下文，不能代替数据库查询条件；映射结果回 `corecrud.NewListResult` 时保留与输入 query 一致的分页信息。

## 契约变更

字段变化须同步审查 descriptor、`ResourcePlan` 生命周期、`ListFields` bind、mapper、create/update setter 和 clear 行为；验证边界见 [测试](testing.md)。

