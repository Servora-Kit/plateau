# Admin 新增用户管理需求：能力核查

核对日期：2026-09-15。依据本地源码，只读检查；未执行运行验收。本文件记录实现事实与规划缺口，不代替删除、会话失效等产品语义的决定。

后续补充：2026-09-16 已重跑 Servora mixin/CRUD 单测及 SQLite live contract，且按用户方向规划迁入 Plateau infra；见 [归属与验证](ent-mixin-ownership.md)。下文“未运行”仅指本文件最初核查，不能覆盖后续实际验证记录。

## 能力对照

| 操作 | 当前服务间管理 RPC | 可复用能力与缺口 |
| --- | --- | --- |
| 创建用户、查询用户、更新 profile | `CreateUser`、`GetUser`、`ListUsers`、`UpdateUser` | 已有；创建入口已确认纳入首期，并沿用本人验证邮箱后才能登录的规则；Admin 创建后无额外交付流程，更新字段与并发控制仍由 IAM 执行 |
| 封禁、解禁 | `DisableUser`、`EnableUser` | 已有；封禁禁用认证器并撤销 IAM/OAuth 会话，解禁不恢复旧会话 |
| 管理员强制设密 | 无 | 自助改密要求旧密码和当前登录，邮件恢复要求 reset token；需要独立的受保护管理入口及对应领域事务 |
| 删除、恢复及到期清理 | 无 | IAM 尚未接入软删除字段或相关用例；用户已选定恢复期内保留邮箱、清理成功后释放邮箱，需要补齐领域行为与清理任务 |
| 用户自助删除 | 无 | AccountService 及 IAM Web 均无此功能；已明确纳入本任务，与管理删除复用同一领域流程 |
| 强制登出目标用户 IAM | 无 | 按用户撤销的内部逻辑用于封禁等流程，尚无独立的按用户登出管理 RPC |
| Admin 自身授权 | 有 OpenFGA 模型草案和 Vben 演示 Web，无 Admin 后端 | 演示登录尚未接 IAM；实际赋权管理、初始化和后端执行尚未实现 |

## 用户管理与 profile

- [user.proto](../../../../app/iam/service/api/protos/iam/user/v1/user.proto) 第 18–55 行列出当前六个管理 RPC；第 72–87 行定义生命周期状态与 profile 字段。
- `UserProfile` 支持 `name`、`given_name`、`family_name`、`nickname`、`preferred_username`、`picture`、`locale`；`UpdateUser` 仅修改 profile。email 为独立登录标识，在该管理更新接口中不可变，`email_verified` 为只读。
- 用户已确认首期展示 `CreateUser` 管理入口，并沿用本人验证邮箱后才能登录的流程；Admin 创建完成后不增加密码交付、邀请或通知，也不把管理员人工交付密码列为本任务要求。现有接口接收初始密码这一事实不自动产生后续交付功能。
- 当前管理创建由 [UserUsecase.CreateUser](../../../../app/iam/service/internal/biz/user.go) 第 48 行附近委托 [AccountUsecase.CreatePendingUser](../../../../app/iam/service/internal/biz/account.go) 第 87 行起执行：规范化并查重邮箱、将输入密码转换为 hash、创建待邮箱验证的用户和密码认证器，再生成验证令牌并调用 `MailSender.SendVerification`。验证邮件由 IAM 发送，不由 Admin 维护另一套验证流程。
- 当前 [AuthenticationUsecase.Login](../../../../app/iam/service/internal/biz/authn.go) 第 54 行附近要求账号 active 且邮箱已验证，因此现有管理创建不产生可立即登录的账号。用户已确认首期沿用该产品行为。本轮只读核查，未验证真实邮件投递。
- 现有 [验证邮件模板](../../../../app/iam/service/internal/assets/mailTemplate/verify_email.html) 第 64–112 行提供验证链接及有效期，不携带密码。[IAM Web 验证页](../../../../app/iam/web/app/verify-email/page.tsx) 第 19–32、49–85 行只读取 fragment token、调用 VerifyEmail 并引导登录，没有设置初始密码的输入或提交。通过邀请链接由用户自行设密需要新增流程，不能直接把现有邮箱验证页面视为该能力。

## 强制设密与现有自助入口

