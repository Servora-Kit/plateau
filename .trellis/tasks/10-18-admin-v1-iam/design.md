# IAM 能力补齐设计

需求来源：[本任务 PRD](prd.md) 与 [父任务 PRD](../10-04-admin-v1/prd.md)。

## 1. 服务职责与调用主体

| 服务 | 调用主体与用途 | 目标身份及访问约束 |
| --- | --- | --- |
| `UserService` | 面向可信服务调用方，例如 Admin 后端，提供用户管理能力。 | IAM 独立验证服务身份及管理权限；目标是被管理的用户，调用服务身份与目标用户身份分别处理。 |
| `AccountService` | 面向用户本人，提供注册、邮箱验证、资料与账号安全等自助能力。 | 本人资料和已登录改密使用当前认证身份；公开注册、验证和恢复流程继续使用各自既有 CAP、一次性令牌及防枚举规则，不统一要求已有登录。 |
| `SessionService.Logout` | 退出用户当前的 IAM 登录。 | 从可信认证上下文取得当前登录引用，保持现有退出语义，不扩充为管理员指定任意用户的入口。 |

浏览器不直接以人的身份调用 `UserService` 代替服务授权。Admin 检查人的管理访问，IAM 检查 Admin 服务自身的身份和操作权限；IAM 不因为被管理用户是 Admin 管理员就赋予其特殊身份类别。

规则沿用 [IAM 授权规范](../../spec/iam/authorization.md) 与 [会话规范](../../spec/iam/sessions.md)。

## 2. 管理设密与强制登出

采用现有 `UserService / UserUsecase` 承担管理动作、Repo 提供明确事务方法的方案。不新增独立密码领域 usecase，不通过 `isAdmin`、`skipOldPassword` 等开关改造自助流程。

### 2.1 RPC 契约

