# Admin 技术设计

状态：2026-09-16 规划待整体评审，尚未实施。端口调整按用户要求作为 AGENTS 与前后端配置的附带改动，不独立设计迁移流程。产品行为以 [PRD](prd.md) 为准；本文裁定调研中的技术候选，不能把 research 中的备选或旧问题当作新增需求。执行与验证见 [implement.md](implement.md)。

## 1. 职责与术语

| 概念 | 所有者及含义 |
| --- | --- |
| IAM 用户 | IAM 保存的稳定 UID、邮箱登录标识、profile、凭据及账号状态 |
| Admin 管理资格 | Admin 拥有、以 IAM UID 引用的一类统一管理权限；OpenFGA 保存关系并作判定 |
| 操作者 | 由 Admin 服务端验证的 IAM OIDC 身份建立的 user Actor；请求体不接受调用者自报身份 |
| Admin 服务身份 | Admin 调用 IAM gRPC 时使用的 client credentials 身份，与人类操作者分别授权 |
| 软删除 / 恢复 / 到期清理 | IAM 生命周期命令；邮箱在物理清理提交前继续被旧 UID 占用 |
| 首次改密 | IAM 仅向初始密码持有者提供的受限认证流程，完成之前没有普通 IAM 登录态 |
| Bootstrap 绑定 | 一次性初始化身份/资格的稳定 ID 与完成事实，不是永久超级管理员身份 |

调用链：Vben 浏览器 → Admin HTTP → IAM gRPC UserService。IAM Web → IAM AccountService/AuthnService 处理用户本人删除、登录与首次改密。Admin 不读取 IAM 数据库，不接管 IAM 凭据和删除事务；IAM 不读取 Admin 关系来决定用户能否自助删除。

Admin 没有 tenant、组织、业务岗位或业务运营模型。所有当前管理操作使用同一资格；不引入角色表、逐项动作勾选或只读岗位。OpenFGA 是权限判定方；Admin 和 IAM 各自执行本入口的权限检查。

## 2. 应用与工程布局

- 实施时在`app/admin/service`路径建立独立 Admin 服务，使用仓库根 Go module；以 Example 的通用工程和 `service → biz ← data` 为起点，按需参考 IAM 的认证、会话、Wire 与 startup 接线。以 Trellis [布局](../../spec/service/backend/layout.md)、[分层](../../spec/service/backend/layers.md)、[编码](../../spec/service/backend/coding.md) 及 [Example](../../spec/example/backend.md)/[IAM](../../spec/iam/backend.md) 后端规范为依据，结合 Admin 已确认职责组织代码，不恢复旧复制树。
- Admin 自有 PostgreSQL 数据库保存服务端会话、OIDC 登录事务和 bootstrap 进度；不复制 IAM 用户/密码表，也不维护第二份管理员关系真相。OpenFGA 中的资格关系是唯一授权来源。
- Admin 的人类管理 API 首先只注册 HTTP transport；新规划 10010 为 HTTP，10011 预留 gRPC，10012 为 Web。生成 gRPC stub 不等于开放管理 gRPC 入口；以后增加入口须重新完成可信 Actor 与授权接线。
- Vben `app/admin/web/apps/web-antd` 保留独立 workspace/lockfile；使用共享 `@plateau/api` 和 `@plateau/client` 的本地包依赖，不复制生成类型。相对该 app package 的路径分别是 `../../../../../api/gen`、`../../../../../web/packages/client`；安装与 lockfile 仍在 `app/admin/web`。
- Admin Proto 放 `app/admin/service/api/protos/admin/**`，在根 `buf.yaml` 注册，生成到根 `api/gen/go` 和 `api/gen/ts`；IAM 的 Proto 扩展继续归 IAM。OpenAPI 随 Admin 服务生成。
- `just/services.just` 增加 Admin 后端；现有 `just/webs.just` 已有 Admin Web。不新增独立 `go.mod`，不依赖本机父级 `go.work` 才能编译。

S1 先建立下列职责对应的应用骨架与 `app/admin/service/api/protos/admin/**` 源合同，再接入根生成/构建；S4 在此基础上完成具体用例与运行接线。旧复制树的 namespace/import 冲突已随用户删除失去当前适用性，不再安排复制树清理步骤。

| Admin 模块 | 新建模块的职责 |
| --- | --- |
| `service` / `biz` | HTTP 适配与管理用例；biz 定义 IAM 管理调用、本地会话、管理员关系和 bootstrap 存储所需的 ports，不接收 transport request 或 import data |
| `data` 的 IAM adapter | 调用 IAM 公开生成客户端、映射资源与失败来源；不使用 Admin 本地 User/Credential repository 管理身份 |
| `data` 的本地 repositories | 仅保存 Admin login/session、登录事务与 bootstrap 进度；Ent schema 和自动建表范围与此一致 |
| `data` 的资格 adapter | 读取/写入限定的 OpenFGA 管理资格关系，不建立第二份关系真相 |
| `oidc` / `authn` | Admin RP 登录与会话到可信 user Actor 的适配；不发行平台 OIDC token、不保留 IAM Provider 或密码登录 |
| `startup` / Wire | 装配上述依赖及清理函数；初始化调用远端 GetBootstrapUser，不创建本地 seed 或输出本地初始密码 |

