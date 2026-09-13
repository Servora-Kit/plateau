# 平台与母框架边界

Plateau 承载平台业务、主要参考应用和具体安全生态；Servora 是独立演进的母框架。规范归属按责任确定，不因两个仓库能在 go.work 中联调就合并职责。

| 内容 | 权威归属 |
| --- | --- |
| 平台总体结构、具体 AuthN/AuthZ、OpenFGA 适配、安全插件 | `plateau` |
| 平台源 Proto、注解声明、共享生成产物 | `api` |
| 平台共享 client | `web` |
| 微服务如何分层和使用 Servora | `service` |
| 母框架自身 CRUD、启动、transport、provider、审计、CLI | `servora` |
| 领域模型、业务流程、应用前端 | `<应用>-service`／`<应用>-web` |

## 身份与业务

IAM 是平台自用的全局身份池，拥有稳定且不可复用的用户 ID、凭据、会话与签发职责。业务 tenant、membership 和资源隔离由各业务服务拥有；业务投影通过 `iam_user_id` 引用 IAM，不复制凭据或向 IAM 填业务资料。email 变化不改变用户 ID。

IAM 自建领域模型，复用 OIDC 协议库；使用协议库不意味着把 IAM 产品生命周期托管给外部 IdP。共享 Actor 只表达已确认执行身份，不能携带业务角色列表冒充实时授权。

## 生成与消费

`@plateau/api` 承载可再生成契约，`@plateau/client` 承载手写 transport 与框架中立错误事实。Cookie/token 持久化、登录跳转和业务文案属于应用；生成目录不容纳手写 runtime。框架内部与业务消费规范通过链接组合，不复制两套规则。

当前内部 Example 是有效参考；Audit 明确停止维护、等待重构。新增服务优先按共通规范和 Example，不能把 Audit 的现状推广为推荐架构。独立 servora-example/servora-transport 的扩展不随平台规范任务自动纳入。

检查：新规则是否有一处权威来源；当前实现是否与既定边界不同；差异是否被显式记录。不要通过规范整理悄悄修改产品范围。