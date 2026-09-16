# IAM 生命周期设计证据

本文服务于 `09-15-admin-initial` 的 design 阶段。只记录基于当前 checkout 的实现证据与技术方案边界，不修改产品代码或规范，也不把方案写成已实现能力。

2026-09-16 后续调整：文中 Servora mixin 路径保留为调研时的源码事实；最新 R22 计划将便利层迁入 Plateau infra。依赖、实际基线测试与迁移范围见 [归属核查](ent-mixin-ownership.md)，实施以主 design 的新归属为准。

## 结论与边界

IAM 需要把“删除用户”设计成一个 IAM 领域命令，由管理删除和 AccountService 自助删除共同调用；两个入口各自完成 AuthN/AuthZ、操作者/目标绑定和确认校验。删除、恢复、到期清理不得由 Admin 直接改表，也不得让 AccountService 通过受管理权限保护的 UserService RPC 间接复用。

当前 `User` 没有软删除字段，只有稳定 ID、状态、profile、etag 和时间字段（`app/iam/service/internal/data/schema/user.go:13-41`）。Servora `SoftDeleteMixin` 可以提供 `delete_time`、`deleted_by`、`purge_time`、默认 tombstone 过滤和 Delete-to-Update 改写（`/Users/horonlee/projects/go/servora-kit/servora/contrib/db/entgo/mixin/soft_delete.go:36-55`、`:72-98`），但不会遍历关联表、实现 Undelete/Expunge、计算 `purge_time` 或运行清理任务（`/Users/horonlee/projects/go/servora-kit/servora/docs/crud.md:473-506`）。因此 IAM 必须显式组合生命周期事务。

软删除期间保留 `User`、稳定 ID、login identifier、密码、邮箱验证事实和删除前账号状态；同时立即撤销所有 IAM login session 及 OAuth token session/access/refresh token，并在同一删除事务中删除 email verification/reset token。后两类 token 没有可复用的撤销字段，若保留会在恢复后复活敏感凭证，因此不应随 User 恢复。保留密码和状态是为了恢复原账号事实，撤销会话保证旧认证事件不能复活。登录、Session Resolve、OIDC user/token 入口都必须把 tombstone 当作不可用身份；恢复清除 tombstone 后，原状态决定是否可登录。恢复不恢复旧 session/token，也不新增或清除首次改密要求。

## 建议的数据模型

给 `User` 加 `SoftDeleteMixin`，生成 `delete_time`、`deleted_by`、`purge_time` 及对应索引。`purge_time` 在删除事务内由当次生效的 `recovery_period` 计算并持久化；不能在清理时按最新配置重算。默认恢复期为 30 天，配置值使用 `google.protobuf.Duration`，建议配置键为 `soft_delete_recovery_period`，默认值 `720h`。

`deleted_by` 保存 IAM 内部约定的操作者标识；管理删除可写入 canonical Actor，用户自助删除可写入当前 IAM user 标识。不要把 Admin 的前端用户输入当作可信操作者。删除原因若需审计，应另行进入审计数据，不把原因塞入软删除字段。

当前 `LoginIdentifier` 的 `(type, canonical_value)` 是全局唯一，且每个用户每种 type 唯一（`app/iam/service/internal/data/schema/login_identifier.go:29-34`）。现状 `LoginIdentifier` 与 `User` 都未接入软删除拦截器；设计若只给 `User` 加 `SoftDeleteMixin`，拦截器只会作用于 User 查询，不会自动过滤 LoginIdentifier。恢复期必须保留该行，因此邮箱继续占用；不要改成仅 active 行唯一，否则会破坏“删除期间不能重新注册同邮箱”的已确认语义。清理成功后才物理删除该 login identifier，数据库唯一约束自然允许新账号使用邮箱；新注册仍由 `NewUserID` 产生全新 ID。

## 删除事务

建议在 data 层提供一个只由 IAM biz 命令调用的删除原语，例如 `DeleteUserLifecycle(ctx, userID, expectedEtag, deletedBy, now, recoveryPeriod)`，而不是让两个 service 入口各自拼 Ent 更新。事务复用现有 `inTx`（`app/iam/service/internal/data/transaction.go:12-33`）和 `lockUser` 的 `SELECT ... FOR UPDATE` 边界（`app/iam/service/internal/data/authn.go:36-40`）：

