# IAM 凭据、验证邮件与登录衔接实施设计

本文补充 [design.md](../design.md) 的具体接口和源码改动安排，仅用于技术规划，不表示产品代码已经修改。产品约束以 [prd.md](../prd.md) 为准。

## 1. 已有实现与需要修改的位置

源码目录下文以 `app/iam/service/` 为服务根、`app/iam/web/` 为 Web 根。

| 当前实现 | 具体问题与设计落点 |
| --- | --- |
| `internal/biz/account.go:75-130`：Register 消费 CAP 后调用 CreatePendingUser；该方法创建聚合，再生成验证 token、写库、同步发送邮件。 | 管理 CreateUser 经 `internal/biz/user.go:49-51` 走同一方法，所以当前管理创建也发信。将 CreatePendingUser 保留为纯聚合创建，Register 显式继续调用共用发信步骤；管理 CreateUser 不再发信。不能把 Register 改成只创建。 |
| `internal/biz/account.go:148-181`：公开重发先消费 CAP，不合资格或未知邮箱返回空成功。 | 管理发信不能调用公开重发、伪造 CAP 或放宽 CAP。增加按 userID 的内部发信方法，共用私有令牌与邮件步骤。 |
| `internal/data/account.go:27-83` 和 `internal/data/user.go:323-341`：验证 token 创建无用户锁；Consume 独立提交后才 ActivateEmail，ActivateEmail 未锁用户。 | token 消费与激活必须合为一个事务；创建、消费、删除、恢复均按用户锁串行化。否则可能消费后激活失败，或删除已经失效的 token 被晚到请求重新创建。 |
| `internal/data/account.go:96-105`：reset token 创建不检查当前用户资格；`108-175` 的消费事务已有用户锁，但只检查 ACTIVE。 | 创建加资格与读取版本复验；消费加已验证邮箱、完整凭据快照和动作复验，不能只在请求开始检查状态。 |
| `internal/biz/authn.go:64-72` 验密后只把 userID 交给 SessionUsecase；`internal/data/authn.go:85-105` 创建登录只检查 ACTIVE。 | 管理设密可在 Compare 与 Create 之间提交，旧密码仍能晚建新登录。SessionRepo.Create 接收已验密的不可变快照及用户 etag，并在同一用户锁中重新检查。 |
| `internal/biz/session.go:54-81` Resolve 检查登录撤销、当前状态及验证事实。 | 增加 required_actions 非空拒绝，沿用统一会话解析与 Actor 投影，不建立第二套认证状态。 |
| `internal/data/oauth.go:26-137` code/refresh 换 token 已有用户锁与登录复验。 | 在现有事务中补充未删除、邮箱已验证及 required_actions 为空；保留服务 token 分支，不对独立服务身份套用户要求。 |
| `internal/oidc/op_auth_storage.go:90-168` 直接写授权请求绑定和授权码。 | Resolve 后仍可能发生删除、撤销或动作变更。两项写入转交现有 OAuthRepo 的用户锁事务，不能只靠回调开始时的 Resolve。 |
| `internal/service/authn.go:34-45` 要求 SCS 已加载，成功后 RenewToken 并 Put 登录引用。 | 支持 `User + nil LoginSession` 的待改密结果，清除旧浏览器绑定且不创建新绑定；首次改密成功后沿用同一 SCS 接线。 |
| `web/app/login/page.tsx:51-59` 任意 Login 成功立即导航。 | 先检查响应 User.requiredActions；有 UPDATE_PASSWORD 时进入同页首次改密表单，不立即访问 account 或 OIDC callback。 |

调用路径先核对 Plateau GitNexus（索引 commit `ff6d8edb`，2026-10-17），再以当前源码及 LSP references 核对。CreatePendingUser 的业务调用者是 Register 和 UserUsecase.CreateUser；SessionUsecase.Create 的产品调用者是 AuthenticationUsecase.Login。图索引不能代替接口分派和生成客户端的实际源码。

## 2. User 动作、存储及资源版本

### 2.1 Proto 与 JSONB

在 `app/iam/service/api/protos/iam/user/v1/user.proto` 增加：

```proto
enum UserRequiredAction {
  USER_REQUIRED_ACTION_UNSPECIFIED = 0;
  USER_REQUIRED_ACTION_UPDATE_PASSWORD = 1;
}
// User 中，delete_time=11、purge_time=12 由删除设计占用。
repeated UserRequiredAction required_actions = 13 [(google.api.field_behavior) = OUTPUT_ONLY];
// CreateUserRequest 中现有 user=1、user_id=2、password=3 保持不变。
optional bool require_password_change = 4;
```

