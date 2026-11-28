# 动态 OAuth Client 管理

## Goal

在 Plateau 平台建立支持动态注册与全生命周期管理的 OAuth 2.0 / OIDC 客户端体系，打通 `oidc.yaml` 静态引导客户端（如 `admin`）与数据库动态客户端的双源安全共存，支持机密与公共客户端（Web / SPA / Native）、参考 Logto 的多密钥零停机轮换、第一方免审与第三方 `/consent/` 授权同意记忆闭环、Google Account 风格的个人中心已授权应用管理，以及 OpenFGA 显式授权和 Admin 控制台可视化管理，使下游业务应用（如 `app/blog` 及外部集成应用）接入 IAM 时无需修改静态配置或重启 IAM 服务。

## Confirmed Decisions

- **D1（全栈闭环交付）**：一期完整交付 `api/protos/plateau/iam/client/v1`（IAM gRPC 领域服务）+ Consent / 已授权应用自助契约 + OpenFGA 权限模型与静态元组 + `api/protos/plateau/admin/client/v1`（Admin BFF HTTP/gRPC 接口）+ `app/iam/web` 与 `app/admin/web` 前端页面。
- **D2（双源分级共存模型）**：`oauth_clients` 按 `source` 区分 `STATIC` 与 `DYNAMIC`：
  - `oidc.yaml` 的 `clients` 支持配置 `0..N` 个客户端（默认保留 `admin`），启动时统一按 `STATIC` 幂等对账入库，并仅清理已从 `oidc.yaml` 移除的 `STATIC` 客户端，绝不误删 `DYNAMIC` 客户端；若 `oidc.yaml` 中的 `client_id` 与库内已有 `DYNAMIC` 客户端重名则拒绝启动。
  - `STATIC` 客户端在 IAM 查询接口与 Admin Web 中正常展示（带静态托管标识），但禁止在线修改、增删密钥、禁用或删除，防止重启后被配置文件静默回退；`DYNAMIC` 客户端完全由 API 与数据库托管。
- **D3（客户端应用类型与令牌端点认证方式）**：
  - 应用类型（`application_type`）支持 `WEB`（服务端 Web / BFF / 机器服务）、`USER_AGENT`（纯前端 SPA）与 `NATIVE`（桌面/移动原生应用或 CLI，遵循 RFC 8252 支持回环动态端口与自定义 URI Scheme）。
  - 令牌端点认证方式（`token_endpoint_auth_method`）支持 `CLIENT_SECRET_BASIC`、`CLIENT_SECRET_POST`（机密客户端）与 `NONE`（无密钥公共客户端，强制依赖全局已启用的 PKCE S256，且禁止授予 `client_credentials`）；OP 发现文档同步声明上述认证方式。
- **D4（第一方免审、第三方 Consent 记忆与 Google Account 风格自助撤销）**：
  - 第一方受信客户端（`trusted = true`）跳过授权同意页直接签发授权码。
  - 非受信第三方客户端（`trusted = false`）在已登录态经过 `/authorize/callback` 时检查 `oauth_consents` 记忆表：若尚未记录授权、请求包含未批准的新增 scope 或显式携带 `prompt=consent`，则重定向至 IAM Web `/consent/?request_id=<id>`；用户同意后持久化合并后的已批准 `scopes` 并继续签发授权码，拒绝则销毁授权请求并按 RFC 6749 / RFC 9207 回跳 `redirect_uri?error=access_denied&state=...&iss=...`。
  - IAM Web `/account/` 整体采用参考 Google 账号中心（`https://myaccount.google.com/`）的布局（顶栏账户信息 + 左侧圆角胶囊导航栏「个人信息 / 安全性与登录 / 第三方应用与服务」+ 右侧居中分组圆角卡片与分割行列表），新增 `/account/applications/` 供用户查看已授权第三方应用并一键移除访问权限（同步撤销 `oauth_consents` 与关联活跃 OAuth 会话）。