按现有实践建立启动、配置加载、可观测性与 transport 骨架，只装配 Admin 所需的依赖。IAM User/Credential/Account 持久化、OIDC Provider、邮件和 seed 创建继续归 IAM。Admin 公开 API 以 5.3 为唯一清单，复用 IAM 公共 User/Profile 消息。OIDC redirect/callback 可用专用 HTTP handler，无需为复用生成器把重定向硬套为普通 JSON RPC。Admin public origin、IAM issuer 与两个 client 凭据分别配置；只建立当前需要的 HTTP server，gRPC 端口继续预留。

## 3. Admin 登录、会话与服务调用

### 3.1 浏览器登录

采用服务端 BFF：IAM 静态注册 confidential OIDC client `admin-web`，授权码流程启用 PKCE S256；允许 `authorization_code`、`refresh_token` grants，scope 为 `openid profile email offline_access`，精确登记 callback。复用当前已依赖的 OIDC/OAuth 库和 SCS 存储，不自行实现签名验证或协议解析。

1. 浏览器访问 Admin 登录入口，服务端生成 `state`、`nonce`、PKCE verifier 与受约束的站内返回路径，保存短期登录事务，再跳 IAM。
2. IAM 完成正常认证，包括邮箱验证和必要的首次改密。Admin 不接收用户密码。
3. Admin callback 校验一次性 state、nonce、issuer、签名、audience、有效期和 PKCE，使用服务端 secret 换码；失败不建立会话。返回路径只能是本站允许路径，不能成为开放跳转。
4. 用已验证 `sub` 创建 user Actor，检查 Admin 资格；无资格显示明确拒绝，不进入工作区。轮换本地会话 ID，保存 UID 和必要的服务端 OAuth 会话材料。
5. 浏览器只持 HttpOnly cookie。生产使用 `__Host-admin_session`、Secure、SameSite=Lax、Path=/ 且无 Domain；本地 HTTP 若关闭 Secure，使用不带 `__Host-` 的独立 `admin_session` 名称。cookie 不按端口隔离，必须与 IAM 名称不同并使用独立服务端存储。OIDC token、client secret、PKCE verifier 不进入 URL、前端 storage 或日志；callback URL 的 code/state 不保留在最终页面地址。

Admin 的 `admin_login_sessions` 保存 UID、issuer/client、服务端 access/refresh token、期限与撤销状态，SCS 只存该记录的 opaque 引用。OAuth 登录事务单独保存并原子消费。本地会话默认绝对有效期 8 小时、空闲 30 分钟，均可配置；OIDC 登录事务默认 10 分钟。token 更新在该 Admin login 行锁内重读当前版本、换取并提交，防止多个实例并发消费同一 refresh token；不能仅使用进程内 mutex 或 SCS 的旧请求快照。服务器端持有的 token 不能超越 IAM 的有效期；refresh 被 IAM 拒绝或用户 token 已失效则终止 Admin 会话、要求重新认证，不使用 UID cookie 无限续期。

### 3.2 每次受保护请求

按顺序解析有效 Admin 本地会话、确认 IAM 用户授权会话仍有效、检查 OpenFGA 资格，再调用管理用例。IAM introspection 要求调用 client 与 token 发行 client 相同，因此必须以 `admin-web` 的凭据检查用户 token，不能使用 `admin-service`。到期前/已到期的刷新在服务端使用同一 `admin-web` Basic 凭据及上述 grant 配置，串行化更新后 introspect 新 token；未到期 token 被判 inactive 时不得绕过撤销继续放行。IAM 失效则清除本地登录；IAM 不可达返回依赖失败，不当成令牌失效成功退出或继续放行。

OpenFGA 检查 `user:<uid> admin admin:global`，使用要求读取最新关系的一致性模式；不长期缓存肯定结果。撤销资格成功后的新请求必须拒绝，已经完成授权检查并进入执行的请求不承诺撤回。列表读取采用同样的一致性要求，不能只在登录时把管理员标志写入 cookie。

所有 cookie 认证的变更请求校验 Origin 与服务端会话绑定的 CSRF token；SameSite 不是唯一措施。会话概要接口返回 UID、显示资料、管理资格及 CSRF token，不返回 OAuth token。OIDC callback 使用自己的 state/nonce 校验。

Admin 本地退出通过 POST 撤销本地 login 记录、删除其中 token 并销毁 SCS 会话与浏览器 cookie；与并发 refresh 使用同一行锁，不能被较晚完成的请求重新激活。不调用 IAM logout/end_session，不撤销 IAM 登录或其他应用会话。未登录的退出可安全重复。

### 3.3 Admin → IAM

另行注册 client credentials client `admin-service`，其 audience 为 IAM 接收方要求的值，映射为 `service:admin-service`。按已部署 OpenFGA 模型写入 tuple `(user=service:admin-service, relation=manage_users, object=iam:global)`。Admin service 缓存服务 token 至有效期前刷新；密钥只在后端配置。

