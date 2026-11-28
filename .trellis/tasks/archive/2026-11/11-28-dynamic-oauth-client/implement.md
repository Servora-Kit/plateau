# 动态 OAuth Client 管理实施计划

## 执行顺序清单

### 阶段 1：OpenFGA 权限模型与 Proto 契约生成
- [x] 更新 [manifests/openfga/iam.fga](../../../../manifests/openfga/iam.fga)（新增 `define manage_clients: [service]`）、[manifests/openfga/admin.fga](../../../../manifests/openfga/admin.fga)（新增 `define manage_clients: admin`）、[manifests/openfga/tuples/platform-services.yaml](../../../../manifests/openfga/tuples/platform-services.yaml)（授予 `service:admin` `manage_clients`）及对应 `manifests/openfga/*.fga.yaml` 测试用例，执行 `just openfga-model-test`。
- [x] 新增 `api/protos/plateau/iam/client/v1/client.proto`（`OAuthClientService`、`OAuthClient`、`OAuthClientSecret`、枚举与 CRUD/AuthZ/Audit/Redact 注解）。
- [x] 新增 `api/protos/plateau/admin/client/v1/client.proto`（Admin BFF `/v1/admin/oauth-clients*` HTTP + gRPC 契约）。
- [x] 更新 [api/protos/plateau/iam/authn/v1/authn.proto](../../../../api/protos/plateau/iam/authn/v1/authn.proto)（扩展 `AuthFlowOidcContext` 增加 `client_name`/`logo_uri`，新增 `GetConsentView` 与 `SubmitConsentDecision`）。
- [x] 更新 [api/protos/plateau/iam/account/v1/account.proto](../../../../api/protos/plateau/iam/account/v1/account.proto)（新增 `ListAuthorizedApps` 与 `RevokeAuthorizedApp`）。
- [x] 更新 [api/protos/plateau/iam/oidc/conf/v1/config.proto](../../../../api/protos/plateau/iam/oidc/conf/v1/config.proto)（允许空 `clients`，补充可选静态客户端展示与认证方法字段）。
- [x] 执行 `GOWORK=off just gen`，通过 `just lint-proto`、`just fmt-proto-check`、`just api-ts-check`。

### 阶段 2：IAM Ent Schema 与持久层实现
- [x] 更新 [app/iam/service/internal/data/schema/oauth_client.go](../../../../app/iam/service/internal/data/schema/oauth_client.go) 与 [app/iam/service/internal/data/schema/oidc_authorization_request.go](../../../../app/iam/service/internal/data/schema/oidc_authorization_request.go)（补充 `prompt` 与 `consent_approved` 字段）。
- [x] 新增 `app/iam/service/internal/data/schema/oauth_client_secret.go` 与 `app/iam/service/internal/data/schema/oauth_consent.go`，执行 Ent 代码生成。
- [x] 新增 `app/iam/service/internal/data/oauth_client.go`：实现 `biz.OAuthClientRepo`（接入 `entcrud` 的 `ListFields`、`ResourceMapper`、`ClearHelper`，实现创建、查询、更新、多密钥增删、状态切换与单事务级联撤销/物理删除）。
- [x] 新增 `app/iam/service/internal/data/oauth_consent.go`（或在 `oauth.go` / `account.go` 中扩展）：实现 `oauth_consents` 读写、并集更新、已授权应用列表联表查询、单应用授权与会话原子撤销，并在 `user.go` `Expunge` 中补充清理 `oauth_consents`。
- [x] 更新 `app/iam/service/internal/data/authn_flow.go`：在加载 OIDC 请求上下文时联查活跃 `OAuthClient` 填充 `client_name` 与 `logo_uri`（客户端已禁用或不存在时拒绝创建/推进关联 Flow）。

### 阶段 3：IAM 领域层、OIDC OP 动态适配与服务层
- [x] 新增 `app/iam/service/internal/biz/oauth_client.go`：提炼静态与动态共享的参数校验规则、`pcs_` 高熵密钥生成、最多 5 把密钥上限与“至少保留 1 把未过期密钥”校验、`STATIC` 只读拦截。
- [x] 新增 `app/iam/service/internal/biz/consent.go`（及扩展 `account.go`）：实现 `GetConsentView`、`SubmitConsentDecision`、`ListAuthorizedApps`、`RevokeAuthorizedApp`。
- [x] 重构 `app/iam/service/internal/oidc/`（`initializer.go`、`op_client.go`、`op_op_storage.go`、`op_auth_storage.go`、`provider.go`、`service_token.go`）：
  - `OIDCInitializer` 支持 `0..N` 个 `STATIC` 客户端对账与静态删除级联清理，保护 `DYNAMIC` 记录，检测重名冲突。
  - `op.Config` 启用 `AuthMethodPost: true`；在请求上下文提取认证承载方式并在 `AuthorizeClientIDSecret` 执行客户端 `active` 状态检查、认证方式严格匹配与多密钥常量时间比对；更新发现文档声明 `["client_secret_basic", "client_secret_post", "none"]`。
  - `op_client.go` 映射 `ApplicationType`、`AuthMethod`、`PostLogoutRedirectURIs` 与受控回环 `DevMode`。
  - `provider.go` `/authorize/callback` 校验客户端 `active` 与 `trusted`：对需要 Consent 的请求重定向至 `/consent/?request_id=<id>`。
- [x] 新增 `app/iam/service/internal/service/oauth_client.go`，更新 `service/authn.go`、`service/account.go` 与 `server/grpc.go`（仅在 gRPC 注册 `OAuthClientService`），运行 `just wire`。