- [account.proto](../../../../app/iam/service/api/protos/iam/account/v1/account.proto) 第 67 行附近的 `ChangePassword` 接收 `current_password`、`new_password`，由已认证用户操作。
- [biz/account.go](../../../../app/iam/service/internal/biz/account.go) 第 184 行附近的密码恢复要求 active 且邮箱已验证；第 236 行附近的自助改密要求当前登录与旧密码。
- [data/authn.go](../../../../app/iam/service/internal/data/authn.go) 第 54 行附近的 `ReplacePassword` 需要当前密码 hash 和保留的 login ID；它不是可直接用于管理员强制设密的领域接口。
- [data/account.go](../../../../app/iam/service/internal/data/account.go) 第 108 行附近的 `ConsumeAndReplacePassword` 需要有效 reset token，并在事务中检查用户、替换密码和撤销会话。

管理员凭管理权限发起的新入口与用户自助流程区分；“不要求邮箱验证”不改变账号的邮箱验证事实。用户已确认强制改密同时撤销目标用户全部 IAM 登录及 OAuth 会话、令牌；普通已有账号的管理重设密码不额外触发首次改密。用户本轮将 seed 与管理创建用户的共用首次改密纳入 R17，尚未完成的要求不能借管理重设密码清除；具体范围以 PRD R8、R17 为准。

### 共用首次改密的成本与边界

本节记录当前能力缺口与技术候选。用户本轮明确 seed 与管理员新建用户共用首次改密，范围已纳入 PRD R17、AC17；详细边界见 [初始化调研](bootstrap-initialization.md)。任务仍在 planning，尚未批准实施，不把已有账号的每次管理重设密码也改为强制首次改密。

- 当前 [AuthenticationUsecase.Login](../../../../app/iam/service/internal/biz/authn.go) 第 41 行起检查账号、邮箱和密码后创建 IAM 登录会话；[AuthnService.Login](../../../../app/iam/service/internal/service/authn.go) 第 30 行起将登录引用写入 SCS。目前没有必须改密状态或只允许改密的认证流程。
- [PasswordAuthenticator schema](../../../../app/iam/service/internal/data/schema/password_authenticator.go) 第 13 行起有密码 hash 和变更时间，但无强制改密标记。后续需要由 IAM 持久化该要求，使其不能被跳过页面或更换设备消除；具体字段位置及模型需单独设计。
- [OIDC callback](../../../../app/iam/service/internal/oidc/provider.go) 第 203 行附近先检查 SCS 中的普通登录引用，再解析 IAM 会话；无有效登录即跳转登录页。[OAuth data](../../../../app/iam/service/internal/data/oauth.go) 第 77、103 行附近校验授权请求绑定的登录及其撤销状态。若改密完成前只持有受限改密凭证、不创建普通 IAM login 或写入 `iam_login_id`，OIDC 可复用现有未登录检查，无须在每个协议入口新增强制改密判断。这里的“覆盖 OIDC”应指验证无法绕过的结果，不等于预设必须修改 OIDC 实现。
- 可复用现有密码校验、hash、替换和会话撤销能力；候选流程是验证管理员所设密码后只发放短期、用途受限的改密凭证，改密成功再建立正常会话并继续登录或 OIDC 流程。现有 AccountService.ChangePassword 要求普通认证上下文，不能未经接口与权限设计直接当作受限流程入口；这不意味着强制改密必须免除旧密码验证。

按当前代码边界判断，该功能属于中等规模的 IAM 认证流程扩展，主要成本在状态持久化、受限凭证、IAM Web 页面及登录/OIDC 衔接、绕过与并发回归测试。实现时不能先发放普通登录态，再仅用前端导航要求改密；管理员设密撤销既有会话与令牌的契约仍须成立。IAM service 执行规则，IAM Web 提供用户交互，Admin service 只授权并提交管理意图；Admin 不承接用户登录后的改密流程。

## 封禁与强制登出

- [data/user.go](../../../../app/iam/service/internal/data/user.go) 第 206 行附近执行封禁状态变更及关联失效；[data/authn.go](../../../../app/iam/service/internal/data/authn.go) 第 135 行附近提供按用户撤销 IAM login、OAuth token session 及 token 的内部逻辑。
- [biz/session.go](../../../../app/iam/service/internal/biz/session.go) 第 26 行附近的 `SessionRepo` 暴露按 login ID 的 `Revoke`，没有按 user 全部会话登出的管理 RPC。
- 封禁附带阻止后续登录的账号状态变更，不能通过“封禁后立刻解禁”拼接独立强制登出功能。
- 用户已确认独立强制登出与管理员强制改密使用相同撤销范围：目标用户全部设备的 IAM 登录及 IAM 维护的 OAuth token session、access/refresh token；强制登出不改密码或封禁状态。此处为已确认设计，当前没有对应管理 RPC。
- IAM 会话撤销与接入应用本地会话结束是不同范围，现状见 [IAM 会话规范](../../../spec/iam/sessions.md)。