当前只有 UPDATE_PASSWORD。空集合表示不要求动作，UNSPECIFIED 不写入集合，去重且不依赖顺序。该字段与 `status`、`email_verified`、`delete_time`、`purge_time` 一致标注 `OUTPUT_ONLY`：响应正常输出，Servora CRUD 将其归为系统字段排除出 `WritablePaths()`，防止请求 `User.required_actions` 在创建时混入或 `UpdateUser` 传入 `update_mask: ["required_actions"]` 时空转更新 `etag`；创建时的开关由顶层 `CreateUserRequest.require_password_change` 独立承载。

`internal/data/schema/user.go` 使用 `field.JSON("required_actions", []string{}).Default([]string{})`，PostgreSQL 列为 JSONB、非空，数据库默认 `[]`，存量行回填 `[]`。数据库迁移属于该实体字段变更，不添加通用 mixin。

`internal/data/user.go` 的 `createUserAggregate` 显式设置稳定字符串集合；`NewUserRepository` 的 mapper post-hook 显式将 `[]string{"UPDATE_PASSWORD"}` 转为公开枚举，不能依赖字符串切片与枚举切片的自动反射转换。`toResources` 是 Get/List、认证和管理响应的共同投影路径，均须包含动作；不把存储字符串或哈希直接透给 API。未知持久化动作作为数据错误处理，不能静默丢弃后误判为“无要求”。

### 2.2 写入来源与版本

- 自助 Register：空集合，仍待验证并自动发验证邮件。
- 管理 CreateUser：`request.RequirePasswordChange == nil || request.GetRequirePasswordChange()` 时写 UPDATE_PASSWORD；显式 false 写空集合。service 在调用 biz 前解析缺省，不让 `Get...()` 丢失 optional 存在性。
- 新 seed：`internal/data/initial_user_creator.go:30` 构造资源时写 UPDATE_PASSWORD，`internal/biz/user_initializer.go` 返回资源同步此字段。`UserInitializer` 改用 `FindByEmailIncludingDeleted` 查重：若记录已存在（含已软删除、封禁或待验证状态），保持原状态不覆盖、不重新创建，对软删除或非活跃状态输出 `Warn` 启动日志并放行启动（同步更新 `biz/bootstrap_test.go`）。
- 首次改密、有效邮箱重置且密码确实改变：只移除 UPDATE_PASSWORD，保留其他动作。
- 管理设密、普通已认证改密、删除与恢复：不自行清除动作；删除与恢复保留原集合。

所有动作变更和密码更新在一个事务中完成；动作变化同时写 `User.update_time` 和新 `User.etag`。首改即使只移除一个动作也必须换版本。只改密码但动作不变的管理设密不把 User.etag 当密码版本；凭据哈希快照负责密码竞争。

## 3. 创建与邮件拆分：保留既有领域与 DI 方向

### 3.1 service / biz 接口

保持现有 `UserUsecase -> AccountUsecase -> Repo` 方向，不反向注入 UserUsecase，不新增 PasswordUsecase 或生命周期协调服务。

```go
// internal/biz/account.go：原方法保留名字，新增动作参数；变为只创建聚合。
func (uc *AccountUsecase) CreatePendingUser(
    ctx context.Context, userID, email, plaintextPassword string,
    profile *userpb.UserProfile, actions []userpb.UserRequiredAction,
) (*userpb.User, error)

// 公开签名不变；CAP -> CreatePendingUser(..., nil) -> 共用发信步骤。
func (uc *AccountUsecase) Register(ctx context.Context, email, plaintextPassword, capToken string, profile *userpb.UserProfile) (*userpb.User, error)

// 新内部服务调用方法，不接收 CAP。
func (uc *AccountUsecase) SendUserVerificationEmail(ctx context.Context, userID string) error

// 同文件私有函数，三种入口共用 token 生成/持久化/邮件组装。
func (uc *AccountUsecase) sendVerificationEmail(ctx context.Context, user *userpb.User) error

// internal/biz/user.go
func (uc *UserUsecase) CreateUser(ctx context.Context, name userpb.UserName,
    prepared corecrud.PreparedCreate[*userpb.User], plaintextPassword string,
    requirePasswordChange bool) (*userpb.User, error)
func (uc *UserUsecase) SendUserVerificationEmail(ctx context.Context, name userpb.UserName) error

// internal/service/user.go
func (s *UserService) SendUserVerificationEmail(ctx context.Context,
    request *userpb.SendUserVerificationEmailRequest,
) (*userpb.SendUserVerificationEmailResponse, error)
```

