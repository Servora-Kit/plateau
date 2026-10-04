# 接口适配层

service 实现生成接口，嵌入 `UnimplementedXxxServiceServer`，持有 Usecase，不直接持有 Ent client。CRUD 合同在构造期创建并逐请求复用，响应统一经过 `ResourcePlan.ToResponse`/`ToResponses`。

本层校验请求格式和资源名，用 `ResourceNameMatcher` 解析作用域、`ListPreparer.PrepareList` 准备列表查询、`ResourcePlan` 规范化生命周期字段和 FieldMask。allow-missing、软删除、etag 冲突及授权选择由 biz 决定。

不将原始 RPC request 或 HTTP transport 类型传入 biz/data，也不新建无职责的 `Params` 包装。错误边界见 [共同编码](coding.md)，合同消费见 [CRUD](crud.md)。
