# IAM 剩余能力核对

范围：父需求 §5–7、§10–11，以及各项验收依据。本文记录规划阶段核对的源码现状和差异，不新增产品需求；后续已确认的技术决定以 [设计](../design.md) 为准，源码核查不代表实现通过验收。

## 1. 管理员设置密码

### 现有能力

- `AccountService.ChangePassword` 是本人改密：`app/iam/service/internal/biz/account.go:237-256` 校验当前登录引用和旧密码，再调用 `CredentialRepo.ReplacePassword`。
- `app/iam/service/internal/data/authn.go:54-82` 的 `ReplacePassword` 要求账号 ACTIVE、存在有效 `keepLoginID`，以旧 hash 做条件更新，保留当前 IAM 登录、撤销其他 IAM 登录及所有关联 OAuth 会话。
- `AccountService.ConfirmPasswordReset` 是凭恢复令牌改密；其持久化路径 `app/iam/service/internal/data/account.go:108-175` 复验并消费恢复令牌、替换密码并撤销全部会话。

### 与已确认需求的差异

- 当前 UserService 没有管理员设置密码的 RPC。
- 已有两个改密入口分别要求旧密码或恢复令牌，不能直接承接“管理员无需本人凭证、无需完成邮箱验证”的语义。
- 密码哈希、凭据存储和会话撤销有可复用实现；管理入口、状态处理和事务职责已按 [设计第 2 节的方案 A](../design.md) 确定，尚未实施。保留首次改密要求的规则沿用父 PRD。

## 2. 强制登出 IAM

- `SessionService.Logout` 从当前认证上下文取登录引用并销毁当前 SCS 会话：`app/iam/service/internal/service/session.go:29-43`。
- `SessionUsecase.Logout` 仅撤销指定登录：`app/iam/service/internal/biz/session.go:84-88`；`data/authn.go:116-132` 撤销该登录及其关联 OAuth 会话。
- `data/authn.go:135-163` 已有 `revokeUserSessions` 和 `revokeOAuthSessions`。前者可按 user ID 撤销全部登录，后者覆盖 OAuth token session、access token 记录和 refresh token 记录。
- 图查询确认该用户级撤销函数被本人改密、密码恢复和账号状态变更使用；当前没有面向管理员的强制登出 RPC/usecase。

管理员入口的目标定位、授权与用户级撤销事务已按 [设计第 2 节的方案 A](../design.md) 确定，尚未实施。当前浏览器退出入口保持原有语义；独立应用会话和离线 JWT 的已确认规则不变。

## 3. 软删除、恢复、显式永久删除和自助删除

- `infra/entgo/mixin/soft_delete.go:25-100` 已有删除字段、查询过滤、删除改写与显式绕过能力；不提供应用的恢复规则或关联数据清理策略。本期不安排清理时间，`purge_time` 保持空。
- `app/iam/service/internal/data/schema/user.go:16-42` 当前没有删除字段或 Mixin 声明。
- `app/iam/service/api/protos/iam/conf/v1/config.proto:10-19` 当前仅有 `bootstrap_user_email`；手动 Expunge 方案不要求新增恢复期或扫描配置。
- UserService 的源 Proto 当前没有 DeleteUser、UndeleteUser 或 ExpungeUser RPC；AccountService 没有自助删除 RPC。
- IAM 源码、Proto 与 Web 中检索 `SoftDeleteMixin`、`SkipSoftDelete`、`delete_time`、`purge_time` 无匹配；结合 schema 与接口清单，未发现已接入的账号删除生命周期。

已确认无限期保留、管理员显式永久删除、删除／恢复仅传 name、重复删除返回 NOT_FOUND，以及 SCS 失效存储行按现有 TTL 清理。schema 与共享组件接入、管理和自助命令复用、Expunge 权限及具体事务按设计继续落实；不引入用户定时清理、队列、补发或重试。

## 4. 首次改密

### 当前登录链

