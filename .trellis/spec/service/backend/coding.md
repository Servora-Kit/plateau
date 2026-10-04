# 跨层编码约定

构造函数由 ProviderSet 装配，依赖通过参数传入，不隐藏在全局状态中。

请求 `context.Context` 从 transport 传至 Usecase 和 Repo，不用 `context.Background()` 替代。构造期固定初始化可使用独立 context；需要取消、超时或重试的后台工作由生命周期拥有者创建和关闭。

错误在拥有语义的层转换：data 用 `errors.Join` 保留可检查的持久化事实与原始错误，不构造公共 Proto/Kratos 错误；biz 输出领域或生成的应用错误；service 处理请求格式错误。同一失败不跨层重复记录。

外部操作失败由拥有操作和资源身份的边界用 `slog.Logger.ErrorContext`/`WarnContext` 记录。日志不得输出明文密码、令牌、密钥或未脱敏身份资料。
