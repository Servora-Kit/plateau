# IAM Auth Flow 认证状态机一期

## Goal

建立 IAM 服务端 Auth Flow 认证状态机骨架（统一托管已通过认证因素、待完成 `required_actions`、关联 OIDC `request_id` 与多时钟 TTL），按主流最佳实践主动重构 `0.x` 认证 API 并对齐相关 RFC / OIDC 标准，将「邮箱密码登录 + 首次登录改密（`UPDATE_PASSWORD`）」作为一期能力落地，消除首次改密第二页重输初始密码与可编辑邮箱的冗余交互，并为二期扩展 MFA（TOTP / 硬件安全密钥 / 恢复码）与 Passkey（含无密码单步 MFA）预留稳定的状态机与 `amr` 透传底座。

## Confirmed Decisions

- **D1（主动重构 `0.x` API 与单轨替代）**：一期直接移除旧 `AuthnService.Login`（`POST /v1/iam/authn/login`）与 `AccountService.ChangeInitialPassword`（`POST /v1/iam/account/change-initial-password`），以 `plateau.iam.authn.v1.AuthnService` 下基于资源的 Auth Flow RPC 单轨替代，不保留旧入口兼容层；同步更新 IAM Web 与 IAM / Admin 测试套件。
- **D2（RFC / OIDC 协议标准对齐范围）**：
  - **RFC 6749 §4.1.2.1 & RFC 9700**：取消关联 OIDC 的 Auth Flow 时原子销毁对应 `oidc_authorization_request` 并按标准回跳 `redirect_uri?error=access_denied&state=<state>&iss=<issuer>`；同步提供流程内“切换账号/重置 Flow”保留 OIDC `request_id` 回到认证起点。RP 侧先校验握手 Cookie、`state` 与 `iss`，再区分用户取消（`ErrAccessDenied`）与握手失效（`ErrHandshakeInvalid`）；Admin 区分 OIDC 取消（`login_cancelled`）与 OpenFGA 无管理员权限（`access_denied`）。
  - **RFC 8176 & OIDC Core 1.0**：使用标准 `amr` 值（一期 `pwd`，预留 `otp`/`hwk`/`swk`/`fpt`/`user`/`pin`/`mfa`），在 `IAMLoginSession` 持久化实际通过的 `amr` 并经 `OIDCAuthorizationRequest`、`OAuthTokenSession` 透传至 ID Token 与刷新令牌，消除硬编码 `"pwd"`。
  - **RFC 9207 & RFC 9700 §4.4**：IAM OP 发现文档声明 `authorization_response_iss_parameter_supported: true`，授权成功与错误重定向响应均附带 `iss` 参数；RP 回调强制校验 `iss` 与配置 `issuer` 一致以防御混合攻击。
  - **RFC 9110 / RFC 9111 & RFC 6265bis**：Auth Flow、OIDC `/authorize`、`/authorize/callback` 与 RP 握手/回调响应统一携带 `Cache-Control: no-store`；Flow 强绑定 `HttpOnly; SameSite=Lax; Secure` 浏览器 SCS 会话并遵循安全/非安全 HTTP 方法语义，认证完成与清除旧登录引用时轮换 SCS 令牌以防御会话固定。
- **D3（Flow 标识承载与浏览器多标签页模型）**：采用「URL 显式 `?flow=<id>` + 浏览器 SCS 归属绑定」资源模型（`/v1/iam/authn/flows/{flow_id}`）。
  - 进入 `/login/`（或 `/authorize/callback` 未登录回跳 `/login?request_id=...`）时由后端创建 Auth Flow，前端以 `history.replaceState` 将 URL 规范化为 `/login/?flow=<id>`，刷新页面凭 `flow_id` 精确恢复当前步骤与关联 OIDC 上下文；同一浏览器多标签页并行登录不同应用各自独立、互不覆盖（同一 OIDC RP 应用的握手 Cookie 仍保持单客户端命名）。
  - 遵循 RFC 9700：`flow_id` 仅作不透明资源定位符而非 Bearer 凭证；服务端将每条 Flow 强绑定至当前浏览器 SCS 会话中的独立归属标识（`iam_auth_flow_owner` 的 SHA-256 哈希），配合响应头 `Cache-Control: no-store` 与已启用的 `Referrer-Policy: no-referrer`，即使 URL 外泄也无法被其他浏览器读取或接管。
