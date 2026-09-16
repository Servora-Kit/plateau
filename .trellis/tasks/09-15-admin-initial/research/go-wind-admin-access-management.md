# go-wind-admin 用户与权限管理借鉴

核查日期：2026-09-16。本地参考为 `/Users/horonlee/projects/go/tx7do/go-wind-admin`，只读查看源码，没有启动应用或声称运行验收。本任务仍为 planning。用户已确认统一管理资格，当前页面以本文“两项一级菜单”一节及 PRD R11、R19 为准；原角色方案只作参考，自定义角色不在本期范围，不修改 spec。

## 用户反馈与此前边界的澄清

用户认为单独提供“管理员名单”过于简陋，希望借鉴 go-wind-admin 的用户管理。此前对该项目本地身份、租户和组织职责的区分，不能推导为 Plateau Admin 只能维护一张管理员名单。用户、角色、权限的管理方式同样可以用于平台管理后台；需要调整的是数据所有权与实际功能范围。

用户要求防误操作简单，已确认的 R18 继续成立。它不意味着整个 Admin 授权结构必须只有一个是否管理员的开关，也不自动授权引入全套多租户、组织、岗位或数据权限系统。

用户随后明确：Admin 没有租户概念，租户是业务自身的领域；当前侧栏围绕用户管理即可，并询问权限编辑放哪里。参考 go-wind-admin 不应自动转化为自定义角色需求，个人 profile 自助编辑继续由 IAM 提供，普通 IAM 用户不能进入 Admin 管理工作区。

## 本地参考的领域结构证据

- [UserRole](</Users/horonlee/projects/go/tx7do/go-wind-admin/backend/app/admin/service/internal/data/ent/schema/user_role.go:29>) 用 user_id 与 role_id 关联，并记录分配者、分配时间、生效区间和状态。可借鉴用户分配角色的管理方式；本任务没有据此确认有效期或主角色功能。
- [Role](</Users/horonlee/projects/go/tx7do/go-wind-admin/backend/app/admin/service/internal/data/ent/schema/role.go:31>) 有名称、标识、保护标记、类型与数据范围，并带租户维度。角色的名称、说明与权限组合适用于 Admin；租户、部门范围与模板机制没有在本任务中确认。
- [RolePermission](</Users/horonlee/projects/go/tx7do/go-wind-admin/backend/app/admin/service/internal/data/ent/schema/role_permission.go:30>) 关联角色与权限，另有 allow/deny 和优先级；后两者不应因参考而自动纳入 Plateau。
- [Permission](</Users/horonlee/projects/go/tx7do/go-wind-admin/backend/app/admin/service/internal/data/ent/schema/permission.go:28>) 用名称、code、分组描述具体操作。这种面向功能的权限名称可用于角色配置。
- [User](</Users/horonlee/projects/go/tx7do/go-wind-admin/backend/app/admin/service/internal/data/ent/schema/user.go:31>) 持有资料、全局账号状态与租户内登录标识约束，[UserCredential](</Users/horonlee/projects/go/tx7do/go-wind-admin/backend/app/admin/service/internal/data/ent/schema/user_credential.go:32>) 持有凭据。Plateau 对应的身份与凭据能力已有 IAM 所有权，不能再复制进 Admin。

## Vben 用户与角色页面证据

- [用户表格](</Users/horonlee/projects/go/tx7do/go-wind-admin/frontend/admin/vue-vben/apps/admin/src/views/app/opm/user/list/user-list.vue:159>) 展示账号、资料、角色、状态与最后登录等列，并有详情、编辑、删除等操作。可借鉴筛选表格、角色列和操作反馈；字段只采用 Plateau 实际领域接口支持的内容，不能据此自动加入手机号、MFA 救援或最后登录数据采集。
- [用户编辑抽屉](</Users/horonlee/projects/go/tx7do/go-wind-admin/frontend/admin/vue-vben/apps/admin/src/views/app/opm/user/list/user-drawer.vue:52>) 提供角色多选、资料和状态等表单；[组织侧栏](</Users/horonlee/projects/go/tx7do/go-wind-admin/frontend/admin/vue-vben/apps/admin/src/views/app/opm/user/list/org-list.vue:182>) 是租户与组织联动。可借鉴抽屉和分组表单，但当前 Plateau Admin 没有租户/部门管理需求，不能将这些筛选迁入 IAM 或凭参考页面新增相关领域。
- [角色编辑抽屉](</Users/horonlee/projects/go/tx7do/go-wind-admin/frontend/admin/vue-vben/apps/admin/src/views/app/permission/role/role-drawer.vue:46>) 包含权限树、数据范围及字段权限；推荐借鉴角色勾选操作权限的交互。组织数据范围、字段级权限黑名单与权限同步是额外能力，本任务未因参考而纳入。

这些证据来自本地页面源码，不是运行截图或端到端行为验收；本次没有复制或修改参考仓文件。

## 原先提出的独立成员/角色方案（未获确认，保留为备选）

| 页面/模块 | 管理对象与主要操作 | 事实归属 |
| --- | --- | --- |
| IAM 用户管理 | 全体 IAM 身份；按既有 R2–R17 提供查询、创建、profile、封禁/解禁、删除/恢复、管理设密、强制登出 | IAM；Admin 调用受保护 RPC |
| Admin 后台成员 | 获准访问后台的 IAM 用户；关联已有身份、查看其后台角色、分配/撤销角色及后台访问资格 | Admin，稳定引用 IAM user ID |
| Admin 角色管理 | 创建和维护角色，将已经实现的操作权限组合成角色，再分配给成员 | Admin |