1. 以 user ID 加锁并读取 tombstone；要求 `delete_time IS NULL`、etag 匹配和目标状态允许删除。锁必须先于状态判断和 `purge_time` 计算，防止删除、恢复、禁用、改密、登录发行并发交错。
2. 在同一事务写入 `delete_time=now`、`purge_time=now+recoveryPeriod`、`deleted_by` 和新 etag。不要调用未提供 `purge_time` 赋值语义的普通 `Delete` 后再补字段；若使用 Mixin 的 Delete 改写，必须在同一事务内显式补齐固定截止时间并核对受影响行数。
3. 调用现有 `revokeUserSessions`，按 user ID 撤销所有 IAM login session 及其 OAuth token session、access token、refresh token（`app/iam/service/internal/data/authn.go:135-163`）。不要删除这些会话行，因为恢复不能复活会话，但清理前保留它们可支持审计与一致性检查；同一事务应删除 verification/reset token 行，避免恢复后旧敏感 token 可用。
4. 不修改 User status、login identifier、password hash、email verified time 或首次改密状态；登录侧通过 tombstone 过滤拒绝身份。这样恢复可以准确保留删除前状态，而不会把 disabled/pending 用户错误恢复成 active。

管理删除的 UserService 目标由资源名和可信 service Actor 绑定；AccountService 自助删除目标只取 `authn.From(ctx)`，重新验证当前密码后调用同一领域命令。自助删除成功后另行销毁当前 SCS cookie；该传输层动作不替代领域撤销。

## 恢复事务

建议提供 `UserService.UndeleteUser`；Admin 入口先校验操作者的人类 Admin 管理资格，随后以 IAM service 身份调用 UserService，由 IAM 仅按 `iam.manage_users` 服务身份授权执行，不让 IAM 回查 Admin 的人类资格。data 原语应在 `SkipSoftDelete(ctx)` 下显式查询 tombstone，再对同一 user 行加锁；Servora 的 `SkipSoftDelete` 同时绕过查询过滤与 Delete 改写，只能作为 IAM 内部恢复/清理实现细节，不能暴露给调用方（`/Users/horonlee/projects/go/servora-kit/servora/contrib/db/entgo/mixin/soft_delete.go:25-33`、`:113-137`）。

恢复检查应采用 `now < purge_time`；`now == purge_time` 视为恢复期已到，由清理竞争者取得锁并执行清理。锁内重新检查 tombstone，成功时只清除 `delete_time`、`deleted_by`、`purge_time` 并写新 etag。不得重建 User、login identifier 或 authenticator，不得恢复任何 login/token；原 status、password hash、email verified time 和未完成首次改密状态自然保留。原本 disabled 或 pending verification 的用户恢复后仍保持对应状态。

恢复与删除并发时，二者必须由同一 user 行锁串行化：恢复先提交则删除随后重新读取 active；删除/清理先提交则恢复看到已到期或缺失 tombstone 并失败。恢复与 purge 不得出现一方清除邮箱而另一方又恢复旧身份的中间结果。

## 到期清理与关联表

清理 worker 只处理 `delete_time IS NOT NULL AND purge_time <= now` 的用户，按批量参数选择 tombstone。每个用户独立事务、先锁 user 行并再次检查条件；清理失败回滚，邮箱继续占用，下一轮重试。成功提交前不能让注册看到邮箱可用。

清理事务应按外键/业务依赖显式删除下列 IAM 记录，然后物理删除 User：

- `email_verification_tokens.user_id`、`password_reset_tokens.user_id`；
- `oauth_access_tokens`（按 token session ID 或 subject）、`oauth_refresh_tokens`（按 token session ID）、`oauth_token_sessions`（按 user ID）；
- `oauth_authorization_codes` 与 `oidc_authorization_requests`：按 subject/user ID 及其 `iam_login_session_id` 关联清除未完成授权材料；
- `iam_login_sessions.user_id`；
- `password_authenticators.authenticator_id`，再删除 `authenticators.user_id`；
- `login_identifiers.user_id`，最后删除 `users.id`。

当前 schema 证据显示上述关联字段分别存在于 `app/iam/service/internal/data/schema/email_verification_token.go`、`password_reset_token.go`、`oauth_authorization_code.go`、`oidc_authorization_request.go`、`oauth_token_session.go:16-38`、`iam_login_session.go:16-29`、`authenticator.go` 和 `login_identifier.go`。OAuth token session 明确保留 `iam_login_session_id`，但 authorization code/request 可能只在流程完成后具备关联，清理必须覆盖 user subject 和 login ID 两条路径。不得删除其他服务的业务用户、OpenFGA tuple、审计记录或历史引用；稳定 ID 的外部引用由所属领域处理。