UserUsecase.CreateUser 按 bool 构造动作参数，继续调用 AccountUsecase.CreatePendingUser。输入 email 的 trim/NFC/case-fold、重复邮箱检查、初始密码 Argon2id 哈希、生成 ID 仍在既有 AccountUsecase；UserRepo.Create 的签名不变，动作通过其 User 参数进入 `createUserAggregate`，身份、标识、认证器、密码和动作共同提交。

UserService.SendUserVerificationEmail 解析 `users/{user}`，UserUsecase 转交 AccountUsecase；AccountUsecase 通过普通 `users.Get` 读取目标，不借管理查询读取软删除账号。未找到或已删除返回 NOT_FOUND；必须同时满足待验证状态、email_verified=false、存在匹配邮箱登录标识和未撤销 ACTIVE 密码认证器，缺失或不满足返回 INVALID_STATE。管理动作保留 UserService 服务级 REQUIRED 身份及 `manage_users` 授权，仅注册到现有 gRPC server，无 HTTP 管理路由。

### 3.2 发送步骤与状态竞争

```go
type VerificationTokenRepo interface {
    Create(ctx context.Context, userID, identifierID, expectedUserEtag, tokenHash string, expires time.Time) error
    ConsumeAndActivate(ctx context.Context, tokenHash string, now time.Time) (string, error)
}
type PasswordResetTokenRepo interface {
    Create(ctx context.Context, userID, expectedUserEtag, tokenHash string, expires time.Time) error
    Peek(ctx context.Context, tokenHash string, now time.Time) (*PasswordResetProof, error)
    ConsumeAndReplacePassword(ctx context.Context, proof PasswordResetProof, newHash string, now time.Time) (string, error)
}
```

两个 Create 都在 `internal/data/account.go` 用 `inTx`，先 `lockUser` 再读关联记录，复验普通未删除用户及 `expectedUserEtag`。验证邮件要求待验证、标识未验证和可用密码认证器；重置邮件要求 ACTIVE、标识已验证和可用密码认证器。`identifierID` 为空时在锁内查该用户邮箱标识；非空时同样验证属于该用户且为邮箱，不能信任传入关联 ID。token hash 不能为空，明文 token 不进入 Repo。

expectedUserEtag 来自发起步骤最初读到的 User；其用途是识别“请求准备之后创建前提已改变”，尤其删除后又恢复，不是邮件版本列。只检查提交时未删除不足以阻止删除前准备的请求在恢复后晚建 token。复用现有 etag，不新增 token generation/version；资料变化造成保守竞争失败也不自动重试。

私有发信步骤：生成 raw token 和 hash -> Create 锁事务提交 -> 同步 MailSender.SendVerification。SMTP 严格在数据库事务外，不能持用户锁等待网络。SMTP 失败保留账号及已持久化 token；管理发信在 UserErrorReason 末尾新增 `USER_ERROR_REASON_VERIFICATION_EMAIL_FAILED=5`、HTTP 503，用于令牌准备或邮件依赖失败的安全错误，不透出内部 cause。目标缺失/已删除仍为 NOT_FOUND，资格或 etag 竞争仍为 INVALID_STATE，不归入邮件故障。没有回滚账号、自动补发或 outbox。token 已提交后发生删除/验证/封禁，邮件可能仍被投递，链接在消费时按当前状态检查而失效；不能宣称数据库能撤回已发邮件。

公开 ResendVerification 原有 CAP、抗枚举和同步发信顺序保留，改为调用同一私有步骤；未知、不合资格、创建时资格或 etag 已改变返回原有空成功且不发信，真实数据库/SMTP 故障仍安全报错。RequestPasswordReset 同样先 CAP、再查用户，Create 时状态竞争不发信且返回空成功。Register 在创建成功后调用共用步骤，仍自动发信；若 token 或邮件失败仍按现有行为报错且保留已提交账号，不替换成新的注册部分成功协议。管理 CreateUser 则明确只返回 User，Admin 自行调用发信 RPC，邮件失败不影响已经取得的 User。

### 3.3 验证消费和激活合并

