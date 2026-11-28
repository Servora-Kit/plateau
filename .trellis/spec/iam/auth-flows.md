# 认证交互生命周期

- `/login/` 统一由服务端 `AuthnService` Auth Flow 状态机驱动：进入页面时若携带 `?flow=<id>` 调用只读 `GetAuthFlow` 恢复步骤与 OIDC 上下文（含活跃客户端 `client_name` 与安全校验后的 `logo_uri`，客户端不存在或已禁用时直接拒绝创建/推进关联 Flow），若携带 `?request_id=<id>` 调用 `CreateAuthFlow({ oidcRequestId })` 并立即通过 `history.replaceState` 规范化为 `/login/?flow=<id>`；浏览器同会话已完成认证时依服务端返回的 `COMPLETED` 与 `continue_url` 跳转 `/authorize/callback?id=...` 或 `/account/`，监听 `pageshow` 防止 bfcache 残留旧表单。
- 第三方非受信客户端（`trusted=false`）在主认证完成后经 `/authorize/callback` 绑定当前浏览器 SCS `auth_flow_owner` 哈希并跳转 `/consent/?request_id=<id>`：页面通过已登录会话调用 `GetConsentView` 展示应用品牌与请求的权限范围，提交 `SubmitConsentDecision` 时同时校验当前登录用户与 `consent_owner_hash`（同意时合并更新 `oauth_consents.granted_scopes`、标记当次请求 `consent_approved=true` 绑定当前 `subject` 后跳回 `/authorize/callback` 避免 `prompt=consent` 死循环；拒绝时销毁授权请求并返回带 RFC 9207 `iss` 的 `access_denied` 回调地址）。用户可在 `/account/applications/` 通过 `ListAuthorizedApps` 查看第三方授权记录，并通过 `RevokeAuthorizedApp` 在单事务内撤销授权同意与该客户端下全部活跃 OAuth 会话。
- 主认证通过后进入 `REQUIRED_ACTION`（如首次 `UPDATE_PASSWORD`）时，展示服务端返回的只读邮箱，仅要求输入新密码与确认密码（无需重复输入初始密码），并提供切换账号（`ResetAuthFlow` 轮换 `flow_id` 保留有效 OIDC 请求）与取消授权（`CancelAuthFlow` 跳转 RP `redirect_uri`）；密码格式或与原密码相同时（400 `INVALID_PASSWORD`）保留原 Flow 可重试，步骤或总时长超时（410 `FLOW_EXPIRED`）与状态漂移（409 `FLOW_INVALIDATED`）自动引导重建或重置。
- 邮件验证与密码重置的一次性 URL fragment 读取后立即用 `history.replaceState` 移除，不放入长期 React 状态、日志或错误文本；提交采用带取消 signal 的单飞请求。
- 独立公开页（如 `/`、`/register/`）通过 `useGuestSession` 区分加载、已认证与未认证状态；已登录账户页区分加载、失败、就绪与未认证状态，未认证时进入登录流程。
- 登出在服务端成功确认后导航，不以本地页面跳转代替会话撤销。
- 使用共享表单的 label、错误关联、`aria-busy` 和禁用状态；已知领域错误给出操作提示，网络、超时、403、5xx 使用安全通用文案，不展示原始错误、凭据或 URL fragment。
