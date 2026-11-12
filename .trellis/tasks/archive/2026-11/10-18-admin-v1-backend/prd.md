# admin-v1：Admin 后端

## 目标

建立面向平台管理员的 Admin BFF 与服务编排后端（`app/admin/service`），通过 IAM（OAuth 2.1 / OIDC）完成身份认证，维护基于 PostgreSQL 的独立应用会话，以 OpenFGA 执行管理访问与细分操作授权，并通过 IAM 公开 gRPC 契约提供用户管理接口。

## 需求来源与范围

来源为 [admin-v1 主 PRD](../10-04-admin-v1/prd.md)。对应父需求的全部约束和验收语义均适用，以下映射只划分职责，不缩减父需求。

涉及 `app/admin/service` 及其 API 契约、生成产物、运行配置，以及从 `app/iam` 提炼至 `infra/`、`security/` 的可复用会话、OpenFGA 默认主体映射与 OIDC 客户端共享组件。IAM 领域实现已完成归档（见 [10-18-admin-v1-iam](../archive/2026-11/10-18-admin-v1-iam/prd.md)），Vben 页面归 [Admin Web](../10-18-admin-v1-web/prd.md)；本任务不直接读写 IAM 数据表。

| ID | 父需求 | 本任务交付 |
| --- | --- | --- |
| B1 | §1 | 基于 OAuth 2.1（Authorization Code + PKCE S256）接入 IAM 登录、建立独立于 IAM 的 Admin PostgreSQL SCS 会话、单独退出与重新认证；IAM 登出不终止有效 Admin 会话。 |
| B2 | §8 | 以 OpenFGA（`admin:global`）作为运行时权限真相源；根据 `admin.yaml` 的 `bootstrap_admin_email` 定位已有 IAM 身份并幂等注入初始 `admin` 关系种子（Ensure Seed，只增不删），本期不提供管理员资格授予、撤销或更换接口。 |
| B3 | §2–7、§14 | 向 Web 提供单前缀 `/v1/admin/*` 下的用户列表、详情、查询、两步创建与发信编排、补发验证邮件、资料编辑、设密、强退，以及软删除、回收站查询、无限期恢复和显式永久删除接口；完整保留 IAM 的状态、唯一性、首次改密和清理语义。 |
| B4 | §4、§9 | 基于可信认证上下文与 OpenFGA 执行自我保护及同级管理员保护，拒绝封禁或删除自身及其他具备 Admin 管理资格的用户；本期不提供管理员之间的管理操作。 |
| B5 | §12 | 按 `access`、`manage_users`、`expunge_users` 三层动作关系检查人的管理权限；使用单一机密客户端 `admin` 的 `client_credentials` 服务令牌经 `servora/transport/client/grpc.Dialer` 调用 IAM；准确区分并转换领域错误、服务凭据故障与依赖失败，保证超时/取消传播、trace 与脱敏日志。 |
| B6 | §13；配合 §14 | 提供可复现的数据库建库、Ent/Proto/Wire 生成、后端运行及真实 Admin → IAM 集成验证；通过会话接口向 Web 返回当前管理员快照与实时权限码列表。 |

## 验收标准