现有 `VerificationTokenRepo.Consume` 映射为 `ConsumeAndActivate`；AccountUsecase.VerifyEmail 仍只在 biz 接收 raw token，HashOpaqueSecret 后调用该方法，再按返回 userID Get User。原 `UserRepo.ActivateEmail` 与 `data.userRepository.ActivateEmail` 在唯一业务调用者迁走后移除，不保留可绕过令牌的旧路径。

ConsumeAndActivate 可在事务外按 token hash 只读定位 userID；事务内必须先锁该用户，然后重读同一 token，检查 hash/用户/邮箱标识一致、未消费、未过期、用户未删除且仍待验证、邮箱未验证、密码认证器有效。条件消费 token、写邮箱 verified_time、改用户 ACTIVE、轮换 User.etag/update_time 在同一事务提交。两次消费仅一方成功。删除先取得锁时，删除把旧 token 标记消费，验证失败；验证先提交时，随后删除仍会撤销身份访问。验证提交后的资源读取遇到删除不返回可登录承诺，也不尝试撤销已经提交的验证事实。

软删除在其用户锁事务内将未消费 verification/reset token 的 consumed_time 写为删除时间；Undelete 不清该字段。永久删除物理移除关联 token。创建、消费与生命周期事务遵守同一锁顺序，不先锁 token 再等用户锁。

## 4. 凭据快照与正常登录的事务复验

### 4.1 完整凭据快照

`internal/biz/authn.go` 的 PasswordCredential 扩充为：

```go
type PasswordCredential struct {
    UserID                  string
    AuthenticatorID         string
    PasswordAuthenticatorID string
    PasswordHash            string
}
```

`credentialRepository.FindActivePassword` 返回上述四项；PasswordAuthenticatorID 来自实际密码行，不以用户 ID 或认证器 ID 代替。查询仍限 ACTIVE、未撤销的密码认证器。调用方取值拷贝 `expected := *credential` 后不再修改；新 hash 放在另一变量中，不覆盖 expected.PasswordHash。此值只存在服务端 biz/data，不能序列化进 User、浏览器或错误 metadata。

所有当前密码 Compare、新旧密码 Compare 与新 Argon2id Hash 在 biz、事务外执行。Repo 只接受 hash 和结构化快照；锁内按用户、认证器类型/归属/状态/撤销、密码行 ID 和当前 hash 等值完整复验。哈希不符就是凭据前提失效，不能在 Repo 重做明文比较或自动重新读密码重试。

### 4.2 正常 Login 保持响应协议，替换不完整 Create 参数

```go
// internal/biz/session.go
// 返回锁内确认的当前 User，login 仅在动作集合为空时创建。
type SessionRepo interface {
    Create(ctx context.Context, expected PasswordCredential, expectedUserEtag string, now time.Time) (*userpb.User, *LoginSession, error)
    Find(context.Context, string) (*LoginSession, error)
    Revoke(context.Context, string, time.Time) error
    // 管理撤销的 RevokeAll 见生命周期设计。
}
func (uc *SessionUsecase) Create(ctx context.Context, expected PasswordCredential, expectedUserEtag string) (*userpb.User, *LoginSession, error)

// 对外 biz 签名不变。
func (uc *AuthenticationUsecase) Login(ctx context.Context, email, plaintextPassword string) (*userpb.User, *LoginSession, error)
```

AuthenticationUsecase.Login 保留 NormalizeEmail、用户状态/验证检查和 FindActivePassword/Compare。验密成功后，不再用读取时的 required_actions 决定返回，而把完整凭据值快照与已读取 User.etag 传给 SessionUsecase.Create。后者只加时钟并转交 Repo，不接受仅 userID 的替代入口。

`data.sessionRepository.Create` 事务顺序：锁用户 -> 比对 etag -> 确认未删除、ACTIVE、邮箱已验证、有效密码认证器及快照 hash -> 投影当前 User -> 若动作非空，返回 User、nil login 且不插登录；若动作为空，插入 IAMLoginSession 并返回同一事务确认的 User 与 login。这样 User.requiredActions 响应和是否插登录依据同一份状态，不能使用事务前的 User 作决定。data 内抽出仅供本包使用的 tx-client 资源投影 helper，复用现有 mapper/toResources 规则，不注入 UserUsecase/AccountUsecase，不在事务中调用会启动新事务的 SessionRepo。

