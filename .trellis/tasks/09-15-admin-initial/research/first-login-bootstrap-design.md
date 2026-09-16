# 首次登录改密与 Bootstrap 稳定绑定设计

本文承接 [PRD](../prd.md) 的 R16、R17、R18 以及[首位管理员初始化调研](bootstrap-initialization.md)，记录进入 design 阶段的推荐方案。本文只描述规划契约；当前仓库没有下述字段、RPC、受限凭证或 Admin 服务实现，也不修改 `.trellis/spec`。

## 设计结论

1. seed 与 `UserService.CreateUser` 写入同一个 IAM 首次改密状态，使用同一套登录、受限改密和完成后建普通会话的流程。
2. 正确初始密码只证明“可以开始首次改密”，不建立普通 IAM 登录会话，不写入 `iam_login_id`，也不能直接完成 OIDC 授权。
3. 受限改密凭证由 IAM 以一次性 opaque challenge 持久化管理；浏览器只在服务端 SCS 中持有 challenge 引用，响应体不返回可重放密码令牌。
4. 完成首次改密时，在一个 IAM 数据库事务内消费 challenge、替换密码、清除首次改密标志、撤销旧会话及 OAuth token，并创建新的普通 IAM 登录会话。事务成功后，页面才可继续原登录或 OIDC 回调。
5. 普通已有用户的管理重设密码不新增首次改密要求，也不清除已有的未完成要求；删除恢复保留该标志。
6. IAM 与 Admin 各自持久化一次性 bootstrap 绑定。邮箱只用于首次部署输入和首次定位，长期关系使用稳定 IAM user ID；完成后重启、seed 被封禁/删除、邮箱变更或同邮箱新建身份都不会自动重建或重新授予。

## 当前边界与新增契约

现有登录在账号 active、邮箱已验证且密码匹配后直接创建普通 `LoginSession`：[AuthenticationUsecase.Login](../../../../app/iam/service/internal/biz/authn.go)；HTTP 服务随后旋转 SCS token 并写入登录引用：[AuthnService.Login](../../../../app/iam/service/internal/service/authn.go)。现有 `PasswordAuthenticator` 只有密码 hash 和变更时间：[password_authenticator.go](../../../../app/iam/service/internal/data/schema/password_authenticator.go)；现有 `ChangePassword` 需要普通认证上下文、当前 LoginSession 和旧密码：[account.go](../../../../app/iam/service/internal/biz/account.go)。因此首次改密不能只在页面登录成功后追加跳转，必须增加 IAM 认证状态和服务端执行边界。

建议新增的协议名称仅作为 design 契约，具体编号与 HTTP 映射在 API 设计时确定：

| 契约 | 推荐行为 |
| --- | --- |
| `LoginResponse.required_action` | 枚举 `NONE`、`CHANGE_PASSWORD`。初始密码正确且仍需改密时返回 `CHANGE_PASSWORD`，RPC 可以成功返回用户概要，但这不表示已建立普通登录态。 |
| `AuthnService.GetFirstLoginStatus` | 公开声明为认证流程 RPC，但仅接受当前 SCS 中的受限 challenge，返回 `REQUIRED`、`EXPIRED`、`COMPLETED` 或 `UNAVAILABLE`，不返回普通会话或 OAuth 令牌。页面刷新时用它确认状态。 |
| `AuthnService.CompleteFirstLoginPassword` | 公开声明为认证流程 RPC，但仅接受受限 challenge 和 `new_password`；成功响应表示普通 IAM 登录会话已建立，页面可继续原流程。请求不接收目标 user ID、旧密码或外部 token。 |
| 受限认证上下文 | `GetFirstLoginStatus` 与 `CompleteFirstLoginPassword` 使用独立的 IAM challenge 认证分支；普通账户、profile、管理 RPC、OAuth/OIDC 入口不能把该上下文当作普通登录。 |

`required_action` 应是结构化字段而不是文案。错误 reason 至少需要区分 challenge 缺失/过期、账号当前不可用、首次改密已完成和新密码不符合策略；错误不得把初始密码或 challenge 写入日志。初始密码错误仍沿用通用登录失败，避免泄露账号或状态。

## 首次改密状态模型

### 密码认证器字段