- **D4（多时钟静态配置解耦与段内自检）**：保持 `iam`、`oidc`、`oidc_client` 三个既有配置段各自独立，各段启动期自行校验不跨段耦合，运行时 `CreateAuthFlow` 按 `min(now + auth_flow.ttl, oidc_request.expires_time)` 自动收敛；已删除被替代的旧草稿任务 `.trellis/tasks/11-27-iam-initial-password-session/`：
  - `iam.auth_flow.ttl`：`30m`（Auth Flow 总时长，`> 0`）
  - `iam.auth_flow.step_ttl`：`10m`（已验密单步动作窗口，`> 0` 且 `step_ttl <= ttl`）
  - `oidc.auth_request_ttl`：`30m`（IAM OP 授权请求有效期，`> 0`）
  - `oidc_client.handshake_ttl`：`30m`（RP 加密握手 Cookie 有效期，`>= 1s`）

## Confirmed Facts

- **现有认证与首次改密契约及全仓调用方**：
  - `api/protos/plateau/iam/authn/v1/authn.proto:19-59`：`AuthnService.Login`（`POST /v1/iam/authn/login`，`AUTHN_MODE_PUBLIC`）接收 `email` 与 `password`，验密后无 `required_actions` 时建 `IAMLoginSession` 并写 SCS `iam_login_id`；有 `required_actions` 时不创建 `IAMLoginSession`，并在浏览器已有登录引用时销毁 SCS（`app/iam/service/internal/service/authn.go:45-55`）。
  - `api/protos/plateau/iam/account/v1/account.proto:112-119, 264-286`：`AccountService.ChangeInitialPassword`（`POST /v1/iam/account/change-initial-password`，`AUTHN_MODE_PUBLIC`）当前无状态接收 `email`、`current_password`、`new_password`；同文件中 `ACCOUNT_ERROR_REASON_INVALID_STATE` 仍被 `ConfirmPasswordReset` 消费，需保留。
  - 全仓非生成消费者仅限 `app/iam/web/app/login/page.tsx`、`app/iam/service/internal/{biz,service,server}/*_test.go` 与 `app/admin/service/internal/server/{integration_test.go,audit_integration_test.go}`；`buf.yaml` 当前处于 `0.x` 阶段并关闭自动 breaking 拦截。
- **浏览器会话、持久层与并发安全约束**：
  - `app/iam/service/internal/server/http.go:31-36`：`/v1/iam/` 全前缀已接入 `security/session.NewFilter`（`security/session/filter.go:19-99`），非安全方法强制同源 + `application/json` 校验并装载 SCS；`/authorize/callback` 属 `SameOriginPaths`，`/authorize` 与 `/end_session` 属 `CrossOriginPaths`。`app/iam/service/configs/{local,docker}/session.yaml` 使用 `__Host-iam_session`（`Secure; HttpOnly; SameSite=Lax; Path=/`）。
  - `app/iam/service/internal/data/data.go:31-120`：IAM 所有交互认证状态（`http_sessions`、`oidc_authorization_requests`、`oauth_authorization_codes`、`iam_login_sessions`、`email_verification_tokens`、`password_reset_tokens`）均持久化在 PostgreSQL（Ent + SCS PostgreSQL Store，`Schema.Create` 幂等建表），并通过 `lockUser`（`app/iam/service/internal/data/authn.go:53-56`）在单一事务内串行化用户变更与认证提交。
  - `app/iam/service/internal/authn/session.go:14-35`、`app/iam/service/internal/biz/session.go:56-87`：正式会话认证器仅认 SCS `iam_login_id` 对应的未撤销 `IAMLoginSession`，并强校验账号 `ACTIVE`、邮箱已验证且 `required_actions` 为空。
  - `app/iam/service/internal/data/authn.go:215-253`：管理设密 `SetPasswordAndRevokeSessions` 更新 `PasswordAuthenticator.ChangedTime` 且调用 `revokeUserSessions`，但不轮换 `User.etag`；`UpdateStatus(DISABLED)`（`data/user.go:456-460`）、`SoftDelete`（`data/user.go:612-614`）、`ReplacePassword`（`data/authn.go:211`）、`ConsumeAndReplacePassword`（`data/account.go:425`）与 `RevokeAll`（`data/authn.go:404`）均经 `revokeUserSessions` 统一撤销用户会话，`Expunge`（`data/user.go:707-796`）在事务内彻底删除全部用户关联记录。
  - `app/iam/service/internal/biz/account.go:382-388`：后端已用 `password.Compare(newPassword, credential.PasswordHash)` 校验新密码不等于当前密码，无需前端重传明文初始密码。
