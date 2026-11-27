# IAM Auth Flow 认证状态机一期实施计划

## 执行顺序清单

### 阶段 1：Proto 契约与多时钟配置生成
- [x] 更新 [api/protos/plateau/iam/authn/v1/authn.proto](../../../../api/protos/plateau/iam/authn/v1/authn.proto)：移除 `Login` / `LoginRequest` / `LoginResponse`，新增 6 个 Auth Flow RPC、`AuthFlow` / `AuthFlowState` / `AuthFactorMethod` / `AuthFlowOidcContext` 消息与扩展后的 `AuthnErrorReason` 及脱敏/审计注解。
- [x] 更新 [api/protos/plateau/iam/account/v1/account.proto](../../../../api/protos/plateau/iam/account/v1/account.proto)：移除 `ChangeInitialPassword`、`ChangeInitialPasswordRequest`、`ChangeInitialPasswordResponse`（保留 `ACCOUNT_ERROR_REASON_INVALID_STATE`）。
- [x] 更新 3 份配置 Proto：
  - [api/protos/plateau/iam/conf/v1/config.proto](../../../../api/protos/plateau/iam/conf/v1/config.proto)：可选子消息 `auth_flow`（`ttl` 默认 `30m`、`step_ttl` 默认 `10m`）。
  - [api/protos/plateau/iam/oidc/conf/v1/config.proto](../../../../api/protos/plateau/iam/oidc/conf/v1/config.proto)：`auth_request_ttl`（默认 `30m`）。
  - [api/protos/plateau/security/oidc/v1/config.proto](../../../../api/protos/plateau/security/oidc/v1/config.proto)：`handshake_ttl`（默认 `30m`）。
- [x] 执行 `just gen` 重新生成 Go、TypeScript 与 OpenAPI 产物，确认 `just lint-proto` 与 `just fmt-proto-check` 通过。

### 阶段 2：IAM Ent Schema 与持久层
- [x] 新增 `app/iam/service/internal/data/schema/auth_flow.go`（Ent Schema `AuthFlow`，表名 `auth_flows`，严禁包含密码/哈希字段）。
- [x] 更新 `app/iam/service/internal/data/schema/login_session.go` 与 [app/iam/service/internal/data/schema/oidc_authorization_request.go](../../../../app/iam/service/internal/data/schema/oidc_authorization_request.go)，新增 `amr` 字段（`jsonb` + `entsql.Default("[]")`）。
- [x] 执行 Ent 代码生成（`go generate ./app/iam/service/internal/data/...`）。
- [x] 新增 `app/iam/service/internal/data/authn_flow.go`（实现 `biz.AuthFlowRepo`：创建、读取、密码提交推进/完成、改密推进/完成、重置、取消及 OIDC 请求协同事务；严格遵循先 `lockUser` 后锁 `auth_flows` 的锁序与微秒截断时间比对）。
- [x] 更新 [app/iam/service/internal/data/authn.go](../../../../app/iam/service/internal/data/authn.go)、[app/iam/service/internal/data/user.go](../../../../app/iam/service/internal/data/user.go)、[app/iam/service/internal/data/oauth.go](../../../../app/iam/service/internal/data/oauth.go)、[app/iam/service/internal/data/data.go](../../../../app/iam/service/internal/data/data.go)：
  - 移除废弃的 `CompleteInitialPassword` 与旧 `SessionRepo.Create` 登录入口；`insertLoginSession` 写入 `amr`。
  - `revokeUserSessions` 级联调用 `invalidateUserAuthFlows`，`Expunge` 物理删除 `auth_flows`。
  - `CompleteAuthorizationRequest` 校验并复制 `LoginSession.Amr` 至 `OIDCAuthorizationRequest.Amr`。
  - 注册 `NewAuthFlowRepository` 至 `data.ProviderSet`。

