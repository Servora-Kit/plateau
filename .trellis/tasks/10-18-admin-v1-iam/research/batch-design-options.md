# IAM 剩余设计批量选择

用户已选择 1B、2A、3A、4A；选定行为已转入父子 PRD 和 IAM design。本文件保留选项对比及证据，不授权实现。下列未选项不再作为待定设计。

## 1. 管理创建与验证邮件

共同条件：CreateUser 保持返回 User；身份与初始凭据事务失败不残留账号；账号提交后邮件失败保留账号并向 Admin 显示失败阶段，不自动重试或回滚已创建身份，不新增密码交付功能。

### 1A：CreateUser 内发信，未采用

- IAM 创建账号，随后复用现有验证令牌与邮件能力发送验证邮件；全部成功返回 User。
- 账号已经提交后，令牌准备、持久化或发信明确失败时，返回专用领域 reason，例如 USER_CREATED_VERIFICATION_FAILED，ErrorInfo.metadata 携带 resource_name。错误只用于已知已提交阶段，不将一般数据库失败或响应丢失推断为已创建。
- Admin 收到此 reason 后调用现有 GetUser(resource_name)，展示已创建身份及邮件失败；GetUser 也失败时仍显示已创建资源名与查询失败，不重新创建。
- 正常路径一个管理 RPC；邮件失败时一次额外查询。客户端必须识别“账号已创建，验证步骤失败”，不能把所有非 OK 响应都显示为创建失败。
- 保持现有 CreateUser 调用会发信的行为，管理 Proto 不新增发信方法；错误映射增加一个领域结果。

### 1B：创建与发信分成两个管理 RPC，已选择

- UserService.CreateUser 只完成账号与初始凭据创建并返回 User。
- Admin 得到 User 后调用 UserService.SendUserVerificationEmail(name)，IAM 发送验证邮件；返回空成功或发信错误。
- Admin 已持有创建结果，因此邮件失败无需再查询即可展示部分成功；正常路径有两次管理调用，多一个 IAM 管理 RPC。
- 新方法通过服务 JWT、manage_users 授权；内部复用验证令牌及发信逻辑。现有 AccountService.ResendVerificationEmail 要求 CAP，不能由 Admin 伪造 CAP 或放宽其本人入口来复用。
- 自助注册保留原有自动发信安排；仅管理 CreateUser 移除发信副作用。PRD 已明确 Admin 负责两步编排，直接调用管理 CreateUser 的调用方须显式调用发信方法。

不列出修改 CreateUserResponse 包装、忽略发信失败、队列异步投递等方案，它们与已有决定冲突。

## 2. 删除／恢复是否携带页面版本

ExpungeUser 继续只接收 name，不重问其契约。两种选择都在事务中锁定身份、核验当前删除状态；差别是是否额外检查操作者看到的资源版本。

### 2A：仅 name，已选择

- DeleteUserRequest、UndeleteUserRequest 仅携带 name。
- 操作作用于提交时的当前记录；资料或动作状态在页面打开后变化，不额外阻止删除／恢复。
- 身份锁和状态检查负责服务端并发一致性，但不检查陈旧页面意图。

### 2B：name 加可选 etag，未采用

- DeleteUserRequest、UndeleteUserRequest 增加可选 etag，非空时必须与当前值匹配；Admin 使用最近读取的 User.etag。
- 不匹配返回并发冲突，页面重新读取后由操作者再次提交；不自动重试。
- 增加版本参数与过期页面交互，可阻止基于旧版本的删除／恢复；首次改密等会改变 User.etag，因此也可能让原页面操作失效。