- **多认证因素、OIDC OP 与 RP 现状**：
  - `app/iam/service/internal/data/schema/authenticator.go` 已具备多态 `authenticators` 主表与 `password_authenticators` 子表。
  - `app/iam/service/internal/data/schema/iam_login_session.go:16-23` 与 `oidc_authorization_request.go:16-36` 当前未持久化 `amr`；`oauth_token_session.go:25` 已存 `amr`，但 `app/iam/service/internal/oidc/op_auth_request.go:20-25` 的 `GetAMR()` 硬编码返回 `[]string{"pwd"}`（`contracts.go:16`），`zitadel/oidc/v3@v3.49.2` 签发 ID Token 时直接消费 `request.GetAMR()`（`pkg/op/token.go:211`）。
  - `app/iam/service/internal/oidc/provider.go:163-244`：`/authorize/callback` 未登录跳转 `/login?request_id=<id>`；`authorizationRequestTTL = 10m` 硬编码在 `contracts.go:11`；`zitadel/oidc/v3@v3.49.2` 的 `oidc.DiscoveryConfiguration` 未内置 `authorization_response_iss_parameter_supported` 字段，`op.AuthResponseURL` 与 `op.TryErrorRedirect` 未附加 `iss` 参数，需由 `IAMProvider` 包装补充。
  - `security/oidc/rp.go:31,110-120,214-230`：`handshakeCookieMaxAge` 硬编码为 `300` 秒；`CompleteLogin` 在校验握手 Cookie 与 `state` 前短接 `error` 参数并统一返回 `ErrHandshakeInvalid`，未校验 `iss` 且未暴露 `ErrAccessDenied`；`app/admin/service/internal/service/auth.go:94-132` 将所有 OIDC 失败跳 `/auth/login?error=login_failed`，`/auth/login?error=access_denied` 专用于 OpenFGA 无管理员权限（`app/admin/web/apps/web-antd/src/views/_core/authentication/login.vue:27-48`）。
  - `app/iam/web/next.config.ts:47` 已全局设置 `Referrer-Policy: no-referrer` 并屏蔽 `/login`、`/authorize/callback` 请求日志；`app/iam/web/hooks/use-one-time-url.ts:42-65` 的 `useOneTimeRequestId` 仅由 `app/iam/web/app/login/page.tsx` 使用，刷新页面会丢失 `request_id`。
  - `api/protos/plateau/iam/conf/v1/config.proto`、`api/protos/plateau/iam/oidc/conf/v1/config.proto`、`api/protos/plateau/security/oidc/v1/config.proto` 均支持 `google.protobuf.Duration` 与 `(servora.conf.v1.field) = {default: "..."}`。

## Requirements