推荐将状态放在密码认证器，而不是公开 `User` 资源或 Admin 数据库：

```text
password_authenticators
  must_change_password             boolean NOT NULL DEFAULT false
  password_change_required_time    timestamp NULL
  password_change_completed_time   timestamp NULL
```

现有 `id`、`authenticator_id`、`password_hash`、`changed_time` 保持不变。新增字段含义如下：

- `must_change_password=true` 表示当前密码只能作为一次性初始/临时密码使用，直到 IAM 成功完成首次改密。
- `password_change_required_time` 只在 seed 或管理创建写入首次要求时设置，便于审计和排查，不承载授权判断。
- `password_change_completed_time` 只在事务成功清除标志时设置；它不能单独使密码失效。

明文初始密码永不入库。`User` 资源不暴露上述字段，避免把内部凭据状态变成 Admin 或普通客户端可自行信任的授权事实。自助注册创建的密码认证器默认 `false`；seed 创建和 `CreateUser` 创建必须在同一个创建凭据事务内写入 `true`。

### 管理重设、恢复和普通改密的规则

- 管理员强制设密更新密码 hash、`changed_time` 并撤销目标全部 IAM/OAuth 会话，但保留 `must_change_password` 原值。无未完成要求的已有用户因此直接获得可用的新密码；已有要求的用户仍须首次改密。
- `ConfirmPasswordReset` 替换密码时也保留该标志；重置后的密码作为当前临时密码，用户仍需完成首次改密。
- 现有自助 `ChangePassword` 仍是正常用户流程：必须有普通 LoginSession 并验证旧密码；成功时只保留当前会话并撤销其它 OAuth token session。它不应被复用为首次改密入口。
- 删除事务应使未完成的受限 challenge 失效，但不得清除 `must_change_password`。恢复使用原凭据和原账号状态，仍保留该标志；用户恢复后重新登录并取得新的受限 challenge。恢复不复活旧会话或 token。
- 封禁用户不能登录或完成首次改密。seed 被封禁不触发启动时自动解禁，且不因启动重新生成身份。

## 受限 challenge 持久化与并发消费

推荐在 IAM 增加专用表 `first_login_password_challenges`，而不是把受限状态伪装成现有 `IAMLoginSession`：

```text
id                  opaque UUIDv7 / immutable
user_id             IAM stable user ID
authenticator_id    password authenticator ID
token_hash          high-entropy opaque value hash, sensitive, unique
created_time        timestamp
expires_time        timestamp
consumed_time       timestamp NULL
invalidated_time    timestamp NULL
```

表中的 `token_hash` 只能由 IAM 比对；原始 challenge 只放在服务端 SCS 的专用字段，例如 `iam_first_login_challenge`，不放 URL、localStorage、普通响应体或日志。SCS 仍是浏览器会话载体，数据库记录负责跨请求、重启和并发的一次性事实。challenge TTL 采用 15 分钟默认值并可配置。

登录成功但 `must_change_password=true` 时，IAM 在持久化 challenge 后旋转 SCS token，写入受限 challenge 引用，并清除/不写入 `iam_login_id`。同一用户再次用临时密码登录时，推荐在用户行锁内使旧的未消费 challenge 失效，再创建一个最新 challenge；这样不会留下多个都能改变密码的窗口。

登录创建 challenge 与所有失效路径统一按“先 user、后 challenge”的锁顺序执行。`CompleteFirstLoginPassword` 先用 token hash 做非锁定定位，再按以下顺序执行：

1. 仅按 token hash 非锁定查询 challenge，得到 challenge ID 和 user ID；找不到时立即返回无效 challenge。
2. 锁定该 user，要求账号未软删除、状态 active、邮箱已验证。
3. 按 challenge ID 重新读取并锁定 challenge，重新校验 token hash、user ID、未消费、未失效且未过期；随后锁定 active 且未撤销的密码认证器，并要求 `must_change_password=true`。
4. 用新密码明文验证密码策略，并用当前 hash 比对，拒绝与当前临时密码相同。Argon2id hash 带随机 salt，不能用 hash 字符串相等代替明文比对。
5. 撤销该用户所有现存 IAM login session、OAuth token session、access token 和 refresh token；这一步应复用同一领域事务中的按用户撤销逻辑。
6. 写入新 hash、`changed_time`、`password_change_completed_time`，并以 `WHERE must_change_password=true` 清除标志。
7. 将当前 challenge 写入 `consumed_time`，把同一用户其它未消费 challenge 写入 `invalidated_time`，并创建新的普通 IAM login session。
8. 提交事务后旋转 SCS token，删除受限 challenge 字段，写入新的 `iam_login_id`。

