# IAM 用户删除、恢复、永久删除实施设计

本文补充 [设计 §4–6](../design.md) 的接口与事务细节；产品规则以 [PRD](../prd.md)、[已选方案](batch-design-options.md) 为准。源码依据来自当前工作树及 `../servora`。

## 1. 已有实现与需要替换的调用

| 源码 | 当前事实 | 确定改动 |
| --- | --- | --- |
| `internal/data/schema/user.go` | User 没有删除字段；status 独立保存。 | 仅 User 接入 `infra/entgo/mixin.SoftDeleteMixin`，保留 status、凭据和登录标识，不增加 DELETED。 |
| `internal/data/authn.go:37–40` | `lockUser` 使用 `FOR UPDATE`，已有设密、登录、OAuth 签发使用它。 | 保留其未删除查询语义，增加专用含删除记录的锁函数；所有用户关联写操作先锁 User。 |
| `internal/data/user.go:167–180` | 管理 GetUser 直接调用认证用 Get。 | 管理查询局部跳过删除过滤；认证读取不跳过。 |
| `internal/data/user.go:259–308` | profile 更新没有用户锁，也没有删除谓词。 | 放入用户锁事务，更新显式加未删除谓词，事务内区分删除与 etag 冲突。 |
| `internal/data/account.go`、`internal/biz/account.go:133–145` | 验证 token 先消费，再另行 ActivateEmail；两类 token Create 不锁 User。 | 消费与激活合并事务，Create 取得用户锁并重验资格和最初读取的 User.etag。 |
| `internal/oidc/op_auth_storage.go:90–108,136–168` | CompleteAuthRequest、SaveAuthCode 直接写 Ent，没有用户锁。 | 将这两个身份关联写操作交给现有 OAuthRepo 的明确事务方法。 |
| `internal/data/schema/login_identifier.go:30–32` | `(type, canonical_value)` 全局唯一，不受 User 可见性影响。 | 保持该唯一约束；只在 Expunge 事务中删除标识后释放邮箱。 |

路径未写前缀时均相对 `app/iam/service`。不新增生命周期 usecase、通用删除框架、任务队列、SCS 用户索引或 token 版本字段。

## 2. API 与持久化字段

`app/iam/service/api/protos/iam/user/v1/user.proto` 新增：

```proto
rpc DeleteUser(DeleteUserRequest) returns (User);
rpc UndeleteUser(UndeleteUserRequest) returns (User);
rpc ExpungeUser(ExpungeUserRequest) returns (google.protobuf.Empty);
message DeleteUserRequest { string name = 1; }
message UndeleteUserRequest { string name = 1; }
message ExpungeUserRequest { string name = 1; }
```

三个 name 都使用现有 REQUIRED 与 `iam.plateau.dev/User` resource_reference 注解及现有资源名解析，不接收 etag、allow_missing、目标邮箱或删除模式。Delete/Undelete 继承 `manage_users`；Expunge 方法覆盖为 `expunge_users`，对象仍为 `iam:global`。没有 HTTP 管理路由。

- User 新增 `delete_time = 11`、`purge_time = 12` 两个 Timestamp 响应字段；`required_actions = 13` 与凭据设计使用同一编号。三个系统维护字段均标为 `OUTPUT_ONLY`，使 Servora CRUD 将其排除在 `WritablePaths()` 之外，防止 `UpdateUser` 传入对应写掩码时空转更新 `etag`。
- `ListUsersRequest` 增加 `bool show_deleted = 5`。缺省 false，true 表示包含全部用户，而不是仅已删除。
- mixin 还提供内部 `deleted_by`；此需求没有新增该字段的 API，不从未经验证 metadata 接收操作者字符串。生命周期事务不调用 WithDeletedBy，保持为空，恢复时一并清空。
- `purge_time` 始终为空；没有期限配置和到期清理任务。删除／恢复轮换 etag 并更新 update_time，status、邮箱验证事实、认证器状态、密码和 required_actions 原样保留。
- 现有 `internal/data/generate.go` 已启用 `intercept,sql/modifier,sql/lock`；沿用生成入口，更新 Ent schema、生成实体／predicate／runtime／migration schema 以及 Go、TypeScript、CRUD descriptor、认证授权规则生成物，不手改生成物。

## 3. 明确 Repo 和 biz 接口

沿用已有 UserRepo，不建立另一组注入对象。新增及变更接口如下，其余接口保持原有职责：