### 阶段 4：Admin BFF 客户端编排层
- [x] 更新 `app/admin/service` 的 IAM gRPC 客户端装配，接入 `iamclientv1.OAuthClientServiceClient`。
- [x] 新增 `app/admin/service/internal/biz/oauth_client.go` 与 `internal/service/oauth_client.go`，注册 HTTP + gRPC 路由并映射领域错误码，运行 `just wire`。

### 阶段 5：IAM Web 升级（品牌展示、Consent 页与 Google 风格个人中心）
- [x] 更新 [app/iam/web/app/login/page.tsx](../../../../app/iam/web/app/login/page.tsx)：在 OIDC 登录上下文展示客户端图标与 `client_name`。
- [x] 新增 `app/iam/web/app/consent/page.tsx`：实现第三方应用授权同意与拒绝跳转交互（含加载态、失效提示与安全回退）。
- [x] 重构 [app/iam/web/components/account-shell.tsx](../../../../app/iam/web/components/account-shell.tsx)、[app/iam/web/app/account/page.tsx](../../../../app/iam/web/app/account/page.tsx)、[app/iam/web/app/account/security/page.tsx](../../../../app/iam/web/app/account/security/page.tsx)，并新增 `app/iam/web/app/account/applications/page.tsx`：对齐 `https://myaccount.google.com/` 顶栏 + 左侧圆角胶囊导航 + 右侧圆角分组卡片布局，打通「第三方应用与服务」查看与撤销。

### 阶段 6：Admin Web 客户端控制台
- [x] 在 `app/admin/web/apps/web-antd` 新增 OAuth 客户端 API 封装、路由菜单与视图组件（列表筛选、4 类预设模板创建向导、一次性明文密钥复制弹窗、详情编辑抽屉、多密钥管理与删除确认、`STATIC` 只读锁定提示）。

### 阶段 7：测试验证与规范文档更新
- [x] 补充 IAM 数据层、领域层、OIDC 协议层与服务层单测/集成测试（覆盖双源共存、3 种应用类型、认证方式防降级、多密钥零停机轮换、剩最后一把密钥拒删、禁用/删除级联撤销、Consent 免审/首次/扩权/`prompt=consent`/拒绝、个人中心撤销授权、明文密钥脱敏）。
- [x] 补充 Admin BFF 集成测试（权限校验、CRUD + 密钥轮换全链路、审计记录）。
- [x] 更新 [docs/iam.md](../../../../docs/iam.md) 及 `.trellis/spec/iam/{oidc,auth-flows,frontend,authorization,startup}.md`、`.trellis/spec/admin/{backend,frontend}.md`。

---

## 高风险文件与防护措施

| 文件/区域 | 风险点 | 防护措施 |
| --- | --- | --- |
| `app/iam/service/internal/oidc/op_op_storage.go` & `provider.go` | 开启 `AuthMethodPost` 后 `client_secret_basic` 客户端被降级通过表单认证，或机密客户端绕过密钥校验 | 在 HTTP 入口提取实际认证承载方式写入 `ctx`（同时带 Basic 与 PostForm `client_secret` 直接报错），`AuthorizeClientIDSecret` 强校验承载方式等于客户端配置的 `token_endpoint_auth_method` |
| `app/iam/service/internal/oidc/provider.go` | `prompt=consent` 在用户点击同意跳回 `/authorize/callback` 后再次触发重定向死循环 | 在 `oidc_authorization_requests` 记录单次请求级 `consent_approved` 标记，用户当次同意后置 `true`，`/authorize/callback` 见 `true` 直接放行 |
| `app/iam/service/internal/data/oauth_client.go` | 禁用或删除客户端遗漏在途 `auth_flows` / `oidc_authorization_requests` 导致残留半完成流 | 单一事务内按固定顺序级联失效关联 `auth_flows`、清理未完成授权请求、消费未用授权码并撤销全部活跃 OAuth 会话与访问令牌 |
| `app/iam/service/internal/oidc/initializer.go` | 启动对账误删动态客户端，或旧开发库残留非空 `oauth_clients.secret_hash` 列导致插入失败 | `Delete` 严格限定 `oauthclient.SourceEQ(SourceStatic)`；在 `docs/iam.md` 说明本地旧库升级时清理 `oauth_clients` 重建引导客户端 |
| `api/protos/plateau/{iam,admin}/client/v1/client.proto` | 一次性明文密钥泄露进审计 CloudEvents 或错误日志 | 所有明文返回字段强制标注 `(servora.redact.v3.value).string = ""` 并编写 `redact_test.go` 断言审计载荷无明文 |

## 验证命令

```bash
# OpenFGA 模型与 Proto 代码生成检查
just openfga-model-test
GOWORK=off just fmt-proto-check
GOWORK=off just gen-check
GOWORK=off just lint

# Go 无 CGO 构建与全仓测试
GOWORK=off CGO_ENABLED=0 go build ./...
GOWORK=off just test

# 前端代码检查、测试与生产构建
just web::lint
just web::iam::test
pnpm --filter @servora/plateau-utils run test
pnpm --dir app/admin/web exec vitest run --dom apps/web-antd
just web::build
```

## 启动实施前检查

- [x] 用户已明确批准最新规划摘要
- [x] `python3 ./.trellis/scripts/task.py validate 11-28-dynamic-oauth-client` 校验通过
- [x] 执行 `python3 ./.trellis/scripts/task.py start 11-28-dynamic-oauth-client` 后再开始修改业务代码
