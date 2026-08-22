# Servora Plateau

Servora 的平台微服务集合。每个服务既是可运行的业务系统，也是 Servora 框架抽象的真实使用场景。

## 使用范围

本文件只统一讨论 Servora Plateau IAM 与跨服务安全时使用的词义，不表示已经决定采用某种资源模型、协议实现、生命周期、存储方案或 API。所有设计决策与行为合同以当前 OpenSpec change 及正式 specs 为准。

## Language

### 基础概念

**IAM**：
身份与访问管理领域，涵盖身份认证、凭据、会话、协议客户端、Actor 生命周期及 IAM 自己的访问策略。
_Avoid_：仅指登录页面、仅指用户表、业务 Tenant 管理。

**Authentication / AuthN**：
确认调用者提供的凭据是否有效，并据此确定当前执行 Actor 的过程。Authentication attempt 与短期 Authentication Challenge 不是 IAM Login Session；只有认证成功才能创建 Session。
_Avoid_：Authorization、登录页面、业务角色加载、把未完成认证状态称为 Session。

**Login Identifier**：
用于定位某个 User 的可规范化标识，例如 email、手机号或 username；它回答“尝试认证谁”，不证明调用方控制该身份。
_Avoid_：Authenticator、稳定 User ID。

**Authenticator**：
绑定到 User、可参与证明身份控制权的长期登记实例，例如密码认证器、Passkey 或 TOTP 登记。一个 User 可以拥有多种或多个 Authenticator；共同生命周期与各类型的验证材料是不同概念。
_Avoid_：Login Identifier、短期 Authentication Challenge、IAM Login Session。

**Authentication Challenge**：
一次认证或登记仪式中的短期状态，例如短信验证码或 WebAuthn challenge；成功或过期后即失效，不是长期 Authenticator。
_Avoid_：Login Session、OAuth Authorization Request。

**Authorization / AuthZ**：
判断当前 Actor 是否可以对指定业务资源执行某项操作的过程。
_Avoid_：Authentication、登录、Actor 身份创建。

**Actor**：
跨服务共享的当前执行主体，只包含稳定的 `Type` 与 `ID` 身份信息。Actor 类型固定为 `human`、`service`、`anonymous`；公开请求使用 `anonymous`，后台任务使用具有明确 ID 的 `service`，Actor 不包含 Tenant、Membership、Role、Scope 或业务自定义字段。
_Avoid_：Principal、PrincipalRef、业务 Viewer、业务角色。

**ActorContext**：
具体业务服务对共享 Actor 的本地可信投影，可以内嵌 Actor 并增加 Tenant、Membership、Role、Profile 或其他业务字段；它不是跨服务身份合同，扩展事实由所属业务服务加载或计算。
_Avoid_：全局 Principal、共享 Token 的任意 claims 容器、跨服务通用权限对象。

**AuthorizationTarget**：
授权检查针对的资源对象，由逻辑资源类型与资源 ID 组成；它与执行主体 Actor、业务服务本地 ActorContext 是不同概念。资源 ID 可以是规则声明的固定目标，也可以从 RPC request 的显式字段提取；不得由共享 security 包隐式从 ActorContext 推断。
_Avoid_：把 Actor 当作资源、把 Tenant 自动当作资源、隐式 provider 默认 object ID。

**Provider Subject**：
具体授权后端用于表示执行主体的标识，由服务边界把共享 Actor 显式映射得到。例如 IAM 的 OpenFGA adapter 将 `Actor{Type: human, ID: <iam_user_id>}` 映射为 `user:<iam_user_id>`；provider subject 不是共享 Actor 合同。
_Avoid_：把 Actor Type 直接当作 OpenFGA type、在 biz/service 中拼接后端 subject、把业务 ActorContext 字段加入稳定主体映射。

**IAM Login Session**：
Human Actor 完成一种或多种认证仪式后，由 Plateau IAM 为一个浏览器或用户代理建立的登录状态。它证明当前用户代理已认证，但不是 access token，也不直接授予业务资源权限。
_Avoid_：Access Token、Refresh Token、OAuth Authorization Grant、业务 BFF Session。

**Token Session**：
OIDC/OAuth Client 基于授权结果取得并轮换 token 的生命周期，可被单独撤销，并关联到创建它的 IAM Login Session。
_Avoid_：IAM Login Session、Access Token 字符串。

**Plateau Access Token**：
Plateau IAM 作为唯一 Authorization Server 签发、供 Plateau Resource Server 验证，并在验证后映射为 Actor 的 OAuth access token。
_Avoid_：IAM Login Session、ID Token、私有 Login API TokenPair。

**OIDC UserInfo Profile**：
Human Actor 面向 OIDC UserInfo 与 ID Token 的标准资料投影。字段遵循 OIDC 标准 claim 语义；`sub` 是稳定身份标识，email、preferred_username 等资料字段不承担跨服务 Actor 身份职责。
_Avoid_：Actor 本体、LoginIdentifier、业务用户自定义角色。

**OAuth Authorization**：
Human Actor 授予 OAuthClient 可使用特定 scopes 的协议授权过程，产出 authorization code 或 token。它不同于 OpenFGA 对 IAM/业务资源执行动作的 Authorization。
_Avoid_：OpenFGA ReBAC 检查、Authentication、业务角色本身。

**OpenFGA Authorization**：
共享 AuthZ 引擎将 Actor 映射为 provider subject 后，对明确资源和 action 执行的 ReBAC 检查。它不签发 OAuth token，也不决定 OAuth client scopes。
_Avoid_：OAuth 授权确认、OIDC 登录、从 JWT claims 直接推导平台 admin。

### OAuth 2.0 与 OpenID Connect