### 阶段 3：IAM 领域状态机、OIDC OP 与服务装配
- [x] 更新 [app/iam/service/internal/biz/authn.go](../../../../app/iam/service/internal/biz/authn.go)、[app/iam/service/internal/biz/authn_flow.go](../../../../app/iam/service/internal/biz/authn_flow.go)、[app/iam/service/internal/biz/session.go](../../../../app/iam/service/internal/biz/session.go)、[app/iam/service/internal/biz/account.go](../../../../app/iam/service/internal/biz/account.go)、[app/iam/service/internal/biz/errors.go](../../../../app/iam/service/internal/biz/errors.go)、[app/iam/service/internal/biz/biz.go](../../../../app/iam/service/internal/biz/biz.go)：
  - `NewAuthenticationUsecase` 直接接收 `*iamconfpb.IAM` 与 `*oidcconfpb.OIDC`（`proto.Clone` 后补空 `AuthFlow` 并调 `Apply()`，复用 `NormalizeOIDCIssuer` 校验提取 `issuer`），执行段内时长自检（`ttl > 0`、`step_ttl > 0`、`step_ttl <= ttl`），不新增独立 `AuthFlowConfig` 包装。
  - 实现 `AuthenticationUsecase` 全部 6 个用例（标量入参解耦 RPC Request，`loadOwnedFlow` 统一归属与有效期校验，`CancelAuthFlow` 内置标准库组装 RFC 6749/9207 取消重定向）、可扩展状态求值器及 `CreateAuthFlow` / `GetAuthFlow` 的已登录会话短接逻辑，移除旧 `Login` 与 `AccountUsecase.ChangeInitialPassword`。
  - `LoginSession` 增加 `AMR []string`。
- [x] 更新 [app/iam/service/internal/authn/session.go](../../../../app/iam/service/internal/authn/session.go)：新增 `AuthFlowOwnerKey = "iam_auth_flow_owner"`。
- [x] 更新 `app/iam/service/internal/oidc/`（`contracts.go`、`storage.go`、`op_auth_request.go`、`op_auth_storage.go`、`provider.go`、`redirect_writer.go`、`initializer.go`）：
  - `OIDCStorage` 读取并校验配置 `auth_request_ttl > 0`，移除硬编码 `authorizationRequestTTL` 与 `passwordAMR`。
  - `authorizationRequest.GetAMR()` 返回持久化的 `entity.Amr`。
  - `IAMProvider` 发现文档输出 `authorization_response_iss_parameter_supported: true`；`/authorize` 与 `/authorize/callback` 统一设置 `Cache-Control: no-store` 并在外部 `redirect_uri` 重定向时经 `redirect_writer.go` 附加 `iss`；`completeAuthorization` 遇空 `amr` 会话防御性重定向至 `/login?request_id=...`；保持纯 HTTP OP 端点适配器定位。
- [x] 更新 [security/session/filter.go](../../../../security/session/filter.go)、[app/iam/service/internal/service/authn.go](../../../../app/iam/service/internal/service/authn.go) 与 [app/iam/service/internal/service/account.go](../../../../app/iam/service/internal/service/account.go)：
  - 实现 6 个 `AuthnService` RPC、SCS `iam_auth_flow_owner` 绑定、清除旧 `iam_login_id`、完成时 `RenewToken` + 写 `iam_login_id`、错误码与审计富化。
  - 移除 `AccountService.ChangeInitialPassword`。
  - 在 `security/session.NewFilter` 对命中的三类浏览器会话路由（含 403/415 早退响应）默认写入 `Cache-Control: no-store`，`app/iam/service/internal/server/http.go` 保持零改动。
- [x] 执行 `just wire` 更新 `app/iam/service/cmd/server/wire_gen.go`。

### 阶段 4：共享 OIDC RP 与 Admin 改造
- [x] 更新 [security/oidc/errors.go](../../../../security/oidc/errors.go) 与 [security/oidc/rp.go](../../../../security/oidc/rp.go)：
  - 新增 `ErrAccessDenied`；使用配置化 `handshake_ttl`（校验 `>= 1s`）替代硬编码 300s。
  - `BeginLogin` / `CompleteLogin` 设置 `Cache-Control: no-store`；`CompleteLogin` 顺序执行 Cookie 校验 → `state` 校验 → `iss` 强校验 → `error=access_denied` 映射 `ErrAccessDenied` → 换码验签。
- [x] 更新 [app/admin/service/internal/service/auth.go](../../../../app/admin/service/internal/service/auth.go)：`ErrAccessDenied` 审计 `OIDC_LOGIN_CANCELLED` 并跳转 `/auth/login?error=login_cancelled`，保持 OpenFGA 拒绝仍跳转 `/auth/login?error=access_denied`。
- [x] 更新 [app/admin/web/apps/web-antd/src/views/_core/authentication/login.vue](../../../../app/admin/web/apps/web-antd/src/views/_core/authentication/login.vue)：增加 `login_cancelled` 提示分支。

