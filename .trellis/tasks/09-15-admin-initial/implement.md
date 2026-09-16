# Admin 建设实施计划

状态：规划待评审。端口调整是 AGENTS 与应用配置的附带改动，不设独立迁移阶段。本文是实施顺序，不代表步骤已执行。需求以 [PRD](prd.md) 为准，接口、事务和兼容性以 [design](design.md) 为准。

## 启动条件与工作区约束

- [ ] 用户评审 PRD、design、本文后明确同意实施，再按 `.trellis/workflow.md` 进入任务启动步骤；逐项产品回答不是实施授权。
- [ ] 重新读取当前根及受影响目录 AGENTS、任务 manifests，核对工作区与生成器版本。保留已有 spec 目录迁移、Vben 骨架和其他会话改动。
- [ ] 当前仅根 Go module；独立验收使用 `GOWORK=off`，避免本机父级 `go.work` 隐含补足缺失依赖。Admin 不新增独立 `go.mod`。
- [ ] Vben 保持 `app/admin/web` 独立 workspace/lockfile，IAM Web 与共享 TS 仍归根 workspace。
- [ ] 规划期间仅编辑本任务目录。实施通过后，才能以实际证据同步通用 spec；不将任务排期、未实现方案或“首期”文案写入规范。

## 实施次序及依赖

本任务交付一个可登录、可授权、可管理 IAM 身份的闭环；按以下阶段验证。阶段顺序不等于已创建子任务。后续多人实施时每人只拥有约定的文件范围，共享 Proto、根配置和生成目录由集成负责人统一维护，不能覆盖其他人的改动。

### S1：接口与持久化基础

依赖：规划获批。覆盖 R2、R3、R8–R17、R22。

- [ ] 用户已删除旧 Admin 后端复制目录。实施先核对届时工作区，再参考 Example 通用工程、IAM 相关接线及 Trellis layout/layers/coding 建立 `app/admin/service` 骨架；在 `app/admin/service/api/protos/admin/**` 定义 Admin 合同与私有配置，配置对应 imports/OpenAPI 输入，再注册 Buf 并生成。只建立 design 第 2 节所需依赖、schema 与 Wire provider，保留 Web 和其他已有改动，不恢复整套 IAM 复制树。
- [ ] R22：将软删除便利实现及专属测试迁到 Plateau `infra/entgo/mixin`，更新 Example schema/repository 与 Ent 生成产物；IAM 新字段接线依赖该步骤。Servora CRUD 的 fixture 改为测试自用字段/显式查询范围，保留相关 CRUD 合同且不 import Plateau。两仓回归及消费确认后移除 Servora 旧包；不迁移 driver/List/Clear，也不顺带重写整个 Ent 层。
- [ ] IAM UserService 增加管理设密、删除、恢复、强制登出；补齐 deleted 视图、邮箱精确过滤和生命周期输出字段，保持原字段号。
- [ ] 为管理创建后的验证邮件失败声明独立 reason 及安全资源引用合同，生成两端错误辅助代码；正常 CreateUser 返回 User 保持不变。
- [ ] AccountService 增加当前用户删除接口；AuthnService 增加用途受限的首次改密完成接口与登录分支响应。
- [ ] 在 IAM UserService 声明 GetBootstrapUser，供 Admin 启动查询持久化记录指定的初始化用户；不公开 binding 存储结构，不按邮箱或创建时间猜测。
- [ ] User 增加软删除字段，密码凭据增加首次改密状态；补齐首次改密凭证与初始化完成记录 schema。
- [ ] 在根 Buf workspace 注册 Admin 源 Proto，输出共享 Go/TS/errors/CRUD sidecar；为 Admin HTTP API 建立所属源合同。
- [ ] 新增配置字段、默认值和校验；更新本地/docker配置样例与环境变量文档。
- [ ] 附带完成 R20：更新根 AGENTS 端口表及 Admin/Example/Test 的前后端启动与代理配置、相关 Compose 宿主映射，IAM 段保持不变；Audit 现有配置按 design 顺移到 10020/10021 解除冲突，表中不写 Audit/CMS。只修改实际配置引用，随最终联调验证，不新增迁移阶段或工具。
- [ ] 验证新增字段可对旧数据库执行非破坏性迁移，不批量给已有普通用户设置首次改密要求；确认旧 seed 的升级策略。