```go
// PasswordCredential 使用凭据设计的四元快照：
// UserID, AuthenticatorID, PasswordAuthenticatorID, PasswordHash。
type DeleteAccountProof struct {
    Credential PasswordCredential
    LoginID    string
    UserEtag   string
}

type UserRepo interface {
    // Get 与 FindByEmail 只读未删除身份；FindByEmailIncludingDeleted 专用于启动引导查重与创建前邮箱占用检查；GetUser 专用于管理读取。
    Get(context.Context, string) (*userpb.User, error)
    FindByEmail(context.Context, string) (*userpb.User, error)
    FindByEmailIncludingDeleted(context.Context, string) (*userpb.User, error)
    GetUser(context.Context, userpb.UserName) (*userpb.User, error)
    ListUsers(context.Context, corecrud.ListQuery, bool) (corecrud.ListResult[*userpb.User], error)
    SoftDelete(context.Context, string, *DeleteAccountProof, time.Time) (*userpb.User, error)
    Undelete(context.Context, string, time.Time) (*userpb.User, error)
    Expunge(context.Context, string) error
    // Create、资料和状态方法继续存在；移除 ActivateEmail，见 §7。
}

func deleteUser(ctx context.Context, users UserRepo, userID string,
    proof *DeleteAccountProof, now time.Time) (*userpb.User, error)

func (uc *UserUsecase) DeleteUser(context.Context, userpb.UserName) (*userpb.User, error)
func (uc *UserUsecase) UndeleteUser(context.Context, userpb.UserName) (*userpb.User, error)
func (uc *UserUsecase) ExpungeUser(context.Context, userpb.UserName) error
func (uc *UserUsecase) ListUsers(context.Context, corecrud.ListQuery, bool) (corecrud.ListResult[*userpb.User], error)
func (uc *AccountUsecase) DeleteAccount(ctx context.Context,
    userID, loginID, currentPassword string) error
```

`deleteUser` 是 `internal/biz/user.go` 的包内函数：检查 ID、核对非空 proof 的 Credential.UserID 与目标一致，然后调用 SoftDelete；不承担密码计算。管理入口传 nil proof，自助入口传完整 proof。当前 `UserUsecase` 已依赖 `AccountUsecase`，AccountUsecase 直接调用包内函数和自己已有的 users Repo，不反向注入 UserUsecase，因此没有依赖环或新增 Wire provider。

AccountUsecase.DeleteAccount 使用 `users.Get` 取得当前 User 及 etag，再使用 `credentials.FindActivePassword` 取得快照；在 biz 事务外比较 currentPassword。未匹配立即失败。匹配后交给共用命令，不先写删除标志、不先撤销登录。UserEtag 防止删除、恢复均已完成后，旧认证快照再次删除恢复的身份；不作为管理 Delete 请求参数，也不改变仅 name 的管理动作语义。

新增 proof 的状态／etag／凭据不匹配统一返回 ErrInvalidCredentials，由现有 accountError 转安全的密码验证失败；已找不到当前登录或身份（含并发软删除或永久删除返回的 ErrNotFound）则转为 ErrSessionRevoked／UNAUTHENTICATED（401），并在 `service/account.go` 的 `accountError` 中补充 `ErrNotFound -> UNAUTHENTICATED` 以防其他需登录方法落入 500。服务身份管理删除不存在仍返回 ErrNotFound。数据库依赖失败保持独立内部错误，不吞成密码错误。

## 4. 用户锁与普通访问保护

在 `internal/data/authn.go` 定义两个固定语义的函数，不给调用方可变的 includeDeleted 开关：

```go
func lockUser(ctx context.Context, tx *entmodel.Tx, userID string) (*entmodel.User, error)
func lockUserIncludingDeleted(ctx context.Context, tx *entmodel.Tx, userID string) (*entmodel.User, error)
```