- **D5（`client_id` 标识规则与展示元数据）**：
  - 创建动态客户端支持**可选自定义语义化 `client_id`**（小写字母、数字与连字符 slug `^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$`，便于在 OpenFGA `service:<client_id>` 与审计记录中直观识别），留空则由服务端自动生成 `cli_` 前缀随机短 ID；创建后 `client_id` 永久不可变（`IMMUTABLE`），资源名为 `oauthClients/{oauth_client}`。
  - 维护轻量展示与协议元数据：`display_name`（必填；`oidc.yaml` 静态客户端可选配置，未填回退 `client_id`）、`description`（可选说明）、`logo_uri`（可选图标 URL，与现有 `User.picture` 统一存储标准公开 URL 字符串，前端无图时以首字母/默认图标徽章兜底，不造过渡性数据库二进制存图）、`post_logout_redirect_uris`（对接 `/end_session` 登出跳回白名单）。
  - `AuthFlowOidcContext` 与 Consent 接口同步输出客户端 `display_name` 与 `logo_uri`，供 `/login/` 与 `/consent/` 展示应用品牌信息。
- **D6（精简多密钥与零停机轮换模型）**：
  - 机密客户端密钥统一由服务端 CSPRNG 生成高熵随机串（带可读前缀如 `pcs_`），仅在创建机密客户端或新增密钥接口响应中**一次性返回明文**；数据库 `oauth_client_secrets` 仅持久化 SHA-256 单向哈希、展示掩码前缀、备注名称、可选过期时间与创建/最近使用时间。
  - 每个动态机密客户端最多同时持有 5 把密钥，支持“先新增密钥 → 部署下游服务 → 再删除旧密钥（或设置过期时间）”的零停机轮换；机密客户端禁止删空最后一把有效密钥；增删密钥不撤销已建立的人类 `oauth_token_sessions`。
- **D7（轻量启停 + 直接删除模型与单一 `manage_clients` 权限）**：
  - 支持 `ACTIVE` / `DISABLED` 启停与二次确认后直接物理删除（不设回收站软删除与 `Expunge` 两段式流程）。
  - 禁用或删除客户端时，在同一事务内原子终止在途 `oidc_authorization_requests`（及关联未完成 `auth_flows`）、失效未消费 `oauth_authorization_codes`、撤销该客户端全部活跃 `oauth_token_sessions` / `oauth_refresh_tokens` / `oauth_access_tokens`；物理删除时同步清理该客户端的 `oauth_client_secrets` 与 `oauth_consents`。
  - OpenFGA 在 `iam.fga` 新增 `define manage_clients: [service]`（静态元组授予 `service:admin`），在 `admin.fga` 新增 `define manage_clients: admin`。

## Industry Reference & Simplification (Logto / Casdoor 对照与精简)

| 能力维度 | Logto (`Application`) | Casdoor (`Application`) | Plateau 定稿方案 |
| --- | --- | --- | --- |
| 应用形态与授权类型 | `Traditional` / `SPA` / `Native` / `M2M`；仅保留安全标准流 | 混杂标准流及已废弃的 `implicit`、`password`（ROPC）、`device_code`、SAML | 对齐 Logto：支持 `WEB` / `USER_AGENT` / `NATIVE` 与 `authorization_code`、`refresh_token`、`client_credentials`；剔除不安全的 `implicit` 与 `password` |
| 登录体验与界面耦合 | 全局统一 Sign-in Experience，应用仅持轻量品牌字段 | 每个应用耦合登录方式开关、`enableSignUp` 与自定义 CSS | 对齐 Logto 精简：保持 IAM 全局统一登录页，客户端仅维护 `display_name`、`description`、`logo_uri` |
| 授权同意页（Consent） | 第一方免审；第三方弹 Consent 并记忆已批准 scopes | `consentPolicy` 支持 `skip` / `once` / `always` | 第一方（`trusted=true`）免审直跳；第三方（`trusted=false`）采用 `once` 记忆策略 + Google 风格个人中心自助撤销 |
| 回调与登出白名单 | `redirectUris` + `postLogoutRedirectUris` + `corsAllowedOrigins` + `backchannelLogoutUri` | 仅 `redirectUris` | 取核心子集：支持精确匹配的 `redirect_uris` 与 `post_logout_redirect_uris`，省略复杂的 Back-channel Logout |
| 客户端密钥存储与轮换 | `/applications/{id}/secrets` 多条命名密钥 + 过期时间 | 单一 `clientSecret` 明文存库回显 | 严禁明文存库；采用单向哈希 + 单客户端最多 5 把命名密钥支持零停机轮换 |
| 生命周期与删除模型 | 直接删除（弹窗二次确认），无回收站 | 直接删除，无回收站 | 轻量 `ACTIVE` / `DISABLED` 启停 + 二次确认直接物理删除并级联撤销活跃会话 |
| 令牌 TTL 覆写 | 少量 `customClientMetadata` 覆写 | 每个应用配置 `expireInHours` | 精简：统一遵循 IAM 全局令牌 TTL，不在单客户端暴露碎片化过期时长配置 |

