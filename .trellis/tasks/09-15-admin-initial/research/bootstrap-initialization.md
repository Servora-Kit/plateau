# 首位管理员初始化方案调研

本文记录当前源码事实、已确认需求与候选方案。任务仍在规划；初始化候选待完整规划评审，首次改密与 Admin 防误操作范围分别以 PRD R17、R18 为准。本文不是 spec 或已完成的设计。

## 用户提出的问题

IAM 已有可配置的 seed 用户，Admin 能否复用该用户；希望避免分别维护 IAM 与 Admin 两份邮箱配置而漏改其中一处。这里需要同时解决部署配置的来源和管理员身份的稳定绑定。

## 当前 IAM 行为

- [local/iam.yaml](../../../../app/iam/service/configs/local/iam.yaml) 与 [docker/iam.yaml](../../../../app/iam/service/configs/docker/iam.yaml) 都通过 `IAM_BOOTSTRAP_USER_EMAIL` 覆盖 `iam.bootstrap_user_email`，默认邮箱为 `admin@plateau.local`。
- [IAM config.proto](../../../../app/iam/service/api/protos/iam/conf/v1/config.proto) 将该字段声明为必填；虽然注释称 platform administrator，实际 [UserInitializer](../../../../app/iam/service/internal/biz/user_initializer.go) 只处理 IAM 身份，不写入 Admin 管理权限。
- `UserInitializer.Initialize` 规范化配置邮箱，查找已有用户，不存在时生成稳定用户 ID 和随机密码并创建 active、邮箱已验证的初始身份；已存在则复用身份，但会拒绝非 active 或未验证的用户。它没有已初始化完成的持久化标记，也不以已保存的 seed user ID 代替邮箱查询。
- 该可信部署初始化路径与普通 `UserService.CreateUser` 的待邮箱验证流程不同，不能将 seed 行为推广为 Admin 管理创建的默认行为。
- 当前 [admin.fga](../../../../manifests/openfga/admin.fga) 描述 `user` 主体的 Admin 管理关系；模型存在不代表初始化已完成。

### 部署、远程查询与启动顺序的实际缺口

- 当前 [docker-compose.apps.yaml](../../../../docker-compose.apps.yaml) 仅定义 Audit，没有 IAM 或 Admin 服务；[.env.example](../../../../.env.example) 尚未登记 `IAM_BOOTSTRAP_USER_EMAIL`。已有 IAM YAML 读取进程环境不等于部署层已经实现统一分发。
- [Just service 模板](../../../../just/service.just) 第 11、75–76 行默认使用 `./configs/local/` 并通过 `go run` 传入配置目录；原生运行与后续容器运行都需要在 design 中明确共享输入如何注入，不能假设 `.env` 自动进入任意容器。
- [UserService Proto](../../../../app/iam/service/api/protos/iam/user/v1/user.proto) 第 132–148 行的 GetUser 只按 `users/{user_id}` 查询；ListUsers 虽有 filter，但 [user data 字段绑定](../../../../app/iam/service/internal/data/user.go) 第 32–46 行不含 email。因此当前管理 RPC 尚不能按邮箱解析 user ID；内部 `FindByEmail` 在同文件第 159–165 行已有此能力，需要新增受保护的解析入口或明确扩展查询能力。
- [newApp](../../../../app/iam/service/cmd/server/main.go) 第 36–41 行将初始化挂到 `BeforeStart`；[startup.Initialize](../../../../app/iam/service/internal/startup/initializer.go) 第 25–33 行先初始化用户再初始化 OIDC。用户初始化错误会终止启动，种子用户被封禁或未验证会阻止 IAM 正常启动。Admin 初始化不能只依赖进程存在或服务注册，应以 IAM RPC 实际成功为准并处理就绪延迟。

## 候选方案比较