同一 challenge 的两个并发请求由 user 锁、challenge 行锁和 `consumed_time IS NULL` 保证只有一个成功。两个不同 challenge 并发消费时，仍先锁 user，再锁定并重新验证各自 challenge；只有一个能清除 `must_change_password`，另一个返回“首次改密已完成”，不能覆盖新密码。登录时失效旧 challenge 也必须先锁 user，再锁 challenge。数据库事务成功后若 SCS 写入发生网络/存储失败，不能回滚数据库；服务应返回基础设施失败并引导重新用新密码登录，不能再次消费已完成 challenge。

`GetFirstLoginStatus` 只读取 SCS challenge 和数据库状态，不创建会话；已过期或账号不可用时清理受限 SCS 字段。普通接口收到只有受限 challenge 的请求时必须拒绝，不能将其映射成当前用户的普通 `authn.From(ctx)`。

## 页面续跳与 OIDC 边界

当前 IAM Web 登录页在成功后，有 request ID 就跳转 `/authorize/callback?id=...`，否则进入 `/account/`：[login/page.tsx](../../../../app/iam/web/app/login/page.tsx)。首次改密应保留这一 request ID，但延后 callback：

1. 登录响应为 `CHANGE_PASSWORD` 时，页面跳转 `/account/first-password`，request ID 只作为经过现有一次性 URL 处理的续跳参数保留，不把 challenge 放进 URL。
2. 首次改密页挂载时调用 `GetFirstLoginStatus`；刷新、换页或 SCS 过期时能显示真实状态，而不是依赖前端内存中的“已登录”标志。
3. 新密码提交成功后，IAM 已建立普通登录会话。页面有 request ID 时再调用现有 `/authorize/callback?id=...`；没有 request ID 时进入 `/account/`。request ID 失效时显示重新从原应用发起登录，不伪造成功。
4. 因为改密前没有 `iam_login_id`，现有 OIDC callback 和 OAuth 授权绑定检查会把该请求视为未完成登录；这形成服务端阻断。完成改密后新建的普通 LoginSession 才能继续原 OIDC 流程，不需要把“必须改密”判断散落到每个 OIDC 协议分支。

正确初始密码后的 RPC 成功只代表获得受限改密资格。服务端不签发普通 bearer token、不建立可用于业务访问的 IAM session、不允许 profile/管理/授权接口继续执行。页面按钮只能改善体验，不能承担这个安全边界。

## seed 与 `CreateUser` 的共用入口

### 管理创建用户

`UserService.CreateUser` 当前委托 `AccountUsecase.CreatePendingUser`，创建待邮箱验证用户、密码认证器和验证 token；登录要求 active 且邮箱已验证。设计保持这一边界：创建事务设置 `must_change_password=true`，但不自动激活、不标记邮箱已验证。用户完成邮箱验证后才可能用初始密码进入首次改密，改密完成后才建立普通 IAM session。

Admin 不承接登录后的改密流程，不新增密码邮件、邀请、通知或 CLI。R16 已确认不把系统外人工交付列为本任务要求或验收前提；本设计保持该范围，`CreateUser` 的初始密码仍只是 IAM 创建请求中的凭据输入。

### seed 创建

新 seed 继续由 IAM 可信启动路径生成随机密码并写入 hash，但创建凭据事务同时设置 `must_change_password=true`。启动日志仍只在首次创建时交付一次明文初始密码；强制改密不能清除历史日志，也不新增邮件或邀请流程。

seed 创建完成后即可让 Admin 初始化关系；`must_change_password` 不影响关系写入，只影响该用户能否以普通 IAM/OIDC 会话进入 Admin。操作者先在 IAM Web 完成首次改密，再通过原 OIDC 登录 Admin。

### 旧 seed 升级