- lockUser：`tx.User.Query().Where(user.IDEQ(userID), user.DeleteTimeIsNil()).ForUpdate().Only(ctx)`。保持默认 interceptor，未找到翻译为 ErrNotFound。
- lockUserIncludingDeleted：同一 ID 的 FOR UPDATE，Only 仅在函数内部使用 `entgomixin.SkipSoftDelete(ctx)`。返回 entity，不返回绕过 context。只由 Undelete、Expunge 调用。
- 两者都锁 `users` 实际记录而非应用进程 mutex；复用 `inTx` 的 ReadCommitted。删除／恢复／永久删除、登录、改密、状态变更、资料修改、token 建立／消费及 OAuth 身份关联写入统一先锁 User，再读取或修改关联行。允许事务外只读定位 userID，但事务内必须重读并核验，不能先锁 token 再锁 User。
- SoftDeleteMixin 只拦截查询和 Delete，不拦截一般 Update。所有普通 User Update 在锁后仍显式使用 `DeleteTimeIsNil()`，不能把“已在 biz 读过”当作写保护。
- `UpdateUser` 的 immutable 校验以 `users.Get(ctx, name.User)` 取得未删除目标，不再使用含删除记录的管理 GetUser。`updateProfileFields` 改为 inTx 内锁用户、比 etag、执行 profile mask 与清空逻辑、更新 etag/update_time。
- `UpdateStatus` 同样锁后检查未删除和最新 status；不能只依据事务外 current 判断 Enable 的合法前态。DisableUser 允许作用于未删除的待验证、正常及已封禁账号（幂等）；EnableUser 在锁内要求当前状态为 `disabled`，并查询主邮箱 `LoginIdentifier.verified_time`——非空恢复为 `active`，为空恢复为 `pending_verification`（认证器均恢复 `active` 并清空 `revoked_time`），防止未验证账号解禁后落入 `ACTIVE` 且未验证的死锁态。删除目标返回 NOT_FOUND；存在的未删除目标 etag 不符才是 ETAG_MISMATCH。
- `CredentialRepo.ReplacePassword`、管理设密、SessionRepo.Create、PasswordResetTokenRepo 的提交和 OAuthRepo.Issue 都使用默认 lockUser。认证入口再检查 ACTIVE、邮箱验证、有效认证器和 required_actions；不是只检查 status。
- `SessionUsecase.Resolve` 继续走 `users.Get`，找不到删除或永久删除用户映射为 ErrSessionRevoked。普通本人资料入口不经管理 GetUser。SCS 内容不能绕过这些检查。
- 事务外读快照不作为写授权凭据，所有安全副作用均在锁内重验；数据库故障保持内部错误，不伪装为未登录或不存在。

## 5. 软删除与恢复事务

### 5.1 SoftDelete

在 `internal/data/user.go` 用一次 `inTx` 完成：

1. 默认 lockUser；不存在／已删除均 ErrNotFound。
2. 若 proof 非 nil，检查 UserEtag、ACTIVE、邮箱已验证、无未完成动作；根据 LoginID 查询同用户且 revoked_time 为空的 IAMLoginSession，复验四元凭据中的 authenticator 归属、password 类型、active、未撤销以及具体 PasswordAuthenticator.ID/hash。任何过期 proof 都失败，不自动用新状态重试。
3. 分配新 etag，User.UpdateOneID 加 `DeleteTimeIsNil()`，SetDeleteTime(now)、SetEtag、SetUpdateTime、ClearPurgeTime。用明确 Update 而非依赖 Delete hook，才能和其余领域变更共用同一 now/etag 及事务。
4. 调用已有 `revokeUserSessions(ctx, tx, userID, "", now)`，撤销全部 IAM 登录以及 OAuth token session/access/refresh。加强 `revokeOAuthSessions` 的 access 更新条件为 `ActorTypeEQ(human)`，使任何用户侧处理都不更新 service token；删除事务再以 `actor_type = 'human' AND subject = U AND revoked_time IS NULL` 补齐用户 access 撤销，不遗漏没有 token_session_id 的 human 记录。
5. 对 `email_verification_tokens`、`password_reset_tokens` 使用 `UserIDEQ(userID), ConsumedTimeIsNil()` 写 `SetConsumedTime(now)`。包括尚未过期的全部旧令牌；不删除身份凭据，不清邮箱验证状态。
6. 按 §6 的 R/C 集合删除目标已绑定的 OAuth authorization code 和 OIDC request。它们没有 revoke 字段，且 expires_time 在 schema 中 Immutable，不能设计成不存在的 SetExpiresTime 更新。保留未关联身份的全新授权请求。
7. 在事务内用该 tx 的 User 和 LoginIdentifier 构造返回 User，局部跳过 tombstone 查询；不要提交后调用默认 repo.Get，否则必然隐藏新删除记录，也不要提交后重新读取而混入另一并发恢复的结果。

