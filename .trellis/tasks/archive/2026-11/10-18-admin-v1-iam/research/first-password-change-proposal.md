# 首次改密：设计依据与调用关系

对应父 PRD §3、§5、§11。业务合同见 [design.md](../design.md) 第 7–10 节，具体签名见 [凭据调用设计](credential-implementation-design.md)。采用 User.required_actions JSONB 与再次提交当前密码的独立完成方法；不引入待改密证明，普通登录保持现有 SCS 接线。

## 1. 现有 PRD 约束与源码现状

- 新 seed 必须首次改密；管理创建默认要求首次改密，管理员可显式关闭。前端默认勾选，IAM CreateUser 负责未提供时默认开启并尊重显式关闭，具体职责见设计第 3 节；自助注册和无未完成要求的已有账号不新增此要求。
- 对有首次改密要求的用户，通过首次改密流程时须满足账号状态和邮箱验证要求、验证初始密码并设置不同的新密码；也可通过现有有效邮箱密码重置设置不同于当前密码的新密码来完成要求。完成前不能取得普通业务登录态或完成 OIDC 授权；没有要求的管理创建用户在邮箱验证和状态允许后可直接用管理员设置的密码登录。
- 管理设密不清除未完成的首次改密要求；软删除恢复也不改变它。
- 邮箱密码重置完成 `UPDATE_PASSWORD` 的规则已确认，见 [设计第 7 节](../design.md) 与父 PRD §11；早期“普通密码恢复保留该要求”的候选不采用，动作清除与密码替换必须共同提交。
- 当前 `AuthenticationUsecase.Login` 验证密码后直接创建普通登录（`app/iam/service/internal/biz/authn.go:42-72`）；`AuthnService.Login` 旋转 SCS token 并写入 `iam_login_id`（`internal/service/authn.go:30-45`）。
- 认证器通过 `iam_login_id` 解析普通登录并建立 human Actor（`internal/authn/session.go:14-34`）；OIDC 回调也要求该登录引用（`internal/oidc/provider.go:196-226`）。
- 当前 PasswordAuthenticator schema 只有 id、authenticator_id、password_hash、changed_time（`internal/data/schema/password_authenticator.go:16-22`）；LoginResponse 只有 user（`api/protos/iam/authn/v1/authn.proto:50-52`）。
- `AccountService.ChangePassword` 要求普通身份和登录引用（`internal/service/account.go:110-126`）；Repo 检查有效 keepLoginID（`internal/data/authn.go:54-82`），保持该契约，登录前首改使用 ChangeInitialPassword。
- 当前 `NewHTTPSessionManager` 使用 `postgresstore.New(db)`（`internal/data/data.go:113-124`）；IAM 显式 Put 的流程状态由 SCS LoadAndSave 经 Store 写入 PostgreSQL 的 sessions 表。SCS 本身支持替换 Store，PostgreSQL 是当前应用的选择。

## 2. Keycloak 模型与本项目可借鉴部分

### 2.1 持久化的用户要求

Keycloak 的 UserEntity 通过 OneToMany 持有 UserRequiredActionEntity 集合，动作保存于 USER_REQUIRED_ACTION 表，属于用户模型。管理员可配置 UPDATE_PASSWORD 等要求；管理侧设置 temporary 密码也可以加入 UPDATE_PASSWORD。requiredActions 是业务要求的持久化来源，用户换设备或认证过程超时不应解除该要求。

requiredActions 只属于 IAM User，不做跨实体通用化；公开 User 返回枚举集合，当前仅支持 UPDATE_PASSWORD。创建输入及 User JSONB 存储已确认，见 [设计第 3、7 节](../design.md)；不并列维护同义 must_change_password，也不复制 Keycloak 的完整动作框架。

#### 已确认公开类型与创建输入