现有数据库没有 seed stable ID、创建来源或首次改密完成事实，只有当前按邮箱查找的启动逻辑；旧 `UserInitializer` 也会要求现有 seed active 且邮箱已验证。[bootstrap-initialization.md](bootstrap-initialization.md) 已记录这一缺口。

推荐采用兼容优先的两阶段策略：

- 新 schema 的 `must_change_password` 默认 `false`，不把所有历史用户强制改密。
- IAM 首次引入绑定时，必须通过一次性显式 stable user ID 迁移输入确认旧 seed，并把该 ID 写入 IAM bootstrap binding；Admin 只消费该 stable ID。迁移不能按邮箱自动认领，也不能在找不到旧身份时换用新 ID。
- 所有既有密码认证器迁移后的 `must_change_password` 默认保持 `false`，不把所有历史用户强制改密。显式确认的旧 seed 迁移可在确认当前凭据可用后设置 `must_change_password=true`，从而纳入同一首次改密流程；不能凭 `changed_time` 猜测，也不增加密码交付、CLI 或通知。
- 旧 seed 已封禁、已软删除、邮箱已被新用户占用或无法证明原稳定 ID 时，迁移失败并保持无 Admin 自动授予；不自动启用、不删除新用户、不因重启重试邮箱绑定。后续恢复必须是显式运维/管理动作，具体入口不在本设计新增。

## IAM 与 Admin 的 bootstrap 稳定绑定

### 单一部署输入

部署层只提供 `BOOTSTRAP_USER_EMAIL` 一份输入。IAM 与 Admin 的内部配置可以各自有 `bootstrap_user_email` 字段，但 local/Compose/生产部署都从同一环境变量映射，不再要求操作者维护两份独立值。当前代码仍使用 `IAM_BOOTSTRAP_USER_EMAIL`，新命名和旧变量兼容属于实施时的配置变更，本文不宣称已经生效。

邮箱只在绑定尚未存在时作为输入。绑定完成后，邮箱配置缺失或变更不应把系统切换到另一身份；记录启动警告并继续使用已持久化 stable ID。完成绑定后不再以该环境变量触发重绑。

### IAM 持久化记录

IAM 增加单例 `iam_bootstrap_bindings`：

```text
binding_key          fixed "primary"
user_id              stable IAM user ID, unique
canonical_email      first-bound normalized email
created_time         timestamp
completed_time       timestamp NOT NULL
```

首次部署时在同一 IAM 数据库事务内生成并写入 stable user ID、规范化邮箱、seed 身份、密码认证器、`must_change_password=true` 和完成标记。事务失败则身份与 binding 均不可见，重试重新执行同一原子初始化；不能先创建一个无 binding 的 seed，也不能每次以邮箱重新认领不同身份。

完成标记存在后 IAM 启动只读取 binding，不要求该 user 仍 active、未封禁或存在于未删除查询中，不自动重建用户或恢复认证器。用户被软删除、物理清理、邮箱被新身份占用或邮箱后来变更，都不能令启动过程按邮箱创建替代身份。

### Admin 持久化记录

Admin 增加单例 `admin_bootstrap_bindings`：

```text
binding_key          fixed "primary"
iam_user_id          stable IAM user ID, unique
state                PENDING / COMPLETED
created_time         timestamp
completed_time       timestamp NULL
```

Admin 第一次初始化不能直接以 `BOOTSTRAP_USER_EMAIL` 查询用户并授予关系。新增 `UserService.GetBootstrapUser`，以空请求返回初始化记录对应的 User；公开接口直接表达获取初始化用户，不暴露 binding 表结构：

- 调用复用既有服务身份认证链和 `iam.manage_users` 服务权限；该 RPC 是待实现的内部读取契约，但不新增 bootstrap 专用 action。它只读取当前部署已绑定的 seed 标识，不提供任意邮箱查找。
- IAM 服务端从自己的 `iam_bootstrap_bindings` 读取 `user_id`，再获取对应 User，而不是根据请求邮箱或创建时间选用户。初始化记录缺失返回未完成初始化错误，原用户被清理返回 NOT_FOUND，不自动选择替代身份；soft-deleted User 带 tombstone 返回供 Admin 拒绝首次授权。IAM 内部完成记录不因 User 不存在而清除，内部 binding 不设 `PENDING` 状态。
- Admin 首次收到响应后把 user ID 写入自己的 `PENDING` binding，再创建统一 Admin 管理关系；关系写入成功并读回验证后才标记 `COMPLETED`。
- Admin 重试只使用本地 `PENDING` 的 user ID，并要求 IAM 返回的 User ID 与本地记录一致。IAM 不可用、初始化记录不存在、邮箱或 user ID 不一致、目标身份被删除/封禁导致无法按产品规则授予时都失败关闭，不回退到邮箱查询。
- `COMPLETED` 后重启不再调用授予流程，即使 OpenFGA 关系后来被外部删除也不自动重授；恢复关系应走显式 Admin 权限管理动作。该约束是为满足“不因 seed 禁用/删除/邮箱重用在重启重建重授”，不是严格保证系统永远存在可用管理员。

