# IAM 首次改密的 HTTP、页面与调用方接线

配合 [凭据事务设计](credential-implementation-design.md) 与 [主设计](../design.md) 使用；路径以 `app/iam/` 为根。

## 8. HTTP 接线与浏览器请求约束

### 8.1 真实注册与注解

`internal/server/http.go:38-51` 为 AuthnService、SessionService、AccountService 注册 session authn、OpenFGA 规则及生成 HTTP handlers；`internal/server/grpc.go:22-38` 只注册 UserService。因此 AccountService 虽然有生成 gRPC stub，当前应用实际上是 HTTP-only，不为新首改或自助删除新增 AccountService gRPC 注册。`/v1/iam/` 已包含在 browserSessionFilter 中，新路径自然装载 SCS。

新 ChangeInitialPassword 使用 PUBLIC；自助 DeleteAccount 使用 REQUIRED，参数仅 current_password、目标来自可信 authn.From。管理 SendUserVerificationEmail 继续 gRPC、REQUIRED service JWT 与 manage_users；不能把其暴露为公开重发或使用浏览器人的 cookie 调用。

### 8.2 现有保护的准确含义

现有 `security/session` 提供 Secure、HttpOnly、默认 SameSite=Lax、host cookie 约束及保存失败处理；不是 CSRF token 中间件。现有 `browserSessionFilter` 仅 LoadAndSave，没有 Origin/Referer 校验。Next `next.config.ts` 同源 rewrite，`@plateau/client` 默认 credentials=same-origin，createTransport 显式 JSON 请求；当前配置没有允许跨源凭据 CORS。Servora CORS 仅响应允许头，不等同于拒绝不可信 Origin 的 CSRF 检查。

首改和自助删除沿用相同同源浏览器入口：不增加跨源凭据 CORS、不放宽 cookie 为跨站、不得将密码改为 query/form-urlencoded 传递。不能把“同源页面 + JSON + Lax”描述成已存在独立 CSRF 校验器。`NewHTTPServer` 注入 `*bootstrap.Runtime` 并在启动期解析规范化 `external_url` 的 IAM public origin（配置缺失或非法直接启动报错）；在 IAM `browserSessionFilter` 对这两个新增 POST 用 `mime.ParseMediaType` 校验媒体类型为 `application/json`（不匹配返回 415），存在 `Origin` 头时必须与该 public origin 相等（不匹配返回 403），不信任客户端可伪造的 forwarded host。不带 `Origin` 的非浏览器请求仍按该方法的独立密码或普通登录规则校验，不增加 Referer 回退或 CSRF token 框架。请求类型或来源不合格在进入业务前拒绝；已有 endpoints 的行为不改变。

## 9. 登录页面与表单生命周期

保留 `app/login/page.tsx` 为 CSR 页面，不新增待改密路由或 URL token。使用现有 useSensitiveValue、useSingleFlight、TextField、PasswordField、StatusMessage 和统一 createIamApi。