任一步失败回滚所有数据库变更。删除完成后邮箱仍占用，密码哈希、认证器状态、status、required_actions 不变；恢复不会复活旧登录、旧 refresh token、旧授权码或一次性令牌。已离线验签的 JWT 仍允许自然过期，不声称跨应用本地 session 被清除。

### 5.2 Undelete

同一事务执行 lockUserIncludingDeleted；不存在 ErrNotFound，delete_time 为空 ErrAlreadyExists。存在删除标记时 ClearDeleteTime、ClearPurgeTime、ClearDeletedBy，更新 etag/update_time；不改 status／凭据／验证事实／required_actions，不清除 revoked_time 或 consumed_time。使用事务内 mapper 返回 User。

待验证用户恢复后仍待验证，从既有 CAP 重发入口重新取得验证邮件。被封禁用户恢复后仍被封禁。没有恢复期限。

## 6. Expunge 的表级集合、谓词与顺序

在用户锁内确认 delete_time 非空；未删除目标 ErrUserInvalidState，缺失 ErrNotFound。锁定后从当前数据库收集这些 ID，不依赖 UI、请求参数或事务外缓存：

- U：目标 User.ID。
- L：`iam_login_sessions WHERE user_id = U` 的全部 ID，包含已撤销。
- A：`authenticators WHERE user_id = U` 的全部 ID，包含 disabled/revoked。
- S：`oauth_token_sessions WHERE user_id = U` 的全部 ID，包含已撤销。
- R：`oidc_authorization_requests WHERE subject = U OR iam_login_session_id IN L` 的 ID。
- C：`oauth_authorization_codes WHERE subject = U OR authorization_request_id IN R OR token_session_id IN S` 的 ID；把这些 C.authorization_request_id 并入 R。然后删除 C 或 authorization_request_id 属于扩展 R 的 code。该关联来自 IAM 自己保存的 ID，不按客户端、邮箱或字符串前缀猜测归属。

各 schema 都使用标量 ID，没有声明可依赖的 Ent edges/数据库级联。按以下顺序显式删除，空 ID 集合不生成无条件 Delete：

| 顺序 | 实际表名 | 删除谓词与注意事项 |
| --- | --- | --- |
| 1 | `oauth_refresh_tokens` | `token_session_id IN S`，不只删未撤销或当前 family。 |
| 2 | `oauth_access_tokens` | `actor_type = 'human' AND (subject = U OR token_session_id IN S)`；subject 分支覆盖没有 session ID 的历史 human 记录。service 类型即使 subject 文本恰等于 U 也保留。 |
| 3 | `oauth_authorization_codes` | `id IN C OR authorization_request_id IN R`，包含已消费／过期。 |
| 4 | `oidc_authorization_requests` | `id IN R`，包含过期或 done 的身份关联请求。 |
| 5 | `oauth_token_sessions` | `id IN S`。 |
| 6 | `iam_login_sessions` | `user_id = U`，不只删活跃。 |
| 7 | `email_verification_tokens` | `user_id = U`，不只删未消费。 |
| 8 | `password_reset_tokens` | `user_id = U`，不只删未消费。 |
| 9 | `password_authenticators` | `authenticator_id IN A`。 |
| 10 | `authenticators` | `user_id = U`。 |
| 11 | `login_identifiers` | `user_id = U`，删除全部类型；不只删 email 类型。 |
| 12 | `users` | `id = U AND delete_time IS NOT NULL`，仅此硬删除调用使用局部 SkipSoftDelete；检查影响行数为 1。 |

软删除 §5 复用同一 R/C 计算仅删 code/request，保留身份、凭据及已撤销会话历史。整个 Expunge 单事务提交（任一步报错全量回滚），全部完成后才释放邮箱唯一键；不触碰 `oauth_clients`、`oidc_signing_keys`、独立 service access token、其他用户或跨服务数据。

`schema/http_session.go` 的 `sessions(token,data,expiry)` 没有 user_id，继续由 `postgresstore.New` 的现有 TTL 清理。Expunge 删掉 IAMLoginSession 后，旧 cookie 的 iam_login_id 不能解析身份。即使另一个已开始的 HTTP 请求晚到保存旧 SCS 容器，该无效引用也不能复活身份；不扫描／解码全表，不增加映射或 Store adapter。

### 6.1 并发结果