阶段门禁：Proto lint、生成器输出检查、共享 TS typecheck、受影响 Go 编译。后续阶段在本阶段合同稳定后开展，不能各自手写不同的 HTTP DTO。

### S2：IAM 账号生命周期

依赖：S1。覆盖 R8–R10、R12–R15。

- [ ] 管理与自助删除复用 IAM 领域命令；自助入口从可信上下文取 UID，在同一用户锁边界内校验当前密码。
- [ ] 删除写入固定 `purge_time`、撤销 IAM/OAuth 会话及敏感一次性材料；保留原 status、邮箱占用、密码、邮箱验证事实和首次改密状态。
- [ ] 管理设密与强制登出复用全量撤销原语；设密不创建新的首次改密要求、不清除未完成要求。
- [ ] 恢复按持久化期限检查，在事务内只清除 tombstone 并更新 etag；不复活旧凭证材料与会话。
- [ ] 将已删除管理查询的 bypass context 限定在 data 层具体读取；覆盖 Get/List/GetBootstrapUser 的 tombstone，并验证 show_deleted 不能影响普通删除、登录查询或开启物理删除。
- [ ] 增加 IAM 清理 worker，逐用户事务锁定、重新检查到期、清理所有关联身份表并释放邮箱；多实例可安全竞争、失败可重试、退出可停止。
- [ ] 覆盖所有身份读取与发行路径，使 tombstone 不能登录、解析旧会话、兑换旧 code 或 refresh token；软删除过滤不能破坏邮箱唯一性检查。
- [ ] 把当前 VerifyEmail 的 token 消费、激活用户及邮箱验证事实合并为同一用户锁事务；重发验证与请求重置也在锁内重验身份状态。恢复 pending 用户使用现有手动重发入口，旧链接始终失效。
- [ ] 增加真实 PostgreSQL 测试验证删除/恢复/purge/注册/登录发行的锁和回滚语义。

阶段门禁：生命周期单元与数据库集成测试通过；对 purge 子表操作注入失败，确认整笔回滚且邮箱仍被占用。未验证清理安全前，不允许打开生产 purge。

### S3：首次改密与 IAM 用户界面

依赖：S1；与 S2 集成后共同验证。覆盖 R15–R17。

- [ ] 新 seed 与管理创建用户通过同一创建参数设置首次改密状态；自助注册默认不设置。
- [ ] 初始密码验证仅发行用途受限凭证，不写普通 IAM login；完成端点在事务内消费凭证、校验当前密码版本并设置不同的新密码。
- [ ] 管理重设、账号禁用、删除、强制登出可使旧首次改密凭证失效；并发请求只能一次成功，失败不清除要求。
- [ ] 注入改密 DB 提交后 SCS 保存失败，验证明确返回“密码已更新、需重新登录”的 reason/UI，重试不重复消费；事务失败与登录交付失败分别验收。
- [ ] 改造 IAM Web 登录路由和首次改密页面，保留合法 OIDC 返回流程，不把凭证放入 URL、持久化前端存储或日志。
- [ ] 新增 IAM Web 删除账号入口、密码确认、错误反馈与成功后的当前浏览器退出。
- [ ] 将用户、登录标识、密码凭据及验证 token 纳入同一创建事务，提交后发送 IAM 原有验证邮件。提交前失败整体回滚；发信失败保留 pending 账号，UserService 返回部分完成 reason 和安全资源引用，公开自助注册维持自己的响应合同。
- [ ] 注入发信失败，经真实 gRPC 验证提交事实、reason/metadata；验证本人可使用 IAM 现有重发入口继续，不增加邀请、密码交付或额外通知。

阶段门禁：直接调用受保护端点与 OIDC 流程均不能绕过首次改密；浏览器覆盖验证邮件、改密、继续授权、自助删除，且错误密码无删除副作用。

### S4：Admin 服务、初始化与两层授权

依赖：S1；初始化需 S3 的 seed 行为；管理功能集成需 S2。覆盖 R1、R4–R7、R11、R18、R20、R21。

