# 浏览器会话与撤销

- 登录会话是可撤销认证事件，cookie 与期限由 SCS 管理；从有效服务端登录引用解析身份并投影 `security.Actor`，下游只通过统一认证入口读取，不伪造 context 或仅凭 cookie 声称已认证。
- 拒绝已撤销会话、缺失、软删除、禁用、非活跃、未验证或存在待完成 `required_actions` 的身份；无效凭据与依赖不可用分别处理。交互式认证经 `AuthnService` Auth Flow（`auth_flows` 表）驱动，以 SCS `iam_auth_flow_owner`（SHA-256 存入 `owner_hash`）绑定当前浏览器但零认证权限；进入 `REQUIRED_ACTION` 时不创建 `LoginSession`，若当前浏览器存在旧 `iam_login_id` 则移除该键并轮换 SCS 令牌；全部要求满足进入 `COMPLETED` 时单事务创建携带真实 RFC 8176 `amr` 的 `LoginSession`、轮换 SCS 令牌并写入 `iam_login_id`。
- 撤销在事务中按「先锁 `users`、后锁会话与 `auth_flows`」固定锁序执行，同时失效对应 `login_sessions`、关联 OAuth token session 及目标用户其余 `active` 状态的 `auth_flows`，不仅删除 cookie 或 access token。
- 密码恢复与管理员改密撤销全部登录、OAuth 会话与活动 `auth_flows`；首次改密完成（`CompleteUpdatePassword`）或已认证改密保留新建/当前登录，撤销其他登录、所有 OAuth 会话及其他活动 `auth_flows`；`Expunge` 物理清理 `login_sessions` 与 `auth_flows`。
- Provider 登录退出撤销当前登录及关联 OAuth 会话并销毁 SCS 会话；OIDC `/end_session` 仅撤销客户端关联 OAuth 会话，不等同于 Provider 登录退出。
- `NewHTTPServer` 通过 `security/session.NewFilter` 声明公开入口 origin、`/v1/iam/`（JSON 前缀）、`/authorize/callback`（同源回调路径，仅允许 `GET`）与 `/authorize`、`/end_session`（跨站协议路径）：非安全跨源请求与非 JSON 写请求在装载会话前即被拒绝（403/415），命中浏览器会话路由默认写入 `Cache-Control: no-store`，协议令牌/JWKS/健康检查路径不装载浏览器会话。
- 未接入退出通知时，不得声称已终止业务应用本地会话；跨应用注销必须覆盖各自会话所有者。
