# Servora Platform

Servora 的平台微服务集合。每个服务既是可运行的业务系统，也是 Servora 框架抽象的真实使用场景。

## 使用范围

本文件只统一讨论 Servora Platform IAM 与跨服务安全时使用的词义，不表示已经决定采用某种资源模型、协议实现、生命周期、存储方案或 API。所有设计决策与行为合同以当前 OpenSpec change 及正式 specs 为准。

## Language

### 基础概念

**IAM**：
身份与访问管理领域，涵盖身份认证、凭据、会话、协议客户端、Actor 生命周期及 IAM 自己的访问策略。
_Avoid_：仅指登录页面、仅指用户表、业务 Tenant 管理。

**Authentication / AuthN**：
确认调用者提供的凭据是否有效，并据此确定当前执行 Actor 的过程。
_Avoid_：Authorization、登录页面、业务角色加载。

**Authorization / AuthZ**：
判断当前 Actor 是否可以对指定业务资源执行某项操作的过程。
_Avoid_：Authentication、登录、Actor 身份创建。

**Actor**：
跨服务共享的当前执行主体，只包含稳定的 `Type` 与 `ID` 身份信息。Actor 类型固定为 `human`、`service`、`anonymous`；公开请求使用 `anonymous`，后台任务使用具有明确 ID 的 `service`，Actor 不包含 Tenant、Membership、Role、Scope 或业务自定义字段。
_Avoid_：Principal、PrincipalRef、Credential、业务 Viewer、业务角色。

**ActorContext**：
具体业务服务对共享 Actor 的本地可信投影，可以内嵌 Actor 并增加 Tenant、Membership、Role、Profile 或其他业务字段；它不是跨服务身份合同，扩展事实由所属业务服务加载或计算。
_Avoid_：全局 Principal、共享 Token 的任意 claims 容器、跨服务通用权限对象。

**AuthorizationTarget**：
授权检查针对的资源对象，由逻辑资源类型与资源 ID 组成；它与执行主体 Actor、业务服务本地 ActorContext 是不同概念。资源 ID 可以是规则声明的固定目标，也可以从 RPC request 的显式字段提取；不得由共享 security 包隐式从 ActorContext 推断。
_Avoid_：把 Actor 当作资源、把 Tenant 自动当作资源、隐式 provider 默认 object ID。

**IAM Login Session**：
Human Actor 完成一种或多种认证仪式后，由 Platform IAM 建立的登录状态。它证明浏览器或用户代理当前已认证，但不是 access token，也不直接授予业务资源权限。
_Avoid_：Access Token、Refresh Token、OAuth Authorization Grant。

**Token Session**：
OIDC/OAuth Client 基于授权结果取得并轮换 token 的生命周期，可被单独撤销，并关联到创建它的 IAM Login Session。
_Avoid_：IAM Login Session、Access Token 字符串、Credential。

**Platform Access Token**：
Platform IAM 作为唯一 Authorization Server 签发、供 Platform Resource Server 验证，并在验证后映射为 Actor 的 OAuth access token。
_Avoid_：IAM Login Session、ID Token、私有 Login API TokenPair。

**OIDC UserInfo Profile**：
Human Actor 面向 OIDC UserInfo 与 ID Token 的标准资料投影。字段遵循 OIDC 标准 claim 语义；`sub` 是稳定身份标识，email、preferred_username 等资料字段不承担跨服务 Actor 身份职责。
_Avoid_：Actor 本体、LoginIdentifier、业务用户自定义角色。

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
OAuth 2.0 协议中已注册的客户端，不天然是 Actor；它可以绑定 Human Actor 或 Service Actor，以对应身份发起授权或 client credentials 流程。
_Avoid_：Service Actor、Workload、Human Actor。

### 跨服务认证与 claims 所有权

每个 token issuer 可以在 JWT Registered Claims 基础上定义自己的 claims 扩展。IAM、CMS、Mall 或其他服务各自拥有自己签发 token 的 claims schema；`security/authn/jwt` 不枚举所有 issuer 的业务 claims，也不提供统一业务 claims struct。当前不为 IAM claims 建立 Platform 共享 Proto 或 `api/gen` claims type。Resource Server 为自己消费的 token 定义本地最小 claims decoder，并遵守 issuer 发布的必要 wire semantics。

CMS、Audit、Admin 等业务服务通常只需要已验证的 Actor 和本服务自己的 Tenant、Membership、Role、资源关系或审计事实，很少需要 IAM token 的完整业务 claims。需要解析 JWT 时，各服务只读取并验证自身所需的标准 claims 与最小 issuer-specific 字段，不导入 IAM 内部 claims 类型。

默认采用“网关预检 + Resource Server 本地验证”：网关可以提前拒绝明显无效请求，但业务 Resource Server 仍需本地校验 JWT 的 issuer、audience、签名、KID、token type 和有效期，不信任网关注入的明文身份 header。opaque token 或即时撤销要求可以使用受保护的 IAM introspection；每请求 introspection 不是默认路径。

网关唯一认证边界只有在内部流量经过受信服务认证且身份 assertion 具备完整性保护时才成立。服务代表 Human Actor 调用下游服务时，后续 IAM change 再决定 token exchange、delegation 或 impersonation；当前共享 Actor 只表达单一执行主体。

### 业务边界

**Tenant**：
由具体业务服务拥有的组织、工作区或其他隔离资源。Tenant 不属于 IAM，也不属于共享 Actor；业务服务通过 Membership 将 Actor 纳入自己的业务边界。
_Avoid_：IAM Realm、身份租户、Actor 类型。

**Membership**：
具体业务服务维护的 Actor 与 Tenant 之间的业务成员关系。Membership 不是 IAM 登录状态，也不由 IAM 统一保存。
_Avoid_：Actor 本体、Credential、Session、全局权限关系。

**Human Actor**：
`Actor.Type = human` 的执行主体，通常由 Human Credential 或 ExternalIdentity 认证产生，并由 IAM 管理其生命周期。
_Avoid_：User Profile、Membership、Tenant Member。

**Service Actor**：
`Actor.Type = service` 的非人类执行主体，代表服务、工作进程、定时任务或自动化；它由 IAM 管理机器凭据或由受信任的内部装配创建。
_Avoid_：Workload 实例、system Actor、OAuthClient。

**Workload**：
以某种身份运行的软件单元，可以是服务、工作进程、定时任务或一组副本。需要平台授权时，Workload 使用具有明确 ID 的 Service Actor 表达稳定执行身份。
_Avoid_：Human Actor、Service Actor 本身、业务领域。

**Credential**：
主体用于证明身份控制权的材料，例如密码、密钥、证书或一次性验证码。Credential 不是 Actor，也不是 ActorContext。
_Avoid_：Actor、Login Identifier、Session。

### 外部身份

**External IdP**：
IAM 可连接的外部身份提供方，例如企业目录、云身份服务或社交身份提供方。
_Avoid_：Human Actor、OAuthClient。

**IdP Broker**：
在外部身份提供方与本地 IAM 之间协调认证及身份映射的职责。
_Avoid_：External IdP 本身、业务授权系统。

**ExternalIdentity**：
外部身份提供方中的主体与本地 Human Actor 之间的绑定概念。唯一键、信任规则与绑定流程属于 IAM 设计事项。
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