依据：[AIP-135](https://google.aip.dev/135) 与 [AIP-164](https://google.aip.dev/164)。可选 etag 不替代事务锁，不扩大到本期未选定的其他方法。

## 3. 删除不存在／已删除目标

此选项仅针对 DeleteUser。UndeleteUser 对未删除目标返回 ALREADY_EXISTS、对不存在目标返回 NOT_FOUND；ExpungeUser 对不存在目标返回 NOT_FOUND，对未软删除目标返回 FAILED_PRECONDITION。

### 3A：返回 NOT_FOUND，不提供 allow_missing，已选择

- DeleteUser 目标不存在或已经软删除时均返回 NOT_FOUND。
- UI 显示当前记录已不存在或已删除，随后由正常列表刷新反映状态。
- 不增加“没有执行删除也返回成功”的请求分支。

### 3B：增加 allow_missing，未采用

- 缺省 false，沿用 3A；显式 true 时，不存在或已经软删除均按成功处理。
- 已软删除且记录仍在时返回该 User；目标完全不存在时返回空 User，不伪造身份字段。调用方须接受无资源载荷的成功结果。
- 适用于调用方只关心最终删除状态的操作，但增加请求选项及成功响应形态。
- 权限始终先验证；未知目标的成功不构成绕过权限检查。

依据：[AIP-164 错误语义](https://google.aip.dev/164)。本期不包含声明式资源协调器，已有需求也未要求 allow_missing。

## 4. 永久删除后的 SCS 存储行

共同条件：目标 IAM 登录和 OAuth 身份关联数据在 Expunge 事务中清除，旧浏览器 cookie 不能再解析出有效身份；不影响其他账号，不联动其他应用本地会话。

### 4A：失效会话容器由现有 TTL 清理，已选择

- Expunge 删除 IAMLoginSession 等身份记录；普通会话解析找不到登录引用，立即拒绝旧 cookie。
- SCS sessions 行保留至其原有过期时间，由现有 PostgreSQL Store 清理；不扫描所有序列化 data。
- sessions 没有 user_id，此方案无需新增索引、映射表或 Store adapter。
- 残余 SCS 行及其中的无效登录引用在 TTL 清理前仍可能存在。父子 PRD 已明确此物理留存例外；Expunge 清理 IAM 身份和令牌记录，旧 cookie 立即失去身份解析能力。

### 4B：Expunge 同步删除关联 SCS 行，未采用

- 建立 SCS token 与 IAM 用户的可靠关联，并在 Store commit/delete、token 轮换和账号删除中维护。
- Expunge 清理目标用户对应的 sessions 行；并发请求的晚到 Commit 不能重新保存已永久删除用户的绑定，需同步复验并协调锁定。
- 满足会话容器立即物理删除，但涉及 Store adapter、关联索引和并发验证，超出现有 sessions(token,data,expiry) 接线。

不采用请求内解码扫描整张 sessions 表的方案；它随全站会话数增长，而且不能单独解决并发保存问题。

## 5. 按既有规则推进的具体设计

下列为沿用既有规则的实现设计建议；四项选择不构成产品代码实施授权。

- 自助删除：AccountService.DeleteAccount，参数 current_password，身份来自普通登录，空响应；HTTP 使用 POST /v1/iam/account/delete。业务层比较密码，事务复验凭据、软删除、撤销旧登录及 OAuth 状态，完成后销毁当前浏览器绑定。
- 管理删除与自助删除共用 IAM 包内删除命令与 Repo 事务，不反向注入 UserUsecase 至 AccountUsecase，不新增生命周期服务。
- Expunge 权限：OpenFGA 在 iam 对象增加 expunge_users: [service]，与 manage_users 分别授权；Admin 的服务身份显式配置两种关系。唯一管理员识别仍由 Admin 后端执行，不新增人的角色配置。
- 永久删除：单用户单事务，按用户关联 ID 清除 human access/refresh token、授权码和已绑定身份的授权请求、OAuth token session、IAM 登录、验证/恢复令牌、凭据、登录标识及 User。保留共享 OAuth client、签名密钥、独立 service token 和其他服务数据；邮箱最后随事务提交释放。SCS 行按第 4 项决定。
- 软删除同步使旧验证/重置令牌不可用，恢复不重新启用这些令牌；待验证用户恢复后从现有本人入口重新申请验证邮件。
- 首次改密与邮箱重置：沿用已选的凭据重验和事务提交规则；错误 reason 区分凭据错误、密码规则、状态冲突、依赖失败，不引入无关业务分支。
- 错误转换：研究中已发现 Kratos 的 HTTP 409 映射可能合并不同 gRPC code。必须保留指定 gRPC code 与 ErrorInfo.reason，不能仅新增同 HTTP code 的 reason 便宣称完成；具体改动按 transport 所有权设计，不要求用户在正确错误与错误映射之间选择。
- JSONB 迁移、公开枚举编号、mapper、Repo 方法签名与验证命令按现有规范确定；不把内部命名作为产品决策反复确认。OAuth 客户端登录实现属于 Admin 后端，此批不扩展 IAM 职责。

## 6. 文献与源码依据

- CreatePendingUser 当前先写身份，再写验证令牌并同步发信；发信失败返回 nil/error：app/iam/service/internal/biz/account.go:86-130。UserService.CreateUser 当前直接映射 error：internal/service/user.go:39-64。
- 公开重发入口要求 CAP：internal/biz/account.go:148-181。不能直接把它用于服务凭据调用。
- SCS schema 只有 token/data/expiry：internal/data/schema/http_session.go:15-25；NewHTTPSessionManager 注入 postgresstore.New：internal/data/data.go:113-124。
- OpenFGA 当前只有 manage_users: [service]：manifests/openfga/iam.fga:7-9。
- Kratos v3.0.0 的 errors.Error.WithMetadata/GRPCStatus/FromError 通过 ErrorInfo 传递 metadata；创建错误携带资源名的 1A 方案未采用。
- Graph 查询绑定 Plateau，索引时间 2026-10-17；CreateUser 引用为 lower-bound，5 个调用点接收者类型未解析。当前说明以所读源码为准，不作完整影响面声明。
- 已核对 [AIP-133](https://google.aip.dev/133) 和 [AIP-136](https://google.aip.dev/136)：Create 用于创建资源并返回该资源，自定义动作表达标准方法不适用的操作；这两篇没有规定每个 RPC 只能执行一项底层操作，也没有针对创建用户后的邮件作统一禁止。创建与发送验证邮件分离是本项目已选择的职责划分；一次调用本身不提供数据库与 SMTP 的跨系统事务保证。

四项选择已同步；后续补齐错误映射、Repo 接线与字段迁移，任务仍为 planning。