“撤销 Admin 访问资格”与“封禁 IAM 用户”必须明确区分：前者不修改 IAM 身份或阻止其使用其他应用；后者是已确认的 IAM 生命周期操作。成员展示的邮箱、姓名等身份资料来自 IAM，不引入第二套密码或身份生命周期。

可以从同一个 IAM 用户详情跳转或发起后台访问授权，但不能因创建 IAM 用户而自动授权 Admin。具体导航层级与表单排布可在 design 中统一，不要求复制参考项目的全部菜单。

## 角色方案的含义与技术边界（非已确认需求）

- “自定义角色”指为一组权限取名后重复分配给用户，例如一个角色包含查看用户与编辑资料。这是解释功能含义的例子，不代表用户已需要该岗位或接受角色管理功能。
- “勾选内置操作权限”指在预先实现的管理动作中选择允许项，例如允许查看、编辑资料，但不允许封禁；不是创建新的功能或编辑业务服务自身权限。当前仍未决定需要按动作配置，不要求用户预测未来岗位。
- 权限点由已实现的功能契约提供，例如查看用户、编辑资料、创建用户、管理设密、封禁/解禁、删除/恢复、强制登出、管理后台成员/角色。具体粒度在 design 中结合批准范围确定。
- 角色可配置不意味着权限 code 可任意创建。候选不提供任意新增权限、API 规则或动态编辑 OpenFGA 模型的界面；新增服务能力时由实现提供可执行的权限契约，再纳入角色选择。
- Admin 拥有权限管理意图、角色和关联关系；实际授权沿用 Plateau 的 OpenFGA/PDP 与服务端 PEP 分工。角色元数据及授权关系如何存储与保持一致，需在 design 中明确，不能同时维护两套独立的权限判定结果。
- 当前 Plateau [admin.fga](../../../../manifests/openfga/admin.fga) 仅有 `admin` 关系和 `manage_users` 派生权限，没有可直接使用的完整角色管理能力；借鉴页面不代表授权模型已支持动态角色。
- R18 的自我保护在角色方案下还需要定义直接撤销自身角色、通过角色变更间接丢失管理资格的处理方式。设计应优先保持规则简单，例如用受保护的内置管理角色承载初始管理能力；此处只是技术候选，不扩大为全局最后管理员保证。

## 单一用户管理入口（此前建议，已由最新菜单讨论调整）

侧栏只有“用户管理”即可；列表展示所管理的 IAM 用户，而非只展示已有管理员。列表可以展示或筛选后台访问资格，不另建一张独立的管理员身份表。

用户详情或编辑抽屉内组织为：

| 区域 | 主要内容 | 所属能力 |
| --- | --- | --- |
| 用户资料 | 展示和管理允许的 profile 字段 | IAM UserService |
| 账号操作 | 已确认的管理设密、封禁/解禁、删除/恢复、强制登出 | IAM 管理接口 |
| 后台权限 | 查看及授予/撤销该用户在 Admin 的管理资格；是否进一步按动作配置待 P2 明确 | Admin 自有授权 |

这些是同一用户详情的区块、页签或操作抽屉，不要求侧栏新增二级菜单，也不等于三类服务事实一起保存。IAM 普通用户出现在被管理列表中，不意味着他们能够进入 Admin。

“管理员编辑某个用户的资料”是已确认 R3；“当前登录者编辑自己的资料”是 IAM 个人账号中心的自助操作。Admin 不复制个人中心编辑页面；头像菜单如需账号设置入口，可导航到 IAM，实际地址在 design 中从现有入口确定。

Admin 没有租户不是暂缓多租户 UI，而是不在该应用建立租户领域、切换或隔离模型。独立角色菜单、部门/组织与租户侧栏均不能因参考项目而成为本任务默认范围。

## 两项一级菜单（当前安排）

用户接受统一管理资格后，提出将权限管理作为独立菜单，询问是否需要“权限管理 / 管理权限”两层。当前采用两个一级入口：

- 用户管理：管理 IAM 用户资料与账号生命周期。
- 权限管理：直接进入管理员管理页，展示已有管理员，从已有 IAM 用户中添加管理员，或撤销他人的管理资格。

只有一个权限子页面时不增加“管理权限”二级菜单。页面标题可用“管理员管理”明确管理对象，侧栏保留用户提出的“权限管理”。新增管理员的选择器使用 IAM 身份查询，不重复建立用户名、密码或 profile。

统一资格已纳入 PRD R11，获准者具有本期全部管理能力，仍受 R18 自我保护与 IAM 领域规则限制；自定义角色和按动作勾选不在本期范围。界面完整性来自用户管理、明确的授权入口与操作反馈，不以增加角色配置数量衡量。

撤销资格仅影响 Admin；对应的 IAM 用户、密码和登录不因撤权被删除或封禁。已有 Admin 会话的后续请求也必须受到撤权结果约束，不能只检查菜单或登录时的旧权限快照。权限粒度与管理页面不再归为待决 P2；其技术装配随 design 完成后提交完整规划评审。
