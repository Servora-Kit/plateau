# 跨层编码约定

构造函数由 ProviderSet 装配，依赖通过参数传入，不隐藏在全局状态中。

请求 `context.Context` 从 transport 传至 Usecase 和 Repo，不用 `context.Background()` 替代。构造期固定初始化可使用独立 context；需要取消、超时或重试的后台工作由生命周期拥有者创建和关闭。

错误只在真正改变语义的层转换一次，禁止跨层二道中转：领域源服务的 `data` 层将底层数据库/存储驱动错误转为 `biz` 领域哨兵错误（`errors.Join` 保留 cause），不构造公共 Proto/Kratos 错误；编排型/BFF 服务的下游 RPC Repo 直接原样返回下游生成的结构化协议错误，不拆成无业务消费的中转哨兵变量；`service` 层以同文件无状态私有函数（如 `xxxError`）统一完成对外 Proto `ErrorReason` 编码、未知错误兜底脱敏及下游服务鉴权故障隔离（503）。同一请求失败由传输层日志中间件统一记录，业务/数据层不重复打印 `Error`。

外部操作失败由拥有操作和资源身份的边界用 `slog.Logger.ErrorContext`/`WarnContext` 记录。日志不得输出明文密码、令牌、密钥或未脱敏身份资料。