1. **R1（服务端 Auth Flow 状态机与契约重构）**：
   - 在 `api/protos/plateau/iam/authn/v1/authn.proto` 下建立基于资源的 `AuthnService` Auth Flow 契约（`CreateAuthFlow`、`GetAuthFlow`、`SubmitPassword`、`CompleteUpdatePassword`、`ResetAuthFlow`、`CancelAuthFlow`），移除旧 `AuthnService.Login` 与 `api/protos/plateau/iam/account/v1/account.proto` 中的 `ChangeInitialPassword`。
   - 新增 PostgreSQL Ent 表 `auth_flows`（Ent Schema `AuthFlow`，与其余 IAM 核心表命名一致）托管认证中间态（`owner_hash`、状态、`state`、`oidc_request_id`、`user_id`、`user_email`、`user_etag`、`authenticator_id`、`password_authenticator_id`、`password_changed_time`、`completed_amr`、`required_actions`、`expires_time`、`step_expires_time`），严禁存储明文密码或密码哈希。
   - 状态迁移由统一求值逻辑依据「`completed_amr` + 认证策略 + 剩余 `required_actions`」驱动，不将流程写死为「密码 → 2FA」，预留二期接入 TOTP / WebAuthn / 无密码单步 MFA 的扩展点。
   - 浏览器通过 SCS 独立键 `iam_auth_flow_owner` 绑定 Flow 归属；跨浏览器或缺失归属访问任意 `flow_id` 统一返回 `AUTHN_ERROR_REASON_FLOW_NOT_FOUND`（404），不泄露存在性或状态。`CreateAuthFlow` 与 `GetAuthFlow`（在通过归属校验、未达终态且未超总有效期后）检测到当前浏览器已存在有效 `iam_login_id`（OIDC 场景同时要求非空 `amr`）时直接返回 `state=COMPLETED` 与对应 `continue_url`（`/authorize/callback?id=...` 或 `/account/`），且保持零写副作用。
   - 未完成全部要求前严禁创建 `IAMLoginSession` 或写入 SCS `iam_login_id`；进入待完成动作阶段若浏览器已有旧 `iam_login_id`，须立即清除该键并轮换 SCS 令牌；所有 `/v1/iam/authn/` 响应统一携带 `Cache-Control: no-store`。
2. **R2（一期接入：密码认证 + 首次改密 `UPDATE_PASSWORD`）**：
   - 密码校验通过且无待完成动作时，在单一事务内消费 Flow 并创建带 `amr: ["pwd"]` 的 `IAMLoginSession`，服务层轮换 SCS 令牌并写入 `iam_login_id`，返回 `state=COMPLETED`、`continue_url` 与 `user`。
   - 密码校验通过且存在 `UPDATE_PASSWORD` 时，推进 Flow 至 `state=REQUIRED_ACTION`、`next_required_action=UPDATE_PASSWORD`、设置 `step_expires_time = min(now + auth_flow.step_ttl, expires_time)` 并返回只读展示邮箱，不创建 `IAMLoginSession`。
   - `CompleteUpdatePassword` 仅接收 `flow_id` 与 `new_password`；新密码格式不合规或与当前密码相同时返回 `AUTHN_ERROR_REASON_INVALID_PASSWORD`（400）并保留当前 Flow 可重试（不延长 `step_expires_time`）。
   - 账号被禁用、软删除、彻底删除、管理员设密、邮箱重置密码、已登录改密或 `required_actions` / `etag` 并发变化时，统一经 `revokeUserSessions` 级联失效对应用户活跃 Flow，且提交事务内二次核对完整快照，立即拒绝并返回 `AUTHN_ERROR_REASON_FLOW_INVALIDATED`（409）。
   - 超过 `step_expires_time` 或 `expires_time` 时拒绝推进并返回 `AUTHN_ERROR_REASON_FLOW_EXPIRED`（410）。