- 恢复与永久删除共享 U 行锁：恢复先提交，Expunge 看到未删除并拒绝；Expunge 先提交，恢复找不到目标。不能两者都成功操作同一个删除状态。
- token/login/code 事务先提交，删除随后锁到 U 并撤销／删除这些记录；删除先提交，晚到的用户关联写操作取得锁后发现删除而拒绝。Expunge 后默认锁找不到用户，不能创建孤儿记录。
- Delete→Undelete 都在晚到请求取锁前完成时，只判断未删除不够。普通登录、首改、reset 提交和 token Create 必须复验最初读到的 User.etag。etag 不匹配表示创建流程前提已变化，保守失败且不自动重试；登录和首改的完整签名由凭据设计提供，使用同一快照，不新增版本字段。
- 自助删除 proof 同样携 UserEtag；管理 Delete 按当前记录执行，故不带此限制。

## 7. 验证／恢复令牌与邮件竞争

两类 token Create 调整为：

```go
type VerificationTokenRepo interface {
    Create(ctx context.Context, userID, identifierID, expectedUserEtag,
        tokenHash string, expires time.Time) error
    ConsumeAndActivate(ctx context.Context, tokenHash string, now time.Time) (string, error)
}
// PasswordResetTokenRepo 中变更的 Create 方法签名：
Create(ctx context.Context, userID, expectedUserEtag, tokenHash string, expires time.Time) error
```

Create 在用户锁中重查 etag 和资格：验证邮件要求 pending_verification、email identifier 属于目标且未验证；reset 要求 ACTIVE 且邮箱已验证。identifierID 为空时在事务中定位 email identifier，非空必须验证 ID/归属/type，不再事务外定位后无条件插入。资格或 etag 已变时返回 ErrUserInvalidState；未知／已删除返回 ErrNotFound。

`AccountUsecase.Register`、公开重发、公开申请 reset、管理 SendUserVerificationEmail 均传最初读得的 etag。公开重发／reset 捕获上述状态竞争并沿用空成功，不发邮件；管理发送转为 NOT_FOUND 或 INVALID_STATE。自助注册保持创建后发信的既有行为，不回滚已创建账号。

ConsumeAndActivate 先只读 tokenHash 定位 userID，随后 inTx：先锁 User（若返回 ErrNotFound 则转为 ErrInvalidVerificationToken，避免软删除或永久删除目标泄漏原始 ErrNotFound），重读 token 并检查 hash、user_id、未消费和期限；检查 token.login_identifier_id 所属邮箱、尚未验证和 pending 状态；同事务设置 token consumed_time、identifier verified_time、User ACTIVE、新 etag/update_time，并返回 userID。移除 VerificationTokenRepo.Consume 与 UserRepo.ActivateEmail 的拆分生产调用，AccountUsecase.VerifyEmail 改调用新方法后读取 User（若提交后立即被并发删除，同样转为无效令牌而非 500）。直接调用 ActivateEmail 的测试夹具改用实际验证流程或显式夹具写入，不保留可绕过锁的生产后门。

Delete 与 VerifyEmail 都先锁 User，因此不存在消费成功但激活失败的拆分提交；删除先完成时旧 token 不可用，验证先完成时删除保存最新 ACTIVE／verified 事实。删除事务失败不消费 token；验证事务失败也不消费 token。

SMTP 保持在 token 事务后同步发送。不在持锁期间调用 SMTP，也不增加第二次“检查后发信”来假装与 SMTP 原子。token 保存先提交、随后删除已使 token 无效，即使邮件在删除后送达，其链接也不可消费。删除后的新 token 不得入库；已发出的邮件不是账号恢复凭证。

邮箱规范化沿用 trim/NFC/case-fold。认证路径的 FindByEmail 隐藏软删除 User，因此创建前预检查与启动 UserInitializer 改用 `FindByEmailIncludingDeleted`：创建前发现软删除或正常记录占用该邮箱即提前返回 ErrEmailAlreadyExists，免去无效的 Argon2id 哈希与写事务回滚；`createUserAggregate` 的唯一约束错误转换仍保留作并发兜底。不要建立仅未删除用户的 partial unique index，不清空／改写被删除用户邮箱。只有 Expunge 成功提交后，同邮箱注册才能成功，新建流程分配新的 UUIDv7 ID；旧 token 不得绑定到新 ID。

## 8. OIDC 身份关联操作的具体修改

在现有 `biz.OAuthRepo` 增加：