## Confirmed Facts

- **现有存储与 OIDC 协议层实现**：
  - `app/iam/service/internal/data/schema/oauth_client.go:13-32`：已存在 `oauth_clients` 表，现有单字段 `secret_hash` 需平滑演进至 `oauth_client_secrets` 子表（公共客户端无密钥记录，机密客户端 `1..5` 条记录）。
  - `app/iam/service/internal/oidc/initializer.go:36-64,137-219`：当前启动期强制要求至少 1 个静态客户端，并执行 `OAuthClient.Delete().Where(oauthclient.IDNotIn(configuredIDs...))` 删除非静态记录；已有 `redirect_uris`、`allowed_grant_types`、`allowed_scopes`、`audiences` 校验逻辑可提炼下沉至 `biz` 层供静态初始化与动态 CRUD 共享。
  - `app/iam/service/internal/oidc/op_op_storage.go:18-41`、`op_client.go:13-67`、`provider.go:66-75,108-248`、`service_token.go:20-42`：运行时按请求实时查询 `oauth_clients`；当前 `AuthMethod()`、`TokenEndpointAuthMethodsSupported` 硬编码为 `client_secret_basic`，`PostLogoutRedirectURIs()` 硬编码为 `nil`，`ApplicationType()` 硬编码为 `op.ApplicationTypeWeb`；`validateAuthorizationRequest` 已全局强制 `PKCE S256`、`state` 与 `nonce`。
  - `zitadel/oidc/v3@v3.49.2` 行为核实（`pkg/op/auth_request.go:337-380`、`token_code.go:101-123`、`token_revocation.go:104-139`、`session.go:115-133`）：`op.Config.AuthMethodPost = true` 控制表单传密钥支持；`ApplicationTypeNative` 原生支持 RFC 8252 回环动态端口与自定义 URI Scheme；`ApplicationTypeUserAgent` 对 `http://` 回环回调需配合受控 `DevMode()` 放行（注册校验阶段严格拦截非回环 `http://`）。
  - `app/iam/service/internal/data/oauth.go:323-333`：已具备 `OAuthRepo.RevokeClient(ctx, userID, clientID, now)`，可直接支撑用户自助撤销单个第三方应用授权。
- **管理服务、权限与前端现状**：
  - `api/protos/plateau/iam/user/v1/user.proto`、`app/iam/service/internal/service/user.go:17-65`、`api/protos/plateau/admin/user/v1/user.proto`：标准 CRUD 消费 `protoc-gen-plateau-crud` 生成的 `CRUDDescriptor`、`ResourcePlan` 与 `ListPreparer`。
  - `manifests/openfga/{iam,admin}.fga` 与 `manifests/openfga/tuples/platform-services.yaml`：平台权限与 `service:admin` 静态关系元组集中维护。
  - `app/iam/web/components/account-shell.tsx:9-75`：现有个人中心仅含「个人资料 / 账户安全」双栏导航，无顶栏身份区与已授权应用入口。

