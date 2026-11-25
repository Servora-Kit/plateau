# audit-v1：IAM 与 Admin 审计接入及查询编排

## 目标

为 IAM 与 Admin 接入平台控制面审计事件生产（覆盖 RPC 规则表自动审计与原生 OIDC HTTP 入口无侵入审计），并在 Admin 后端实现面向平台管理员的审计查询 BFF 编排与权限控制。

## 需求来源与范围

来源为 [audit-v1 主 PRD](../11-23-audit-v1/prd.md) §5、§6。涉及 `infra/kafka/`、`security/audit/**`、`app/iam/service/**`、`app/admin/service/**`、`manifests/openfga/{admin.fga,admin.fga.yaml}` 及 `docs/iam-security.md`（`openfga.sh` 与 `justfile` 保持纯 Store/模型职责，不硬编码服务元组）。前置依赖 [底座迁移与共享审计运行时](../11-23-audit-v1-runtime/prd.md) 与 [Audit 微服务与存储重建](../11-23-audit-v1-service/prd.md)。

| ID | 父需求 | 本任务交付 |
| --- | --- | --- |
| P1 | §5 | IAM 控制面审计生产：`iam.user.v1.UserService`（10 个变更 RPC）、`iam.authn.v1.AuthnService.Login`、`iam.account.v1.AccountService`（9 个非只读 RPC）、`iam.session.v1.SessionService.Logout` 启用声明式规则审计，仅在 4 个公开验密入口与本人无返回体入口补充 `SetActor` / `SetResource`（零业务 Proto 字段改动）；依托 `security/audit.HTTPHandler` 在路由外层无侵入包装原生 OIDC 端点（`/authorize/callback`、`/end_session`、`/oauth/token` 失败），保持 `internal/oidc/` 协议实现与构造签名纯净；全部受控接口的 403 拒绝由 `audit.Server` 自动捕获。 |
| P2 | §5 | Admin 控制面审计生产：`admin.user.v1.UserService`（10 个管理写 RPC）与 `admin.auth.v1.AuthService.Logout` 启用声明式规则审计（记录 `human:<admin_uid>` 与浏览器 IP/UA，并与下游 IAM `service:admin` 审计共享同一 `trace_id`）；原生浏览器 OIDC 回调 `handleCallback` 通过 `audit.HTTPHandler` 覆盖全部 4 类退出口（OIDC 交换失败、基础设施鉴权/落库异常、非管理员 `access_denied`、登录成功），`operation` 统一记为 `<METHOD> <PATH>`。 |
| P3 | §6 | Admin 审计查询编排：新增 `admin.audit.v1.AuditService`（HTTP `GET /v1/admin/audit/events`、`GET /v1/admin/audit/{name=events/*}`，直接复用 `audit.event.v1.Event` 零重复结构体，生成合并 OpenAPI 与 `@plateau/api` TS 客户端）；扩展 `admin.fga` 增加 `read_audit: admin` 并纳入 `GetSessionResponse.permissions`；为 IAM OAuth `admin` 客户端补充 `audit` 受众（`audiences: [iam, audit]`），Admin 后端复用 `data.dialer` 持 Service Token 调用 `audit.service` gRPC，并将下游鉴权/连接/存储异常统一转换为 `503` 以保护管理员本地会话。 |

## 验收标准

- [x] AC-P1（P1、P2）：IAM 与 Admin 各受控写操作、登录成功/失败、首次改密、密码重置、会话登出/强制撤销、OIDC 关键节点及 403 拒绝均能产生符合契约的 CloudEvents 审计事件；全部事件载荷与扩展属性不含明文密码、会话 Cookie、JWT、授权码或一次性验证码。
- [x] AC-P2（P1、P2）：Admin 发起跨服务用户管理操作时，Admin 产生的 `human` 事件与 IAM 产生的 `service` 事件保持各自真实 `Actor` 且拥有相同的非空 `trace_id`；常规只读查询（`GetUser`、`ListUsers`、`GetProfile`、`GetSession`）及未登录 OIDC `302 /login` 引导、成功 `client_credentials` 换牌不产生审计噪音。
- [x] AC-P3（P3）：Admin 审计查询 HTTP 接口拒绝未登录会话与非管理员用户（返回 401/403）；当下游 `audit.service` 不可用、存储报错或服务凭据异常时，返回 `503 ADMIN_AUDIT_ERROR_REASON_DOWNSTREAM_UNAVAILABLE` 且不清除管理员有效本地会话。

## 依赖与交付协作

- 前置依赖：`11-23-audit-v1-runtime`、`11-23-audit-v1-service`。
- 向 `11-23-audit-v1-web` 提供 `@plateau/api` 中的 `admin.audit.v1` TypeScript 客户端契约。
- 详细接口与权限映射见 [技术设计](design.md)，执行步骤见 [实施计划](implement.md)。