IAM gRPC 接收方验证 bearer 签名、issuer/audience、有效期、可信 service Actor 和 `manage_users`；Admin 的人类资格检查不替代此授权。当前服务 JWT 采用离线验证，不查询 OAuth token/client 的数据库活动状态；删除 client 阻止后续获取令牌，不等于立即使已签发的 JWT 失效。沿用当前服务 token 有效期与独立 OpenFGA 授权合同，本任务不把即时 token 吊销写成已有能力或顺带新增撤销系统。人类 UID 可用于 Admin 本地记录操作上下文，但不能把任意传入 header 当作 IAM 可信用户身份。本任务未建设 Audit，不宣称已有跨服务完整审计链。

服务 client、密钥和 `iam:global#manage_users@service:admin-service` 由部署配置预先装配；Admin 用该机器身份调用 GetBootstrapUser，再按初始化合同授予人的 Admin 资格。获取服务令牌不依赖人类管理员登录或已存在的 Admin 管理资格，避免初始化循环依赖。

IAM client/connection 由 Admin data provider 创建并由 Wire 注入，使用应用生命周期上下文和有界建连超时，返回 cleanup 随服务退出关闭；每个请求复用连接并传入自己的 RPC context，不能在 handler 中逐次 Dial。服务 token 缓存归该 IAM client 的凭据组件，浏览器会话中的用户 token 不参与服务凭据注入。

### 3.4 Servora gRPC client 实践与协同迭代

