# service 层

service 实现生成接口，嵌入 `UnimplementedXxxServiceServer`，持有 Usecase，不直接持有 Ent client。Example 的 [UserService](../../../../app/example/service/internal/service/user.go) 给出完整形状：`NewUserService` 一次构造不可变 CRUD 合同，RPC 方法解析 name/parent、准备输入、调用 biz，最后调用 `plan.ToResponse` 或 `plan.ToResponses`。

请求在本层完成格式和资源名校验。`ListUsers` 用 `ResourceNameMatcher` 取得 scope，再通过 `ListPreparer.PrepareList` 生成后端无关查询；`CreateUser`/`UpdateUser` 用 `ResourcePlan` 规范化生命周期字段和 FieldMask。业务的 allow-missing、软删除、etag 冲突和权限选择仍由 biz 决定。

不要将原始 RPC request 或 HTTP transport 类型传入 biz/data，也不要在服务层新建没有职责的 `Params` 包装。data 不构造公共 Proto/Kratos 错误；biz 可以按 [共同编码](coding.md) 将领域和持久化结果映射为生成的应用错误，Example 的 [translateRepoError](../../../../app/example/service/internal/biz/user.go) 是当前例子。CRUD 具体构造见 [CRUD](crud.md)。
