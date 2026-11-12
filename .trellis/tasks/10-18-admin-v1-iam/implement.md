# IAM 能力补齐实施计划

需求：[PRD](prd.md)。技术合同：[设计](design.md)。本文件安排获得实施批准后的改动，不启动任务或服务。

## 1. 契约与持久化字段

- [ ] 修改 IAM User/Account 源 Proto：SendUserVerificationEmail、SetUserPassword、RevokeUserSessions、DeleteUser、UndeleteUser、ExpungeUser、DeleteAccount、ChangeInitialPassword，使用 design 中的请求、响应和方法认证授权注解。
- [ ] User 增加 delete_time=11、purge_time=12、required_actions=13（均标注 OUTPUT_ONLY）；CreateUserRequest 增加 optional require_password_change=4；ListUsersRequest 增加 show_deleted=5。保留所有既有字段编号。
- [ ] User schema 接入 SoftDeleteMixin，新增 required_actions JSONB 非空集合；Ent 客户端默认空切片、SQL 默认表达式 `'[]'`。沿用 Schema.Create 和当前 Ent 生成入口，不增加独立动作表或迁移框架。
- [ ] 增加两个领域 reason：User VERIFICATION_EMAIL_FAILED=5/HTTP503；Account INVALID_STATE=7/HTTP409。其余错误沿用设计第 10 节的既有 reason。
- [ ] 更新源 Proto 对应的 Go/TS、CRUD descriptor、authn/authz rules、redact、OpenAPI 和 Ent 生成物；不手改生成代码。

## 2. 用户读取、生命周期与授权

依赖第 1 节；接口和表级谓词见 [生命周期调用设计](research/lifecycle-implementation-design.md)。

- [ ] 拆开管理 GetUser 和认证 Get/FindByEmail 可见性；ListUsers 显式传 show_deleted 并绑定 page-token scope，使用现有 null 过滤语法。
- [ ] 增加 lockUserIncludingDeleted，仅用于 Undelete/Expunge；默认 lockUser 只读取未删除身份。普通资料／状态 Update 加入用户锁和未删除谓词；UpdateUser 改读未删除身份校验 immutable，EnableUser 在锁内按主邮箱 verified_time 恢复为 ACTIVE 或 PENDING_EMAIL_VERIFICATION。
- [ ] 实现 UserRepo.SoftDelete/Undelete/Expunge；管理删除直接调用 `SoftDelete(..., nil, ...)`，自助删除传入 `DeleteAccountProof{LoginID, PasswordHash}` 并在锁内复验；事务内投影返回资源，不在提交后用默认 Get 读取删除记录。
- [ ] 删除同步失效验证／重置令牌、用户会话和身份绑定授权请求，恢复不清撤销或消费字段。
- [ ] Expunge 按设计清除目标用户十二类表记录，保留 service token、共享 OAuthClient/密钥、其他用户与 SCS 失效容器。
- [ ] OpenFGA 增加 expunge_users 独立关系及模型案例；运行部署的 Admin 服务身份显式配置 manage_users、expunge_users。继续使用现有 JWT/FGA 中间件。

## 3. 创建、验证邮件、密码与登录

与第 2 节共享用户锁和 User 映射；签名见 [凭据调用设计](research/credential-implementation-design.md)。

- [ ] CreatePendingUser 改为纯创建并接收动作集合，创建前邮箱查重覆盖软删除占用记录；管理入口解析 optional 默认；UserInitializer 查重覆盖软删除记录，对已存在但处于软删除／封禁／待验证的 seed 保持现状、打印 Warn 日志并正常启动（同步更新 bootstrap_test.go）；Register 保留 CAP 后创建与发信，自助重发保留 CAP/防枚举语义。
- [ ] SendUserVerificationEmail 使用资源名定位，调用内部发信能力；token 提交后同步发 SMTP，不持锁发网络请求，不在管理 CreateUser 中发信。
- [ ] 两类 token Create 在用户锁内复验资格；VerifyEmail 改为 ConsumeAndActivate 单事务，移除独立 ActivateEmail 生产入口并迁移所有调用者。
- [ ] PasswordCredential 增加具体密码行 ID；SessionRepo.Create 接收完整凭据及 User.etag，在锁内决定返回有动作 User+nil login 或创建普通登录。迁移原仅 userID 的生产调用。
- [ ] 实现 CompleteInitialPassword 原子更新密码、动作、版本、旧会话撤销与新登录；普通 Login/ChangePassword、管理设密沿用各自授权前提。
- [ ] 邮箱重置采用 Peek 与原子提交：业务层比较密码，Repo 重验 token、快照与锁后的有效期限，清除动作但不自动登录；无动作的账号保持既有重置行为。
- [ ] OIDC 绑定请求和保存 code 改为现有 OAuthRepo 的用户锁方法；Issue/Resolve 与 OIDCStorage.populateUserinfo 统一检查未删除、ACTIVE、邮箱已验证与当前用户动作为空，独立 service grant 不受影响。
- [ ] NewUserUsecase 注入 CredentialRepo 与 SessionRepo（同步更新 Wire 与测试夹具）；管理设密按 user_id 与 password 类型定位认证器（兼容封禁账号已写 disabled/revoked_time 的记录并原样保留其状态），与强制登出分别更新关联 IAM/OAuth 状态，保持离线 JWT 到期语义；并发请求不使用删除恢复前或管理设密前的凭据继续建立登录。

## 4. 服务接线与 IAM Web