### 阶段 5：IAM Web 改造
- [x] 更新 [app/iam/web/lib/errors.ts](../../../../app/iam/web/lib/errors.ts)：新增 Auth Flow 领域错误判定辅助函数（`FLOW_NOT_FOUND`、`FLOW_EXPIRED`、`FLOW_INVALIDATED`、`INVALID_PASSWORD`、`OIDC_REQUEST_INVALID`）。
- [x] 更新 [app/iam/web/hooks/use-one-time-url.ts](../../../../app/iam/web/hooks/use-one-time-url.ts) 与 [app/iam/web/hooks/use-guest-session.ts](../../../../app/iam/web/hooks/use-guest-session.ts)：移除废弃的 `useOneTimeRequestId`，精简 `useGuestSession` 移除仅服务于旧 `/login/` 的 `enabled` / `requestId` 选项。
- [x] 重构 [app/iam/web/app/login/page.tsx](../../../../app/iam/web/app/login/page.tsx)：
  - 移除 `useGuestSession`，统一接入 `CreateAuthFlow` / `GetAuthFlow` / `SubmitPassword` / `CompleteUpdatePassword` / `ResetAuthFlow` / `CancelAuthFlow`。
  - 规范化 URL 为 `/login/?flow=<id>`，支持刷新与 `pageshow` 恢复步骤及 OIDC 上下文；同浏览器已登录时依服务端 `COMPLETED` 与 `continue_url` 精准跳转 `/authorize/callback?id=...` 或 `/account/`。
  - `UPDATE_PASSWORD` 步骤展示只读邮箱，移除初始密码输入框，提供“切换账号”与“取消登录”按钮及完整错误/超时回退体验。

### 阶段 6：测试迁移、新增用例与规范同步
- [x] 迁移并扩充 `security/oidc/oidc_test.go`（`handshake_ttl`、`iss` 缺失/重复/错配、伪造 `error=access_denied` 缺 Cookie/`state` 拒绝、合法 `error=access_denied` 返回 `ErrAccessDenied`、`Cache-Control: no-store`）。
- [x] 迁移并扩充 IAM 单元测试与集成测试：
  - `app/iam/service/internal/data/`（Ent 表迁移、快照校验、`revokeUserSessions` 级联失效、`Expunge` 清理、并发改密竞争）。
  - `app/iam/service/internal/biz/{authn_test.go,account_test.go}`（`iam.auth_flow` 段内配置校验、状态机流转、双时钟超时、400 重试、快照漂移失效）。
  - `app/iam/service/internal/oidc/{provider_test.go,audit_test.go}`（发现文档 `authorization_response_iss_parameter_supported`、`/authorize` 与 `/authorize/callback` `iss` 注入与内部跳转排除、真实 `amr` 透传至 ID Token 与刷新令牌、取消重定向 URL）。
  - `app/iam/service/internal/service/{service_test.go,redact_test.go}`（SCS 归属绑定、跨浏览器即使已登录也对他人 `flow_id` 返回 404、`GetAuthFlow`/`CreateAuthFlow` 已登录短接、空 `amr` 不短接防循环、未完成不写 `iam_login_id`、清除旧登录引用、脱敏）。
  - `app/iam/service/internal/server/{fixture_test.go,http_integration_test.go,grpc_integration_test.go,account_integration_test.go,audit_test.go}`（端到端 Flow + 审计 + 多标签页并行与跨标签页已登录续接 OIDC `continue_url` + `Cache-Control: no-store` + SCS 存储故障回退）。
- [x] 迁移并扩充 Admin 集成测试：`app/admin/service/internal/server/{integration_test.go,audit_integration_test.go}` 及 Admin auth service 单测（`login_cancelled` vs `access_denied`）。
- [x] 同步更新规范文档：
  - `.trellis/spec/iam/sessions.md`
  - `.trellis/spec/iam/oidc.md`
  - `.trellis/spec/iam/auth-flows.md`
  - `.trellis/spec/iam/frontend.md`
  - `.trellis/spec/admin/backend.md`
  - `.trellis/spec/admin/frontend.md`