1. Login 请求成功先检查 `result.value.user.requiredActions`。UPDATE_PASSWORD 时清除登录密码，保存已提交的邮箱作为当前表单邮箱，展示“初始密码/新密码/确认新密码”，保留内存中的 request_id。无动作时才进入原成功导航；未知动作不能作为普通登录成功继续。
2. 首改表单的当前密码必须再次输入，不能沿用首次 Login 的 submittedPassword。新/确认密码仅在各自敏感 hook 与请求闭包中持有，不写 URL、localStorage、sessionStorage、日志、公共状态或 SCS。
3. 初始密码和新密码使用现有长度校验；确认新密码仅前端比较，不发给服务端。通过生成 `account.ChangeInitialPassword({email,currentPassword,newPassword})` 提交。
4. Login 与首改共享单飞控制，pending 禁用字段、取消与冲突提交；沿用卸载 AbortController。提交结束（含失败）显式 clear 三个密码输入；取消返回同页登录表单，清所有密码并保留 request_id。
5. 首改成功沿用现有导航：有 request_id 到 `/authorize/callback?id=...`，否则 `/account/`。失败不继续 OAuth，保留邮箱/request_id；UNAUTHENTICATED/401 统一提示邮箱、当前密码或账号状态不满足要求，INVALID_PASSWORD/400 提示新密码须符合 8 至 128 字符规则且不同于当前密码，INVALID_STATE/409 提示账号或凭据状态已变化、请重新登录。页面不拆分同一 reason 的内部原因；网络/403/5xx 使用安全通用文案，响应丢失提示可尝试新密码登录，不宣称事务未提交。
6. request_id 继续由 `useOneTimeRequestId` 从 URL 读取后移除；刷新或换设备重新开始 Login，不新增跨刷新恢复存储。动作始终在 User 中，重新 Login 仍能得到同样要求。
7. `lib/errors.ts` 增加新增 reason 的判定，不在页面比较原始错误字符串；`lib/iam-api.ts` 原有 account client 已能承载生成新方法，无需另建 client。

## 10. 最小改动与消费者清单

### 10.1 手写源码

| 文件 | 必要修改 |
| --- | --- |
| `service/api/protos/iam/user/v1/user.proto` | UserRequiredAction、required_actions=13（OUTPUT_ONLY）、optional require_password_change=4、SendUserVerificationEmail 请求/空响应及方法；沿用管理默认授权。 |
| `service/api/protos/iam/account/v1/account.proto` | ChangeInitialPassword 请求/空响应、PUBLIC HTTP 注解、密码校验/redact、新 reason。 |
| `service/internal/biz/account.go` | 创建/发信拆分、Register/Resend 保留原流程、VerifyEmail 原子提交调用、reset Peek/commit、首次改密、内部证明类型。 |
| `service/internal/biz/authn.go` | PasswordCredential 四元值、CredentialRepo 首改事务方法、正常 Login 传已验证快照。 |
| `service/internal/biz/session.go` | Create 新参数/返回 User、Resolve 动作检查；不保留 userID-only 创建旁路。 |
| `service/internal/biz/user.go` | NewUserUsecase 注入 CredentialRepo 与 SessionRepo；管理创建 bool 与动作、管理发信委托、管理设密与强制登出，UpdateUser 读未删除身份，移除 ActivateEmail Repo 旧接口。 |
| `service/internal/biz/oauth.go` | 现有 OAuthRepo 增加绑定授权请求和保存授权码事务接口。 |
| `service/internal/data/schema/user.go`、用户字段迁移 | JSONB 默认/存量空集合，无通用 mixin。 |
| `service/internal/data/user.go` | 聚合动作写入、mapper 显式转换、事务可用的同一 User 投影；移除旧 ActivateEmail。 |
| `service/internal/data/account.go` | token 创建用户锁/etag、验证消费激活事务、reset Peek/commit。 |
| `service/internal/data/authn.go` | 完整凭据投影、正常登录快照复验、管理设密（兼容封禁账号已写 disabled/revoked_time 的认证器并保留其状态）、首改原子更新/清动作/撤销/建登录；复用本包 tx helper。 |
| `service/internal/data/oauth.go` | 人类 token 发行资格、请求绑定及保存授权码的锁事务。 |
| `service/internal/data/initial_user_creator.go`、`biz/user_initializer.go` | 新 seed 持久化及返回 UPDATE_PASSWORD；启动查重改用 FindByEmailIncludingDeleted，对已存在但软删除或非活跃的 seed 打 Warn 日志并正常启动。 |
| `service/internal/service/user.go`、`account.go`、`authn.go` | 新 RPC 映射、optional 缺省、Loaded 前置、User+nil login 分支、SCS 更新与安全错误（accountError 补充 ErrNotFound -> UNAUTHENTICATED/401）。 |
| `service/internal/authn/session.go` | 必需动作错误映射为无效凭据而非依赖失败。 |
| `service/internal/oidc/op_auth_storage.go`、`oidc/claims.go` | CompleteAuthRequest/SaveAuthCode 迁移为 OAuthRepo 事务调用；populateUserinfo 对齐邮箱已验证及动作为空检查。 |
| `service/internal/server/http.go`、`cmd/server/wire_gen.go` | 保持现有 HTTP-only 注册；NewHTTPServer 注入运行配置解析公开 origin，在现有 filter 用 mime.ParseMediaType 限定两个新增 POST 的 JSON（415）和存在 Origin 时的配置来源检查（403），执行 `just -f app/iam/service/justfile wire` 同步生成物。 |
| `web/app/login/page.tsx`、`web/lib/errors.ts` | 根据 User 动作切表单、新生成方法调用、密码清理和失败结果。 |
| `web/app/reset-password/page.tsx` | 有要求而新旧相同时的安全提示，保留一次性 token 现有生命周期。 |