## Requirements

1. **R1（双源客户端模型、多密钥存储与静态启动对账）**：
   - 升级 `OAuthClient` Ent Schema（`oauth_clients`）：移除内联必填 `secret_hash`，维护 `id`、`source`（`static` | `dynamic`）、`status`（`active` | `disabled`）、`display_name`、`description`、`logo_uri`、`application_type`（`web` | `user_agent` | `native`）、`token_endpoint_auth_method`（`client_secret_basic` | `client_secret_post` | `none`）、`redirect_uris`、`post_logout_redirect_uris`、`allowed_grant_types`、`allowed_response_types`、`allowed_scopes`、`audiences`、`trusted`、`etag`、`create_time`、`update_time`。
   - 新增 `OAuthClientSecret` Ent Schema（`oauth_client_secrets`）：存储 `id`（UUIDv7）、`client_id`、`name`（备注名称）、`secret_prefix`（脱敏展示前缀）、`secret_hash`（SHA-256，`Sensitive`）、`expires_time`（可选）、`last_used_time`（可选）、`create_time`。
   - 向后兼容扩展 `api/protos/plateau/iam/oidc/conf/v1/config.proto`：允许 `oidc.clients` 为空列表；静态 `OAuthClient` 配置新增可选 `display_name`、`description`、`logo_uri`、`post_logout_redirect_uris`、`application_type`、`token_endpoint_auth_method`（省略时默认 `WEB` + `CLIENT_SECRET_BASIC`，公共客户端无需 `client_secret`）。
   - 重构 `OIDCInitializer`：仅对账 `source = static` 客户端及其静态密钥记录，仅删除已从 `oidc.yaml` 移除的 `static` 客户端（同步执行级联令牌清理），遇同名 `dynamic` 客户端报错拒绝启动。
2. **R2（IAM `OAuthClientService` 领域校验、多密钥轮换与生命周期级联撤销）**：
   - 新增 `api/protos/plateau/iam/client/v1/client.proto`（仅注册到 IAM gRPC，`AUTHN_MODE_REQUIRED` + OpenFGA `manage_clients` on `iam:global` + `AUDIT_CATEGORY_RESOURCE`），提供：`CreateOAuthClient`、`GetOAuthClient`、`ListOAuthClients`（接入 `crud/` 分页、过滤与排序）、`UpdateOAuthClient`（`update_mask` + `etag` 并发保护）、`DisableOAuthClient`、`EnableOAuthClient`、`DeleteOAuthClient`、`CreateOAuthClientSecret`、`DeleteOAuthClientSecret`。
   - 统一领域校验规则（静态初始化与动态 API 共用）：
     - `token_endpoint_auth_method == NONE` 时禁止包含 `client_credentials` 且不生成密钥；机密客户端创建时自动生成首把默认密钥并在 `CreateOAuthClientResponse.client_secret` 一次性返回明文。
     - `USER_AGENT` 仅允许机密性为 `NONE`；`WEB` 与 `USER_AGENT` 的回调及登出跳回地址要求 `https://` 或 RFC 6761 回环 `http://`（无通配符、无 `#fragment`）；`NATIVE` 遵循 RFC 8252 允许自定义 URI Scheme（如 `myapp://callback`）、回环 `http://` 或 `https://`。
     - 包含 `authorization_code` 时必填 `redirect_uris` 且 `allowed_scopes` 必含 `openid`；包含 `refresh_token` 时必含 `authorization_code`；包含 `client_credentials` 时必填 `audiences` 且认证方式不得为 `NONE`。
   - 密钥管理约束：仅 `DYNAMIC` 机密客户端允许调用 `CreateOAuthClientSecret`（上限 5 把）与 `DeleteOAuthClientSecret`（若删除后剩余未过期密钥数为 0 则返回 409 `INVALID_STATE` 拒绝删除）；对 `STATIC` 客户端执行任何写操作统一返回 403/409 领域错误（`STATIC_CLIENT_IMMUTABLE`）。
   - 禁用与删除级联：禁用置 `status = disabled`，物理删除移除客户端、`oauth_client_secrets` 与 `oauth_consents`；两者均在同一事务内删除未完成 `oidc_authorization_requests`、将绑定这些请求的活跃 `auth_flows` 置为失效、消费/失效未用 `oauth_authorization_codes`，并撤销该 `client_id` 的全部活跃 `oauth_token_sessions`、`oauth_refresh_tokens` 与 `oauth_access_tokens`。