3. **R3（OIDC 协议标准衔接与 RP / Admin 改造）**：
   - `CreateAuthFlow` 接收可选 `oidc_request_id` 并校验其存在、未完成且未过期，将 `flow.expires_time` 钳制至 `min(now + auth_flow.ttl, oidc_request.expires_time)`；无效 `oidc_request_id` 返回 `AUTHN_ERROR_REASON_OIDC_REQUEST_INVALID`（400）。
   - `IAMLoginSession` 与 `OIDCAuthorizationRequest` 持久化 RFC 8176 `amr`；`CompleteAuthorizationRequest` 从 `IAMLoginSession` 复制 `amr`，`authorizationRequest.GetAMR()` 返回真实 `amr` 并透传至 `OAuthTokenSession`、ID Token 与刷新令牌流程。
   - `CancelAuthFlow` 原子终止 Flow，若绑定活跃 `oidc_request_id` 则同步销毁 `oidc_authorization_request` 并返回 RFC 6749 §4.1.2.1 + RFC 9207 重定向地址（`error=access_denied&state=...&iss=...`）；`ResetAuthFlow` 在有效总时长内轮换 `flow_id` 回到 `AUTHENTICATE` 状态并保留有效 OIDC `request_id`。
   - IAM OP 发现文档声明 `authorization_response_iss_parameter_supported: true`；`/authorize` 与 `/authorize/callback` 发往客户端 `redirect_uri` 的授权成功与错误重定向响应均附加 `iss` 参数并携带 `Cache-Control: no-store`（内部 `/authorize/callback`、`/login` 跳转不附加 `iss`）。
   - `security/oidc/rp.go` 新增 `ErrAccessDenied`；`CompleteLogin` 携带 `Cache-Control: no-store`，严格按「消费并校验握手 Cookie → 校验 `state` → 校验 `iss` 等于配置 `issuer` → 区分 `error=access_denied` 与其他错误 → 换码验签」顺序执行。
   - `app/admin/service/internal/service/auth.go` 将 `ErrAccessDenied` 映射为 `/auth/login?error=login_cancelled`（审计错误码 `OIDC_LOGIN_CANCELLED`），保持 OpenFGA 无管理权限仍映射为 `/auth/login?error=access_denied`；`app/admin/web` 登录页展示区分后的提示文案。
4. **R4（多时钟静态配置对齐）**：
   - 在 `plateau.iam.conf.v1.IAM` 新增可选子消息 `auth_flow`（`ttl` 默认 `30m`、`step_ttl` 默认 `10m`）；在 `plateau.iam.oidc.conf.v1.OIDC` 新增 `auth_request_ttl`（默认 `30m`）；在 `plateau.security.oidc.v1.OIDCClient` 新增 `handshake_ttl`（默认 `30m`）。
   - 各段启动装配期独立自检：`AuthenticationUsecase` 校验 `auth_flow.ttl > 0`、`auth_flow.step_ttl > 0` 且 `step_ttl <= ttl`（`auth_flow` 省略时按默认值补齐）；`OIDCStorage` 校验 `auth_request_ttl > 0`；RP 校验 `handshake_ttl >= 1s` 并用于加密握手 Cookie `MaxAge`。
5. **R5（审计语义、脱敏与 IAM Web / 规范同步）**：
   - `CreateAuthFlow` 与 `GetAuthFlow` 设为 `AUDIT_MODE_DISABLED` 避免页面加载噪声；`SubmitPassword`、`CompleteUpdatePassword`、`ResetAuthFlow`、`CancelAuthFlow` 设为 `AUDIT_MODE_ENABLED`（`AUDIT_CATEGORY_AUTHN`）。
   - 未验密失败及无效/跨浏览器 Flow 操作保持 `anonymous` 且无资源名；已验密阶段与登录成功富化 `human/<user_id>` 与 `users/<user_id>`；`password`、`new_password` 均标记 `(servora.redact.v3.value).string = ""`，审计数据与响应严禁泄露明文密码或会话令牌。
   - IAM Web `/login/` 全面切至 Auth Flow（移除 `/login/` 对 `useGuestSession` 的依赖，统一由 `GetAuthFlow` / `CreateAuthFlow` 处理已登录短接与 OIDC 上下文续接），URL 规范化为 `/login/?flow=<id>`，刷新不丢失步骤与 OIDC 上下文；首次改密页展示只读邮箱，仅提供新密码与确认新密码输入，提供“切换账号”与“取消登录”操作，正确处理错误重试、超时/失效回退与 bfcache 恢复；移除废弃的 `useOneTimeRequestId` 并精简 `useGuestSession` 废弃参数。
   - 同步更新 `.trellis/spec/iam/{sessions,oidc,auth-flows,frontend}.md` 与 `.trellis/spec/admin/{backend,frontend}.md`。

