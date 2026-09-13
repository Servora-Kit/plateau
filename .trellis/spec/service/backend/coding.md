# 跨层编码约定

构造函数在 ProviderSet 中装配，依赖通过参数传入，避免隐藏全局状态。Example 的 [data ProviderSet](../../../../app/example/service/internal/data/data.go)、[biz ProviderSet](../../../../app/example/service/internal/biz/biz.go) 与 [service ProviderSet](../../../../app/example/service/internal/service/service.go) 是当前样式。

`context.Context` 从 transport 传到 Usecase 和 Repo；后台生命周期由 bootstrap/组件显式创建。不要用 `context.Background()` 替代一条请求的 context。构造期的固定初始化可使用独立 context：Example 的 [NewDBClient](../../../../app/example/service/internal/data/data.go) 建 schema，IAM 的 [NewServiceAuthenticator](../../../../app/iam/service/internal/authn/jwt.go) 构造已注入 verifier 的认证器；这两者都不允许业务查询丢失请求 context。需要取消、超时或重试控制的后台工作由其生命周期拥有者创建和关闭。

错误只在拥有语义的层转换。data 返回可检查的持久化事实，例如 `errors.Join(biz.ErrNotFound, err)`；biz 解释 etag、allow-missing、软删除等业务结果并输出生成的应用错误；service 负责请求格式错误。参考 [Example data 约定](../../../../app/example/service/internal/data/AGENTS.md) 与 [UserUsecase](../../../../app/example/service/internal/biz/user.go)。不在 data 构造公共 Proto/Kratos 错误，也不对已由 data 记录的同一失败重复记录。

外部操作失败在拥有操作和资源身份的边界使用 `slog.Logger` 的 `ErrorContext`/`WarnContext`；Example data 的 [CreateUser](../../../../app/example/service/internal/data/user.go) 附带 tenant、user、err。日志不得输出明文密码、令牌、客户端密钥或未脱敏身份资料。