```go
CompleteAuthorizationRequest(ctx context.Context,
    requestID, userID, loginID string, now time.Time) error
SaveAuthorizationCode(ctx context.Context,
    requestID, codeID, codeHash string, now, expires time.Time) error
```

`data/oauth.go` 实现事务；`oidc/op_auth_storage.go` 保留适配方法签名，不新增构造依赖：

- CompleteAuthRequest 委托 CompleteAuthorizationRequest。事务先 lockUser(userID)，验证 ACTIVE、已验证邮箱、required_actions 为空、loginID 属于 U 且未撤销；重查 pending request 的 done=false 和期限；auth_time 从数据库 login.CreateTime 获取，不信任旧调用参数，设置 subject/login/auth_time/done，检查影响一行。
- SaveAuthCode 在 adapter 中生成 ID、hash 及期限后委托 SaveAuthorizationCode。Repo 事务外仅按 requestID 定位 subject；事务先锁该 User，重读请求并复验同 subject、done、未过期及绑定 login 有效，再从最新请求复制 client/redirect/scopes/PKCE 保存 code。删除后请求缺失或登录撤销时统一 grant 无效，不能根据先前 request snapshot 插入孤儿 code。
- 当前 OAuthRepo.Issue 已在 human 分支先锁 User，再复验 code/request/login/refresh；补充邮箱验证及 required_actions 条件。service grant 继续独立 `issueServiceToken`，不锁被管理用户，也不以用户名映射服务身份。
- 未绑定的 CreateAuthRequest 没有用户关联，无需用户锁。DeleteAuthRequest 保持按 requestID 的幂等清理；其动作只减少记录，不建立身份能力。
- 撤销入口 RevokeSession、RevokeClient 保持同一用户锁顺序；Expunge 后读不到相关 login/session 时按其既有幂等撤销约定返回，不重建任何关联。

上述 OIDC 写入统一纳入 `lockUser` 串行化，避免与删除／Expunge 并发产生孤儿记录。

## 9. 管理读取、过滤与分页

`data/user.go` 的 Get 与 GetUser 分别使用明确可见性：

- Get(ctx,id) 默认 `User.Get`，保持 tombstone interceptor；FindByEmail 找到 identifier 后仍走默认 Get。用于登录、SessionUsecase、AccountUsecase 和 OIDC 当前身份读取。
- GetUser(ctx,name) 仅查询与 mapper 调用内部使用 SkipSoftDelete，返回真实 status/delete_time/purge_time；其管理 usecase 调用结束不把绕过标志放入外部 context。
- `toResources` 拆出接受 `*entmodel.Client` 的 helper，普通读取传 repo.ent，生命周期响应传 tx.Client()。mapper 从实际 entity 转出删除时间并与现有 LoginIdentifier 一起填 email/verified，不伪造 DELETED status。

新增列表绑定：

```go
entcrud.Bind(userpb.UserFields.DeleteTime, user.FieldDeleteTime).
    Filter().Order().Nullable()
```

`service.UserService.ListUsers` 沿用 ListPreparer，只把 request.GetShowDeleted() 单独传 usecase/repo；不向不存在的 corecrud.ListInput.ShowDeleted 写字段。data 在 showDeleted 为 true 时局部 SkipSoftDelete，最后一个 scope 参数取固定的以下 bytes：

- false：`[]byte("iam.users.v1:active")`
- true：`[]byte("iam.users.v1:all")`

`entcrud.List(ctx, builder, query, fields, scope)` 已将 scope 与 filter/order/resource/collection 一起纳入 fingerprint（`../servora/contrib/db/entgo/crud/list_execution.go:124–146`）。因此拿 false 页 token 切 true 或反向切换都返回 INVALID_PAGE_TOKEN；不静默重启分页。默认 create_time desc 与 ID asc 稳定游标保持不变。

仅已删除列表的精确请求是 `show_deleted=true, filter="delete_time != null"`。现有 `query_binding.go:91–95,230–235` 将 nullable 的 null 不等比较转换为 SQL IS NOT NULL；框架现有 `live_contract_integration_test.go:676–680` 也包含同一表达式。未删除过滤为 `delete_time = null`；false 配合 `delete_time != null` 得空列表，不能偷偷开启 show_deleted。不要发明 `status=DELETED`、`has(delete_time)` 或 SQL 字符串输入。改变过滤也由既有 fingerprint 拒绝沿用原 token。

## 10. 自助删除 HTTP、页面与失败语义