管理设密先提交：旧 hash 快照失配，Login 失败，不产生晚到登录。Login 先提交：管理设密随后拿锁并撤销它。删除/恢复已经完成：etag 改变，旧快照不能借恢复晚建登录。状态、凭据、etag 竞争返回凭据拒绝或明确状态冲突，不对陈旧认证结果自动重试。

## 5. 独立首次改密与建立普通登录

### 5.1 HTTP 契约和分层签名

`app/iam/service/api/protos/iam/account/v1/account.proto` 增加 `ChangeInitialPassword`，HTTP `POST /v1/iam/account/change-initial-password`，`body: "*"`；请求 `email=1,current_password=2,new_password=3`，空响应 `ChangeInitialPasswordResponse {}`。两个密码均 REQUIRED、8–128 长度及已有 redact 注解；email 由 biz 规范化。该方法明确 `AUTHN_MODE_PUBLIC`，沿用 AccountService 的 `AUTHZ_MODE_NONE`，不要求 CAP、普通登录或先前 Login 结果。它不是管理设密入口。

```go
// internal/biz/account.go
func (uc *AccountUsecase) ChangeInitialPassword(ctx context.Context,
    email, currentPassword, newPassword string) (*LoginSession, error)

// internal/biz/authn.go：加入既有 CredentialRepo，未另造 Repo/usecase。
CompleteInitialPassword(ctx context.Context, expected PasswordCredential,
    expectedUserEtag, newHash string, now time.Time) (*LoginSession, error)

// internal/service/account.go
func (s *AccountService) ChangeInitialPassword(ctx context.Context,
    request *accountpb.ChangeInitialPasswordRequest,
) (*accountpb.ChangeInitialPasswordResponse, error)
```

AccountUsecase 查询用户与凭据，确认账号未删除、ACTIVE、邮箱已验证并验证当前密码；未知用户、不可用状态、缺失凭据或当前密码错误统一返回 UNAUTHENTICATED/401。只有当前密码验证通过后才检查 UPDATE_PASSWORD，已无该动作返回 INVALID_STATE/409，避免通过动作缺失响应泄露身份事实。随后用新密码对 expected.PasswordHash 做 Compare，相同返回现有 INVALID_PASSWORD/400且不消除动作；不同才生成新 hash。

AccountErrorReason 只新增 `ACCOUNT_ERROR_REASON_INVALID_STATE=7`、HTTP 409，涵盖已无待完成动作和事务提交时凭据快照等前提变化；不新增 PASSWORD_UNCHANGED、REQUIRED_ACTION_NOT_PENDING 或 CREDENTIAL_CHANGED。相同密码及密码规则失败共用现有 INVALID_PASSWORD/400，首次改密的认证失败统一使用现有 UNAUTHENTICATED/401。通过 service.accountError 映射安全错误，已有编号保持不变；具体 gRPC code 与 ErrorInfo.reason 按统一错误设计保留，不因 HTTP code 相同丢失约定的 gRPC 语义。

### 5.2 一个事务内完成

`data.credentialRepository.CompleteInitialPassword`：

1. `lockUser`，比对 expectedUserEtag；核对未删除、ACTIVE、已验证邮箱、完整 credential 快照，且仍存在 UPDATE_PASSWORD。
2. 更新对应 PasswordAuthenticator.hash 和 changed_time，条件包含旧 hash；受影响行数必须恰好为一。
3. 仅移除 UPDATE_PASSWORD，写 User.required_actions、新 etag/update_time。若还有其他动作，不得建立普通登录；当前协议只支持 UPDATE_PASSWORD，遇到其他动作按状态冲突回滚，不交付可访问登录。
4. 复用 `revokeUserSessions(ctx,tx,userID,"",now)` 撤销全部旧 IAM 登录及关联 OAuth token sessions、access/refresh 记录。
5. 在同一 tx 直接插入新的普通 IAMLoginSession（与正常 Create 共用 data 内插入 helper，不调用会另开事务的 SessionRepo.Create），AuthTime 为此次完成时间。
6. 任一步失败回滚密码、动作、撤销和新登录。并发首改仅一方成功，另一方因 etag/hash/action 失配失败；不自动重试。

AccountUsecase 不持有 SessionUsecase，也不在事务后另调用 Login；因此没有“密码已改但普通登录未创建”的数据库中间结果。依赖仍为 AccountUsecase -> CredentialRepo，既有 UserUsecase -> AccountUsecase 无循环。