清理实现不能只依赖 User 的 SoftDeleteMixin。Mixin 不处理关联模型或唯一索引（`/Users/horonlee/projects/go/servora-kit/servora/docs/crud.md:483-506`）；所有子表删除、影响行数检查和最终 email 释放必须在 IAM 领域事务中完成。清理查询可以使用 `SkipSoftDelete`，但每个表仍需显式 user/subject/session 谓词，禁止无条件全表清理。

## 并发与注册邮箱

删除、恢复、purge、禁用、密码替换和 OAuth issuance 都应先使用同一 `lockUser` 边界。当前登录和 OAuth issuance 已在事务中锁 User（`app/iam/service/internal/data/oauth.go:26-37`、`:103-119`）；新生命周期命令应保持该顺序，避免 issuance 在删除提交后继续创建 token。

注册先按 canonical email 查找 login identifier，再读取 User（`app/iam/service/internal/data/user.go:159-164`）；若只给 User 加软删除拦截器，前者仍能查到 tombstone 的 identifier，后者会因默认 User 过滤而返回不存在。因此注册路径必须继续用独立唯一索引作为最终保护，并把 tombstone identifier 冲突转换为“邮箱已占用”；不能因为 User 查询被过滤就允许注册。purge 与注册的竞争由 login identifier 的全局唯一约束兜底：purge 未提交前 insert 必须失败，purge 成功提交后才允许新注册。恢复与新注册不会同时成功，因为恢复期仍保留 identifier。

## 配置与调度

IAM 当前配置由 `bootstrap.Scan` 在启动时读取并传入 `wireApp`（`app/iam/service/cmd/server/main.go:51-69`），时长惯例是 `google.protobuf.Duration`，YAML 使用带单位值，环境覆盖采用 `${ENV:default}`。建议在 `iam.conf.v1.IAM` 增加恢复期、purge 扫描间隔和批量大小；默认恢复期 30 天，扫描间隔和批量大小使用明确的正值校验。每次删除把计算后的 `purge_time` 固化，配置变更只影响后续删除。

当前未发现 IAM 已有生命周期 scheduler 或热更新 API。设计上应由 IAM 进程启动一个可停止的后台 worker，按 `purge_scan_interval` 批量查询并调用同一 purge 领域原语；worker 不复制删除 SQL，不按最新恢复期重算记录。配置仍是启动时加载，变更需重启。关闭流程须等待当前用户事务完成，避免进程退出时半清理。

## 建议入口与验证重点

建议公开接口为 `UserService.DeleteUser`、`UserService.UndeleteUser`、`UserService.ForceLogoutUser`，以及 `AccountService.DeleteAccount`；强制设密若已在同一 design 中增加，应与删除共用用户锁和全量撤销原语，但不能把 Admin 调用转成 AccountService 自助入口。所有入口返回领域状态冲突、过期、无权限和依赖失败，不把默认查询隐藏误报为“已清理”。

实施后至少应覆盖：

- delete/restore/purge 的提交与回滚；恢复截止点前后竞争及重复请求；
- User、login identifier、authenticator、verification/reset token、IAM login、OAuth session/access/refresh、授权 request/code 的完整关联清理；
- 删除后旧密码、旧 SCS login、旧 access/refresh token 均不可用；恢复保留原 status/password/email verification/首次改密状态且不恢复旧 session/token；
- 删除期间同邮箱注册失败，purge 成功后新 ID 注册成功，purge 失败时邮箱仍被占用；
- login、password/OAuth issuance、disable、delete、restore、purge 的同 user 并发锁与 etag 冲突；
- 配置默认 30 天、非法 Duration/扫描参数拒绝、已有记录的 `purge_time` 不随配置变化。

可执行检查命令（实施后，在当前根单 Go module 下）：

```bash
rtk proxy go test ./app/iam/service/internal/biz ./app/iam/service/internal/data ./app/iam/service/internal/service ./app/iam/service/internal/oidc
rtk proxy just lint
rtk proxy git diff --check
```

这些命令是后续实施验收建议，本次仅完成源码与设计证据整理，未运行测试、lint 或数据库清理验证。