3. **R3（OIDC OP 协议层动态适配与认证方式隔离）**：
   - `GetClientByClientID`、`AuthorizeClientIDSecret`、`ClientCredentials`、`/authorize/callback` 授权完成及刷新令牌换取均强校验客户端存在且 `status == active`（已禁用客户端立即拒绝签发新授权、换码、刷新及自省）。
   - `op_client.go` 动态映射 `ApplicationType()`、`AuthMethod()`、`PostLogoutRedirectURIs()`，并在回环 `http://` 回调场景安全适配 `USER_AGENT` 校验；`op.Config` 开启 `AuthMethodPost: true`，同时在存储/校验层严格校验实际请求认证方式与客户端注册的 `token_endpoint_auth_method` 一致（防止配置为 `CLIENT_SECRET_BASIC` 的客户端退化接受表单提交，或机密客户端走无密钥路径），多密钥常量时间遍历比对未过期 `oauth_client_secrets`。
   - 发现文档 `/.well-known/openid-configuration` 声明 `token_endpoint_auth_methods_supported: ["client_secret_basic", "client_secret_post", "none"]`（Introspection / Revocation 同步更新）。
4. **R4（第三方 Consent 授权同意闭环与 Google 风格个人中心）**：
   - 新增 `OAuthConsent` Ent Schema（`oauth_consents`）：`(user_id, client_id)` 唯一索引，持久化 `granted_scopes`、`create_time`、`update_time`；用户 `Expunge` 与客户端物理删除时同步清理。
   - `/authorize/callback` 在验证浏览器活跃登录会话后，若客户端 `trusted == false` 且（显式请求 `prompt=consent` 或 `oauth_consents` 尚未覆盖全部请求 `scopes`），重定向至 `/consent/?request_id=<id>`（内部跳转不注入 `iss`）。
   - 在 `api/protos/plateau/iam/authn/v1/authn.proto` 扩展 `AuthFlowOidcContext`（补充 `client_name`、`logo_uri`）及已登录 Consent RPC（`GetConsentView`、`SubmitConsentDecision`：挂载于 `/v1/iam/authn/consents/{request_id}`，`AUTHN_MODE_REQUIRED`）：同意时幂等并集更新 `oauth_consents` 并返回 `/authorize/callback?id=<id>`，拒绝时销毁 `oidc_authorization_request` 并返回 RFC 6749 / RFC 9207 `access_denied` 重定向 URL。
   - 在 `api/protos/plateau/iam/account/v1/account.proto` 新增 `ListAuthorizedApps`（`GET /v1/iam/account/applications`）与 `RevokeAuthorizedApp`（`POST /v1/iam/account/applications/{client_id}:revoke`），同一事务删除 `oauth_consents` 并调用 `RevokeClient` 撤销该客户端活跃 OAuth 会话。
   - IAM Web 升级：
     - `/login/` 在 OIDC 登录场景展示应用图标与 `client_name`。
     - 新增 `/consent/` 授权同意页，展示应用品牌、当前账号邮箱、请求的权限说明列表及「同意授权 / 取消」操作。
     - 重构 `AccountShell` 及 `/account/`、`/account/security/` 并新增 `/account/applications/`（「第三方应用与服务」），界面布局对齐 `https://myaccount.google.com/`（顶栏品牌与当前邮箱徽章、左侧圆角胶囊导航栏、右侧分组圆角卡片与行式详情/移除授权交互）。