- `AuthenticationUsecase.Login`：`app/iam/service/internal/biz/authn.go:42-72`，检查 ACTIVE、邮箱验证和密码，随后直接创建普通登录会话。
- `AuthnService.Login`：`app/iam/service/internal/service/authn.go:30-45`，更新 SCS token、写入登录引用并返回用户。
- `LoginResponse` 当前只有 `user`：`app/iam/service/api/protos/iam/authn/v1/authn.proto:50-52`。
- 登录页成功后进入账号页或原 OIDC 回调：`app/iam/web/app/login/page.tsx:43-59`。
- OIDC 回调通过 `SessionUsecase.Resolve` 解析正常登录，再完成授权请求：`app/iam/service/internal/oidc/provider.go:196-226`。
- `SessionUsecase.Resolve` 当前检查撤销状态、用户存在性、ACTIVE 与邮箱验证：`app/iam/service/internal/biz/session.go:54-81`。

### 缺失部分

- User、Authenticator、PasswordAuthenticator 的实际 schema 字段中均无首次改密标记。
- 新 seed 创建入口为 `app/iam/service/internal/biz/user_initializer.go:79-101`，当前直接创建已激活且已验证的用户；管理创建也未写入首次改密要求。
- 当前登录接口、普通会话校验和页面跳转均无首次改密分支。
- 已有 `ChangePassword` 要求普通登录态，不能原样充当“尚未取得普通登录态”的首次改密交互。

设计采用 User.required_actions JSONB 集合与 AccountService.ChangeInitialPassword，完成请求重新提交邮箱、当前密码、新密码，不保存待改密认证证明；成功后建立普通登录并继续原 OIDC 或账号页。接口、错误映射与 Repo 签名见 [设计第 7–10 节](../design.md) 和 [凭据调用设计](credential-implementation-design.md)。当前产品代码仍待实施。

## 5. 验收依据

| 能力 | 以后实施时必须证明的行为 |
| --- | --- |
| 管理设密 | 无本人旧密码/恢复令牌可执行；旧密码失效；邮箱验证事实与已有首次改密要求不被改变；旧 IAM 登录和 refresh token 失效。 |
| 强制登出 | 同一用户多设备 IAM 登录及关联 OAuth 会话失效，其他用户和独立服务身份不受影响，密码与封禁状态不变。 |
| 删除/恢复/永久删除 | 未 Expunge 前资料及邮箱一直保留、一直可恢复，旧会话不复活；只允许有独立权限的管理员永久删除已软删除用户，事务成功才释放邮箱，不跨服务清理或改绑身份；恢复和 Expunge 不能同时成功。 |
| 自助删除 | 目标来自可信认证上下文；正确密码才能删除；失败不产生删除或撤销副作用。 |
| 首次改密 | 初始凭据验证后仍无普通访问能力；直接接口、重启、换设备与 OIDC 均无法绕过；失败不清除要求，成功继续原流程。 |

## 6. 当前契约与 schema

依据服务 Proto 和 Ent schema，当前结构如下：

- UserService：`CreateUser`、`GetUser`、`ListUsers`、`UpdateUser`、`DisableUser`、`EnableUser`。
- AccountService：`Register`、`VerifyEmail`、`ResendVerificationEmail`、`GetProfile`、`UpdateProfile`、`ChangePassword`、`RequestPasswordReset`、`ConfirmPasswordReset`。
- AuthnService：`Login`；SessionService：`Logout`。
- `LoginResponse`：`user`；IAM 配置：`bootstrap_user_email`。
- User schema 无 mixin、无删除字段；User、Authenticator、PasswordAuthenticator 均无首次改密标记。

## 7. 后续设计顺序

服务职责和管理动作已确认，详见 [设计第 1–2 节](../design.md)。选用方案 A：现有 UserUsecase 承担管理设密及强制登出，Repo 提供明确事务方法，不新增独立密码领域 usecase。接下来设计删除生命周期与首次改密；管理设密与登录并发的凭据复验接口随登录链继续核对。

管理创建与发信已拆分：CreateUser 只创建并返回 User，Admin 再调用 SendUserVerificationEmail(name)；发信失败保留先前创建结果。公开自助注册和 CAP 重发沿用原行为，内部复用创建和验证邮件能力。具体契约见设计第 3 节，当前代码尚未切换。