IAM binding 与 Admin binding 的数据库写入和 OpenFGA tuple 写入无法天然组成单一跨系统事务。Admin 以 `PENDING -> tuple 幂等写入并读回 -> COMPLETED` 顺序处理；崩溃时保留 pending，重试同一 stable ID。完成标记不得在关系写入前落库。

### 防止首次部署被邮箱抢注

新部署无 IAM binding 时，IAM 应在同一事务中保护性地创建自己的 stable binding 和 seed 身份：

- 目标邮箱已被普通用户占用时，不静默采纳该用户作为 seed，也不授予 Admin；初始化报告冲突并停止该次 bootstrap。
- 目标邮箱不存在时，IAM 创建指定 stable ID 的 active、邮箱已验证 seed，并设置首次改密标志，然后才完成 IAM binding。
- 现有历史部署只能走前述一次性 legacy migration；该迁移接收显式 stable user ID，Admin 仍只能通过 IAM binding 查询，不得自己按邮箱认领。
- 不采用 Admin 直连 IAM 数据库、不把邮箱相等当作长期授权条件、不把启动日志中的初始密码当作 IAM/Admin 绑定凭据。当前日志密码是一次性启动交付，既不能从 hash 恢复，也没有现成的跨服务证明接口。

可选方案是增加独立共享 bootstrap secret 保护一次性查询，但这会增加第二个部署秘密、轮换与泄露处理。主 design 采用 IAM 持有的 bootstrap binding 加既有 `iam.manage_users` service authentication 的内部读取契约；RPC 及服务凭据配置仍需在完整 API 评审中落定。

## Design 技术裁决与产品边界

1. challenge TTL 采用 15 分钟默认值并可配置；同一用户再次使用临时密码登录时只保留最新 challenge。
2. 所有既有用户 flag 默认 `false`；邮箱找回替换密码时保留 flag，未完成首次改密的用户继续受限。
3. 新 seed 与 IAM bootstrap binding 在同一事务中完成；完成后只使用持久化 stable UID，不因环境变量、seed 状态或邮箱重用重绑。
4. Admin bootstrap 失败时保留本地 `PENDING` binding，按同一 stable UID 重试；完成后不因重启自动重授关系。
5. R16 的密码交付、邀请、通知和 CLI 排除范围保持不变；本设计不添加额外交付步骤。首次改密仍由 IAM Authn RPC 和 IAM Web 承接。

## 规划验收重点

- seed 与管理创建用户都在 IAM 持久化 `must_change_password=true`；普通注册和无要求的旧用户保持 `false`。
- 正确初始密码返回结构化首次改密动作，但数据库中没有普通 `IAMLoginSession` 或 `iam_login_id`；改密前直接访问业务接口和 OIDC 只能失败或回到登录。
- 单 challenge 双提交、不同 challenge 并发提交、过期 challenge、禁用/删除后提交、邮箱验证前登录均有明确状态结果；只有一个事务能清除 flag 和替换密码。
- 管理重设、密码找回、删除恢复分别验证“不新增/不清除/保留”首次改密 flag 的契约；新密码不能等于当前临时密码。
- 改密事务完成后普通 session 能继续原 IAM 登录和 OIDC request ID；旧 IAM/OAuth session、access/refresh token 均已撤销。
- 首次部署邮箱被抢注时 IAM 拒绝认领；IAM/Admin binding 的 stable ID 与完成标记在重启、seed 禁用/删除、邮箱变更和同邮箱新身份场景下不会重建或重新授予。