### 阶段 7：联调缺陷修复（登录页死循环与 Kafka 审计阻塞）
- [x] 修复 [app/iam/web/app/login/page.tsx](../../../../app/iam/web/app/login/page.tsx)：解构 `useSensitiveValue()` 稳定 `clear` 回调写入 `clearSensitiveInputs` 依赖数组，消除 `GetAuthFlow` 重渲染死循环。
- [x] 补齐本地 Kafka Topic 预建：新增 [manifests/scripts/kafka/create-topics.sh](../../../../manifests/scripts/kafka/create-topics.sh)，在 [docker-compose.yaml](../../../../docker-compose.yaml) 增加 `kafka-provision` 并在 [process-compose.yaml](../../../../process-compose.yaml) 等待 `postgres-provision` / `kafka-provision` 退出码 `0`。
- [x] 解耦审计投递与主链路：在 [security/audit/auditor.go](../../../../security/audit/auditor.go) 扩展 `BackendFunc` 透传 `logger`；将 [security/audit/kafka/kafka.go](../../../../security/audit/kafka/kafka.go) 切换为 `context.WithoutCancel` + `TryProduce` 异步非阻塞投递与回调错误日志；在 [infra/kafka/kafka.go](../../../../infra/kafka/kafka.go) 补齐 `DefaultMaxBufferedRecords`、`DefaultRecordDeliveryTimeout`、`UnknownTopicRetries` 与清理期有界 `Flush`。
- [x] 补充 `infra/kafka` 与 `security/audit/kafka` 回归测试并同步更新 `.trellis/spec/{plateau/infra/kafka,plateau/security/audit,plateau/project/structure,iam/frontend}.md` 与 `manifests/AGENTS.md`。

---

## 高风险文件与注意事项

| 文件/区域 | 风险点 | 防护措施 |
|---|---|---|
| `app/iam/service/internal/data/authn_flow.go` & `authn.go` | 事务锁序反转引起死锁；PostgreSQL `timestamptz` 微秒截断导致 `changed_time` 误判漂移 | 统一先 `lockUser` 后锁 `auth_flows`；所有 `changed_time` 读写统一 `.UTC().Truncate(time.Microsecond)` |
| `app/iam/service/internal/service/authn.go` | 跨浏览器信息泄露（404 vs 409/410 或已登录探测他人 Flow）；未完成流程误保留旧 `iam_login_id`；`CreateAuthFlow` 外接口误写 Cookie；空 `amr` 短接造成重定向循环 | `GetAuthFlow` 归属哈希校验永远先于 `iam_login_id` 检查，不匹配一律 404 且不写 SCS；OIDC 短接强制要求非空 `amr`；进入 `REQUIRED_ACTION` 清除旧 `iam_login_id` 并轮换令牌 |
| `app/iam/service/internal/oidc/provider.go` | `iss` 误注入内部 `/login` 或 `/authorize/callback` 重定向；`encoding/json/v2` 嵌入结构体序列化行为 | 仅在 `u.IsAbs()` 且含 `code` 或 `error` 参数时附加 `iss`；发现文档单测显式反序列化校验顶层键 |
| `security/oidc/rp.go` | 先判 `error` 后校验 Cookie/`state`/`iss` 会导致伪造取消攻击 | 严格固定校验顺序并用单测覆盖伪造 `error=access_denied` 场景 |
| `app/iam/web/app/login/page.tsx` | 刷新丢失 OIDC 上下文、跨标签页已登录后盲跳 `/account/`、URL 残留 `request_id`、bfcache 回退暴露旧页面 | 移除 `useGuestSession`，统一由 `GetAuthFlow`/`CreateAuthFlow` 判定并按 `continue_url` 跳转；统一 `?flow=<id>` 并及时 `replaceState`，监听 `pageshow` 刷新状态并在提交后清空敏感输入 |

## 验证命令

```bash
# Proto、代码生成与 Wire
just lint-proto
just fmt-proto-check
just gen-check
just api-ts-check

# Go 测试与静态检查（含 CGO -race 与无 CGO 生产构建门禁）
just test
just lint
CGO_ENABLED=0 go build ./app/iam/service/cmd/server ./app/admin/service/cmd/server

# 前端构建、检查与测试（IAM Web 与 Admin Web）
just web::lint
just web::test
just web::build
```

> 若本地执行集成测试依赖 PostgreSQL / OpenFGA / Redis，按需先通过 Compose 启动依赖并确认 `IAM_TEST_POSTGRES_DSN` / `IAM_TEST_FGA_*` 环境变量覆盖全部集成用例。

## 启动实施前检查

- [x] 用户已明确批准最新规划摘要
- [x] `python3 ./.trellis/scripts/task.py validate 11-27-iam-auth-flow` 通过
- [x] 执行 `python3 ./.trellis/scripts/task.py start 11-27-iam-auth-flow` 后再开始修改代码