在 `app/iam/service/api/protos/iam/account/v1/account.proto` 增加：

```proto
rpc DeleteAccount(DeleteAccountRequest) returns (DeleteAccountResponse) {
  option (google.api.http) = {
    post: "/v1/iam/account/delete"
    body: "*"
  };
  option (plateau.security.authn.v1.rule) = {mode: AUTHN_MODE_REQUIRED};
}
message DeleteAccountRequest { string current_password = 1; }
message DeleteAccountResponse {}
```

current_password 使用现有 8–128 校验、REQUIRED 和 redact 注解；不含 name/user_id/email。AccountService 默认 AUTHZ_MODE_NONE 保持，本人授权来自普通 session 身份而非 manage_users。AccountService 仍只注册在 HTTP Server，不把需要浏览器加载状态的方法挂到管理 gRPC。

service 顺序固定：先确认 `security/session.Loaded(ctx, manager)`，然后 `authn.From(ctx)`，再调用 AccountUsecase.DeleteAccount；数据库事务成功后 Destroy 当前 SCS，成功才返回空响应。不先 Destroy，不在缺失 HTTP session 接线时执行删除。

`security/session/session.go:72–75` 已在保存失败时写安全的 500 并停止成功正文。沿用该行为：数据库已经提交，Destroy／Store／响应写入失败不能回滚删除；旧 cookie 后续仍无法解析有效 IAM login。页面对未知错误提示“无法确认是否已完成删除，请重新确认账户状态”，不能显示“账号未删除”，也不能自动重复删除请求。明确密码错误留在表单；未认证转登录；成功使用完整导航到 `/login/` 清理页面状态，不额外调用 Logout 以制造第二个易失败步骤。

在 `app/iam/web/app/account/security/page.tsx` 现有安全页添加独立删除区：明确账号将退出、仅管理员可恢复、不是用户永久删除；用户确认后输入当前密码并提交。复用 useSensitiveValue、useSingleFlight、StatusMessage 和生成 account client。请求结束与取消清空密码；禁用并发提交；不把密码写 URL/localStorage；不渲染或提交任何目标用户 ID。页面提示不替代后端凭据复验。

## 11. 独立永久删除权限

`manifests/openfga/iam.fga` 的 iam 类型增加：

```fga
define expunge_users: [service]
```

不定义 `expunge_users: manage_users`，不通过 Admin 人的 admin 关系继承。`ExpungeUser` 的方法 authz rule 显式指定 REQUIRED、action=expunge_users、resource_type=iam、resource_id=global。现有 `server/grpc.go` 的服务 JWT 与 OpenFGA 中间件读取生成规则即可执行；服务 token 的 actor_type、issuer、audience 继续按现有可信 mapper 校验，人的 token 或伪造 metadata 不得替代。

`manifests/openfga/iam.fga.yaml` 的 service:admin 显式增加 expunge_users tuple；增加只有 manage_users 的 fixture service、只有 expunge_users 的 fixture service，并分别断言：普通管理不推导永久删除，永久删除授权不推导普通管理；无关系服务、错误对象 iam:other、人类 user:admin/user:alice 均没有该权限。Admin 运行身份须在部署配置中显式具有两种关系，测试夹具不是运行授权数据。

`internal/server/grpc_integration_test.go` 当前仅写 manage_users tuple，后续增加分离授权的真实消费者案例，并按 fixture 创建的准确 tuple 清理。`fixture_test.go` 保持单独的隔离 store/model 配置，不在应用构造函数自动写授权关系。模型装配沿用已有 manifests/OpenFGA 流程，不改 SDK 所有权。

错误要求：管理 Delete 重复／不存在为 NOT_FOUND；Undelete 活用户为 ALREADY_EXISTS；Expunge 活用户为 FAILED_PRECONDITION。UserUsecase 对 ErrAlreadyExists 作明确映射，不把邮箱重复和恢复冲突混为同一文案。现有 Kratos 409 不能保证 gRPC ALREADY_EXISTS，必须采用主设计的准确 gRPC code + ErrorInfo.reason 转换方案；不得仅依赖同 HTTP code 的生成 helper。数据库失败返回安全内部错误，不泄漏 token/hash/cause；权限检查先于目标读取。

## 12. 必须同步的文件与调用方

