# Platform 使用统一 Actor 作为跨服务执行主体

> Status: accepted

Platform 安全 runtime 采用一个共享的 `security.Actor` 作为跨服务当前执行主体，不再公开 `Principal`、`PrincipalRef` 或分别维护 AuthN/AuthZ 主体上下文。Actor 只表达稳定的 `Type` 与 `ID`，固定支持 `human`、`service`、`anonymous`；公开请求使用 `anonymous`，后台任务使用具有明确 ID 的 `service`，不引入无归属的 `system` Actor。

## 参考事实

- Ech0 使用 `pkg/viewer.Context` 作为唯一请求身份视图，认证中间件写入 `UserViewer`，可选认证使用 `NoopViewer` 表达匿名；它没有 Principal 类型。
- GoWind Admin 没有 Principal 或 Actor，而是并行维护已验证 `UserTokenPayload`、Ent `UserViewer/SystemViewer` 与操作人 Metadata，说明缺少单一主体模型会产生重复上下文。
- 当前 Platform `security/authz` 依赖 `security/authn.SubjectFrom`，而 OpenFGA Adapter 又适配通用 `Authorizer`；统一 Actor 可以消除兄弟包之间的身份模型依赖，同时不把业务 Tenant 或角色提升为共享字段。

## 决策

- `security/authn/*` 负责验证凭据并写入 Actor；`security/authz/*` 读取 Actor，不定义第二套主体模型。
- `security/actor.go` 提供共享 Actor 类型与 context 读写 API。由于当前 Platform 由单一维护者负责，公开 `WithActor` 是可接受的工程取舍。
- Actor 不包含 Tenant、Membership、Role、Scope、Permission 或业务自定义字段。
- IAM、CMS、Mall 等业务服务可以定义自己的 `ActorContext`，内嵌 `security.Actor` 并加载本服务的 Tenant、Membership、Role、Profile 等可信业务事实；这些扩展不属于跨服务身份合同，也不直接作为共享 token claims 传递。
- `security/authz/openfga` 正确依赖 `security.Actor`，负责 Actor 到 OpenFGA subject 的映射；`infra/openfga` 保持独立，只负责 SDK client 与组件接线，不理解 Actor 或业务关系。

## 考虑过的方案

- **全局 Principal + PrincipalRef + Actor 嵌套**：拒绝。当前参考业务使用 Viewer 或已验证 TokenPayload，不需要三层身份对象；嵌套模型增加命名和映射成本。
- **AuthN 与 AuthZ 各维护自己的 context/principal**：拒绝。会重复表达当前操作人，并使 AuthZ 不必要地依赖 AuthN 包。
- **把 Tenant、Role、Membership 放进共享 Actor**：拒绝。没有多租户的服务会承担无关字段；同一 Actor 在不同业务中的关系也不应变成全局身份属性。
- **引入 system Actor**：暂缓并默认拒绝。后台任务使用明确 ID 的 Service Actor，保证动作可归属和可审计。

## 后果

共享 Actor 成为所有 Platform 微服务的最小身份合同。业务服务可以自由扩展本地 `ActorContext`，但必须自行从可信数据源加载扩展事实；基础 Actor 的 `Type/ID` 在跨服务间保持稳定。公开请求的匿名表示和 OpenFGA 对匿名主体的处理需要在后续 AuthZ specs 中明确为 fail-closed 语义。
