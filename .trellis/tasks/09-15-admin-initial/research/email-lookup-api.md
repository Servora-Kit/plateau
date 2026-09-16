# IAM 邮箱查询 API 方案

任务处于规划阶段。本文记录 2026-09-15 查阅的 AIP 原文与当前代码边界，推荐方向由后续 design 具体化，不修改 spec 或实现。

## 当前接口

`UserService.GetUser` 按 `users/{user_id}` 查询；`ListUsers` 已有 `filter`，但当前仓储字段绑定没有 email。内部 `FindByEmail` 已通过规范化后的 LoginIdentifier 查找 stable user ID。源码锚点见 [初始化调研](bootstrap-initialization.md)。用户询问是否应新增 `GetUserByEmail`，并明确可在优雅、易用的前提下不严格遵循 AIP。

## 官方依据

- [AIP-131 Get](https://google.aip.dev/131)：标准 Get 以资源 name 为必需字段，返回该资源；不适合将 GetUser 改成 name/email 任意二选一。
- [AIP-132 List](https://google.aip.dev/132) 与 [AIP-160 Filtering](https://google.aip.dev/160)：集合查询可通过 string filter 表达字段条件；字段来自资源，不要求 API 暴露数据库实际表结构。email 的规范化与支持的运算符仍需由服务明确文档化。
- [AIP-136 Custom methods](https://google.aip.dev/136)：优先使用能够自然表达需求的标准方法；自定义方法采用动词与名词，避免 Get 等标准动词及 By 等介词。原文用 GetBookByAuthor 作为属性专用查询容易使 API 膨胀的例子。

## 推荐：复用 ListUsers 的邮箱精确过滤

请求示意：`ListUsers(filter='email = "admin@example.com"')`。这表示新增能力，不是当前已可调用的查询条件。

保留 GetUser(name) 的唯一职责，把邮箱作为 ListUsers 已有过滤语言的一种明确查询条件。Admin 初始化的邮箱定位与用户列表的邮箱查询可以共享该能力，不为一个字段再增加 RPC。

设计需明确以下行为：

- IAM 负责邮箱规范化，精确匹配与登录/唯一性判断使用相同 canonical email 语义；调用者不能自行按显示字符串比较来建立授权关系。
- 本用途必须使用完整邮箱的精确条件，不能用模糊搜索结果、任意第一条或用户输入直接拼接 filter 后自动授予权限。filter 字符串应正确转义；按 stable user ID 绑定。
- 邮箱唯一性意味着逻辑匹配应为零或一个用户。List 的无匹配表现为空列表，权限或查询失败必须作为错误处理，不能伪装成不存在；遵循分页协议，不能把仍有 next_page_token 的空页等同查无结果。
- 默认按本任务软删除规则排除已删除用户；初始化遇到不存在或不可用身份应停止本次授权并按既定初始化重试策略处理，不能新建 Admin 身份或猜测目标。
- 禁止客户端拉取全部用户后过滤。服务端应通过邮箱标识查询表达式完成匹配；具体如何接入 Servora CRUD 过滤能力由 design 查明，不假设 email 能直接绑定到 User 数据表列。
- 复用现有管理服务授权边界；当前不因此增加匿名邮箱枚举接口或更改 GetUser 的资源名格式。

## 备选：LookupUser

如果后续确实需要请求明确接收 email、响应为单个 user，并在不存在时返回 NOT_FOUND 的独立操作，可用自定义 `LookupUser(LookupUserRequest) returns (LookupUserResponse)`，响应包装 user。该名称比 `GetUserByEmail` 更贴近 AIP-136 的自定义方法词汇；集合查找的 HTTP 映射与全部注解在需要时另行设计。

这属于对 AIP 指南的设计应用，不是 AIP 要求必须增加 LookupUser。当前已有 ListUsers 且 Admin 本身需要集合查询，推荐先扩展 filter。内部仓储方法保留 FindByEmail，不要求内部 Go 方法名照搬公共 RPC 名称。