| 文件组 | 必须变更 |
| --- | --- |
| `app/iam/service/api/protos/iam/user/v1/user.proto`、`iam/account/v1/account.proto` | 删除字段、show_deleted、管理三动作、自助删除及各自注解／错误。 |
| `internal/biz/user.go`、`account.go`、`user_initializer.go`、`oauth.go` | 上述 Repo/usecase 签名、共用命令、含软删除邮箱查重、VerifyEmail 原子提交、公开反枚举竞争处理。 |
| `internal/data/schema/user.go`、`data/user.go`、`authn.go`、`account.go`、`oauth.go` | mixin、两个锁函数、全部删除保护、表级事务、OAuth 保存方法及 token 创建／消费。 |
| `internal/service/user.go`、`account.go` | 资源解析、show_deleted 传递、空动作响应、可信本人身份及 SCS 销毁。 |
| `internal/oidc/op_auth_storage.go`、`internal/oidc/claims.go` | 将身份绑定和 code 保存从直接 Ent 写入切到现有 tokens Repo；populateUserinfo 对齐未删除、邮箱已验证与动作为空检查。 |
| API/Ent/TypeScript/CRUD/authn/authz 生成目录 | 随 schema/proto 统一再生成；删除过时接口生成物，不留别名。 |
| `app/iam/web/app/account/security/page.tsx`、必要的 `lib/errors.ts` | 本人删除表单、清密码、错误与导航，不改 Admin Web 产品范围。 |
| `manifests/openfga/iam.fga`、`iam.fga.yaml` | 独立关系与正负 fixture；部署关系明确分开。 |
| `internal/biz/*_test.go`、`internal/data/*_test.go`、`internal/server/*_test.go`、`internal/oidc/*_test.go` | 同步签名、移除 ActivateEmail 假接口，覆盖下面的消费者结果；不保留绕过新事务的旧 helper。 |
| `.trellis/spec/iam/identity.md`、`sessions.md` 与任务主设计／实施说明 | 固化管理读取与认证读取区别、统一用户锁顺序、无限保留和明确 Expunge 授权。 |

上述接口与文件清单在规划期固定，实施时按此直接落地。

## 13. 确定性的消费者验收案例

以下为后续业务验收应观察到的结果：

1. 对 pending／ACTIVE／disabled 用户分别 Delete，再管理 Get：ID、原 status、hash/验证事实/required_actions 保留，delete_time 有值、purge_time 空；默认列表和认证隐藏，恢复后原状态不变。
2. Delete 缺失／已删除均 NOT_FOUND；恢复活用户 ALREADY_EXISTS；Expunge 活用户 FAILED_PRECONDITION，已物理删除则 NOT_FOUND。响应必须经真实 gRPC 链保持指定 code 与 reason。
3. 默认列表、全部列表、删除专用过滤给出各自正确集合；切 show_deleted 或 filter 使用原 page token 得 INVALID_PAGE_TOKEN，不漏回收站记录、不重复伪造 status。
4. 单个用户拥有多 login、多 OAuth client、多代 refresh、消费／未消费 code 和验证／reset token；Expunge 后 §6 所有目标记录均无，其他用户与独立 service token 保留。故意选取 subject 文本等于 U 的 service token 仍保留。
5. 数据库任一步失败，User、标识、关联记录共同保留；恢复与 Expunge 交错只有一种合法成功；删除与资料／状态／设密／登录／token 生成交错不会写出活的孤儿身份能力。
6. Delete→Undelete 在等待中的旧登录或发信操作前完成，旧 etag 快照被拒绝；用户重新发起请求读取当前状态后才可执行。旧验证码／reset 链接和旧 refresh token 不因恢复重新生效。
7. VerifyEmail 与 Delete 交错：验证先完成则删除保留已验证事实；删除先完成则验证失败且没有分离的激活提交。重复消费不能再次成功。
8. 自助删除密码缺失／错误、未登录或伪造目标字段均不产生删除副作用；正确本人操作撤销全部 IAM/OAuth 登录，成功退出。事务提交后 SCS 删除失败时不返回虚假成功、也不声称删除回滚，旧 cookie 仍不能继续访问。
9. 软删除邮箱继续阻止同规范化邮箱注册；Expunge 回滚仍占用；Expunge 成功后同邮箱新注册获得新 ID，原验证码不能验证新用户。
10. manage-only 服务可 Delete/Undelete，不可 Expunge；expunge-only 服务只可执行被授予动作；human、错 audience、错对象或无授权调用均在读取目标前拒绝。Admin 正式运行身份明确配置两种授权。