## Out of Scope (Phase 1)

- 具体 MFA / Passkey 认证器实现（TOTP、WebAuthn/Passkey、恢复码、MFA 策略管理 UI）——留作二期增量接入。
- 浏览器外无 Cookie 客户端认证（原生 App PKCE 公共客户端、微信小程序登录）。
- OIDC Consent 授权同意页与 `prompt=login` / `max_age` 强制重认证扩展。
- 动态数据库配置表（一期全部使用静态 Proto + YAML 配置）。
- 普通已登录改密（`ChangePassword`）、忘记密码邮箱重置（`ConfirmPasswordReset`）、公开注册与邮箱验证流程。

## Acceptance Criteria

- **AC-1（契约与生成物一致）**：`api/protos/plateau/iam/authn/v1/authn.proto` 暴露 6 个 Auth Flow RPC 并移除 `Login`；`api/protos/plateau/iam/account/v1/account.proto` 移除 `ChangeInitialPassword`；3 份配置 Proto 落地 4 个 TTL 默认值；`just lint-proto`、`just fmt-proto-check`、`just gen-check`、`just api-ts-check` 全部通过。
- **AC-2（Flow 归属隔离与安全响应头）**：同一浏览器多标签页可并行持有独立 `flow_id` 互不覆盖；跨浏览器或无 SCS Cookie 访问他人 `flow_id` 全部返回 404（`AUTHN_ERROR_REASON_FLOW_NOT_FOUND`）且不写入 Cookie；所有 `/v1/iam/authn/*`、`/authorize`、`/authorize/callback`、Admin `/v1/admin/auth/{login,callback}` 响应均带 `Cache-Control: no-store`。
- **AC-3（未完成 Flow 零认证权限）**：密码校验通过但存在 `UPDATE_PASSWORD` 时不创建 `IAMLoginSession`、不写入 SCS `iam_login_id`（若原浏览器有登录引用则清除并轮换 SCS 令牌）；持未完成 Flow 调用受保护 API 返回 401，访问 `/authorize/callback` 仍按未登录重定向至 `/login?request_id=...`。
- **AC-4（密码直登与首次改密闭环）**：
  - 无 `required_actions` 的用户 `SubmitPassword` 一步完成，轮换 SCS 令牌并建立含 `amr=["pwd"]` 的 `IAMLoginSession`，返回 `state=COMPLETED` 与正确 `continue_url`（OIDC 场景为 `/authorize/callback?id=...`，独立登录为 `/account/`）。
  - 含 `UPDATE_PASSWORD` 的用户 `SubmitPassword` 进入 `state=REQUIRED_ACTION` 并返回只读邮箱；`CompleteUpdatePassword` 仅传 `flow_id` 与 `new_password` 即可完成改密、清空 `UPDATE_PASSWORD`、撤销其他旧会话、轮换 SCS 令牌并建立 `IAMLoginSession`。
  - 新密码格式不合规或等于当前密码返回 400（`AUTHN_ERROR_REASON_INVALID_PASSWORD`），原 Flow 保持有效可继续重试直至成功。
- **AC-5（并发失效与多时钟超时）**：
  - 在 `UPDATE_PASSWORD` 等待期间发生管理员设密、禁用账号、软删除、彻底删除、邮箱重置密码或 `required_actions` 变更时，后续提交立即返回 409（`AUTHN_ERROR_REASON_FLOW_INVALIDATED`）且不得修改密码或创建会话。
  - 超过 `auth_flow.step_ttl` 或 `auth_flow.ttl` 返回 410（`AUTHN_ERROR_REASON_FLOW_EXPIRED`）；启动配置违反段内时长约束（`step_ttl <= ttl`、`ttl > 0`、`step_ttl > 0`、`auth_request_ttl > 0`、`handshake_ttl >= 1s`）时拒绝启动；并发重复提交 `CompleteUpdatePassword` 至多一次成功。