- [ ] 在 S1 新建的 Admin 工程和合同上实现管理用例、远端 IAM adapter、本地存储与运行接线，参考 IAM 的认证/会话/startup 实践并遵循 Trellis 分层；加入 `just/services.just` 和启动配置，与 S1 的 Buf 注册保持一致。端口使用新规划的 10010 段，Web 为 10012。
- [ ] 实现 confidential OIDC 登录、服务端会话、可信 Actor、CSRF、受保护会话查询和 Admin 本地退出。
- [ ] 用户 token introspection/refresh 必须使用发行方 `admin-web` client；验证错用 `admin-service` 被 IAM 拒绝，以及原 token 被撤销时现有 Admin 会话不能继续管理。
- [ ] Admin 到 IAM 使用独立 client credentials 服务身份；IAM 接收方验证令牌及 `iam.manage_users`，前端不得持有该凭据。
- [ ] Wire provider 创建并清理应用生命周期的 IAM connection/client，请求复用连接；服务凭据/服务授权故障映射为依赖错误，不让浏览器误判为自己的登录失效或资格不足。保留现有服务 JWT 离线验证合同，不把 client 删除或 token 数据库状态误报为即时吊销。
- [ ] 复用 Kratos errors 的 code/reason 与本地 cause 链，按 design 明确 HTTP 来源映射和安全 metadata；验证跨 RPC 仍能识别已知领域错误，不依赖错误字符串或假定远端 cause/任意 details 已恢复。检查 client/server 完成日志、实际 trace 关联与请求/error 脱敏，避免在 adapter/usecase 重复打印相同错误。
- [ ] 先核对 Servora gRPC client 的现有 context、middleware、credentials 和 dial 扩展点，再接入请求上下文与目标服务 token；不透传浏览器认证材料或未经验证的操作者身份。服务配置和 OpenFGA 服务权限先于首位人类管理员初始化完成装配。
- [ ] 按 R21 对确认属于框架的缺陷、通用缺口或可复用优化在 Servora 所属模块实施，补充回归并由 Plateau 接入；若无需上游改动，记录实际复用的扩展点。跨仓修改前读取 Servora 规范，记录依赖提交/版本与实际消费方式，不依赖未提交的本机替换才能验收。
- [ ] 用真实 gRPC 接收方覆盖服务凭据缺失/无效/错误 audience、有效身份无权限、有权限、deadline/cancel、trace 和错误传播；检查变更 RPC 无盲目自动重试、凭据不泄露且不跨目标复用。超时不能被解释为已提交变更自动回滚。
- [ ] 实现 OpenFGA 统一资格检查与列举/授予/撤销，所有受保护请求重新判断；以已部署模型和关系验收，不能用 mock/fixture 替代。
- [ ] 持久化 IAM bootstrap UID 与 Admin 初始化进度，保证重启、撤权、seed 禁用/删除或邮箱复用不触发创建/重新授权。
- [ ] 后端拒绝操作者禁用自己、删除自己或撤销自己的资格；IAM 自助删除保持已确认行为。
- [ ] 下游超时/拒绝/冲突正确映射；前端提示成功之前确认实际响应。列表拼接身份时保留无效 UID 占位，不误绑同邮箱新用户。

阶段门禁：服务层授权测试、OpenFGA 模型测试、OIDC callback 与 CSRF 测试，以及初始化故障恢复测试通过。运行配置与服务授权分别有真实调用证据。

### S5：Vben 管理页面

依赖：S4 的 HTTP 合同及 S2/S3 的领域行为。覆盖 R1–R4、R6、R11、R19。

- [ ] 替换实际 Ant Design Vue 应用中的演示登录和权限数据，使用 Admin 服务会话；移除本次入口的 mock 数据依赖。
- [ ] 侧栏两个平级入口：用户管理、权限管理；后者直接到管理员管理页。删除演示菜单对当前用户的可见入口，不无关重构整个 Vben 仓库。
- [ ] 用户页面支持列表/详情、精确邮箱查询、创建、七个 profile 字段编辑、封禁/解禁、设密、软删除/恢复、强制登出。
- [ ] 邮箱只读，头像保持 URL；删除视图显示恢复期限及是否可恢复，不提供即时物理删除按钮。
- [ ] 管理员页面从已有 IAM 用户授予资格，按稳定 UID 列举和撤销；不设置密码、不创建身份、不加入角色或租户编辑器。
- [ ] 把 IAM 用户信息与 Admin 资格操作做成分别提交的动作，避免部分成功显示成全部成功；保留领域错误和依赖失败反馈。
- [ ] 管理创建收到已创建但验证邮件失败的 reason 时，结束创建表单、提示部分完成并刷新身份；查询失败也保留已知创建结果，不显示普通“创建失败”或自动重复创建。浏览器覆盖发信失败与 IAM 重发后继续验证。
- [ ] 适配生成 HTTP client、cookie 与 CSRF transport，保留 Vben 独立依赖边界；禁用 localStorage access/refresh token 和 demo 角色作为真实权限来源。
- [ ] 头像退出只执行 Admin 本地退出；账号设置仅导航到 IAM，不复制个人中心。