- User.required_actions 已确认采用枚举集合，Proto 为 repeated UserRequiredAction，Go 对应枚举切片；无重复、不依赖顺序，仅支持 UPDATE_PASSWORD，UNSPECIFIED 零值不作为动作。当前代码尚未实现，具体定义以设计第 7 节为准。
- 存储已选择 users.required_actions JSONB 集合，稳定动作名与公开枚举显式映射，不使用动作关联实体。
- 创建输入已选择 CreateUserRequest.optional require_password_change，复选框只控制是否加入 UPDATE_PASSWORD；bool 仅作为请求选项，不增加同义持久化属性。
- 未提供及 true 表示要求，显式 false 表示不要求；字段存在性沿 Admin Web → Admin 后端 → IAM 保留。
- [Proto 字段存在性](https://protobuf.dev/programming-guides/field_presence/)：裸 repeated 字段不能区分未提供和显式空集合。已确定的缺省开启/显式关闭规则要求创建契约保留该差异；不能在读取到空列表时一律补入 UPDATE_PASSWORD。

#### 创建与存储方案已确认

详细契约转入 design.md 第 3、7 节：CreateUserRequest 增加 `optional bool require_password_change = 4`；User 动作采用 JSONB 字符串集合。未选择通用初始动作包装或 Keycloak 式关联表。选择依据为当前只有一个创建选项，IAM 已有 JSON scopes/amr 集合（internal/data/schema/oauth_client.go:19-23、oauth_token_session.go:23-25），且未要求动作级查询、元数据或生命周期。

在 userRepository mapper/post-hook 增加字符串与枚举映射；PostgreSQL 新列使用空 JSON 数组作为默认值并用于旧行；动作改变同步更新 User.etag/update_time，不将 etag 用作密码凭据版本。沿用 NewDBClient 的 Schema.Create（internal/data/data.go:68-78）。

#### 独立动作与密码重置事务

首次改密使用 AccountService.ChangeInitialPassword，成功响应为空。请求提交 email、current_password、new_password，每次重新验证本人凭据，不依赖前一次 Login；HTTP 路由及具体流程见 [设计第 8 节](../design.md)。管理设密继续使用 UserService.SetUserPassword。

现有 ConfirmPasswordReset 先在 biz 生成新 hash，再调用 ConsumeAndReplacePassword（internal/biz/account.go:221-234）；data 只接收新 hash，随后在用户锁内消费令牌、替换密码、撤销会话（internal/data/account.go:108-175）。要判断有 UPDATE_PASSWORD 时新密码与旧密码不同，必须拿到当前凭据并在业务层调用 password.Compare；不能直接比较带不同随机盐的两个 hash，也不把明文送进 Repo。

采用“读取待重置快照 + 原子提交”：具体 PasswordResetProof、Peek 和 ConsumeAndReplacePassword 签名见凭据调用设计 §6。biz 比较及计算 hash，事务内复验用户、token 和凭据；无 UPDATE_PASSWORD 时保持现有同密码处理语义。

首次改密的密码替换、动作移除、会话撤销及新普通登录记录在用户锁内共同提交，完成后绑定浏览器会话，见设计第 8 节。此路径不使用普通 ChangePassword 的 keepLoginID，也不新增待改密认证证明。


### 2.2 认证过程模型参考

Keycloak 独立维护 AuthenticationSessionModel，认证过程完成或过期时销毁；LoginActionsService.processRequireAction 会验证认证流程、code 与 action。该模型用于参考，不纳入本任务的首次改密实现。

Plateau 的 requiredActions 存在 User JSONB 中；首次改密通过重新提交当前密码验证本人，无待改密 token 或会话。普通登录建立后继续使用现有 SCS 浏览器绑定。

#### 登录衔接选择结果

现有证据：AuthnService.Login 在 biz 返回普通 LoginSession 后写 iam_login_id（internal/service/authn.go:30-45）；SessionUsecase.Resolve 接受该引用并核验当前用户（internal/biz/session.go:54-82）；OIDC completeAuthorization 要求有效普通登录（internal/oidc/provider.go:196-226）。IAM 登录页在收到结果后立即清除密码，再继续授权或账号页（web/app/login/page.tsx:42-58）。GitNexus 对 Login 的引用结果为 lower-bound，存在接收者类型未解析的调用点；以读取源码为当前流程证据，不据此宣称已穷尽调用者。

用户已选择第二步再次输入初始密码：完成请求携带邮箱、当前密码、新密码，独立核验后修改密码并继续原登录/OIDC。首次改密不保存中间认证状态；只输入一次密码的短时证明方案不采用。接口处理、事务与页面行为见设计第 8 节，设计文档不保留候选对比或讨论过程。


### 2.3 UserService 与 AccountService 的共享

- 当前 IAM gRPC server 只注册 UserService，通过服务 JWT 和 OpenFGA 验证管理调用，不加载目标用户的浏览器 SCS（internal/server/grpc.go:22-45）。
- 管理创建仍通过 UserService.CreateUser 提交初始密码和是否要求用户改密；选择要求时在同一创建事务中记录用户动作，不需要目标用户已有会话。已有用户管理设密使用已确认但未实现的 UserService.SetUserPassword。
- AccountService 的本人动作使用同一 IAM 用户及凭据数据，在核验本人资格、业务前提并成功更新密码后，清除相应 UPDATE_PASSWORD 要求，保留其他动作。
- 两个服务当前已经共享 UserRepo；见 internal/biz/user.go:19-35。它们是同一 IAM 服务中的两个接口服务，无需为复用同一用户状态先建立跨实体 mixin。
- 可共用领域数据访问、密码规则和事务操作；管理调用与本人调用的授权证明分别处理，不以统一存储为理由省略其中一种校验。

首次改密的接口名称、请求字段及登录衔接见设计第 8 节；错误映射、数据迁移和部分 Repo 签名继续细化。上述设计尚未实施。

## 3. Ent 复用范围已确定

当前只有 User 有 requiredActions 需求，用户明确不做预先通用化。直接在 IAM 用户模型中设计动作状态，UserService 与 AccountService 共用该模型；不创建 RequiredActionsMixin，不向其他实体注入字段或动作行为。

Ent Mixin 支持字段、关联、索引、Hooks、Policy 和 Annotations 的复用，这是技术能力参考；本需求没有第二个实际复用实体，不据此引入抽象。存储已确定为 User JSONB 集合。

## 4. 文献与源码依据

- [Keycloak UserEntity](https://github.com/keycloak/keycloak/blob/main/model/jpa/src/main/java/org/keycloak/models/jpa/entities/UserEntity.java) 与 [UserRequiredActionEntity](https://github.com/keycloak/keycloak/blob/main/model/jpa/src/main/java/org/keycloak/models/jpa/entities/UserRequiredActionEntity.java)：用户的一对多关联、USER_REQUIRED_ACTION 表及动作字段。
- [Keycloak UserResource](https://github.com/keycloak/keycloak/blob/main/services/src/main/java/org/keycloak/services/resources/admin/UserResource.java)：UserRepresentation.requiredActions 与 temporary 密码触发 UPDATE_PASSWORD 的管理路径。
- [Keycloak caching guide](https://github.com/keycloak/keycloak/blob/main/docs/guides/server/caching.adoc)：认证过程使用 authenticationSessions，完成或过期后销毁；用户持久化资料和普通登录会话有不同生命周期。
- [Keycloak LoginActionsService](https://github.com/keycloak/keycloak/blob/main/services/src/main/java/org/keycloak/services/resources/LoginActionsService.java)：processRequireAction 校验流程证明并使用 AuthenticationSessionModel，所需动作列表不能替代执行者的认证。
- [Keycloak required actions](https://github.com/keycloak/keycloak/blob/main/docs/documentation/server_admin/topics/users/con-required-actions.adoc) 与 [AIA](https://github.com/keycloak/keycloak/blob/main/docs/documentation/server_admin/topics/users/con-aia.adoc)：登录时的必需动作与应用主动发起的动作；未据此选择 Plateau 的通用执行器。
- [Ent mixin](https://github.com/ent/ent/blob/master/doc/md/schema-mixin.md)：可组合的 schema 声明能力及各实体分别生成字段的示例。
- [Ent JSON 字段与默认值](https://github.com/ent/ent/blob/master/doc/md/schema-fields.mdx)：JSON slice、客户端默认与 SQL 默认定义；本仓 OAuthClient/OAuthTokenSession 已采用该字段类型。

## 5. 设计落点

- 公开字段、迁移和错误映射：design.md 第 9–10 节。
- 凭据快照、SessionRepo 返回当前 User、首改和邮箱重置事务：凭据调用设计第 4–7 节。
- 业务验收和实施顺序：[implement.md](../implement.md)。
- 任务保持 planning，整体规划待用户评审；没有修改产品代码。

## 6. 首次改密之外的后续设计

- IAM：落实永久删除独立权限与关联清理事务、登录并发的 Repo 接线、错误转换及字段迁移。删除／恢复仅传 name、重复删除返回 NOT_FOUND、SCS 按 TTL 清理，以及创建与独立发信的两步编排均已确定，见 design.md 第 3–6 节。
- Admin 后端：登录接入、本地会话、唯一管理员识别、自我保护、IAM client 装配、对 Web 的接口与错误映射。
- Admin Web：在真实接口约束下明确总览、用户列表/详情、创建和生命周期操作的页面及状态反馈。
- IAM 已补齐 design.md、implement.md 与上下文清单；Admin 后端和 Web 继续按各自 PRD 设计，最后进行父任务跨服务集成评审。