- [x] AC-B1（B1、B2）：仅经 IAM OIDC（PKCE S256 + `state` + `nonce`）认证且在 OpenFGA `admin:global` 具备对应权限的身份可访问管理接口；`bootstrap_admin_email` 幂等同步初始 `admin` 元组且在 IAM 晚启动时可于登录回调惰性完成；其他已登录 IAM 用户、未认证请求或伪造身份均被拒绝，且无权限用户在回调时不建立 Admin 会话、不陷入重定向死循环。
- [x] AC-B2（B1）：退出 Admin 仅销毁 Admin 本地 SCS 会话，不主动撤销 IAM 会话；IAM 登出后有效 Admin 会话仍可继续访问；Admin 会话失效后须重新经 IAM 认证并校验 OpenFGA 权限。
- [x] AC-B3（B3）：真实 IAM 用户查询及第 3–7 项管理操作均可通过 Admin `/v1/admin/users*` 接口完成；邮箱只读且仅允许修改规定的 7 个资料字段；删除与恢复仅向 IAM 传递资源名，重复删除的 `NOT_FOUND` 如实反馈；已软删除用户可无限期恢复，或由具备 `expunge_users` 权限的管理员显式永久删除，未软删除账号不可直接永久删除。
- [x] AC-B4（B3）：保留首次改密选项的未提供、开启、关闭语义。`CreateUser` 先调用 IAM `CreateUser` 并保留返回的 `User`，成功后自动调用 IAM `SendUserVerificationEmail(name)`；创建失败不发信；发信失败仍返回 HTTP 200、已创建 `User` 及 `verification_email_sent = false`，不回滚或重复创建，并支持通过独立补发接口重发验证邮件。
- [x] AC-B5（B4）：直接请求、伪造操作者字段或绕过页面均不能自封禁、自删除（返回 `SELF_ACTION_FORBIDDEN`/403），也不能封禁或删除具备 `admin` 资格的用户（返回 `TARGET_IS_ADMIN`/403），失败不产生 IAM 变更；种子管理员遵守相同规则。
- [x] AC-B6（B5）：Admin 使用服务自身 `client_credentials` 令牌调用 IAM gRPC，IAM 独立校验 `service:admin` 的 `manage_users` 与 `expunge_users` 权限；Admin 侧 `ExpungeUser` 同时要求操作者具备 `admin:global` 的 `expunge_users` 权限，不能仅凭 `manage_users`、浏览器 Cookie 或自报 metadata 绕过。
- [x] AC-B7（B5）：人的未登录（401）、人的权限不足（403）、自我/管理员保护拒绝（403）、领域冲突（400/404/409）与下游服务凭据或网络依赖故障（503 `DOWNSTREAM_UNAVAILABLE`）严格区分；下游故障绝不清除有效 Admin 会话，也不向浏览器返回 401 误报管理员登录失效；上游请求 deadline/cancel 立即中断下游调用与令牌刷新等待，不增加自动重试，不泄露敏感凭据或内部原始错误。
- [x] AC-B8（B5、B6）：基于真实调用证明 `servora/transport/client/grpc.Dialer`（`WithMiddleware`）与共享安全模块（`HTTPSessionMixin`、`session.NewPostgres`、`openfga.NewDefault`、`security/oidc`）的复用路径，且 `app/iam` 同步切换至提炼后的共享组件并通过回归验证。
- [x] AC-B9（B6）：提供完整的本地配置、生成、启动与验证命令，直接通过真实 Admin HTTP 入口接通真实 IAM、PostgreSQL、OpenFGA 完成登录、会话权限码读取、查询、创建与发信失败分支、变更、自我保护、无权限和下游故障集成测试，配合 Web 子任务完成浏览器端联调。

## 依赖与交付协作

- 消费已归档任务 [IAM 能力补齐](../archive/2026-11/10-18-admin-v1-iam/prd.md) 落地的 `iam.user.v1.UserService` gRPC 契约与 IAM OIDC Provider，真实管理操作验收直连真实 IAM 实现。
- 向 [Admin Web](../10-18-admin-v1-web/prd.md) 提供统一注册在根 `buf.yaml` 下的 `admin.auth.v1` 与 `admin.user.v1` 契约（生成至 `api/gen/{go,ts}/admin/...`）及 `/v1/admin/*` 单一前缀路由（配合 Vben `/auth/login` 选项 1 登录页衔接）。
- 技术架构、共享包提炼、Proto 合同与错误映射矩阵见 [技术设计](design.md)，分阶段实施与验证安排见 [实施计划](implement.md)。

## 约束与不做

- 沿用父任务全部产品约束和排除项；Admin 仅负责 BFF 会话、管理员门禁与跨服务编排，身份状态转换及清理留在 IAM，也不代收用户在 IAM 的主密码。
- 本期不新增管理员资格授予/撤销/列表页面与接口、动态菜单持久化表、租户或组织管理；但 OpenFGA 模型与种子设计保持向多管理员和角色分级的平滑兼容。
- 不做操作审计接入、Back-Channel Logout 跨应用登出联动、自动重试、异步补发队列或总览统计接口。