5. **R5（OpenFGA、Admin BFF 与 Admin Web 客户端控制台）**：
   - 更新 `manifests/openfga/iam.fga`（新增 `manage_clients: [service]`）、`manifests/openfga/admin.fga`（新增 `manage_clients: admin`）与 `manifests/openfga/tuples/platform-services.yaml`（新增 `service:admin manage_clients iam:global`），补充对应 `.fga.yaml` 模型测试。
   - 新增 `api/protos/plateau/admin/client/v1/client.proto`（暴露 `/v1/admin/oauth-clients` REST + gRPC，`manage_clients` on `admin:global`），在 `app/admin/service` 实现对 IAM `OAuthClientService` 的编排调用与错误映射。
   - 在 `app/admin/web` 新增「OAuth 客户端管理」路由与视图：支持分页搜索与来源/类型/状态筛选；新建客户端抽屉/弹窗按预设场景（Web 应用、SPA 单页应用、原生应用、M2M 机器服务）智能联动默认授权类型与认证方法；创建机密客户端或新增密钥成功后弹出一次性明文复制提示框；详情抽屉支持编辑配置、启停、多密钥列表与零停机轮换、`STATIC` 客户端只读锁定提示，以及输入 `client_id` 二次确认物理删除。
6. **R6（安全审计、敏感脱敏与规范文档）**：
   - 所有一次性返回的明文密钥字段（`client_secret`、`secret_value`）必须标注 `(servora.redact.v3.value).string = ""`；读接口仅返回 `OAuthClientSecret` 脱敏元数据（含 `secret_prefix`），严禁日志、响应或审计载荷出现明文密钥或 `secret_hash`。
   - `GetOAuthClient`、`ListOAuthClients`、`GetConsentView`、`ListAuthorizedApps` 设为 `AUDIT_MODE_DISABLED`；客户端增删改、启停、密钥增删设为 `AUDIT_CATEGORY_RESOURCE`；`SubmitConsentDecision` 与 `RevokeAuthorizedApp` 记录审计事件。
   - 同步更新 `docs/iam.md` 与 `.trellis/spec/{iam,admin}/` 相关规范文档。

## Out of Scope

- 上游外部/内置 IdP 联邦登录（如 GitHub、Google、企业 OIDC 登录与身份绑定）——留作下一独立任务。
- 已淘汰或不安全的授权类型（`implicit`、`password` ROPC）及设备授权流（`device_code`）、`private_key_jwt`、SAML 2.0。
- OIDC Back-channel / Front-channel 跨应用广播登出通知与客户端级自定义令牌 TTL 覆写。
- 平台对象存储（如 RustFS / MinIO / S3）与统一图片文件上传（用户头像 `User.picture` + 客户端图标 `logo_uri` + 业务配图）——留作独立后续基础设施任务落地，届时上传后直接回填 URL 至现有字段，无需改动表结构或 OIDC 契约。

## Acceptance Criteria

