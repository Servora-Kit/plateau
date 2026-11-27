# 认证交互生命周期

- `/login/` 统一由服务端 `AuthnService` Auth Flow 状态机驱动：进入页面时若携带 `?flow=<id>` 调用只读 `GetAuthFlow` 恢复步骤与 OIDC 上下文，若携带 `?request_id=<id>` 调用 `CreateAuthFlow({ oidcRequestId })` 并立即通过 `history.replaceState` 规范化为 `/login/?flow=<id>`；浏览器同会话已完成认证时依服务端返回的 `COMPLETED` 与 `continue_url` 跳转 `/authorize/callback?id=...` 或 `/account/`，监听 `pageshow` 防止 bfcache 残留旧表单。
- 主认证通过后进入 `REQUIRED_ACTION`（如首次 `UPDATE_PASSWORD`）时，展示服务端返回的只读邮箱，仅要求输入新密码与确认密码（无需重复输入初始密码），并提供切换账号（`ResetAuthFlow` 轮换 `flow_id` 保留有效 OIDC 请求）与取消授权（`CancelAuthFlow` 跳转 RP `redirect_uri`）；密码格式或与原密码相同时（400 `INVALID_PASSWORD`）保留原 Flow 可重试，步骤或总时长超时（410 `FLOW_EXPIRED`）与状态漂移（409 `FLOW_INVALIDATED`）自动引导重建或重置。
- 邮件验证与密码重置的一次性 URL fragment 读取后立即用 `history.replaceState` 移除，不放入长期 React 状态、日志或错误文本；提交采用带取消 signal 的单飞请求。
- 独立公开页（如 `/`、`/register/`）通过 `useGuestSession` 区分加载、已认证与未认证状态；已登录账户页区分加载、失败、就绪与未认证状态，未认证时进入登录流程。
- 登出在服务端成功确认后导航，不以本地页面跳转代替会话撤销。
- 使用共享表单的 label、错误关联、`aria-busy` 和禁用状态；已知领域错误给出操作提示，网络、超时、403、5xx 使用安全通用文案，不展示原始错误、凭据或 URL fragment。
