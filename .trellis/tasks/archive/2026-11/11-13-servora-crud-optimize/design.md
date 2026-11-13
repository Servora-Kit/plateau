# 技术设计（简版）

关联需求：[PRD](prd.md)。

## 1. `servora/core/crud/mapper` 嵌套点路径映射

1. **构造期解析与校验（`NewResourceMapper`）**：
   - 将 `mapping.field.String()` 按 `.` 切分为段列表：
     - 单段路径：保持现有顶层 `copier.FieldNameMapping` 行为不变；
     - 多段路径（`len(parts) > 1`）：从根 `descriptor` 起逐段解析 `FieldDescriptor` 与 Go 结构体字段链 `[]reflect.StructField`：
       - 中间段必须是 singular 子消息（`Kind() == MessageKind && !IsList() && !IsMap()`），且排除 `google.protobuf.*` WKT 叶子类型（如 `Timestamp`、`Duration`）；
       - 末段 `FieldDescriptor.FullName()` 必须与 `mapping.field.Descriptor().FullName()` 一致，且通过 `typesMappable(source.Type, leafField.Type, converters)` 校验；
       - 将 `mapping.poField` 记入 `seenSources`（阻止顶层同名隐式拷贝误把如 `ent.User.Name` 拷到顶层 `IDENTIFIER` `User.Name`），将叶子 `target.FullName()` 记入 `seenTargets`，编译为不可变的 `nestedFieldProjection` 计划（预缓存反射索引路径与绑定的类型转换函数，避免热路径查找字段名）。
2. **运行期按需投影（`TryToDTO`）**：
   - 先执行顶层 `copier.CopyWithOption`；
   - 再遍历编译好的 `nestedFieldProjection`：读取 PO 源字段值（若源字段为 nil 指针/空切片，或经 converter 转换后为 nil 指针/零值且叶子字段具有 Proto presence，则跳过不创建空父消息）；当存在有效值时，沿父消息链按需初始化 `nil` 子消息指针并写入叶子字段；
   - 随后继续执行 `hooks`、`resourceName` 格式化与末尾 `proto.Clone`（保证零内存别名共享）。

## 2. `servora/cmd/protoc-gen-servora-crud` 结构化错误生成

- 在 `generate_go.go` 新增纯协议错误包引用 `crudv1Import = protogen.GoImportPath("github.com/Servora-Kit/servora/api/gen/go/servora/crud/v1")`（不引用 `core/crud`，符合插件依赖规则）。
- `Parse<Resource>Name` 未命中任何合法 pattern 时返回：
  `crudv1.ErrorCrudErrorReasonInvalidResourceName("invalid %s resource name %q", value)`。
- `Validate()`（及由其驱动的 `Format()`）中的非法段/未选 pattern 分支同样返回 `crudv1.ErrorCrudErrorReasonInvalidResourceName(...)`。

## 3. `plateau` 消费侧迁移与清理

1. **`app/iam/service/internal/data/user.go`**：
   在 `newUserResourceMapper()` 中删除 `WithPostToDTOHook`，使用 1 行强类型 `crudmapper.WithFlatten(userpb.UserFields.Profile)` 完成全部 7 个同名字段的按需投影。
2. **三服务 `service/user.go` 清理**：
   - `app/iam/service/internal/service/user.go`：删除私有 `parseUserName`，各方法直接调用 `userpb.ParseUserName(...)`。
   - `app/admin/service/internal/service/user.go`：删除私有 `parseUserName` 及 `UserService` 上仅用于 `ParseName` 的 `plan` 字段，各方法直接调用 `userpb.ParseUserName(...)`。
   - `app/example/service/internal/service/user.go`：简化 `parseUserName`（直接透传 `examplev1.ParseUserName` 的结构化错误，仅保留 `example` 特有的 `name.User == "-"` 通配符拦截；若无需单独方法也可内联）。
3. **文档与待办同步**：
   - 更新 `.trellis/spec/servora/framework/crud.md` 与 `.trellis/spec/servora/cmd/plugins.md`；
   - 移除 `plateau/TODO.md` 中已完成的 `## Servora CRUD 生态` 章节。