- **AC-1（契约、OpenFGA 与生成物门禁）**：新增 `plateau.iam.client.v1`、`plateau.admin.client.v1` 并扩展 `authn.proto`、`account.proto`、`oidc/conf/v1/config.proto`；OpenFGA 模型测试 `just openfga-model-test`、`just lint-proto`、`just fmt-proto-check`、`just gen-check`、`just api-ts-check` 全部通过。
- **AC-2（双源共存与静态保护）**：`oidc.yaml` 为空列表或含 `admin` 均可正常启动；`OIDCInitializer` 对账更新/清理 `STATIC` 客户端时保留全部 `DYNAMIC` 客户端；`oidc.yaml` 与已有 `DYNAMIC` `client_id` 冲突时拒绝启动；通过 API 修改、轮换密钥、禁用或删除 `STATIC` 客户端均被拒绝并返回明确领域错误。
- **AC-3（多形态客户端与认证方式隔离）**：动态创建的 `WEB`（`client_secret_basic` / `client_secret_post`）、`USER_AGENT`（`none` + PKCE S256）、`NATIVE`（`none` + 自定义 Scheme / 回环动态端口）、`M2M`（`client_credentials` + `audiences`）客户端均可走通完整 OIDC/OAuth 协议流；客户端使用非注册的认证方式（如 `basic` 客户端改用表单传密钥、机密客户端不传密钥）被严格拒绝；发现文档准确声明三种认证方式。
- **AC-4（多密钥零停机轮换）**：机密客户端创建时一次性返回高熵明文密钥，后续查询仅返回掩码前缀；新增第 2 把密钥后新旧两把密钥均可成功换取令牌，删除旧密钥后旧密钥立即失效、新密钥持续可用且已有用户 `oauth_token_sessions` 保持有效；超过 5 把上限或尝试删除最后一把未过期密钥时返回 409 拒绝。
- **AC-5（启停与物理删除级联撤销）**：禁用或删除动态客户端后，该客户端的 `/authorize`、`/oauth/token`（含授权码、刷新令牌、客户端凭据）、`/oauth/introspect` 立即拒绝，在途 `oidc_authorization_requests`、关联未完成 `auth_flows`、未消费授权码与活跃 OAuth 会话在同一事务内全部失效；删除客户端额外清空 `oauth_client_secrets` 与 `oauth_consents`。
- **AC-6（第一方免审与第三方 Consent 闭环）**：`trusted=true` 客户端登录后直接签发 `code`；`trusted=false` 客户端首次登录跳转 `/consent/?request_id=<id>`，拒绝时按 RFC 6749/9207 回跳 `error=access_denied&state=...&iss=...`，同意后写入 `oauth_consents` 并签发 `code`；二次同范围登录自动免审放行，请求新增未批准 scope 或携带 `prompt=consent` 时重新弹出同意页。
- **AC-7（IAM Web 登录品牌、Consent 页与 Google 风格个人中心）**：`/login/` 在 OIDC 场景显示应用 `display_name` 与图标；`/consent/` 正常完成同意与取消交互；`/account/` 升级为参考 `https://myaccount.google.com/` 的顶栏 + 左侧圆角胶囊导航 + 右侧分组卡片布局，`/account/applications/` 可列出已授权第三方应用并一键撤销授权及活跃令牌会话。
- **AC-8（Admin BFF 与 Admin Web 控制台）**：具备 `manage_clients` 权限的管理员可在 Admin Web 完成客户端分页检索、分场景创建、一次性密钥复制、详情编辑、多密钥增删、启停与输入 `client_id` 确认删除，`STATIC` 客户端界面只读锁死；无权限用户返回 403 并记录拒绝审计。
- **AC-9（脱敏、审计与全仓门禁）**：响应日志与 CloudEvents 审计事件零明文密钥或哈希泄露；`GOWORK=off CGO_ENABLED=0 go build ./...`、`just test`、`just lint`、`just web::lint`、`just web::test`、`just web::build` 全部通过。

## Risks & Deferred Items

- **开发库 `oauth_clients` 表结构升级**：现有开发库 `oauth_clients` 含非空列 `secret_hash`（`Ent Schema.Create` 仅补列不自动删非空旧列）；本地开发升级时按 `docs/iam.md` 约定清理旧 `oauth_clients` 表（静态 `admin` 客户端会在启动期由 `OIDCInitializer` 自动重建），并在文档与实施步骤中明确标注。
- **无状态服务 JWT 的自然到期窗口**：与 `docs/iam.md` 既有约定一致，接收方基于本地 JWKS 验签的 `actor_type=service` 访问令牌在客户端禁用/删除后，最长在 `service_access_token_ttl`（默认 `5m`）窗口内自然过期，期间新令牌签发与在线 Introspection 立即阻断。
- **第三方应用自定义业务 API Scope**：一期第三方客户端可选用户范围保持标准 OIDC Scopes（`openid`、`profile`、`email`、`offline_access`）；细粒度业务 API Resource Scope 待具体开放 API 场景出现后再扩展。