## 删除及关联事实

- [User schema](../../../../app/iam/service/internal/data/schema/user.go) 第 13 行起定义稳定 ID、状态、profile 等信息，当前没有软删除字段。
- [LoginIdentifier schema](../../../../app/iam/service/internal/data/schema/login_identifier.go) 第 29 行附近定义 `(type, canonical_value)` 唯一性及每用户每种类型唯一性。此约束可继续保留软删除账号的邮箱占用；清理完成后的释放须与关联数据处理共同设计。
- 用户相关身份数据包含 login identifier、authenticator、IAM login session、OAuth token session/access/refresh token；当前没有执行这些数据删除的领域事务。
- [OAuthTokenSession schema](../../../../app/iam/service/internal/data/schema/oauth_token_session.go) 第 13 行起含 IAM login session 关联。用户稳定 ID 也可能被其他服务用于权限或历史记录引用，IAM 删除不能被解释为自动清除其他领域的数据。

用户已确定采用带恢复期的软删除，默认 30 天且可配置：删除时固定清理时间，后续配置不追溯已有删除记录；清理前不能用同一邮箱注册，到期清理成功后才允许新注册，且新用户取得新 ID。`FindByEmail` 先查询独立登录标识再读取 User，见 [data/user.go](../../../../app/iam/service/internal/data/user.go) 第 159–166 行；仅给 User 加删除时间不能自动释放登录标识的唯一约束，也不能把“用户被查询过滤”误判为邮箱可注册。恢复已确定仅由 Admin 经 UserService 发起，并保留删除前的账号状态、原密码与邮箱验证事实，原本封禁或未验证的账号不会自动变为可登录；具体需求与验收以 PRD 的 R13、AC12 为准，当前恢复 RPC 尚未实现。

### 管理删除、自助删除与恢复的入口

- 用户已明确管理删除属于 `UserService`，自助删除属于 `AccountService`，恢复仅由 Admin 通过 `UserService` 发起。两个删除入口共享 IAM 内部领域命令；不通过从 AccountService 调用受服务身份管理授权保护的 UserService RPC 来复用逻辑。
- 当前 [account.proto](../../../../app/iam/service/api/protos/iam/account/v1/account.proto) 第 18–95 行仅有注册、验证、profile、改密与密码找回接口，没有删除或恢复账号。已认证自助接口要求 AuthN，`AUTHZ_MODE_NONE` 不代表允许操作其他用户。
- [service/account.go](../../../../app/iam/service/internal/service/account.go) 第 89–142 行的现有自助接口通过 `authn.From(ctx)` 取得当前用户；新增自助删除应沿用可信目标绑定，不接受任意目标用户 ID。
- [biz/account.go](../../../../app/iam/service/internal/biz/account.go) 第 236–259 行的 `ChangePassword` 已能读取当前有效密码并验证旧密码。用户已确认自助删除也须重新输入并验证当前密码；该现有能力可作为实现依据，但自助删除接口及验证流程仍需新增。
- IAM Web [账号安全页](../../../../app/iam/web/app/account/security/page.tsx) 第 205–229 行当前只有改密与退出登录；第 24–90 行展示旧密码校验、请求后清理输入及错误反馈。该页面是自助删除入口的接入位置；[useProfile](../../../../app/iam/web/hooks/use-profile.ts) 第 25–47 行处理当前用户获取及未认证跳转，[iam-api.ts](../../../../app/iam/web/lib/iam-api.ts) 暴露已有 account client。

方法名拟沿用已有资源命名惯例：`UserService.DeleteUser`、`AccountService.DeleteAccount`、`UserService.UndeleteUser`；具体请求、响应与 HTTP 映射在设计阶段确定，当前并不存在这些 RPC。自助删除成功应结束当前浏览器 IAM 登录态，个人页面不增加自行恢复入口。

### Servora 已有软删除机制

Plateau 的 `go.work` 使用相邻 Servora checkout；以下依据该 checkout 的当前实现，不以历史 CRUD 实现作为依据。