| 方案 | 配置与行为 | 取舍 |
| --- | --- | --- |
| IAM、Admin 各自填写邮箱 | 两份独立值，Admin 首次查找该邮箱对应的用户 | 接入直接，但仍需人工同步，未解决用户担心的配置漂移 |
| 部署层单一输入，各服务首次初始化后绑定稳定 ID | 部署环境只设置一份 bootstrap 邮箱，映射到 IAM 与 Admin 各自配置；Admin 首次解析为 IAM user ID，持久化初始化结果及管理关系 | 当前推荐候选；无需每次手工同步邮箱，也不把邮箱当作长期授权依据，但要补齐初始化完成标记与失败恢复 |
| 独立平台初始化任务编排 | 初始化任务取得 IAM 用户 ID，再显式调用 Admin 完成首次授权；日常服务不再消费 bootstrap 邮箱 | 适合后续统一部署编排，额外引入初始化任务、专用入口及执行依赖，当前尚无此完整能力 |

共享输入属于部署组合，不要求 Admin 读取 IAM 配置文件或专门查询“哪个人是 IAM 管理员”。两个服务仍分别拥有身份创建与管理资格授予的职责。用户后续建议将共享环境变量命名为 `BOOTSTRAP_USER_EMAIL`；该命名可行，服务内部配置仍分别归属 IAM/Admin。local/Compose 映射和旧 `IAM_BOOTSTRAP_USER_EMAIL` 的兼容处理留给 design，当前配置文件尚未更名。

## 推荐候选的行为约束

1. 邮箱仅用于首次定位目标身份。Admin 通过受保护的 IAM 接口得到稳定 user ID，检查目标处于可用、邮箱已验证的状态，然后以 user ID 建立管理关系。
2. 持久化初始化绑定及完成状态。重启、邮箱变更、管理员资格撤销或管理员人数变为零都不能触发重新按邮箱授予资格；恢复管理入口应是显式操作。
3. 原用户清理后，同邮箱注册的新用户获得新 ID，不继承原管理员资格；不能用“当前邮箱相等”进行日常授权，也不能在重启后重新绑定。
4. 首次初始化需处理 IAM 尚未就绪、查找失败、授予权限失败、多实例并发及部分成功。应先固定本次初始化目标 ID，重试同一目标；不能在重试期间因邮箱重新绑定而切换授权对象。失败不能标记为初始化成功。
5. 初始化后管理资格由 Admin 自身流程维护，用户已确认 R11 的统一管理资格；当前页面按 R19 从一级“权限管理”直接进入管理员管理。防误操作仅限 R18，操作者不能撤销自己的管理资格，不为 seed 设置永久保护。初始化技术方案仍需在 design 与完整规划中评审。

## 与本任务用户生命周期的交点

IAM 当前 seed 是按邮箱重新查找和校验的逻辑；封禁已有 seed 用户后重启会失败已由调用链确认。新增删除、恢复及到期清理后，还需要覆盖其邮箱变更或清理后重启是否创建另一个身份。推荐一并设计持久化的首次初始化语义，初始化完成后不再将该用户的持续存在或 active 状态作为服务每次启动的要求；该调整属于候选方案，尚未承诺实施。

如果继续保留每次启动按邮箱保证身份存在的方式，即使 Admin 使用稳定 ID 防止权限继承，也仍需明确 IAM 的账号重建及启动失败行为。不能靠复制两份邮箱配置解决这类生命周期问题。

后续验收应覆盖：首次部署、重复启动、授予部分失败后的重试、原管理员被撤权、原用户邮箱变更、原账号被封禁/删除，以及同邮箱新身份注册。具体持久化、事务与重试设计待方案确认后完成。

## 本轮追问：查询、初始密码与封禁保护

### 现有查询方式

- `GetUser` 接收资源名 `users/{user_id}`，不接受邮箱作为资源名。
- `ListUsers` 接收分页、filter、order_by；当前字段绑定允许 user_id、status、create_time、update_time 及现有 profile 字段的过滤和排序，不含 email。返回的用户对象可以带邮箱，不等于支持邮箱过滤。
- IAM 内部 `FindByEmail` 已通过规范化邮箱查找 LoginIdentifier，再按其 user ID 读取用户；登录与初始化可使用它，当前 Admin 不能直接调用仓储方法。
- 用户进一步询问 GetUserByEmail 与 AIP。查阅 AIP 原文后的推荐方向是为 ListUsers.filter 增加 email 精确条件，保留 GetUser(name)，依据与备选 LookupUser 见 [邮箱查询 API 调研](email-lookup-api.md)。

### seed 密码与共用首次改密