NewAccountUsecase/NewUserUsecase 不因首改新增反向依赖；首改能力由现有 CredentialRepo 实现。无新增协调层 usecase，但 `NewUserUsecase`（新增 `CredentialRepo`、`SessionRepo`）与 `NewHTTPServer`（新增运行配置以解析公开 origin）的构造参数扩展后，须重新执行 `just -f app/iam/service/justfile wire` 并同步 `server/fixture_test.go` 与 `biz` 单测构造调用。

### 10.2 调用者与生成物同步

- CreatePendingUser 两个生产调用点（Register、UserUsecase.CreateUser）同时迁移；Register 加显式发信，管理方去发信。
- SessionUsecase.Create 的 AuthenticationUsecase.Login、SessionRepo.Create 的现有 data/业务测试及 fake 同步新快照和返回 User；直接调用 repo 创建测试身份也必须给真实 credential，不留只传 userID 的捷径。
- VerifyEmail 不再调用 Consume+ActivateEmail；业务 fake、data 测试与 server fixture 使用 ConsumeAndActivate。
- reset 旧 ConsumeAndReplacePassword 调用包括 `biz/account.go`、`data/data_test.go`、`data/session_integration_test.go`；全部迁为 Peek 得 proof 后提交，fakes 同步。
- 已有 `biz/account_test.go`、`biz/authentication_test.go`、`biz/bootstrap_test.go`（更新原拒绝非活跃已有 seed 的断言为告警并放行启动，同时覆盖已软删除 seed 不重复创建）、`data/data_test.go`、`data/session_integration_test.go`、`data/oauth_concurrency_test.go`、`server/account_integration_test.go`、`server/http_integration_test.go`、`server/fixture_test.go` 及 `oidc/provider_test.go` 按各自真实消费者检查创建副作用、四元快照、动作与 cookie/OIDC 行为；测试安排属于实施，不在技术规划时执行。
- 按已有生成流程更新 `api/gen/go/iam/{user,account}/v1` 的 pb、HTTP/gRPC、校验、脱敏、错误/规则/CRUD 字段产物，`api/gen/ts/iam/{user,account}/v1` 及 `service/api/internal/assets/openapi.yaml`，Ent user/schema/migrate 产物；不手改生成代码，不把生成了 Account gRPC stub 当作注册了 gRPC 服务。
- Admin 两步调用与 optional bool 存在性由 Admin 后端任务同步；IAM Web 使用 LoginResponse.User 的 requiredActions，不增加同义布尔响应。管理创建返回 User 形状保持。

## 11. 行为核对要点

正常业务验收应观察：管理创建不发信而 Register 仍自动发信；公开 CAP 失败无查询/写入/发信；验证激活、删除与恢复无旧 token 复活；旧密码在管理设密后不能晚建登录；首改只成功一次且密码/动作/旧会话撤销/新登录共同提交；有要求的邮箱重置相同密码不清动作，无要求账号的原有重置行为不变；旧 SCS/授权码/refresh 不能绕过当前动作；cookie 保存或网络响应失败不能伪装首改未提交。
