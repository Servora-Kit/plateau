# IAM 浏览器会话与认证投影

IAM 的登录会话是可撤销认证事件；浏览器 cookie 与期限由 SCS 管理，业务记录只保存登录引用。`SessionUsecase.Create/Resolve/Logout` 在 [biz/session.go](../../../app/iam/service/internal/biz/session.go) 中通过 `SessionRepo` 原子管理登录及关联 OAuth 状态。

`authn.NewSessionAuthenticator` 从 SCS 的 `iam_login_id` 解析当前 IAM 用户与会话，映射为共享 `security.Actor`，再写入 request context；[authn/session.go](../../../app/iam/service/internal/authn/session.go) 把已撤销、禁用或非活跃身份归为无效凭据，依赖故障归为不可用。下游读取身份只能用 [authn.From](../../../app/iam/service/internal/authn/context.go)，不得伪造 context value 或只凭 cookie 声称已认证。

会话撤销在 data 事务中锁定用户，再撤销对应 login 和它关联的 OAuth token session，见 [sessionRepository.Revoke](../../../app/iam/service/internal/data/authn.go)。密码重置撤销全部用户登录及 OAuth session；已认证改密只保留当前 login session，但仍撤销所有 OAuth session，见 [ReplacePassword](../../../app/iam/service/internal/data/authn.go)。这些是凭据变更的当前失效边界，不能只删除浏览器 cookie 或只撤销 access token。

修改会话撤销、cookie、服务端 session 存储或登录回调时，检查 `internal/authn/session_test.go`、`internal/service/session_test.go` 以及 [data session 集成测试](../../../app/iam/service/internal/data/session_integration_test.go)。这些测试覆盖的层次应在变更说明中如实标注。

## 当前跨应用注销能力

IAM 当前登录退出通过 [SessionService.Logout](../../../app/iam/service/internal/service/session.go) 撤销当前 login 及关联 OAuth token session，并销毁当前 SCS 会话。OIDC `/end_session` 的 [TerminateSession](../../../app/iam/service/internal/oidc/op_auth_storage.go) 则按 `userID + clientID` 调用 `RevokeClient`，没有接入这条 IAM login 撤销路径。两者都尚未向接入应用发送退出通知，因此不具备终止关联业务应用本地会话的完整链路。
