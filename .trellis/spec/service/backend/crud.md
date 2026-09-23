# Servora CRUD 消费流程

适用于使用生成 CRUD descriptor 的业务服务。公共框架内部契约归 [Servora CRUD 框架规范](../../servora/framework/crud.md)，其源码证据见 [core/crud](../../../../../servora/core/crud/)；这里记录 Plateau 消费方式。Example 的 [UserService](../../../../app/example/service/internal/service/user.go)、[UserUsecase](../../../../app/example/service/internal/biz/user.go)、[userRepo](../../../../app/example/service/internal/data/user.go) 是完整、可追溯的参考。

资源 descriptor 的启用条件、多 pattern 资源名判别、`field_behavior`/字段设置状态、REQUIRED/IMMUTABLE、FieldMask 生命周期、标准 CRUD error reason、filter/order/page token/wildcard/cursor 契约由 [Servora CRUD 框架规范](../../servora/framework/crud.md) 权威定义。业务服务按这些已验证契约消费 `ResourcePlan`/`ListPreparer`，不在本正文复制框架内部规则，也不把 Example 的 tenant、etag、软删除和密码选择推广为通用要求。

## 一次构造，逐请求使用

在 `UserService` 中显式保存一次构造、逐请求复用的合同：

```go
type UserService struct {
  examplev1.UnimplementedUserServiceServer
  usecase       *biz.UserUsecase
  plan          *corecrud.ResourcePlan[*examplev1.User]
  listPreparer  *corecrud.ListPreparer
  parentMatcher *corecrud.ResourceNameMatcher
}

func NewUserService(usecase *biz.UserUsecase) (*UserService, error) {
	listPreparer, err := corecrud.NewListPreparer()
	if err != nil {
		return nil, fmt.Errorf("create User ListPreparer: %w", err)
	}
	parentMatcher, err := corecrud.NewResourceNameMatcher("tenants/{tenant}")
	if err != nil {
		return nil, fmt.Errorf("create User parent matcher: %w", err)
	}
	return &UserService{
		usecase:       usecase,
		plan:          corecrud.MustBuildResourcePlan[*examplev1.User](examplev1.UserCRUDDescriptor()),
		listPreparer:  listPreparer,
		parentMatcher: parentMatcher,
	}, nil
}
```

这与 [NewUserService](../../../../app/example/service/internal/service/user.go) 的逐项错误处理一致。请求进入后，`parentMatcher.Parse` 取得 `tenant`，由 biz 建立有领域语义的 scope；`listPreparer.PrepareList(plan, ListInput{...})` 生成 `ListQuery`，再交给 Usecase。`Get/Create/Update/Delete/Undelete` 分别用 `ParseUserName`、`PrepareCreate`、`PrepareUpdate` 和 `DeleteOptions`，从 Usecase 返回的资源一律用 `plan.ToResponse`；列表用 `plan.ToResponses`，不要绕过响应清理。

## data 的固定查询与映射合同

Repo 显式保存同样一次构造的字段、映射和清空合同；构造期验证失败即阻止应用启动：

```go
type userRepo struct {
  data       *Data
  listFields *entcrud.ListFields[*entmodel.User]
  mapper     *crudmapper.ResourceMapper[*examplev1.User, entmodel.User]
  clear      *entcrud.ClearHelper[*entmodel.UserMutation]
  log        *slog.Logger
}
```

Example 的 `NewUserRepo` 中，列表配置形状是：

```go
listFields, err := entcrud.NewListFields[*entmodel.User](
  entcrud.Columns(entuser.ValidColumn),
  entcrud.Bind(examplev1.UserFields.DisplayName, entuser.FieldDisplayName).Filter().Order(),
  entcrud.Bind(examplev1.UserFields.Nickname, entuser.FieldNickname).Filter().Order().Nullable(),
  entcrud.Bind(examplev1.UserFields.CreateTime, entuser.FieldCreateTime).Filter().Order(),
  entcrud.DefaultOrder(examplev1.UserFields.CreateTime, corecrud.OrderDescending),
  entcrud.CursorKey[int](entuser.FieldID, corecrud.OrderAscending),
)
if err != nil {
  return nil, fmt.Errorf("build User list fields: %w", err)
}
```

随后构造并写入 Repo 的 mapper/clear 合同；每个构造错误都在启动期返回：

```go
resourceMapper, err := crudmapper.NewResourceMapper[*examplev1.User, entmodel.User](
  crudmapper.WithResourceName(func(value *entmodel.User) (string, error) {
    return examplev1.NewUserName(value.TenantID, value.ResourceID).Format()
  }),
)
if err != nil {
  return nil, fmt.Errorf("build User resource mapper: %w", err)
}
clear, err := entcrud.NewClearHelper(
  entcrud.ClearToValue(examplev1.UserFields.DisplayName, func(mutation *entmodel.UserMutation) error {
    mutation.SetDisplayName("")
    return nil
  }),
  entcrud.RenameClear[*entmodel.UserMutation](examplev1.UserFields.TemporaryPassword, entuser.FieldPasswordHash),
)
if err != nil {
  return nil, fmt.Errorf("build User Clear helper: %w", err)
}
return &userRepo{
  data: data, listFields: listFields, mapper: resourceMapper, clear: clear,
  log: logger.With("scope", "example/data/user"),
}, nil
```

`Columns` 提供生成 Ent 表的列白名单；每个 `Bind` 明确 public field 到 Ent column 的可过滤/排序和 nullable 能力，不能把客户端字段名直接拼入 SQL。`ResourceMapper` 的 name formatter 从持久化对象的 tenant/resource ID 生成 canonical public name，读取路径直接使用 `repo.mapper.ToDTO`/`ToDTOs`。`ClearHelper` 显式把可清空字段映射到 mutation：Example 对 `display_name` 使用 `ClearToValue`，对临时密码使用 `RenameClear`，并在更新前调用 `repo.clear.Apply(resource, mask, builder.Mutation())`。

业务层持有 scope、allow-missing、etag、软删除和密码哈希语义。[`PreparedUpdate.WriteMask`](../../../../../servora/core/crud/lifecycle.go) 返回已由 `ResourcePlan.PrepareUpdate` 规范化的 `*fieldmaskpb.FieldMask`；biz 将该值传给 Repo，data 不应把原始 RPC `update_mask` 原样下传。列表执行时，data 先把业务作用域写入 Ent builder，再传查询合同和分页 scope fingerprint：

```go
builder := repo.data.Ent(ctx).User.Query().Where(entuser.TenantIDEQ(scope.TenantID()))
result, err := entcrud.List(ctx, builder, query, repo.listFields, scope.Fingerprint(includeDeleted))
```

`builder.Where(...)` 执行数据库 tenant 筛选；`scope.Fingerprint(...)` 只将该业务上下文绑定到 page token 校验，不能代替查询条件。返回时将 Ent list result 映射回 `corecrud.NewListResult`，使 page token 与输入 query 一致。完整实现在 [NewUserRepo 与 ListUsers](../../../../app/example/service/internal/data/user.go)。

## 检查

新增字段必须同步审查 Proto descriptor、`ResourcePlan` 生命周期规则、`ListFields` bind、mapper、create/update setter 和 clear 行为。先运行应用 CRUD 测试，再按改动范围运行框架的 [list fields 测试](../../../../../servora/contrib/db/entgo/crud/list_fields_test.go) 或合同集成测试；未运行数据库环境时只能称静态或单元验证。