阶段门禁：Admin lint/typecheck/build 和真实浏览器操作通过；不以菜单隐藏或 mock 测试证明后端授权。

### S6：联合验收与收尾

依赖：S1–S5。

- [ ] 逐项验收 PRD AC1–AC22，将结果与可复现步骤记录于本任务的验证记录；未跑、失败、环境阻塞分开记载。
- [ ] 运行下述质量命令，检查生成 diff 与所有跨层合同；只在新变更、失败或未解决风险出现时扩大/重复测试。
- [ ] 同时记录仓库支持、实际启用配置和端到端结果，不将其中一种等同于另外两种。
- [ ] 评审本任务最终 diff，确认无产品范围扩展、无明文密码/token日志、无 spec 提前承诺或未归属改动。
- [ ] 实施通过后，再提炼实际实现支持的通用规则到 spec；同步索引/链接，保留 task 中的需求和实施历史。
- [ ] 按 Trellis 收尾，不自动提交、推送、发布或改写用户已有提交。

## 验证命令

以下均为实施后的命令，不表示本次规划已经运行。管理员后台不存在的命令需在 S4 登记之后执行。

仓库根目录，根 Go module、Buf 和生成输出：

```bash
rtk proxy env GOWORK=off just gen
rtk proxy env GOWORK=off go test ./app/iam/service/... ./app/admin/service/... ./security/...
rtk proxy env GOWORK=off just lint
rtk proxy just openfga-model-validate
rtk proxy just openfga-model-test
rtk proxy git diff --check
```

`just lint` 当前包含 Proto、Go lint 与共享 TS 检查，不包含 Web lint。OpenFGA model apply 属于环境写操作，在实施联调的明确目标环境执行并记录 store/model ID；上面两个静态命令不能证明模型已部署。

若 R21 涉及 Servora 改动，在 `../servora` 按实际受影响包执行回归与对应 lint，并将确切命令、依赖提交/版本、Plateau 更新方式补充到验证记录。父级 go.work 可辅助开发，但最终还需验证 Plateau 的独立依赖消费及真实受保护 RPC；不自动发布或打标签。

R22 的旧实现基线已在本轮运行，结果见 [归属核查](research/ent-mixin-ownership.md)。迁移后须重新执行 Plateau 新共享包与 Example 的相关测试、软删除的生成 Ent/数据库合同，以及 Servora 独立 `core/crud`、Ent CRUD 单测和 SQLite live contract；IAM 的真实 PostgreSQL 生命周期并发验证仍不能由 SQLite 替代。检查源 schema、生成引用及活跃文档不存在未处理的旧包依赖。此段是迁移后的验收要求，不把基线通过记作迁移通过。

数据库并发验证：先给当前进程提供隔离测试库的 `IAM_TEST_POSTGRES_DSN`，不把连接密码写入记录。IAM 当前测试会创建独立 schema，并在结束时清理该 schema。检查输出确认 PostgreSQL 测试没有因为缺少变量而 SKIP。

```bash
rtk proxy env GOWORK=off go test ./app/iam/service/internal/data -run '^TestPostgres' -count=1 -v
```

新增生命周期与初始化并发测试采用相同 PostgreSQL 测试入口。Admin DB 测试同样使用隔离 schema 和专用 `ADMIN_TEST_POSTGRES_DSN`；此变量和测试入口在 S4 新增，不描述为已有功能。OpenFGA 写入与初始化中断需要真实测试实例及单独隔离对象，不能修改用户实际管理员关系。

Web 检查，均从仓库根调用：

```bash
rtk proxy just web::iam::lint
rtk proxy pnpm --filter @plateau/iam-web typecheck
rtk proxy just web::iam::build
rtk proxy just web::admin::lint
rtk proxy pnpm --dir app/admin/web --filter @vben/web-antd typecheck
rtk proxy just web::admin::build
```

