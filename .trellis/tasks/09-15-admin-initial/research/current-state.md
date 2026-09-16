# Admin 规划：当前事实与参考边界

核对日期：2026-09-15。以下区分已确认设计、仓库能力和待验收运行行为；本次未启动应用或验证真实部署。

## 已确认设计

- Admin 面向 Plateau 平台管理与运维人员，提供基础共享服务的管理与查询入口。
- Web 采用 Vben 的 Ant Design Vue 版本，按最新根 AGENTS 保留独立 workspace 与 lockfile；平台 [pnpm-workspace.yaml](../../../../pnpm-workspace.yaml) 已通过 `!app/admin/web` 排除该目录。
- 用户管理链路为 Admin Web → Admin HTTP API → IAM gRPC UserService。具体 HTTP 契约尚未定义。
- Admin 检查人类操作者的管理权限；IAM 检查调用服务及其 IAM 管理权限。全局身份和状态转换继续由 IAM 拥有。
- 用户重新明确首期包含强制设密、删除、强制登出 IAM，以及 Admin 自有权限结构；这些目标超出以下已有 RPC，缺口见 [新增需求核查](management-capabilities.md)。
- 参考：[Admin 架构](../../../spec/admin/architecture.md)、[前端规范](../../../spec/admin/frontend.md)、[后端规范](../../../spec/admin/backend.md)。

## 当前 Admin Web 骨架

- 新增的 `app/admin/web` 是独立 Vben checkout，保留自身 workspace 与 lockfile；主应用位于 [apps/web-antd](../../../../app/admin/web/apps/web-antd/package.json)，应用包标记版本为 `5.8.0`，外层 [package.json](../../../../app/admin/web/package.json) 标记为 `5.7.0`。这是当前本地快照，不替后续升级决定版本。
- 根 [Just registry](../../../../just/webs.just) 已登记 Admin dev/build/lint；命令存在不代表本任务执行过构建或登录联调。
- 当前 [登录页](../../../../app/admin/web/apps/web-antd/src/views/_core/authentication/login.vue) 使用演示账号；[开发环境](../../../../app/admin/web/apps/web-antd/.env.development) 启用 Nitro mock，[auth API](../../../../app/admin/web/apps/web-antd/src/api/core/auth.ts) 调用 `/auth/login`，[mock handler](../../../../app/admin/web/apps/backend-mock/api/auth/login.post.ts) 验证演示用户并签发自有令牌。当前不是 IAM/OIDC 接入。
- `app/admin/service` 仍不存在。后续实施从现有 Web 骨架继续，保留其已有变更，新增 Admin 后端并替换演示认证链路。

## IAM 已有用户管理能力

源接口：[user.proto](../../../../app/iam/service/api/protos/iam/user/v1/user.proto)。

| RPC | 当前接口含义 |
| --- | --- |
| `CreateUser` | 接收初始密码，创建待邮箱验证的用户；不等同于邀请注册或立即激活 |
| `GetUser` / `ListUsers` | 查询全局 IAM 用户；List 提供分页、过滤与排序字段 |
| `UpdateUser` | 更新允许修改的资料，使用 field mask；不能任意改变身份生命周期字段 |
| `DisableUser` | 禁用用户；凭据、登录和 token 的相关效果由 IAM 领域执行 |
| `EnableUser` | 启用已禁用用户，不恢复已撤销的会话 |

- Proto 第 18–26 行声明 UserService 的认证要求及 `iam:global` 的 `manage_users` 授权要求；第 28–55 行列出六个 RPC。
- Proto 第 98–114 行约束用户身份与资料；第 124–129 行声明创建密码字段；第 151–154 行限制更新资料。
- [UserUsecase](../../../../app/iam/service/internal/biz/user.go) 将创建委托给 `AccountUsecase.CreatePendingUser`，更新时检查 etag/不可变字段，状态变更调用领域仓储。
- Admin 消费远程服务接口，不直接调用 IAM 内部 usecase，不连接 IAM 数据表。

## 授权现状

- [admin.fga](../../../../manifests/openfga/admin.fga) 第 3–6 行：`admin` 对象的 `admin` 关系接受 `user`，并派生 `manage_users`。
- [iam.fga](../../../../manifests/openfga/iam.fga) 第 7–9 行：`iam.manage_users` 接受 `service` 主体。
- 两个同名 action 的资源与主体不同；管理员 human token 不等于 Admin 的服务调用凭据。
- 现有模型不代表已完成 Admin 后端鉴权装配、真实管理员关系初始化或部署验收。
- 参考：[IAM 授权](../../../spec/iam/authorization.md)、[OpenFGA 模型运行边界](../../../spec/plateau/infra/openfga.md)。

## 生命周期与暂缓能力