### 5.3 SCS 绑定、旧绑定和失败反馈

AccountService.ChangeInitialPassword 必须在任何业务变更前调用现有 `security/session.Loaded(ctx,manager)`，缺失 HTTP session 接线直接拒绝。业务事务成功后 `manager.RenewToken(ctx)`，再 `manager.Put(ctx,authn.LoginReferenceKey,login.ID)`，空响应；仍由原 LoadAndSave 保存 cookie，没有 pending-proof、pending SCS key 或临时登录表。

AuthnService.Login 分支：

- 正常 login 非 nil：沿用 RenewToken + Put，返回 LoginResponse.User。
- required_actions 非空、login 为 nil：先 `manager.Remove(ctx, LoginReferenceKey)`，再 RenewToken；返回 User 但不 Put。清除此前浏览器的旧账号引用，不调用旧账号 Logout，不撤销其他设备，也不把待改密用户视作已登录。Remove 放在 RenewToken 前，使轮换失败返回错误时不主动保留旧绑定。
- nil login 且动作为空属于内部不一致，不能成功响应或解引用 nil。

首次改密数据库提交后如果 RenewToken 失败，应移除当前浏览器的旧 LoginReferenceKey 并返回安全错误，不继续 OIDC。新密码与新 IAM 登录已经提交，不作虚假的数据库回滚承诺。`security/session/session.go:72-101` 已在 SCS Store 保存失败时输出 500 并终止后续成功正文；沿用该机制，不重复添加 Store adapter。浏览器拒收 cookie 不一定能被服务端发现，只有普通 GetProfile 或 OIDC callback 的实际解析成功才证明 cookie 可用；失败页面提示“无法确认登录结果，可用新密码重新登录”，不能提示“密码未修改”，也不能前端自行伪造已登录状态。

## 6. 邮箱密码重置：Peek 与提交证明

### 6.1 证明值与签名映射

```go
// internal/biz/account.go：均为服务端内部值，不是浏览器凭证。
type PasswordResetProof struct {
    TokenID                 string
    TokenHash               string
    UserEtag                string
    Credential              PasswordCredential
    RequiresPasswordChange  bool
}
```

现有 `PasswordResetTokenRepo.ConsumeAndReplacePassword(ctx,tokenHash,newHash,now)` 改为第 3 节的 `Peek + ConsumeAndReplacePassword(ctx,proof,newHash,now)`；名称保留，参数不保留兼容重载或旁路。

ConfirmPasswordReset 的公开签名保持 `(ctx, token, newPassword) error`：biz 校验非空 token -> HashOpaqueSecret -> Peek -> 若 proof.RequiresPasswordChange，则 Compare(newPassword,proof.Credential.PasswordHash)，相同返回现有 INVALID_PASSWORD/400 -> Hash(newPassword) -> 带完整 proof 调用提交方法。raw token 仅 biz/service 请求短暂持有，Repo 只见 hash；不把 raw token、token ID、证明值返回 Web。无 UPDATE_PASSWORD 时不增加“新密码不得相同”限制，沿用原重置允许相同密码重设的行为；提交时凭据快照等前提变化返回新增 INVALID_STATE/409，令牌无效或过期继续使用既有对应 reason。

Peek 只读，不消费 token、不改变动作；返回有效未消费未过期 token 对应的当前 User.etag、credential 四元值及动作快照，同时检查当前未删除、ACTIVE、已验证邮箱和有效密码认证器（目标不存在或已软删除统一返回 `ErrInvalidPasswordResetToken`）。多条读取之间发生变化不使其成为可信提交证明，提交仍须逐项复验。

### 6.2 提交事务

ConsumeAndReplacePassword 先按 proof.Credential.UserID 锁用户（若返回 `ErrNotFound` 转为 `ErrInvalidPasswordResetToken`，不泄漏原始 `ErrNotFound`），再核对当前 etag、未删除/ACTIVE/已验证、认证器和 hash；重新按 TokenID+TokenHash 读取 token，确认归属、未消费和有效期限。RequiresPasswordChange 必须与当前动作一致；不信任调用参数来决定任意清除动作。

随后条件消费 token、替换密码 hash/changed_time、如有 UPDATE_PASSWORD 则只移除该动作并轮换 User.etag/update_time、撤销全部 IAM/OAuth 会话，在一个事务提交。不自动建立 IAM 登录。相同密码但有要求时在 biz 就拒绝，不消费 token；竞争改变凭据、动作、删除状态或 token 后，事务拒绝且不清动作。管理设密先提交使 expectedHash 失配，即使 email token 仍有效也不能沿用旧快照；用户可自行重新发起现有流程，不自动重试。