- [ ] service 解析请求与领域错误（需登录的 AccountService 方法将目标缺失／已删除归一为 UNAUTHENTICATED/401，公开令牌消费方法将目标缺失／已删除归一为 INVALID_TOKEN/400），直接返回原生 `kerrors.Error` 供 `userpb.IsUserErrorReason*` 识别，不添加应用级 gRPC 状态码重包装中间件。
- [ ] 新 Account HTTP 方法注册到现有 server，使用指定 PUBLIC/REQUIRED 注解；在 `security/session` 提供统一的 `NewFilter`（`FilterConfig`），由 `NewHTTPServer` 内联声明 `/v1/iam/`、同源 `/authorize/callback`（限 `GET`）与跨站 `/authorize`、`/end_session` 范围，在装载会话前拦截非 `application/json`（415）及跨源写请求（403）。
- [ ] Login 有动作时若存在旧浏览器会话绑定则 `Destroy` 当前 SCS 会话（匿名请求不写空 cookie）并返回 User；首改成功轮换 token 后绑定新登录；自助删除提交后 `Destroy` 当前绑定。提交后绑定失败不伪造数据库回滚。
- [ ] 登录页增加同页首改表单，要求再次输入初始密码；保留内存中的邮箱/request_id，清除敏感值并使用现有单飞控制。成功继续 OIDC/账号页，取消回登录表单，刷新不新建持久化续接状态。
- [ ] 账号安全页加入自助删除确认及密码输入；成功进入登录页；仅依赖安全 reason 反馈错误，未知结果不自动重发删除。
- [ ] 同步原有测试夹具和 fakes 的签名；删除已被替换的接口、冗余回退和绕过事务的旧生产路径。

## 5. 业务验收与命令

以下在产品代码完成后执行；对应 PRD AC-I1–AC-I10，不在设计阶段运行。

| 验收组 | 可观察结果 |
| --- | --- |
| 创建与发信（I2） | CreateUser 成功时未发信；独立发信成功或错误不改变创建结果；Register/CAP 重发保持原行为；首次改密未传/true/false 分别生成要求/要求/空集合 |
| 查询及生命周期（I1/I4） | 管理 Get 可见删除记录，默认认证不可见；show_deleted 与过滤改变不能复用旧页 token；删除保留状态/邮箱，恢复不恢复旧会话；重复 Delete NOT_FOUND、恢复活目标 ALREADY_EXISTS、Expunge 活目标 INVALID_STATE |
| 授权与永久清理（I4/I7） | manage-only 服务不能 Expunge；目标用户关联记录清除，service token 和其他用户保留；SCS 旧引用立即失效；事务失败仍保留邮箱归属 |
| 本人操作（I5/I6） | 错误密码不改状态；首改成功同时更新密码/动作/登录；相同密码不清动作；有效邮箱重置可完成动作但不自动登录；自助删除后旧 cookie 不可用 |
| 并发（I3–I6） | 管理设密后旧凭据不能晚建登录；删除后晚到 token/code 被拒绝；删除恢复后旧快照被拒绝；验证激活与令牌消费无分离提交；恢复与永久删除不产生冲突成功 |
| 页面与 OIDC（I8） | 两步首改后继续原授权；普通用户不能利用动作结果访问受保护数据；取消/失败/刷新反馈符合设计，cookie 保存或响应丢失不误报密码未变 |

采用已有 biz、data、server 和 OIDC 测试分层。数据库用 `IAM_TEST_POSTGRES_DSN` 对应的隔离测试 schema；不使用用户现存账号作为破坏性测试目标。原测试跳过不能记作真实集成验收。Admin 服务的完整调用和浏览器链路由 Admin 子任务完成后共同执行。

仓库已有命令入口：

```bash
# 生成顺序：先契约与 Ent，再 Wire（在对应代码已齐备后）。
just api
just -f app/iam/service/justfile gen-ent
just -f app/iam/service/justfile openapi
just -f app/iam/service/justfile wire

# 实施完成后的检查。
just lint-proto
just api-ts-check
just openfga-model-test
GOWORK=off go test ./app/iam/service/internal/... ./infra/entgo/mixin/... ./security/...
GOWORK=off go build ./app/iam/service/cmd/server
pnpm --filter @plateau/iam-web typecheck
pnpm --filter @plateau/iam-web lint
pnpm --filter @plateau/iam-web build
```

真实服务与页面使用既有本地配置和 `just service::iam::run`、`just web::iam::dev`；IAM HTTP/gRPC/Web 分别使用 10000/10001/10002。依赖的 PostgreSQL、Redis、OpenFGA、CAP、邮件配置按现有环境准备。业务验收结果按现有任务记录方式保存。

## 6. 切换、文档与最终检查

- [ ] 服务切换前备份并停止旧 IAM 实例；自动加列后启动新版本，禁止旧代码在新删除／动作状态上继续认证。回退采用停流量后的备份恢复，不删除字段来绕过新规则。
- [ ] 同步 `.trellis/spec/iam/identity.md`、`sessions.md`、`oidc.md` 中已实现的锁序、动作与生命周期规则；具体业务规则以实现为依据，不提前写成现有能力。
- [ ] Admin 后端消费两步创建/发信、准确的 gRPC reason、expunge_users 权限及回收站查询；IAM 不实现 Admin RP 登录和本地会话。
- [ ] 运行变更检查并确认生成物与源码一致，所有旧调用者已迁移；工作区已有用户改动保留。提交另经用户授权。