- IAM `SessionUsecase.Resolve` 检查当前用户与登录状态，SCS 本身不检查用户是否被禁用。
- 当前 IAM 会话撤销不构成其他应用本地会话同步注销的证据；Admin 退出体验需单独设计，不能承诺已实现全局退出。
- OAuth Client 当前由静态配置与启动协调管理；动态管理须先由 IAM 提供，不能从 Admin 直接改库。
- Audit 等待重构，首期不把其未来 API 当作已存在的依赖。
- 参考：[IAM 会话](../../../spec/iam/sessions.md)、[IAM OIDC](../../../spec/iam/oidc.md)、[IAM 启动](../../../spec/iam/startup.md)、[Audit](../../../spec/audit/backend.md)。

## Admin 登录参考的实际范围

- `app/example` 目前是 CRUD 参考，不包含 OIDC 登录入口、RP callback、token 或应用 session 装配。证据：[Web router](../../../../app/example/web/src/router/index.ts)、[Web transport](../../../../app/example/web/src/api/transport.ts)、[HTTP server](../../../../app/example/service/internal/server/http.go)、[gRPC server](../../../../app/example/service/internal/server/grpc.go)。
- Example 的 tenant 路径参数是示例 CRUD scope，不能作为 Admin 已认证身份与权限的来源，见 [UserUsecase](../../../../app/example/service/internal/biz/user.go)。
- `app/test/web` 是生成 HTTP client 的构建验证页，不是已完成的 OIDC RP 参考应用。
- IAM Web 的登录及 `/authorize/callback` 属于身份提供方自己的认证流程，不能直接当作 Admin RP 的回调。Admin 仍需设计自身 client 注册、redirect URI、code/token/session 的持有位置以及退出行为。

调研结论：可复用已有框架、生成接口和 IAM 协议能力，当前没有可直接照搬并宣称验收完成的 Admin 登录应用。

## go-wind-admin 对照

本地参考：`/Users/horonlee/projects/go/tx7do/go-wind-admin`，已做只读核对。

| 事实 | 源码依据 |
| --- | --- |
| Admin 本地持有用户身份、状态与租户字段 | [user.go](/Users/horonlee/projects/go/tx7do/go-wind-admin/backend/app/admin/service/internal/data/ent/schema/user.go:18) |
| 本地持有角色、权限、菜单及其关联 | [role.go](/Users/horonlee/projects/go/tx7do/go-wind-admin/backend/app/admin/service/internal/data/ent/schema/role.go:18)、[permission.go](/Users/horonlee/projects/go/tx7do/go-wind-admin/backend/app/admin/service/internal/data/ent/schema/permission.go:17)、[menu.go](/Users/horonlee/projects/go/tx7do/go-wind-admin/backend/app/admin/service/internal/data/ent/schema/menu.go:19) |
| 本地拥有组织、租户等管理领域 | [org_unit.go](/Users/horonlee/projects/go/tx7do/go-wind-admin/backend/app/admin/service/internal/data/ent/schema/org_unit.go:17)、[tenant.go](/Users/horonlee/projects/go/tx7do/go-wind-admin/backend/app/admin/service/internal/data/ent/schema/tenant.go:19) |
| HTTP 管理接口装配本地 repo/service，不只是远程领域接口代理 | [wiring_ent.go](/Users/horonlee/projects/go/tx7do/go-wind-admin/backend/app/admin/service/cmd/server/wiring_ent.go:85)、[rest_server.go](/Users/horonlee/projects/go/tx7do/go-wind-admin/backend/app/admin/service/internal/server/rest_server.go:139) |

按 Plateau 已确定的边界，参考项目的身份、组织、租户等领域不能原样移入平台 Admin；接入时仍将全局身份交给 IAM，并保留各服务自己的领域规则。但用户、角色、权限的管理方式同样适用于平台后台，前述数据归属区别不意味着 Plateau 只能提供一张管理员名单。2026-09-16 用户要求进一步借鉴用户管理，具体证据和产品候选见 [用户与权限管理调研](go-wind-admin-access-management.md)。此比较描述本地参考实现，不限制该上游项目的所有可能部署方式。

## 任务与规范

- 本任务的 `prd.md` 记录用户需求和验收；需求确认后，`design.md` 承接技术方案，`implement.md` 承接执行计划。
- 用户已要求停止在规划期间修改 spec。本任务的产品需求、设计候选和排期只写入任务目录；此前误写入 spec 及全局词汇的未实现规则已撤回，目录迁移、原有开发边界和其他线程的 Vben 实际约定保留。
- 本地 [workflow.md](../../../workflow.md) 的 Phase 3.3 安排实施检查后的 spec 更新；[trellis-update-spec](../../../../.agents/skills/trellis-update-spec/SKILL.md) 也允许将已确定的技术决策或新约定写入规范，并没有“只能从 PRD/design 同步”的绝对限制。本任务按用户要求采用实施验证后再提炼同步的时机，规范不承载阶段目标或待决问题。
- 用户同意进入规划不等于批准实施。当前不运行 `task.py start`，不创建业务应用代码。

此前讨论的长期退出方向是 IAM 退出联动相关应用，业务应用的本地退出只结束自身会话；会话关联、跨设备范围与协议通知尚未设计，且不在本任务交付范围。这是保留在任务中的讨论背景，不是当前已实现契约。
