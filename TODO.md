# TODO - plateau

## Cookie 会话与 HTTP 安全

- [ ] 整理 Go 后端可复用的浏览器 Cookie 会话安全接线。
  - 基于现有 `security/authn/session`，隔离业务逻辑与 Cookie 读写、安全属性、CSRF、Origin/Referer 校验及 CORS 配置。
  - 通用机制统一维护，应用只声明公开入口与保护范围；移除来源防护对 OIDC Provider 对象的直接依赖。
  - 优先评估 Go 标准库 `http.CrossOriginProtection`，明确缺失来源头、可信来源及反向代理场景的策略，避免迁移时悄然放宽防护。
  - 区分 Cookie 会话接口、Bearer/mTLS 接口与 OIDC 协议端点，不给所有微服务一刀切挂载。
  - 保留来源拒绝无业务副作用、登录 CSRF、会话生命周期等行为回归；CORS 不能替代 CSRF 防护，业务授权规则仍由业务定义。