- 实际存储机制是 [SoftDeleteMixin](../../../../../servora/contrib/db/entgo/mixin/soft_delete.go) 第 43–101 行：加入 `delete_time`、`deleted_by`、`purge_time`，默认查询过滤已删除行，将 Ent `Delete`/`DeleteOne` 改为写入删除时间的 `Update`。
- 同文件第 83–95 行的 Hook 只自动写 `delete_time` 和可选 `deleted_by`；`purge_time` 仅有字段与索引声明，不自动计算或赋值。恢复期配置、到期时间保存与清理调度需由 IAM 实现，不能把字段存在视为已提供自动清理。
- 同文件第 113–137 行的 `SkipSoftDelete(ctx)` 同时绕过查询过滤与删除改写，可用于显式读取已删除行，也可使删除变成物理删除；不能当作无副作用的普通查询标志向管理调用方开放。
- [core/crud/lifecycle.go](../../../../../servora/core/crud/lifecycle.go) 第 14–30 行提供 `ListOptions.ShowDeleted` 和 `DeleteOptions` 等选项，删除生命周期与业务事务仍归消费方；框架没有 `SoftDeletePolicy`、`DeletePlan` 或 `PrepareDelete` API。
- [Servora CRUD 文档](../../../../../servora/docs/crud.md) 第 473–506 行说明：Mixin 不处理关联模型、恢复入口、清除任务或仅有效行的唯一索引；`show_deleted`、恢复与数据库唯一约束需要消费方明确装配。
- Plateau 已有 [Example DeleteUser](../../../../app/example/service/internal/biz/user.go) 第 165 行和 [data 实现](../../../../app/example/service/internal/data/user.go) 第 213 行作为接入参考；IAM 需在此机制上组合凭据、登录和 OAuth 会话的领域失效，不能直接套用 Example 的删除业务规则。

证据层次：[Mixin 单元测试](../../../../../servora/contrib/db/entgo/mixin/soft_delete_test.go) 覆盖字段、默认过滤、删除改写和 bypass；[数据库合同测试](../../../../../servora/contrib/db/entgo/crud/live_contract_integration_test.go) 第 599–630 行覆盖删除后默认不可见、显式查询和清除标记后恢复。此次仅阅读测试，未运行这些测试，也未完成 IAM 接入。

### IAM 配置惯例与恢复期参数

- 当前 [IAM config.proto](../../../../app/iam/service/api/protos/iam/conf/v1/config.proto) 仅有 bootstrap email，尚无恢复期或清理参数。
- 既有时长字段使用 `google.protobuf.Duration`，见 [OIDC config.proto](../../../../app/iam/service/api/protos/iam/oidc/conf/v1/config.proto) 第 23 行和 [Session config.proto](../../../../api/protos/plateau/security/session/v1/config.proto) 第 10 行附近；[session.yaml](../../../../app/iam/service/configs/local/session.yaml) 已使用 `2592000s` 表示 30 天。
- 配置文件支持 `${ENV:default}`，见 [iam.yaml](../../../../app/iam/service/configs/local/iam.yaml) 和 [oidc.yaml](../../../../app/iam/service/configs/local/oidc.yaml)。新增恢复期与清理参数计划沿用该方式；专用字段和环境变量尚未实现。
- [main.go](../../../../app/iam/service/cmd/server/main.go) 第 56 行附近在启动时通过 `bootstrap.Scan` 扫描配置后传入 `wireApp`。当前没有配置管理 API、watcher 或运行时热更新的实现证据；按当前装配方式，配置改变需重启加载。

用户已确认：在软删除时根据生效配置保存确定的 `purge_time`，后续配置只影响新发生的删除，清理按持久化时间执行。此处为已确认设计，尚未实现；可配置本身不自动包含 Admin 在线设置页面。

## Admin 权限所有权

- [admin.fga](../../../../manifests/openfga/admin.fga) 第 3–6 行以 `user` 为主体描述 Admin 管理关系；[iam.fga](../../../../manifests/openfga/iam.fga) 第 7–9 行以 `service` 为主体描述 IAM `manage_users`。
- [user_initializer.go](../../../../app/iam/service/internal/biz/user_initializer.go) 初始化 IAM 用户；配置中的 bootstrap administrator 命名不等于已实现 Admin 的管理员角色或授权关系。
- Admin 管理权限属于 Admin；共享 OpenFGA 可以承接授权决策与关系存储，IAM 负责验证服务调用的身份与权限。拥有权限结构不意味着另建用户身份或自制授权引擎。