**OAuth 2.0**：
授权框架，用于让客户端获得访问受保护资源的权限。其核心输出是面向 RS 的 access token。
_Avoid_：用户身份协议、登录协议的全部内容。

**OIDC**：
建立在 OAuth 2.0 之上的身份层，增加 `id_token`、UserInfo、发现文档等能力。关系为 OIDC ⊂ OAuth 2.0。
_Avoid_：“OAuth 2.0 属于 OIDC”。

**OP**：
实现 OIDC 身份提供能力的一方，负责处理授权请求并签发 `id_token`。
_Avoid_：RP、RS。

**RP**：
向 OP 发起认证请求并消费 `id_token` 的一方。
_Avoid_：RS、浏览器。

**RS**：
接受并校验 access token、保护业务资源的一方。

**OAuthClient**：
OAuth 2.0 协议中在 Authorization Server 注册的软件客户端，由 `client_id` 与客户端认证方式标识；它不是 Actor。授权码流程中，OAuthClient 代表已认证 Human Actor 请求其获准的 scopes；client credentials 等机器流程属于独立能力。
_Avoid_：Service Actor、Resource Server、业务服务整体、Human Actor。

**BFF**：
面向浏览器前端的服务端组件。作为 confidential OAuthClient / OIDC RP 时，BFF 在服务端持有 client secret 与 token，浏览器只持有 BFF 自己的安全 session cookie。一个业务产品可以同时包含作为 OAuthClient 的 BFF 与作为 RS 的业务 API。
_Avoid_：浏览器 SPA、IAM 登录 UI、Resource Server 本身。

**授权确认**：
Human Actor 确认某个 OAuthClient 可取得哪些 scopes 的交互或既有授权策略。它不是身份认证，也不是 OpenFGA 对业务资源的授权检查。
_Avoid_：登录、Membership、业务角色。


### 业务边界

**Tenant**：
由具体业务服务拥有的组织、工作区或其他隔离资源。Tenant 不属于 IAM，也不属于共享 Actor；业务服务通过 Membership 将 Actor 纳入自己的业务边界。
_Avoid_：IAM Realm、身份租户、Actor 类型。

**Membership**：
具体业务服务维护的 Actor 与 Tenant 之间的业务成员关系。Membership 不是 IAM 登录状态，也不由 IAM 统一保存。
_Avoid_：Actor 本体、Credential、Session、全局权限关系。


**Account**：
当前 IAM User 的 self-service facade，组织注册、邮箱验证、资料、Authenticator 与恢复操作；Account 没有独立 ID、资源实体或第二套 User 生命周期。
_Avoid_：第二份用户表、OAuthClient account、管理员 User CRUD。

**User**：
IAM 管理的全局身份资源，是跨业务服务的唯一身份权威。每个 User 拥有稳定、不可复用的唯一 ID；email 是登录与联系标识，在有效 User 范围内全局唯一，可以在新地址验证成功后变更，但不是跨系统主体的永久主键。资料字段仅为 OIDC 标准 claim，不承载业务自定义字段。
_Avoid_：Human Actor（运行时执行主体）、业务 profile、Tenant Member、Membership。

**Human Actor**：
`Actor.Type = human` 的运行时执行主体；其 ID 使用对应 IAM User 的稳定 ID。具体授权 adapter 再将它映射为 provider subject；IAM OpenFGA 使用 `user:<iam_user_id>`，而不是由 Actor Type 决定 OpenFGA type 名。通常由 Authenticator 或 ExternalIdentity 认证产生，并由 IAM 管理其身份生命周期。
_Avoid_：User Profile、Membership、Tenant Member、OpenFGA type 定义。

**Service Actor**：
`Actor.Type = service` 的非人类执行主体，代表服务、工作进程、定时任务或自动化；它由 IAM 管理机器凭据或由受信任的内部装配创建。
_Avoid_：Workload 实例、system Actor、OAuthClient。

**Workload**：
以某种身份运行的软件单元，可以是服务、工作进程、定时任务或一组副本。需要平台授权时，Workload 使用具有明确 ID 的 Service Actor 表达稳定执行身份。
_Avoid_：Human Actor、Service Actor 本身、业务领域。

**Credential**：
Authenticator 用于验证身份控制权的敏感材料或可验证表示，例如密码派生结果、Passkey 公钥或 TOTP secret。Credential 不是 User、Authenticator 的生命周期记录，也不是短期 Authentication Challenge。
_Avoid_：Actor、Login Identifier、Authenticator、Session。


### 外部身份

**External IdP**：
IAM 可连接的外部身份提供方，例如企业目录、云身份服务或社交身份提供方。
_Avoid_：Human Actor、OAuthClient。

**IdP Broker**：
在外部身份提供方与本地 IAM 之间协调认证及身份映射的职责。
_Avoid_：External IdP 本身、业务授权系统。

**ExternalIdentity**：
外部身份提供方中的主体与本地 Human Actor 之间的绑定概念。绑定唯一键是 `issuer + subject`；email 只是外部 profile claim，不用于无条件自动合并本地 User。信任规则与绑定流程属于 IAM 设计事项。
_Avoid_：email 匹配结果、Human Actor、业务用户档案。

**JIT Provisioning**：
外部身份首次登录时按策略即时创建或接入本地 Human Actor 的过程。
_Avoid_：自动账号合并、无条件注册。

### 工作负载身份

**mTLS**：
客户端与服务端双方使用证书完成 TLS 身份验证的机制。
_Avoid_：仅服务端 TLS、OAuthClient secret。

**SPIFFE ID**：
SPIFFE 信任域内用于标识 workload 的 URI。
_Avoid_：User ID、证书本身。

**SVID**：
Workload 用于证明 SPIFFE ID 的短期可验证身份文档，可以是 X.509-SVID 或 JWT-SVID。
_Avoid_：Service Actor 资源、OAuth access token。