- **AC-6（OIDC RFC 8176 / 9207 / 6749 全链路）**：
  - `/.well-known/openid-configuration` 返回 `"authorization_response_iss_parameter_supported": true`。
  - `/authorize` 错误重定向与 `/authorize/callback` 成功/错误重定向至客户端 `redirect_uri` 时均带 `iss=<issuer>`，内部跳转不带 `iss`。
  - `CancelAuthFlow` 销毁绑定 `oidc_authorization_request` 并返回 `redirect_uri?error=access_denied&state=<state>&iss=<issuer>`；`ResetAuthFlow` 轮换 `flow_id` 回到 `AUTHENTICATE` 并保留仍有效的 OIDC `request_id`。
  - 经由登录与改密后的 OIDC 令牌换取与刷新令牌流程，ID Token `amr` 与 `OAuthTokenSession.amr` 均来自 `IAMLoginSession.amr`（不再硬编码）。
- **AC-7（RP 与 Admin 回调处理）**：`security/oidc.RelyingParty` 按配置 `handshake_ttl` 签发加密 Cookie；回调先校验 Cookie 与 `state`，再强制校验 `iss` 匹配配置 `issuer`（缺失/重复/不匹配均返回 `ErrHandshakeInvalid`），校验通过后对 `error=access_denied` 返回 `ErrAccessDenied`；Admin 回调将 `ErrAccessDenied` 重定向至 `/auth/login?error=login_cancelled`，OpenFGA 拒绝仍定向至 `/auth/login?error=access_denied`，Admin Web 登录页准确区分显示两类提示。
- **AC-8（审计与脱敏）**：`CreateAuthFlow` / `GetAuthFlow` 不产生审计事件；密码校验失败与无效/跨浏览器 Flow 操作保持 `anonymous` 且无资源名；已验密阶段操作与完成登录富化 `human/<user_id>` 与 `users/<user_id>`；审计事件 JSON 中零泄露明文密码与会话令牌。
- **AC-9（IAM Web 交互与规范文档）**：`/login/` 页面自动规范化为 `/login/?flow=<id>`，刷新保持当前步骤与 OIDC 上下文；同浏览器另一标签页先完成登录后，在 `/login/?flow=<id>` 标签页刷新或触发 `pageshow` 能准确续接该 Flow 托管的 OIDC `continue_url`（或独立场景跳 `/account/`），不因 URL 无 `request_id` 盲跳 `/account/`；首次改密页展示只读邮箱且无初始密码输入框，支持“切换账号”与“取消登录”；`just test`、`just lint`、`just web::build`、`just web::lint`、`just web::test` 与 Admin Web 构建检查通过，相关 `.trellis/spec/` 文档同步更新。

## Risks & Deferred Items

- **`amr` 非空防御不变量**：`iam_login_sessions.amr` 与 `oidc_authorization_requests.amr` 使用 PostgreSQL `jsonb`（默认 `"[]"`）经 Ent `Schema.Create` 自动同步；建登录会话必写入非空 `amr`，`CompleteAuthorizationRequest` 显式拒绝空 `amr` 而不回退伪造 `"pwd"`，`/authorize/callback` 及 OIDC 短接路径遇空 `amr` 拒绝放行并引导重新认证。
- **过期 `auth_flows` 清理**：一期与 `oidc_authorization_requests` 一致通过 `expires_time` 索引及查询条件隔离过期记录（并在用户 `Expunge` 时物理删除）；周期性后台清扫任务待平台统一清理机制建立后再合并落地。
- **同 RP 应用并行多标签页握手 Cookie**：按 D3 保持单客户端握手 Cookie 命名（`__Host-<client_id>_oidc_handshake`），同浏览器并行登录同一 RP 应用的不同标签页会覆盖同一 RP 握手 Cookie，属已知现行约定；并行登录不同 RP 应用互不影响。
