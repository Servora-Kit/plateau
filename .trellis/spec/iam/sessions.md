# 浏览器会话与撤销

- 登录会话是可撤销认证事件，cookie 与期限由 SCS 管理；从有效服务端登录引用解析身份并投影 `security.Actor`，下游只通过统一认证入口读取，不伪造 context 或仅凭 cookie 声称已认证。
- 拒绝已撤销会话、缺失、软删除、禁用、非活跃、未验证或存在待完成 `required_actions` 的身份；无效凭据与依赖不可用分别处理。`Login` 返回待完成 `required_actions` 时，若当前浏览器已绑定登录引用则 `Destroy` 当前 SCS 会话，匿名请求保持无 cookie，不创建空会话或登录记录。
- 撤销在事务中锁定身份并同时失效对应登录及关联 OAuth token session，不仅删除 cookie 或 access token。
- 密码恢复撤销全部登录和 OAuth 会话；已认证改密保留当前登录，撤销其他登录及所有 OAuth 会话。
- Provider 登录退出撤销当前登录及关联 OAuth 会话并销毁 SCS 会话；OIDC `/end_session` 仅撤销客户端关联 OAuth 会话，不等同于 Provider 登录退出。
- `NewHTTPServer` 通过 `security/session.NewFilter` 声明公开入口 origin、`/v1/iam/`（JSON 前缀）、`/authorize/callback`（同源回调路径，仅允许 `GET`）与 `/authorize`、`/end_session`（跨站协议路径）：非安全跨源请求与非 JSON 写请求在装载会话前即被拒绝（403/415），协议令牌/JWKS/健康检查路径不装载浏览器会话。
- 未接入退出通知时，不得声称已终止业务应用本地会话；跨应用注销必须覆盖各自会话所有者。