新增认证/表单状态转换的有意义测试纳入各自现有测试入口；不为静态菜单文案另写镜像实现测试。受影响的 `@plateau/client` 如有修改，再运行该包自身测试/typecheck。

## 本地运行和浏览器验收矩阵

先按配置启动 PostgreSQL、OpenFGA 和 IAM 使用的邮件/会话依赖，记录 IAM issuer、两个 Admin client、IAM 服务授权 tuple、Admin bootstrap 结果。原生运行使用下列独立终端命令；同一端口不得再被应用容器占用。

```bash
rtk proxy env GOWORK=off just service::iam::run
rtk proxy just web::iam::dev
rtk proxy env GOWORK=off just service::admin::run
rtk proxy just web::admin::dev
```

| 场景 | 验收结果 | 需求 |
| --- | --- | --- |
| seed 初次进入 Admin | IAM 初始密码后必须改密；成功再完成 OIDC，进入两个菜单 | AC1、AC17、AC19 |
| 普通 IAM 用户 | IAM 能登录，但 Admin 后端拒绝；直接 API 同样拒绝 | AC1、AC4 |
| 创建与验证 | 待邮箱验证，只有 IAM 原有验证邮件；验证后初始密码仍须改密 | AC15、AC17 |
| 创建后发信失败 | 保留已创建 pending 账号，明确邮件失败，不删除或重复创建；本人经 IAM 重发后继续验证与首次改密 | AC5、AC15、AC17 |
| profile 与并发 | 七个字段正确持久化，邮箱不可改；旧 etag 更新得到明确冲突 | AC2、AC3、AC5 |
| 密码与会话 | 管理设密、强制登出均撤销全部 IAM/OAuth 旧会话；保留邮箱验证事实 | AC7、AC9 |
| 删除与恢复 | 原 UID/密码/验证/状态/首次改密要求保留，旧会话不恢复 | AC8、AC12 |
| 期限与清理 | 默认 30 天；测试配置缩短后新删除采用新值，旧记录期限不变；失败不释放邮箱 | AC11、AC13 |
| 自助删除 | 当前密码正确才删除；错误无副作用；不能指定他人 UID | AC14 |
| 授权撤销 | 他人的现有 Admin 会话下一次请求被拒绝，IAM 身份未封禁或删除 | AC10 |
| 防误操作 | 自封禁/自删除/自撤权均在 Admin 后端拒绝；他人操作与 IAM 自删按既定规则 | AC18 |
| Admin 退出 | Admin cookie 失效，IAM 仍登录；再进 Admin 仍需检查资格 | AC16 |
| 初始化重试 | 重启、断点重试、配置变化、禁用/删除和同邮箱新 UID 均不导致重新赋权 | R11 设计合同 |
| 依赖失败 | IAM/OpenFGA 不可用时明确失败，不沿用“管理员”缓存放行或显示成功 | AC5、AC6 |
| 受保护 gRPC 调用 | 服务身份与人类资格分开检查；无效凭据/无权限被拒绝，deadline/cancel/trace 和错误按约定传播；有上游改动时附框架回归与独立消费证据 | AC21 |
| 端口重排 | IAM 保持原段，Admin/Example/Test 使用新段，dev/preview/代理/callback/宿主映射一致，Audit 冲突已解除，新表不含 Audit/CMS 占位 | AC20 |

浏览器优先使用 Codex 内置集成。双浏览器会话、多个设备 token、邮箱验证链接、短恢复期仅使用隔离测试账号与环境，不能拿真实用户数据做 purge 验收。

## 回退与停止点

- S1 采用加字段/表的迁移；旧普通用户默认无首次改密要求。IAM 激活软删除和强制改密后，不能直接回滚到忽略新字段的旧服务。
- 清理 worker 可配置关闭；数据验证失败时先停止新清理并保留 tombstone/邮箱占用，修复后重试。已成功物理清理的数据不能靠代码回退恢复。
- Admin 发布失败可停 Admin 服务并撤去新入口，IAM 已创建的用户、状态和会话变更不自动反向恢复；保留 bootstrap 完成记录，避免下次部署重新赋权。
- 凭据、会话或软删除升级出现绕过风险时停止发布，按 design 的兼容边界修复；不能通过关闭后端授权来绕过联调问题。
