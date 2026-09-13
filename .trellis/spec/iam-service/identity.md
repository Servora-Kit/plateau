# 身份与账号

IAM 拥有用户和服务身份的目录事实及认证流程，不替业务服务计算其资源权限。`biz.NormalizeEmail` 会 trim、NFC 规范化并大小写折叠比较值；`NewUserID` 生成不可由调用方复用的 UUIDv7，见 [identity.go](../../../app/iam/service/internal/biz/identity.go)。新增身份字段或唯一性规则应先在 biz/data/API 三处核对，不在 transport 层做持久化决定。

账号创建保存规范化前的 display email，并以 canonical email 查重。密码由 [security/password](../../../security/password/password.go) 生成带新随机 salt 的 Argon2id PHC hash，明文不得进入 Repo 或日志。注册在 [AccountUsecase.Register](../../../app/iam/service/internal/biz/account.go) 中先消费 CAP，再创建 pending user、持久化验证令牌并发送邮件；CAP 是副作用和账号查询之前的门槛，不把它放到创建/发信之后。

公开重发验证和请求密码重置同样先验证 CAP。CAP 成功后，格式错误、未知邮箱和不合资格账号都返回空成功，避免向调用者泄露账号是否存在或已经可用；注册的“邮箱已注册”错误是当前不同的显式注册语义，不能把这条抗枚举规则泛化到注册。密码重置只为已激活且邮箱已验证的用户创建令牌，见 [AccountUsecase](../../../app/iam/service/internal/biz/account.go)。

确认密码重置通过 [ConsumeAndReplacePassword](../../../app/iam/service/internal/data/account.go) 在一个事务内重新检查并消费令牌、替换密码，并撤销该用户的全部 IAM 登录与 OAuth token session。此语义不同于已认证的改密：改密保留当前 login session、撤销其他登录及所有 OAuth session。修改恢复流程时不得拆开令牌消费、密码写入和撤销步骤。

身份状态参与会话解析：`SessionUsecase.Resolve` 会拒绝撤销会话、缺失用户、disabled、非 active 或未验证邮箱的用户，见 [session.go](../../../app/iam/service/internal/biz/session.go)。因此状态转换的修改必须同时审查用户查询、会话解析、OIDC 授权回调和相关测试。

用户的认证身份与业务资源授权分开：能解析 IAM user 或服务 Actor 只说明身份已验证，不说明业务服务允许某个动作。
