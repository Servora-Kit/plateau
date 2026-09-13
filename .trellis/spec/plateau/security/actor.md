# 执行主体 Actor

适用于认证结果、授权输入和进程内身份传递。Actor 的权威定义是 [actor.go](../../../../security/actor.go)，不是业务用户对象或角色快照。

## 不变量

- `human` 和 `service` 必须有非空 ID；`anonymous` 的 ID 必须为空；其他类型无效。
- `WithActor` 只保存 context 值，不验证凭据、不授权、不查询业务事实。
- `ActorFrom` 对 nil context、缺失或无效 Actor 返回失败；调用方必须检查 bool。
- AuthN 的应用 mapper 只在凭据已验证后映射 Actor。认证成功不能返回 anonymous；PUBLIC 路由显式写入 anonymous。
- 人类 ID 与服务 ID 的语义由各应用的可信 mapper 确定。不能把浏览器传入的角色、Header 或任意用户 ID 经 WithActor 直接变为可信身份。

```go
actor, ok := security.ActorFrom(ctx)
if !ok || actor.Type == security.ActorTypeAnonymous {
    // 由当前层按公开错误契约拒绝需要认证的操作。
}
```

AuthZ 把 Actor 映射为具体引擎 subject；tenant/membership 等仍由业务和授权模型拥有。Session 的 ContextExtender 可以附加业务可信状态，但不得改变共享 Actor。

检查：无效 Type/ID 组合、匿名路由覆盖旧 context、mapper 失败与 extender 改写 Actor。依据 [actor_test.go](../../../../security/actor_test.go)、[session AuthN](../../../../security/authn/session/authn.go)、[AuthZ](authZ.md)。