- [UserInitializer.create](../../../../app/iam/service/internal/biz/user_initializer.go) 第 76–98 行生成随机密码，保存 hash，并在创建成功后将明文初始密码写入一条启动日志。仅实际创建时输出，不是每次启动都输出，也不能从 hash 取回明文。
- 当前没有 seed 首次强制改密规则。现有登录直接建立正常 IAM 会话，缺少必须改密状态和受限改密流程；源码证据与可复用边界见 [认证流程调研](management-capabilities.md)。
- 用户本轮明确 seed 与管理员新建用户复用首次改密流程，已纳入 PRD R17、AC17。部署生成或管理创建时设置的初始密码验证通过后，只能进入 IAM 改密流程；设置不同的新密码成功后才完成普通登录并继续原 OIDC 流程。由 IAM service 执行规则，IAM Web 承接交互，不由 Admin service 处理用户的登录改密。
- 两种创建入口写入同一首次改密要求，共用 IAM 的状态和交互；seed 仍走可信部署的 active、已验证路径，管理创建仍先完成 R16 的邮箱验证。当前两边均无首次改密实现，“复用”指本次设计与建设中共用能力。
- 管理重设已有用户密码不自动新增首次改密要求，也不能清除尚未完成的要求。删除恢复同样保留未完成要求，而不凭恢复动作新增要求。旧 seed 的升级处理留给 design 明确，不要求所有现存用户自动改密。
- 初始密码已进入日志的事实是建议替换它的理由；强制首次改密并不能清除历史日志，也不能阻止在本人之前拿到临时密码的人使用它。初始日志的交付边界仍需在 design 说明，不据此扩展邮件或邀请流程。

### 启动可靠性与管理入口保护

当前 [UserUsecase.DisableUser](../../../../app/iam/service/internal/biz/user.go) 第 80–104 行仅通过 `updateStatus` 加载目标、校验可选 ETag 并更新状态，没有 seed、本人或最后管理员保护；[UserService](../../../../app/iam/service/internal/service/user.go) 第 119–132 行也没有额外保护。[数据层](../../../../app/iam/service/internal/data/user.go) 第 206–249 行统一禁用认证器并撤销会话，没有 seed 特例。调用仍须通过现有 IAM `manage_users` 授权，不能将缺少 seed 特例描述为任意普通用户均能封禁。

“seed 用户保持可用”和“IAM 可以启动”是不同要求。持久化首次初始化完成状态用于解除服务启动对某个用户持续可用的依赖，本身不阻止封禁，也不保证 Admin 始终有人能管理。

用户先要求保护逻辑尽可能简单，随后明确接受 Admin 入口防误操作范围，已纳入 PRD R18、AC18。以下保留备选比较以说明保证强度的取舍，不再将全局最后管理员保护列为本期需求。

| 候选保证 | 实现范围与代价 | 明确限制 |
| --- | --- | --- |
| Admin 入口防止操作者误伤自己（已确认） | 后端 PEP 比较可信操作者 ID 与目标 ID，拒绝自封禁、自删除及撤销自己的管理资格；无需按全体管理员统计 IAM 可用状态 | IAM 自助删除、其他受权调用者、两名管理员并发互相处理仍可能使管理入口全部失去；不是严格的最后管理员保护 |
| Admin 至少保留一条管理资格关系 | Admin 统一串行化关系变更并在同一一致性边界内检查，当前管理员先撤权再管理删除/封禁；资格存储与 OpenFGA 的一致性必须由 design 明确 | 一条关系不等于其 IAM 身份仍然 active、未删除，不能保证可登录 |
| 所有入口始终保留至少一个可用管理员 | Admin 权限变更和 IAM 管理封禁/删除、自助删除等入口都参与一致的状态协调 | 复杂度显著增加，一次计数、一次 IAM 查询或仅本进程锁不够；不作为用户未确认的默认范围 |

采用已确认的第一种防误操作范围，不将 seed 设为永久不可封禁用户，也不让 IAM 保存全局管理员身份。IAM 自助删除仍遵循 R15，不因 Admin 自我保护增加反向权限查询。管理资格丢失后不能通过重启自动重授；显式运维恢复方式需要后续说明，但不因此自动新增 CLI 或脚本交付。