两个动作归 `UserService`，遵循 [AIP-136](https://google.aip.dev/136) 的自定义方法约定；继续使用现有服务身份认证和 `manage_users` 授权，保持 gRPC 接入，不顺带新增公开 HTTP 入口。

接口形状如下；实际 Proto 沿用既有 required、资源引用、字段校验和敏感字段脱敏注解。

```proto
rpc SetUserPassword(SetUserPasswordRequest)
    returns (SetUserPasswordResponse);

rpc RevokeUserSessions(RevokeUserSessionsRequest)
    returns (RevokeUserSessionsResponse);

message SetUserPasswordRequest {
  string name = 1;
  string new_password = 2;
}

message SetUserPasswordResponse {}

message RevokeUserSessionsRequest {
  string name = 1;
}

message RevokeUserSessionsResponse {}
```

- `name` 必填（`users/{user}`），不设邮箱或独立 `user_id` 参数；`new_password` 沿用 8–128 长度校验与脱敏注解，明文不进 Repo、日志或响应。
- 不接收操作者 ID、旧密码、恢复令牌、目标会话或 `User.etag`（并发由身份锁与事务串行化，不拿资料版本充当凭据版本）；成功返回空响应，不额外回查 `User`。

### 2.2 行为与目标状态

- 设置密码允许作用于未删除且有密码凭据的待验证、正常或封禁账号；保留邮箱验证事实、账号状态与首次改密要求，不自动解禁或激活，同时撤销全部 IAM 登录及关联 OAuth 状态。
- 强制登出不改密码、状态或首次改密要求；无活跃会话时幂等成功，不影响其他用户、独立服务身份或其他应用本地会话（已签发离线 JWT 允许用至自然过期，旧 refresh token 失效）。
- `ChangePassword`、`ConfirmPasswordReset` 与 `SessionService.Logout` 保持本人自助语义，不混入管理员目标操作。

### 2.3 分层与调用职责

| 操作 | service | biz | data Repo |
| --- | --- | --- | --- |
| 管理设密 | `UserService.SetUserPassword` 解析资源名、调用 usecase 并映射错误。 | `UserUsecase.SetUserPassword` 承担管理业务语义与密码哈希。 | `CredentialRepo.SetPasswordAndRevokeSessions` 在同一事务中更新凭据并撤销会话。 |
| 强制登出 | `UserService.RevokeUserSessions` 解析资源名、调用 usecase 并映射错误。 | `UserUsecase.RevokeUserSessions` 承担管理动作语义。 | `SessionRepo.RevokeAll` 在同一事务中确认目标并撤销该用户全部会话。 |

新增管理动作通过对应 Repo 事务完成，不调用本人改密或密码恢复接口。`NewUserUsecase` 补充注入 `CredentialRepo` 与 `SessionRepo`，同步更新 Wire 生成代码及测试构造调用。

### 2.4 事务

管理设密在事务外生成密码哈希，避免计算期间长期持锁。Repo 内依次执行：

1. 复用 `lockUser` 锁定目标身份。
2. 确认目标未删除且状态允许管理（待验证、正常或封禁），按 `user_id` 与 `password` 类型定位密码认证器（封禁账号认证器含 `disabled` 与 `revoked_time`，禁止加 `ACTIVE` 或 `RevokedTimeIsNil` 过滤）；更新密码哈希与变更时间，原样保留认证器的 `state` 与 `revoked_time`。
3. 复用 `revokeUserSessions` 撤销全部 IAM 登录及关联 OAuth 令牌记录，与密码更新同事务提交或回滚。

强制登出在同一用户锁事务内确认目标并调用用户级会话撤销；两项动作均同步完成，无后台队列或自动重试。

### 2.5 错误

| 情况 | 契约 |
| --- | --- |
| 资源名格式错误、密码不符合规则 | 沿用现有资源名及参数错误。 |
| 目标不存在或已删除 | `USER_ERROR_REASON_NOT_FOUND`。 |
| 目标存在但凭据缺失或状态不允许操作 | `USER_ERROR_REASON_INVALID_STATE`。 |
| 服务身份无效或无管理权限 | 由既有认证／授权中间件拒绝。 |
| 数据库或其他内部失败 | 返回安全的内部错误，不向页面透出原始 cause 或敏感值。 |

### 2.6 并发与验收关注点

- 覆盖待验证、正常、封禁及已删除目标，验证密码更新与多端 IAM/OAuth 会话撤销同事务提交回滚，且其他用户与独立服务身份不受波及。
- 与登录共用 `lockUser`：普通登录创建事务按 `PasswordCredential` 四元快照复验账号状态、邮箱验证、认证器及密码哈希且无未完成必需动作（见 [能力核对记录](research/iam-capability-review.md)），不新增全局 token 版本字段。

## 3. 创建用户与发送验证邮件

`UserService.CreateUser(CreateUserRequest) returns (User)` 遵循 [AIP-133](https://google.aip.dev/133) 直接返回 `User`，仅在事务内创建身份、登录标识、初始凭据与首次改密要求，不生成验证令牌、不发邮件。

`CreateUserRequest` 新增 `optional bool require_password_change = 4`：缺省或 `true` 写入 `UPDATE_PASSWORD`，显式 `false` 写空集合（Admin 表单默认勾选并显式传值，只持久化 `User.required_actions`）。发送验证邮件拆分为独立的 `UserService` gRPC 动作（服务 JWT + `manage_users`，无公开 HTTP 路由）：

```proto
rpc SendUserVerificationEmail(SendUserVerificationEmailRequest)
    returns (SendUserVerificationEmailResponse);

message SendUserVerificationEmailRequest {
  string name = 1;
}

message SendUserVerificationEmailResponse {}
```

- `name` 为 `users/{user}`：目标缺失或已删除返回 `NOT_FOUND`，非待验证状态返回 `INVALID_STATE`；令牌提交后同步发 SMTP（不入创建事务、不重试、无队列），失败返回 `VERIFICATION_EMAIL_FAILED` 且保留已建账号。
- Admin 后端先调 `CreateUser` 保留返回 `User`，再调 `SendUserVerificationEmail`；发信失败直接携带已创建 `User` 与邮件错误提示，不重跑创建。
- `CreatePendingUser` 接收动作集合并改为纯创建，创建前用含软删除记录的查询预检规范化邮箱（提前拦截墓碑占用邮箱并返回 `ALREADY_EXISTS`，免去无效 Argon2id 哈希）；`Register` 在 CAP 校验与创建后显式调用共用发信逻辑，`SendUserVerificationEmail` 按用户 ID 委托 `AccountUsecase`（签名见 [凭据调用设计 §3](research/credential-implementation-design.md)）。

## 4. 软删除状态模型

- 保留 `status` 表达待验证、正常、封禁；软删除与恢复均不改写 `status`，不新增 `DELETED` 枚举或 `status_before_delete`。
- 以 `delete_time` 非空标识软删除（恢复后清空），`purge_time` 保持为空；删除期间原样保留资料、凭据、邮箱验证事实、认证器状态及 `required_actions`，但认证与普通管理变更均拒绝带 `delete_time` 的账号。

### 4.1 管理删除与恢复契约

```proto
rpc DeleteUser(DeleteUserRequest) returns (User);
rpc UndeleteUser(UndeleteUserRequest) returns (User);

message DeleteUserRequest { string name = 1; }
message UndeleteUserRequest { string name = 1; }
```

- 两个请求只接受 `users/{user}` 资源名，不添加 etag、allow_missing；返回变更后的 User，继续通过服务身份与 `manage_users` 授权。
- 操作在身份锁内检查当前删除状态并写入，不校验操作者页面版本。User.etag/update_time 仍随资源变更更新。
- DeleteUser 对不存在或已经软删除目标返回 NOT_FOUND；UndeleteUser 对不存在或已永久删除目标返回 NOT_FOUND，对未删除目标返回 ALREADY_EXISTS。
- UserRepo 新增 SoftDelete、Undelete、Expunge；管理入口经 UserUsecase，自助入口经 AccountUsecase，调用同一包内 deleteUser 命令和 SoftDelete 事务。管理删除不传凭据 proof，自助删除传已验密码快照、LoginID 和最初读取的 User.etag；不反向注入 UserUsecase。接口定义及锁函数见 [生命周期调用设计 §3–4](research/lifecycle-implementation-design.md)。

### 4.2 删除、恢复与关联状态事务

- 普通 lockUser 仅锁未删除记录；lockUserIncludingDeleted 在函数内部局部使用 SkipSoftDelete，仅用于 Undelete、Expunge。用户关联写入统一先锁 User 再读取关联记录；不得把管理查询 context 传给认证读取。
- SoftDelete 锁内复验未删除，自助入口再复验账号资格、User.etag、LoginID 归属及未撤销、密码行 ID/hash；写 delete_time 和新 etag/update_time，保持 purge_time 为空。保留 status、密码认证器和邮箱验证事实。
- 同一事务撤销 IAM 登录、OAuth 用户会话和 human access/refresh；将未消费的邮箱验证／重置令牌标记为 consumed；删除目标身份已绑定的授权码和授权请求。恢复只清删除标记及 deleted_by，更新 etag/update_time，不清任何 revoked_time 或 consumed_time。
- 用 tx.Client() 投影事务内的变更后 User；删除返回资源时局部包含删除记录，不在提交后调用默认 Get 造成 NOT_FOUND，也不返回另一请求恢复后的状态。
- 全部步骤使用同一事务，任一步失败回滚。恢复、Expunge 共享用户行锁，先恢复则 Expunge 拒绝未删除目标，先 Expunge 则恢复返回 NOT_FOUND。

### 4.3 本人删除

`AccountService.DeleteAccount(DeleteAccountRequest) returns (DeleteAccountResponse)`，请求仅 `current_password = 1`，空响应；HTTP `POST /v1/iam/account/delete`、body `*`、AUTHN_MODE_REQUIRED。密码沿用校验及脱敏注解，目标 ID/LoginID 来自 authn.From(ctx)。

service 先检查 SCS 已加载；biz 读取未删除用户和当前凭据、比较密码后调用共用删除命令。事务成功后 Destroy 当前浏览器 SCS，不额外调用 Logout。SCS 删除失败不回滚数据库、不声称账号未删除；旧 cookie 仍因 IAM 登录已撤销而失效。

IAM Web 在现有账号安全页提供删除确认及密码输入；成功跳转 `/login/`，输入错误留在表单，未认证返回登录；不提供本人恢复或永久删除。密码在取消和请求结束时清除。完整表级副作用见生命周期调用设计 §5。

## 5. 软删除查询语义

- `ListUsers` 默认只返回未删除用户。`ListUsersRequest.show_deleted` 为 true 时包含未删除与已删除用户，不表示“仅看已删除”。
- 有管理权限的调用方可通过 `UserService.GetUser` 读取软删除记录，返回原账号状态及删除时间信息；物理清理后返回 NOT_FOUND。
- 登录、会话解析、本人资料等身份访问路径继续排除已删除身份。管理查询可见性不改变认证或管理变更的状态限制。
- service 的 ListPreparer 保持现有输入，request.show_deleted 单独传 UserUsecase.ListUsers、UserRepo.ListUsers。data 局部选择可见性，entcrud.List 的 scope 分别固定为 `iam.users.v1:active`／`iam.users.v1:all`；原页 token 切可见性或 filter 时返回 INVALID_PAGE_TOKEN。
- 给 DeleteTime 增加 `.Filter().Order().Nullable()` 绑定；仅已删除请求为 `show_deleted=true, filter="delete_time != null"`。false 加该过滤得到空列表，不隐式放宽可见性。沿用现有 null 比较语法与游标排序。
- 管理 GetUser 局部包含删除记录，Get/FindByEmail 保持默认隐藏。资料和状态写入在用户锁后显式加未删除谓词；immutable 检查读默认 Get。现有 mapper/toResources 增加接受 Ent client 的内部投影函数，读请求用 repo.ent，生命周期响应使用 tx.Client()。
- DisableUser 允许作用于未删除的待验证、正常及已封禁账号（幂等）；EnableUser 在用户锁内要求当前状态为 DISABLED，并根据该用户主邮箱 `LoginIdentifier.verified_time` 是否非空决定解禁后的状态：已验证恢复为 ACTIVE，未验证恢复为 PENDING_EMAIL_VERIFICATION（同时将认证器恢复为 active 并清空 revoked_time），避免未验证账号解禁后落入 ACTIVE 且 `email_verified=false` 的死锁状态。

## 6. 无限保留与显式永久删除

- 软删除记录无限期保留，未永久删除前管理员随时可恢复；不设恢复期限、定时扫描器或批量清理，Admin Web 仅在已删除记录提供需二次确认的永久删除入口。
- 仅管理员可经 `UserService.ExpungeUser(ExpungeUserRequest) returns (google.protobuf.Empty)`（仅 `name = 1`）永久删除已软删除用户；方法级授权固定为 `expunge_users`（对象 `iam:global`），OpenFGA `iam` 类型独立声明 `expunge_users: [service]`（不从 `manage_users` 派生，部署时为 `service:admin` 显式授予两项关系）。
- `Expunge` 在 `lockUserIncludingDeleted` 内确认 `delete_time != nil`，收集关联 login、authenticator、OAuth session 及已绑定授权请求 ID，单事务按下表顺序硬删（空 ID 集合跳过对应条件，完整谓词见 [生命周期调用设计 §6](research/lifecycle-implementation-design.md)），全部提交后才释放邮箱唯一键。
- `IAMLoginSession` 删除后旧浏览器 SCS 引用即无法解析身份；PostgreSQL `sessions` 表继续走既有 TTL 回收，不扫描全表或新增 Store 映射。

| 清理顺序 | 表及范围 |
| --- | --- |
| 1 | oauth_refresh_tokens：目标 OAuth token session 的所有代际 |
| 2 | oauth_access_tokens：human 且 subject 属目标，或 human 且 session 属目标；保留 service token |
| 3–4 | oauth_authorization_codes、oidc_authorization_requests：subject、IAM login、OAuth session 所确定的身份关联记录 |
| 5–6 | oauth_token_sessions、iam_login_sessions：目标用户的全部记录 |
| 7–8 | email_verification_tokens、password_reset_tokens：目标用户的全部记录 |
| 9–10 | password_authenticators、authenticators：目标认证器及密码 |
| 11–12 | login_identifiers、users：全部标识，最后硬删带删除标记的 User |

login_identifier 的唯一键释放只在整个事务提交后生效；中途失败恢复全部记录。共享 OAuthClient、签名密钥、其他用户、独立服务令牌和跨服务数据不参与清理。

## 7. 用户必需动作的领域归属

- requiredActions 仅用于 IAM User，不创建通用 Ent mixin 或跨实体动作框架。
- 必需动作属于持久化用户状态；UserService 与 AccountService 共用该状态，分别使用管理授权和本人认证。
- User.required_actions 使用 `repeated UserRequiredAction`，无重复、不依赖顺序，当前动作值为 `UPDATE_PASSWORD`；`UNSPECIFIED` 不作为动作，空集合表示没有要求。
- users.required_actions 使用 JSONB 集合；Ent 使用 `field.JSON("required_actions", []string{})` 保存稳定动作名，如 `[]`、`["UPDATE_PASSWORD"]`，通过 User mapper 显式转换公开枚举。
- 数据库默认及旧账号迁移为空集合；自助注册写空集合，管理创建按 require_password_change 写入，新建 seed 写 UPDATE_PASSWORD。`UserInitializer` 启动查重使用包含软删除记录的邮箱查询（不复用过滤墓碑的认证 `FindByEmail`）：若已有记录存在（含软删除、封禁或待验证状态），保持原记录不覆盖，对软删除或非活跃状态输出 `Warn` 启动日志并放行 IAM 启动，仅在记录完全不存在时创建新 seed（同步更新 `bootstrap_test.go` 原启动门禁断言）。新增列的 SQL 默认使用空 JSON 数组，沿用 Schema.Create 完成表结构升级。
- 独立首次改密成功，或 `AccountService.ConfirmPasswordReset` 通过有效邮箱重置凭证设置不同于当前密码的新密码，均完成 UPDATE_PASSWORD。密码更新与移除对应动作共同提交，保留其他动作；失败、仅申请重置或提交相同密码均不完成该要求。
- 管理设密保留已有动作；邮箱密码重置完成动作后不自动登录，无该动作用户的既有重置行为不变。
- 动作变更同步更新 User 的 etag/update_time；密码并发校验使用凭据标识和密码哈希。

## 8. 登录与首次改密衔接

### 8.1 登录结果与表单

1. AuthnService.Login 按现有规则验证邮箱、密码、账号状态及邮箱验证事实。
2. 没有待完成动作时，建立普通 IAM 登录并返回 User。
3. 存在 UPDATE_PASSWORD 时，返回包含该动作的 User，不创建 IAMLoginSession，不写入新的 iam_login_id。移除当前浏览器原有的登录引用，使其不能凭前一账号的绑定完成本次授权；不因此撤销其他设备的登录。
4. IAM Web 清除登录密码，在同一登录页显示初始密码、新密码、确认新密码表单，保留邮箱及当前 OIDC request_id。根据 User.required_actions 选择表单，不增加同义的 password_change_required 字段。
5. 用户再次输入初始密码。提交邮箱、current_password、new_password，确认新密码仅供前端一致性校验。提交结束清除密码输入，不将密码保存到浏览器持久化存储。

### 8.2 独立完成接口

- `AccountService.ChangeInitialPassword(ChangeInitialPasswordRequest) returns (ChangeInitialPasswordResponse)`（`email=1, current_password=2, new_password=3`；`POST /v1/iam/account/change-initial-password`，`body: "*"`，`AUTHN_MODE_PUBLIC`）。不依赖前置登录或待改密会话，每次请求独立核验邮箱（IAM 规范化）、密码长度脱敏、当前密码、账号可用性、邮箱已验证及 `UPDATE_PASSWORD` 存在性。

### 8.3 事务与普通登录

1. `AccountService` 先确认 SCS 已加载；`AccountUsecase` 读取用户与密码凭据，在事务外核验初始密码、拒绝新旧相同并计算新哈希（明文不入 Repo）。
2. Repo 经 `lockUser` 复验 `User.etag`、未删除、ACTIVE、邮箱已验证、有效认证器、密码行 ID/哈希及仍含 `UPDATE_PASSWORD`；同事务替换密码、移除 `UPDATE_PASSWORD`、轮换 `etag`/`update_time`、撤销旧 IAM/OAuth 会话并创建新 `IAMLoginSession`（并发前提变化直接失败，不自动重试）。
3. 提交后轮换浏览器 session token 绑定新 `iam_login_id`；若绑定或写响应失败，不声称密码未改，前端不自动推进 OIDC，用户可用新密码重新登录。

### 8.4 页面继续与中断

- 完成请求成功后，保留的 request_id 非空则跳转 `/authorize/callback?id=...`，否则进入 `/account/`；无需用户再次手工登录。
- 凭据或密码输入错误时留在表单，保留邮箱及 request_id 并清除密码；动作或凭据快照发生竞争时返回登录表单，依赖失败不自动再次提交。
- 取消改密返回同页登录表单，清除密码输入并保留 request_id；未完成动作继续保存在 User 中。
- 刷新页面沿用当前登录页的处理：重新登录；若一次性 URL 中的 request_id 已被移除，需要从接入应用重新发起 OIDC 授权，不新增跨刷新流程恢复存储。
- 普通账号访问和 OIDC 完成授权均通过有效普通登录校验，并检查当前 User 的未完成动作；待改密结果不具备普通访问能力。

### 8.5 业务与 Repo 契约

`PasswordCredential` 使用 UserID、AuthenticatorID、PasswordAuthenticatorID、PasswordHash 四元值快照，明文只在 biz 比较。User.etag 单独随最初读取的资源传递；不覆盖 expected.PasswordHash 来保存新 hash。

| 调用 | 接口及提交结果 |
| --- | --- |
| 普通登录 | SessionUsecase.Create(ctx, expected, expectedUserEtag) → SessionRepo.Create(ctx, expected, expectedUserEtag, now)，返回 User、LoginSession、error。锁内复验后有动作返回 User+nil login，无动作才插普通登录；不保留仅 userID 的生产入口 |
| 首次改密 | AccountUsecase.ChangeInitialPassword(ctx,email,currentPassword,newPassword) → CredentialRepo.CompleteInitialPassword(ctx,expected,expectedUserEtag,newHash,now)，返回新 LoginSession。密码、动作和新登录在同一事务处理，不在事后再次调用 Login |
| 邮箱重置 | PasswordResetTokenRepo.Peek(ctx,tokenHash,now) 返回 TokenID/TokenHash/UserEtag/PasswordCredential/RequiresPasswordChange；biz 比较新密码，再调用 ConsumeAndReplacePassword(ctx,proof,newHash,now)。事务内重验 token、账号与快照，更新密码、消费 token、移除动作及撤销会话；不创建登录 |

详细 Go 签名、生产调用替换及事务内投影见 [凭据调用设计 §4–6](research/credential-implementation-design.md)。原始 reset token 在 biz 哈希后才传 Repo；事务在取得用户锁后重读 token，并按此时的当前时间检查过期条件，不能仅沿用请求开始时的期限判断。

### 8.6 一次性令牌与 OIDC 写入

- 两类 token Create 增加 expectedUserEtag，在用户锁内比对并复验对应资格；阻止删除前准备的请求在 Delete→Undelete 后晚建有效 token。公开重发／申请 reset 的未知目标、资格变化仍返回原有空成功；管理发信返回具体领域失败。SMTP 在 token 事务提交后执行，不持锁等待网络。
- VerificationTokenRepo.Consume 与 UserRepo.ActivateEmail 合并为 ConsumeAndActivate(ctx,tokenHash,now)：先定位 userID，再锁 User 重读 token，条件消费、验证标识、激活用户及更新版本同一事务；当 `lockUser` 因目标不存在或已软删除返回 `ErrNotFound` 时（含 PasswordResetTokenRepo 的 Peek 与 ConsumeAndReplacePassword），统一转换为各自的无效令牌领域错误（映射 INVALID_TOKEN／400），不将原始 `ErrNotFound` 泄漏为 500 或 401。删除旧的独立 ActivateEmail 生产路径，避免消费与激活分别提交。
- OAuthRepo 增加 CompleteAuthorizationRequest(ctx,requestID,userID,loginID,now)、SaveAuthorizationCode(ctx,requestID,codeID,codeHash,now,expires)。OIDC adapter 委托现有 Repo，User 锁内检查未删除、ACTIVE、邮箱验证、无动作和有效登录，再绑定请求或保存授权码；auth_time 从登录记录取值。
- 未绑定身份的授权请求可以独立创建；晚到的绑定、code、refresh 发行必须复验当前身份和登录。现有 OAuthRepo.Issue 的 human 分支及 `OIDCStorage.populateUserinfo`（`/userinfo` 与 token introspection）同步补齐未删除、ACTIVE、主邮箱已验证与 `required_actions` 为空检查，service grant 保持独立。SessionUsecase.Resolve 对有动作的普通身份返回无效凭据，不能把它映射成依赖故障。
- 两类 token 的创建／消费、登录创建、改密、自助删除、User 资料与状态更新统一先锁 User。读取可在事务外定位，写入资格在锁内重查；不先锁 token 再等待 User。恢复/Expunge 专用含删除记录锁不传播到认证 context。

### 8.7 浏览器接入

AccountService 继续仅注册 HTTP；管理 gRPC 只注册 UserService。新 ChangeInitialPassword 的认证注解为 PUBLIC，由密码校验完成本人认证；DeleteAccount 为 REQUIRED，使用普通 session。两者沿用 AUTHZ_MODE_NONE。

`NewHTTPServer` 注入运行配置并在启动时解析规范化 IAM public origin（配置缺失或非法则启动失败）；新增两个 POST 在 `browserSessionFilter` 中经 `mime.ParseMediaType` 校验媒体类型为 `application/json`（不匹配返回 415 Unsupported Media Type），存在 `Origin` 头时对照配置的 IAM public origin 校验（不匹配返回 403 Forbidden）；没有 `Origin` 的直接请求仍执行各自密码／登录校验。只在这两个 HTTP 入口加过滤，不改既有端点，不开放跨源带凭据调用。SCS 当前提供 HttpOnly/Secure/SameSite cookie 与 LoadAndSave，没有独立的 CSRF token 机制，不引入新的认证会话模型。

## 9. Proto 字段、映射与表结构升级

| 源消息 | 新字段或枚举 | 编号与含义 |
| --- | --- | --- |
| User | delete_time、purge_time | Timestamp 11、12，标注 `OUTPUT_ONLY`；未删除／未安排清理时为空 |
| User | required_actions | repeated UserRequiredAction 13，标注 `OUTPUT_ONLY`（排除出 Servora CRUD 可写路径，防止 UpdateUser 空转更新 etag） |
| UserRequiredAction | UNSPECIFIED、UPDATE_PASSWORD | 完整枚举名使用 USER_REQUIRED_ACTION_ 前缀，值分别为 0、1 |
| CreateUserRequest | require_password_change | optional bool 4；缺省或 true 加入 UPDATE_PASSWORD |
| ListUsersRequest | show_deleted | bool 5；参与分页 scope |

- 既有字段号和方法签名不重排。源 Proto、Go/TS 生成物、CRUD 字段描述及响应映射一并更新。User 的动作集合按既有业务路径维护，AccountService 的本人请求使用各自密码／令牌字段。
- User Ent schema 接入现有 `infra/entgo/mixin.SoftDeleteMixin`；新增 required_actions 的客户端默认空切片，SQL 默认表达式 `'[]'`，PostgreSQL 列类型指定 jsonb。旧行随新增列得到空集合；delete_time/purge_time 为 nullable Timestamp，旧行保持 null。
- 使用当前 NewDBClient 的 Schema.Create，在服务提供请求前完成加列；不启用删除旧列或表的选项，不引入第二套迁移器。已有账号不批量添加 UPDATE_PASSWORD，已有 seed 不重置动作。
- JSON 字符串 UPDATE_PASSWORD 与公开枚举显式映射；未知存储值作为内部数据错误返回，不忽略为无动作。空集合输出为空列表，不把 UNSPECIFIED 写进数据库。
- required_actions 变化时更新 etag/update_time；其余密码替换只更新密码凭据时间。软删除与恢复分别更新用户的时间戳及 etag。
- 升级前备份数据库并停用旧 IAM 实例，完成 schema 和新服务切换后再开放流量；旧程序不认识删除字段和动作限制，不与新版本混跑。回退需要先停止流量并恢复升级前备份及旧程序，不能直接运行忽略新状态的旧程序。

## 10. 错误与传输映射

data 返回可检查的领域错误和原始 cause，不生成公共响应；biz 保留业务状态，service 将领域错误映射为所属 Proto reason，server 只处理 transport 的状态码表达。页面使用 reason 选择提示，不解析内部错误文本。

### 10.1 管理 gRPC

| 情形 | reason | gRPC code |
| --- | --- | --- |
| 资源名或请求格式不符 | 现有 CRUD 参数错误 | InvalidArgument |
| 不存在、已删除的普通管理目标或重复 Delete | USER_ERROR_REASON_NOT_FOUND | NotFound |
| 邮箱冲突、Undelete 的目标未删除 | USER_ERROR_REASON_ALREADY_EXISTS | AlreadyExists |
| 未软删除即 Expunge、账号不满足管理发信或设密条件 | USER_ERROR_REASON_INVALID_STATE | FailedPrecondition |
| 现有资料／状态变更的 etag 不符 | USER_ERROR_REASON_ETAG_MISMATCH | Aborted |
| 验证邮件发送失败 | USER_ERROR_REASON_VERIFICATION_EMAIL_FAILED（新增值 5，HTTP 503） | Unavailable |
| 缺少有效服务身份／缺少相应权限 | 现有 SecurityErrorReason | Unauthenticated／PermissionDenied |
| 未知数据库或内部失败 | 安全的内部 reason/message | Internal |

- 保持现有 UserErrorReason 的编号和 HTTP 注解；INVALID_STATE 与 ALREADY_EXISTS 均可能带 HTTP 409，不能仅依赖 Kratos 的 HTTP→gRPC 数值转换。
- 在 IAM `internal/server/grpc.go` 的服务 JWT/FGA middleware 之后追加应用级 `userErrorStatus`，包住 UserService 调用，按上述 User reason 生成原生 `status.Status` 并附带 `errdetails.ErrorInfo`。只转换上述 IAM 领域 reason；对已有原生 status、context 取消及其他错误保持原状，不把所有 409 改成同一个 code。
- 使用已有 Servora WithMiddleware 接线。Kratos unary server/client interceptor 将错误原样返回，现有日志 middleware 只读取错误；无需复制框架或修改 Servora。Admin 调用端先保留 `status.Code` 和 ErrorInfo，再转为自身 Web 错误，不调用 FromError 后重新编码成 gRPC。

### 10.2 本人 HTTP

| 情形 | reason／HTTP |
| --- | --- |
| Proto 字段缺失、格式或长度校验失败 | 现有参数校验错误／400 |
| 业务密码规则失败或新旧密码相同 | ACCOUNT_ERROR_REASON_INVALID_PASSWORD／400 |
| ChangeInitialPassword 邮箱或当前密码不符、账号不存在或不能登录 | ACCOUNT_ERROR_REASON_UNAUTHENTICATED／401；与登录一致，不披露账号是否存在 |
| 当前密码已验证，但已无 UPDATE_PASSWORD，或提交时凭据快照改变 | ACCOUNT_ERROR_REASON_INVALID_STATE（新增值 7）／409；提示重新登录，不重复提交 |
| DeleteAccount 已登录但当前密码错误 | ACCOUNT_ERROR_REASON_INVALID_PASSWORD／400 |
| DeleteAccount 及其他需登录的 AccountService 方法遇到登录已撤销或目标已软删除／永久删除（ErrNotFound） | ACCOUNT_ERROR_REASON_UNAUTHENTICATED／401（service.accountError 统一转换，不落入 500） |
| 邮箱重置令牌无效／过期 | 现有 INVALID_TOKEN／TOKEN_EXPIRED，400 |
| 内部失败 | 现有安全 InternalServer，500；原始 cause 只用于受控日志 |

ChangeInitialPassword 的 401 表示本次凭据验证失败，页面停留并清空密码输入；409 表示流程前提改变，页面返回登录入口。旧账号不带 UPDATE_PASSWORD 时，邮箱重置保持原先同密码处理方式。公开注册／重发／恢复的防枚举与 CAP 规则不扩展到管理 RPC。

