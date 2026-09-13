# 跨层契约检查

当改动经过 Proto、生成代码、service/biz/data、共享 client 和页面中的多个边界时，先列出真实读写路径，再定位每项规则的权威所有者。

## 检查顺序

1. 入口：字段来自用户请求、生成契约还是可信内部配置；不要混淆信任来源。
2. 规范化：资源名、presence、FieldMask、enum、时间和 int64 的表示在哪一步转换；数据过边界后是否仍保留原语义。
3. 业务：身份和资源 scope 由谁建立，etag/软删/allow_missing 由谁决定；框架 helper 不应暗中承担业务授权。
4. 存储：公共字段到列的绑定是否显式，事务和资源生命周期是否有单一负责人。
5. 输出：INPUT_ONLY 是否清理、错误事实是否稳定，页面是否仍拥有交互与文案。
6. 验证：用缺失/零值/非法值/取消/错误与往返行为检查关键边界，避免仅用构建成功推断端到端正确。

## 本项目的检查入口

- 安全变更组合 [API annotations](../api/proto/annotations.md)、[codegen](../plateau/codegen/index.md)、[AuthN](../plateau/security/authN.md)／[AuthZ](../plateau/security/authZ.md)。声明、生成、装配三者分别确认。
- CRUD 变更组合 [API contracts](../api/proto/contracts.md)、[service CRUD](../service/backend/crud.md)、对应业务规范和前端请求主题。共同检查 read/write mask、绑定、mapper 和响应清理。
- Web 契约组合 [生成归属](../api/proto/generation.md)、[shared client](../web/client/index.md) 与应用自己的状态/路由规范；不能把 Example 自有 fetch 的现状假定成所有应用共用 client。

发现冲突先给出真实文件、输入和可观察结果。区分推荐规范、现状与尚未验收能力；不要仅凭目录名称、文档标题或存在一个测试文件推断功能已可用。