Web 原 `reset-password/page.tsx` 在提交前即清本地 token，失败后不恢复使用；保持该一次性链接处理，不引入持久化 token。现有 INVALID_PASSWORD/400 同时可能表示密码规则失败或有首次改密要求时密码未改变，页面使用兼容文案“请使用 8 至 128 个字符的新密码；若账号要求首次改密，新密码须不同于当前密码。请重新申请重置链接。”，不能根据该 reason 断言必然是相同密码，也不能泛化成对所有普通重置要求不同密码。INVALID_STATE/409 提示账号或凭据状态已变化，需重新开始流程。

## 7. 普通会话和 OIDC 一致性

`SessionUsecase.Resolve` 在现有 user status/email_verified 检查后增加动作非空拒绝，新增 `ErrRequiredActionPending`；`internal/authn/session.go:37-46` 将其映射到现有 session.ErrInvalidCredentials，而不是依赖故障。AccountService 受保护方法仍经普通认证上下文，AccountUsecase 不接受待改密结果作身份证明。`authn.From`、Actor 投影和 `iam_login_id` 含义均不改变。

OAuthRepo 保留既有 Issue/撤销接口并增加：

```go
CompleteAuthorizationRequest(ctx context.Context, requestID, userID, loginID string, now time.Time) error
SaveAuthorizationCode(ctx context.Context, requestID, codeID, codeHash string, now, expires time.Time) error
```

`internal/oidc/op_auth_storage.go` 的 CompleteAuthRequest 签名保留用于已有 provider 调用，但内部转发到新 Repo 方法；authTime 不信任外部值，锁内从对应 IAMLoginSession.CreateTime 取得。SaveAuthCode 仍在 OIDC adapter 生成 codeID、HashOpaqueSecret(rawCode) 和期限，再转交 Repo。adapter 不再直接写两类用户关联记录。

- CompleteAuthorizationRequest：锁 user，检查未删除、ACTIVE、已验证、动作为空及 login 属于用户且未撤销；重读 request，要求未完成且未过期，条件更新 subject/login/auth_time/done。
- SaveAuthorizationCode：先只读 request 定位 subject，锁对应用户后重读 request；检查 subject 未变、done、未过期、已绑定普通有效 login，当前用户仍可认证且动作为空，再插入 code。user-first 顺序使删除/永久删除后无法晚插授权码。
- Issue：在现有 `internal/data/oauth.go:32-38` 用户锁之后增加未删除、已验证邮箱、动作为空，再执行既有 code 条件消费或 refresh 轮换与 login 复验。状态失败映射 `ErrOAuthGrantInvalid`，对 OIDC 返回 invalid_grant；数据库故障不伪装 invalid_grant。
- service grant 继续在用户锁之前走原独立分支，不受用户动作及人类会话规则影响。
- `internal/oidc/claims.go` 的 `populateUserinfo`（支撑 `/userinfo` 与 token introspection）同步校验主邮箱 `VerifiedTime != nil` 且 `required_actions` 不含待完成动作，与 `SessionUsecase.Resolve`、`OAuthRepo.Issue` 保持同一活跃可认证判定。
- 管理设密 `CredentialRepo.SetPasswordAndRevokeSessions` 在 `lockUser` 确认目标未软删除后，按 `authenticator.UserIDEQ(userID), authenticator.TypeEQ(biz.AuthenticatorPassword)` 定位密码认证器；因 `DisableUser` 会对认证器同时写入 `state = disabled` 与 `revoked_time = now`，此处严禁附加 `StateEQ(Active)` 或 `RevokedTimeIsNil()`，且更新 `password_authenticators` 的 `password_hash`/`changed_time` 时原样保留认证器的 `state` 与 `revoked_time`。

Provider.completeAuthorization 继续使用已加载 SCS 和 SessionUsecase.Resolve；这层用于页面导航与普通认证，Repo 事务复验用于阻止其后的竞争，二者不能互相替代。首次改密撤销旧 login 后，已绑定旧 login 的授权请求或旧授权码不能换 token；未绑定的原 request_id 可在新登录后继续原授权过程。

HTTP、页面及调用方清单见 [credential-web-and-callsites.md](credential-web-and-callsites.md)。