Admin → IAM 优先通过 Servora 的客户端入口和既有扩展点接线。按 [Servora 协同迭代规则](../../spec/plateau/project/boundaries.md#servora-协同迭代) 先确认责任，再决定是否改动框架；不预先把尚未验证的接入问题判为框架缺陷。

- 请求上下文：管理 RPC 延续当前请求的 deadline、cancel 和 trace；初始化调用使用服务生命周期上下文并有明确超时。不能用无边界的 Background 替代请求上下文。取消不保证已经提交的 IAM 变更回滚，超时后的结果未知须保留，不自动视为失败且无副作用。
- 凭据上下文：Admin 后端提供面向 IAM 的服务 token，限定到对应 client/目标；不无差别转发浏览器 Authorization、cookie 或全部 metadata。令牌获取、缓存和刷新遵循本节 3.3；通用凭据注入扩展点可由 Servora 提供，IAM issuer/client/audience 与服务身份语义由 Plateau 接线。
- 授权上下文：Admin 验证人类管理资格，IAM 验证服务身份及 `manage_users`。客户端携带 token 不代表已完成授权；没有经过信任合同验证的操作者字段不得在 IAM 变为 user Actor。本任务不新增代理用户授权或通用身份委托协议。
- 错误与重试：保留认证失败、权限拒绝、超时/取消和领域冲突的可区分语义；检查实际 client middleware、dial option 和运行配置，不对创建、设密、删除等变更盲目自动重试。token 获取失败必须终止本次受保护调用，不退化成匿名请求。

若上述链路暴露出通用客户端扩展、上下文传播或错误处理缺口，优先在 `../servora` 所属模块补齐并添加回归验证，再接入 Plateau。已有能力满足要求时直接复用；业务规则、OpenFGA 关系与 IAM 专属认证逻辑不迁入框架。验收同时覆盖框架改动与实际 Admin → IAM 链路，并记录独立依赖消费方式；不以本机 go.work 或单侧测试代替联合验收。

## 4. 统一管理员资格

复用当前模型 `admin:global#admin@user:<uid>` 与派生 `manage_users`，所有管理员资格操作也要求统一 `admin` 关系；不为当前相同的权限集合额外构建角色体系。

Admin 的权限管理用例通过 OpenFGA Read 按固定 object/relation 及 continuation token 分页读取、幂等添加或删除直接 user tuple。已核实当前 SDK v0.8.2 的 Read 支持 page_size、continuation_token 和 Consistency；ListUsers 没有相同分页合同，也不应用推导权限集合冒充授予记录。不接受任意 object/relation/subject 类型写入。当前共享 Authorizer.Check 没有一致性参数，需要补充向 SDK 传递 HIGHER_CONSISTENCY 的选项及相应测试，不能只在文档写成已可使用。

- 添加：先由 IAM GetUser 验证目标稳定 UID 存在且未软删除，再写入资格；pending/disabled 身份的资格不使其绕过 IAM 状态和邮箱验证要求。相同资格重复授予视为已存在。
- 撤销：要求 target UID 与当前 actor UID 不同，删除该 tuple；已不存在可幂等成功。完成之后再来的请求按最新关系拒绝，不修改 IAM 密码、状态或会话。
- 列表：分页读取 tuple，再对当页 UID 查询 IAM 资料；soft-deleted/缺失身份显示状态或 UID 占位，仍可撤销旧资格。IAM 故障不得伪装成“用户已删除”。绝不按旧邮箱替换成新 UID。
- 软删除/物理清理不会让 IAM 去删 Admin/OpenFGA 关系；恢复旧 UID 可继续使用仍存在的资格，同邮箱新 UID 不继承。旧 UID 永不复用。
- Admin 后端同样拒绝 actor UID == target UID 的封禁、删除和资格撤销。前端禁用按钮只提供反馈；不增加全局“最后管理员”计数，也不拦截 IAM 自助删除。

## 5. 对外接口合同

以下为新增设计名称；源码尚无相应实现。源 Proto、注解、领域 reason 和生成客户端共同维护合同，不手写第二份前端协议。

### 5.1 IAM 管理接口（gRPC）

保持 `iam.user.v1.UserService` 现有创建、获取、列举、profile 更新、封禁、解禁，均要求 service `iam.manage_users`。

| 方法/扩展 | 请求与结果 |
| --- | --- |
| `ListUsers` | 保持分页/排序；增加 canonical email 精确过滤和 `show_deleted`。虚拟过滤字段 `deleted=true/false` 明确映射 tombstone 谓词；`deleted=true` 要求 `show_deleted=true`，冲突参数拒绝 |
| `GetUser` | 仍以 `users/{uid}` 为 name；可显式 `show_deleted=true` 查询管理 tombstone，默认隐藏 |
| `User` 输出 | 增加只读 `delete_time`、`purge_time`；status 保留原领域状态，不增加与 tombstone 重叠的 DELETED 状态 |
| `SetUserPassword` | name、实际新密码、etag；密码字段带 redact 注解，走受保护传输且禁止日志记录。返回空结果；独立管理命令，无旧密码/邮箱凭证 |
| `DeleteUser` | name、etag；返回 tombstone User，包含确定恢复截止时间 |
| `UndeleteUser` | name、etag；返回恢复后的 User；截止时刻及之后拒绝 |
| `ForceLogoutUser` | name；返回空结果。撤销全部 IAM/OAuth 会话和受限首次改密凭证，账号状态与密码不变 |

profile 更新继续用 `update_mask` 和资源 etag，仅允许已确认七字段；字段缺席与显式清除不同，邮箱/status/delete_time 等不可经 Update 改写。Admin 页面提交已读取 etag；过期 etag 返回冲突并重新加载，不悄悄覆盖他人的更新。

邮箱过滤先使用 IAM 现有 trim、Unicode NFC、case folding 规范化，再与 LoginIdentifier 的 canonical value 精确比较。缺失为零条结果，格式/过滤语法错误明确报错；禁止模糊匹配后取第一条或全量拉取客户端筛选。保留 `GetUser(name)`，不新增 `GetUserByEmail`。

### 5.2 IAM 自助与认证接口

- `AccountService.DeleteAccount(current_password)`：HTTP `POST /v1/iam/account:delete`，普通登录必需；目标只取可信上下文，提交实际当前密码，字段带 redact 注解，走受保护传输且禁止日志记录。成功返回空结果并清除当前 IAM cookie。AccountService 不提供恢复方法。
- `AuthnService.LoginResponse` 增加 `required_action`（默认 NONE 或 CHANGE_PASSWORD），保留现有 `user` 字段号；CHANGE_PASSWORD 不表示普通登录成功。
- `AuthnService.GetFirstLoginStatus`：`GET /v1/iam/authn/first-login`，仅解析当前 SCS 的受限凭证，返回 required/expired/unavailable/completed_login_required 状态，不泄露 token；已消费状态只对持有原受限会话者可见。
- `AuthnService.CompleteFirstLoginPassword(new_password)`：`POST /v1/iam/authn/first-login:complete`，只使用受限 SCS 上下文，不接收目标 UID/token 参数。可在普通 AuthN 注解标为 public，但内部必须验证独立 challenge、Origin/CSRF；不得把 public 注解理解为无需凭证。
- `iam.user.v1.UserService.GetBootstrapUser(GetBootstrapUserRequest) returns (User)`：仅 gRPC、复用 service `iam.manage_users`。请求不需要 ID 或邮箱；IAM 先读取持久化的初始化用户 ID，再返回对应 User。初始化记录缺失返回未完成初始化错误；用户已物理清理返回 NOT_FOUND，不另找替代身份；软删除用户可返回带 tombstone 的 User，由 Admin 拒绝首次授予。接口直接表达“获取初始化用户”，内部 binding 的表名/主键/完成标志不成为公开协议；不查询创建时间最早的用户，不公开给浏览器。

### 5.3 Admin HTTP

Admin 源 Proto 定义 HTTP API，用户投影和可复用消息引用 IAM 公开 Proto，避免复制 UserProfile 定义；Admin 授权由 Admin API 注解/中间件执行，不能沿用 IAM service-only 授权假装已验证人类。

| 入口 | 用途 |
| --- | --- |
| `GET /auth/login`、`GET /auth/callback` | OIDC 登录/回调，服务端 redirect handler |
| `GET /v1/admin/session`、`POST /v1/admin/session:logout` | 会话概要/本地退出 |
| `/v1/admin/users`、`/v1/admin/users/{uid}` | list/create/get/profile patch，对应 IAM 公共资源名 |
| `POST /v1/admin/users/{uid}:disable` / `:enable` / `:setPassword` / `:forceLogout` | 独立管理命令 |
| `DELETE /v1/admin/users/{uid}`、`POST ...:undelete` | 软删除/恢复，携带对应 etag |
| `GET /v1/admin/administrators` | 分页列举资格与当页身份概要 |
| `PUT /v1/admin/administrators/{uid}`、`DELETE .../{uid}` | 为现存 IAM 身份幂等授予/撤销资格 |

缺少认证为 401；无资格/自我保护为 403（不同 reason）；etag/状态冲突为 409；非法输入为 400；依赖不可达为 503/504。沿用生成 reason 表达领域错误，不将所有错误映射成“操作成功”。超时可能有已提交变更，UI 提示结果待确认并刷新；不自动重放创建、设密等敏感动作。

上述 401/403 分别指 Admin 本身的人类认证和资格检查失败。IAM adapter 必须按错误来源和领域 reason 区分：下游服务凭据无效或缺少服务权限属于 Admin 的依赖故障，返回 503 与可识别 reason，不能原样透传成浏览器 401/403或清除人的有效会话；IAM 的已知 NOT_FOUND、etag/状态冲突按 404/409 映射。下游不可达/截止时间按 503/504，保留请求取消语义，不为返回状态而重放变更。日志保留操作、trace 和脱敏的失败来源，不记录 bearer/secret。

复用当前 Kratos v3 的错误合同：gRPC status 承载映射后的状态和安全 message，ErrorInfo 承载 reason/metadata；远端的内部 cause 和任意附加 details 不保证恢复。通过 `errors.FromError` 等既有入口识别结构化错误，不解析 `err.Error()` 字符串；本进程需要包装时使用 `%w`/`WithCause` 保留链。Admin 按已知 IAM reason 映射用户可见结果，metadata 仅允许显式确认安全的字段，未知错误使用安全的 Admin reason/message，不把远端原始 Error/cause/metadata 无差别透传到 HTTP。

诊断日志以 client/server transport 各自的一次完成记录为主，这两端记录各有意义；adapter/usecase 仅在确有新增上下文时补记录，不逐层重复相同 Error。结构化字段包含调用方向、下游服务、RPC operation、code、reason、耗时及 trace/span 关联；核对 stdout/file 与 OTEL 实际 handler，当前并非所有输出都自动带 trace 字段。请求体沿用生成的 Redact，检查错误 cause 和新增 metadata 不拼接密码/token/secret，不全量记录 header。该约定不撤销已明确的 seed 首次创建受控密码交付日志，也不新增 Audit 功能。若日志字段/脱敏扩展属于通用能力且现有入口不足，按 R21 的证据与上游边界处理。

### 5.4 管理创建账号与验证邮件的部分完成

对应 R16/AC15，用户已确认保留已创建的待验证账号并明确反馈邮件失败。IAM 创建用户、登录标识、密码凭据及验证 token 使用同一数据库事务；生成或保存失败则整体回滚。提交之后调用现有 MailSender，外部发信不放在数据库事务内，也不通过删除已提交账号补偿。

UserService.CreateUser 保留成功返回 User 的合同。提交后的发信失败返回独立的 `USER_CREATED_VERIFICATION_EMAIL_FAILED` reason，gRPC 状态使用 Unavailable；安全 metadata 仅携带 `resource_name=users/{uid}`，不能携带验证 token、密码或邮件系统原始错误。Admin adapter 识别此 reason，映射为 HTTP 503 的同等部分完成语义并保留经过校验的资源引用；它优先于普通依赖故障分支处理。返回该 reason 必须意味着创建事务已经提交，不能把一般超时或不可达推断为已创建。

Admin Web 对该结果展示“账号已创建，但验证邮件发送失败”，结束创建表单并刷新已创建身份，避免通用错误拦截器再提示“创建失败”。刷新失败时仍保留已知的创建结果和资源引用，提示查询暂不可用，不重新提交创建。本人通过 IAM 已有重发验证入口继续；不增加 Admin 发信按钮、自动重放创建、密码交付或邀请流程。共享创建用例对发信结果保留提交事实，公开自助注册入口继续采用自己的响应合同，不直接暴露管理端 metadata。

测试分别覆盖创建事务失败、提交后发信失败、成功发信、未知 RPC 超时，以及部分完成后的查询失败；真实 gRPC 验证 reason/安全 metadata 传播，浏览器验证明确提示、账号可查询和 IAM 重发后继续邮箱验证及首次改密。测试发信失败只证明可观测的发送失败，不宣称 SMTP 返回失败一定意味着收件人未收到邮件。

## 6. IAM 删除、恢复与清理

### 6.1 存储与锁

User 组合迁入 Plateau `infra/entgo/mixin` 的 `SoftDeleteMixin` 获得 tombstone 字段与默认查询过滤；IAM 显式设置 `purge_time`，共享便利包不处理恢复、关联撤销或定期清理。`SkipSoftDelete` 同时绕过查询过滤和删除改写，IAM data 层只在授权的 Get/List 已删除视图、GetBootstrapUser、恢复和 purge 的确切查询上局部创建派生 context，不能将其传播到整个请求或无关写入。物理 Delete 显式 bypass 仅归内部 purge 命令；公共 `show_deleted` 只控制可见性，不能改变 DeleteUser 的软删除语义。

R22 先迁移当前实现、单测和软删除专属数据库合同，保持 Example 的已有语义；更新 Example schema/repository 并重新生成 Ent，再让 IAM 消费新路径。Servora 的 Ent driver、CRUD runtime/List/Clear 保持所属框架；其测试 fixture 改为自有字段和显式 query scope，只保留验证 CRUD 消费该范围的职责，不 import Plateau。完成两仓消费与回归后移除旧 mixin 包及活跃引用，不通过永久双份实现过渡。具体依赖和本轮基线验证见 [归属核查](research/ent-mixin-ownership.md)；包路径迁移不意味着 IAM 生命周期已经实现。

保留 LoginIdentifier 的全局 `(type, canonical_value)` 唯一索引。软删除不移走该行，不把索引改为只约束可用用户；注册冲突最终由数据库约束兜底，即使默认 User 查询已隐藏 tombstone。

同一用户的登录发行、密码替换、验证/重置、封禁、删除、恢复、清理与首次改密统一先锁 user，再锁凭据/一次性材料和更新关联会话。token 可先非锁定定位 user ID，但锁定之后必须重读并验证。禁止部分路径先锁 challenge、另一路径先锁 user 形成死锁。

当前 VerifyEmail 先消费验证 token，再由独立操作激活用户，尚不满足上述边界；实施必须合并 token 校验/消费、User 状态与 identifier verified_time 更新为同一事务。重发验证/请求重置生成 token 时也需在用户锁内重验当前状态，防止删除提交后发行新的有效一次性材料。

### 6.2 删除与恢复

删除事务在 user 锁内验证状态、etag和入口条件；自助删除的当前密码也在该边界内验证当前版本。原子写入 `delete_time=now`、`purge_time=now+当次恢复期`、可信 `deleted_by`、新 etag，并撤销全部 IAM/OAuth session/access/refresh token、首次改密 challenge 以及旧授权 code/request 的可用性。email verification/password reset token 立即失效，防止恢复后旧链接重新生效。

保留 status、UID、邮箱、密码、邮箱验证事实和首次改密标志。管理入口的 deleted_by 记录已验证 service Actor；自助入口记录 user Actor，不伪造 IAM 不曾验证的人类委托。

恢复事务以同一 user 锁读取 tombstone，要求 `now < purge_time`，仅清除 tombstone 并更新 etag。截止后即使清理尚未完成也不能恢复，邮箱仍占用直到 purge 提交。原 pending 账号由用户调用 IAM 已有重发验证入口取得新邮件，恢复命令不自动发送邮件；原 disabled 仍 disabled。旧会话和一次性材料不恢复。

已删除对象再删除不得延长截止时间；携带旧 etag 返回冲突，无 etag 的重复删除返回当前 tombstone。已恢复对象再次恢复返回当前状态或明确状态冲突，但不能再次更改凭据/会话；采用明确 INVALID_STATE，避免把另一方操作误当本次成功。

### 6.3 到期清理

IAM 生命周期 worker 默认每小时扫描 100 条到期 tombstone，可配置启停、扫描间隔、批量。每个用户独立事务、先锁 user、重新检查 `delete_time` 和 `purge_time <= now`，多实例重复扫描也不能让恢复与 purge 同时成功；可用 `SKIP LOCKED` 避免相互阻塞。

清理集合：email verification/reset token；首次改密 challenge；关联 authorization code/request；OAuth access/refresh token、token session；IAM login session；password authenticator、authenticator；login identifier；最后 User。按真实外键顺序和 user/subject/login/token-session 关联条件清理；不能只按一条关联路径漏掉未完成授权记录。服务账号 token 无人类 UID 归属，不在范围内。

bootstrap stable binding 不随 User cascade 删除；SCS 中失效引用依既有过期机制清理，不能恢复认证。外部 OpenFGA、Audit 和业务历史不在 IAM 物理清理范围内。任何一步失败整笔回滚，邮箱继续占用；成功后新注册获得新 UID。worker 使用已保存期限，不按最新配置重新计算。

worker 接入应用 start/stop，支持上下文取消，等待或回滚当前事务后退出；失败有无敏感数据的可观测记录。持续失败不能静默当成处理成功；不增加 Admin 运行参数编辑页面。

## 7. seed 与管理创建共用首次改密

密码认证器增加 `must_change_password`（旧数据默认 false）与递增 `credential_version`；新 seed 与 UserService.CreateUser 在创建凭据的同一事务设为 true，自助注册为 false。管理设密、找回密码及恢复都不能清除未完成标志；普通已有用户管理设密不新增标志。只有完成首次改密事务可将 true 变为 false。

创建 `first_login_password_challenges`：token hash、user/authenticator ID、credential version、过期/消费/失效时间。opaque token 由高熵随机数生成，仅存入服务端 SCS 专用字段，数据库保存 hash。默认 15 分钟，可配置；再次凭初始密码登录可使旧 challenge 失效并创建新 challenge。

登录先校验账号可用、未删除、邮箱已验证与密码。存在要求时轮换 SCS ID、清除 `iam_login_id`、存受限 challenge，返回 CHANGE_PASSWORD；没有创建普通 IAM LoginSession。IAM OIDC 仍要求普通 login ID，自然阻止提前授权，不把首次改密逻辑散布到每个 OIDC 入口。

完成端点先以 token hash 定位目标，再按 user → challenge/credential 的统一顺序加锁、重验状态/期限/版本/未消费及标志；拒绝与当前密码相同的新密码。原子替换 hash、递增版本、清除要求、消费并失效所有该用户 challenge、撤销旧 IAM/OAuth 会话，创建新的普通 IAM login。提交后轮换 SCS ID、清除受限字段并写入普通 login ID。

DB 已提交但 SCS 保存失败时返回 503 与 `FIRST_LOGIN_COMPLETED_LOGIN_REQUIRED` reason，页面明确“密码已更新，请使用新密码重新登录”，不显示改密未完成或自动重提。原受限 SCS 引用仍可解析时，状态查询对已消费 challenge 返回 completed_login_required；没有有效引用则返回登录入口。HTTP 响应丢失后重试同样不得再次消费。该分支已经完成改密，不能恢复初始密码或把 flag 重新置 true；未交付的普通登录会话作补偿撤销，补偿失败依期限清理并记录基础设施错误。禁用、删除、管理设密、强制登出使旧 challenge 失效。并发完成至多一个成功；事务本身失败则不清标志、不创建普通会话。

IAM Web 以结构化 `required_action` 跳到独立首次改密页。页面刷新向 IAM 查询受限状态；完成后才继续原 OIDC request ID，或进入 IAM account。合法续跳仅为现有授权请求/本站路径，过期时从应用重新发起；challenge 不放 URL/localStorage。管理创建仍先验证邮箱，Admin 创建后无新增密码交付、邀请或通知要求。

## 8. 一次性初始化与配置

### 8.1 稳定绑定

IAM 增加不随用户删除的单例 `bootstrap_binding(primary, user_id, canonical_email, completed_time)`。首次新部署在同一事务创建 active、email verified 的 seed、带首次改密要求的随机密码和 binding；邮箱已被普通身份占用则冲突，不自动把它认领为 seed。密码继续只在新建成功时经现有受控启动日志输出一次，重启不重新输出。

IAM binding 已存在后，启动不再要求该账号 active、未删除或仍存在，也不创建替代身份/改绑邮箱。Admin 通过受保护 GetBootstrapUser 取得初始化用户及真实 stable UID，邮箱仅作未完成首次绑定的配置一致性校验；不得改用 ListUsers(email) 的返回值授予。用户不存在或不可用不意味着清除 IAM 初始化记录。

Admin 自有单例记录 `primary, iam_user_id, state(PENDING/COMPLETED), completed_time`。首次持久化目标，再幂等写入和读回 OpenFGA tuple，最后标记完成；失败重试只用同一 UID。目标不可用、返回 UID 与本地记录不一致或依赖失败均停止初始化，不能回退到新邮箱用户。Admin 在完成前不开放管理变更，实例间通过本地数据库锁序列化初始化；目标的首次改密标志不妨碍绑定。

完成后重启只看完成记录，不因资格不存在、管理员人数为零、seed禁用/删除或邮箱新用户而重授。OpenFGA 与 Admin DB 没有分布式事务，PENDING 重试通过幂等 tuple 写入/最新读回收敛；初始化期间不支持绕过 Admin 在 OpenFGA 手工并发撤权并要求自动流程理解操作者意图。已完成后的撤权绝不触发自动补回。

### 8.2 配置与旧部署

| 配置建议 | 默认/约束 |
| --- | --- |
| `BOOTSTRAP_USER_EMAIL` | 一份部署输入供 IAM/Admin 首次绑定；沿用现有开发默认邮箱，生产显式配置 |
| 旧 `IAM_BOOTSTRAP_USER_EMAIL` | 兼容期在部署/加载归一化层作 fallback；新旧同时非空且归一化值不同则报配置冲突，不能依赖未验证的嵌套 `${...}` 语法 |
| IAM recovery_period | 720h（30天），正 Duration；每次删除保存固定截止时间 |
| IAM purge_enabled / interval / batch_size | true / 1h / 100；interval、batch 正值，允许显式关闭 purge 便于发布控制 |
| IAM first_login_challenge_ttl | 15m，正 Duration |
| Admin session lifetime / idle / login transaction TTL | 8h / 30m / 10m；正 Duration，idle 不大于 lifetime |
| IAM/Admin public origin、issuer、clients、secrets、gRPC endpoint、OpenFGA store/model | 明确独立配置；没有合法 client secret 和权限装配不得启动成“可管理”状态 |

时长使用 protobuf Duration，沿用配置文件与环境覆盖；启动校验，无热更新。初始化完成后的邮箱输入不用于改绑，缺失可从 binding 继续运行，变化记录提示。新旧环境变量同时冲突仍属于输入错误。日常只配置一份邮箱，不要求长期维护同值的两份独立变量。

旧数据库没有可信 seed 来源标记，不能凭邮箱或 password changed_time 猜测。新增一次性迁移参数 `bootstrap_legacy_user_id`：运维显式指定已确认旧 seed UID，IAM 校验对应邮箱且无冲突后写入 binding；未提供且邮箱已存在则拒绝自动认领。参数只用于旧部署迁移，完成后忽略，不建设新 CLI。旧普通用户和既有 seed 的 must_change_password 默认 false；已确认 R17 适用于新创建 seed，不强制所有历史账号改密。

## 9. Web、部署与回退边界

### 9.1 本地端口布局

按本次用户决定重排；以下是实施目标，当前根 AGENTS 与运行配置仍是旧分配。每应用十个端口，+0 HTTP、+1 gRPC、+2 Web，+3～+9 预留；缺失的服务槽位只保留位置，不新增不需要的服务。

| 应用 | 端口段 | HTTP | gRPC | Web |
| --- | --- | --- | --- | --- |
| IAM | 10000–10009 | 10000 | 10001 | 10002 |
| Admin | 10010–10019 | 10010 | 10011（预留） | 10012 |
| Example | 10080–10089 | 10080 | 10081 | 10082 |
| Test | 10090–10099 | 10090（预留） | 10091（预留） | 10092 |

10020–10079 留给后续基础微服务，从 Admin 后面的空闲段按需登记，不能因为 Example/Test 编号较大就从 10100 继续顺排。Audit/CMS 暂不在新登记表占位。现有 Audit 配置占用 10010/10011，会与 Admin 冲突；按用户给出的“在 Admin 后面”顺序，将现有 Audit 的本地监听/宿主映射顺移到 10020/10021，表中暂不增加该服务条目。后续登记须检查实际配置占用，不能因表中暂未列 Audit 再将同段分配给其他服务。

本规则用于本地监听和 Docker 宿主映射，不重排容器内部/共享中间件/生产公开端口；dev 与 preview 共用 Web 槽位。当前 AGENTS 的“已有编号不重排”由本次明确调整覆盖，之后新登记仍保持已有应用编号稳定。实际改动限定为根 AGENTS 与相关配置/启动脚本（含 proxy、静态 callback、宿主映射），随应用联调检查；不批量改写历史文档或 spec，不新增迁移工具。

### 9.2 页面与公开地址

Vben 仅提供两个平级菜单。用户管理对 IAM 用户执行独立命令，已删除视图展示原状态、删除/清理时间和恢复按钮；权限管理直接展示管理员资格列表。资格变更与身份编辑分别保存并反馈，不构造跨 IAM/OpenFGA 的假原子“保存全部”。资料字段全选定，邮箱只读，头像 URL 不增加上传能力。IAM 个人设置可通过导航进入。

本地默认 issuer 为 `http://localhost:10002`，IAM HTTP/gRPC 为 10000/10001，Admin Web 为 `http://localhost:10012`，Admin HTTP 为 10010；Admin OIDC callback 走 Web 同源代理 `/auth/callback`，浏览器 `/auth` 与 `/v1/admin` 请求代理到 10010。Example Web/后端代理一起迁到 10082/10080，Test Web 的 dev 与 preview 都迁到 10092。生产仍采用各应用同源反向代理及 HTTPS，不把容器内部地址注册为公开 callback。

新增 Admin 服务 local/docker配置和数据库登记；当前应用 Compose 只有 Audit，不能宣称现成 IAM/Admin 一键容器运行。本任务以原生服务 + 容器基础设施完成端到端验收，记录四个启动入口和静态client/tuple配置；不顺带重建全套容器部署。

部署先做非破坏性 schema/配置/API准备，再发布支持新状态的 IAM 与 IAM Web、装配真实模型/服务关系、部署 Admin 后端与 Vben。首次改密 LoginResponse 新分支需要配套新 IAM Web，旧 Web 不识别该分支会出错，不能长期混跑。软删除启用后旧 IAM 可能忽略 tombstone，回滚到该旧版本不安全。

清理不可逆，先在隔离库验证完整关联集合与失败回滚；生产升级可先关闭 purge，验证后开启。回退优先停 Admin入口或清理 worker、保留新增字段与初始化记录；不得通过回滚旧二进制绕过首次改密/软删除，不能重新激活已撤销会话。现存备份恢复不在本任务自动执行范围内。

## 10. 设计依据与实施门禁

源码事实见 [管理能力](research/management-capabilities.md)、[邮箱查询与 AIP](research/email-lookup-api.md)、[删除生命周期](research/lifecycle-design-evidence.md)、[首次改密与初始化](research/first-login-bootstrap-design.md)、[Admin 集成](research/admin-integration-design.md)。本设计选择取代 research 中未采用的候选，例如不重开密码交付要求、不新建 bootstrap 专属动作、不把受限改密端点归普通登录的 AccountService。

实施时重点验证：真实 PostgreSQL 的并发与回滚、OpenFGA 最新关系检查/分页/幂等写入、IAM 对 Admin用户token introspection 的 client权限与audience接线、RP库的state/nonce/PKCE/cookie行为、SCS提交失败、旧seed迁移以及独立 Vben workspace 消费生成TS。若当前依赖不支持所选接口，按已查证缺口修正方案并补研究，不以 mock 或跳过权限检查替代。

该设计没有提供跨应用注销传播、全局最后管理员保证或新通知系统；这些限制与 PRD 已确认的范围一致。规划校验通过不等于产品测试或部署验收通过。
