# 浏览器会话与撤销

- 登录会话是可撤销认证事件，cookie 与期限由 SCS 管理；从有效服务端登录引用解析身份并投影 `security.Actor`，下游只通过统一认证入口读取，不伪造 context 或仅凭 cookie 声称已认证。
- 拒绝已撤销会话、缺失、禁用、非活跃或未验证身份；无效凭据与依赖不可用分别处理。
- 撤销在事务中锁定身份并同时失效对应登录及关联 OAuth token session，不仅删除 cookie 或 access token。
- 密码恢复撤销全部登录和 OAuth 会话；已认证改密保留当前登录，撤销其他登录及所有 OAuth 会话。
- Provider 登录退出撤销当前登录及关联 OAuth 会话并销毁 SCS 会话；OIDC `/end_session` 仅撤销客户端关联 OAuth 会话，不等同于 Provider 登录退出。
- 未接入退出通知时，不得声称已终止业务应用本地会话；跨应用注销必须覆盖各自会话所有者。
