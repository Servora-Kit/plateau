# Provider UI 与 BFF 边界

本应用是 OIDC Provider 自有的 UI，不是 OAuth relying party 的 BFF。架构决策见 [`ADR 0004`](../../../docs/adr/0004-separate-iam-op-ui-from-oidc-rp-bff.md)：Go service 负责 IAM API、HttpOnly 登录会话、CAP 和 OIDC 协议；Next 负责页面与资源，通过同源路径转发保持单一公开 origin。

## 当前同源实现

[`next.config.ts`](../../../app/iam/web/next.config.ts) 的 `rewrites()` 把 `/v1`、`/cap`、`/.well-known`、`/keys`、`/authorize`、`/oauth`、`/userinfo`、`/revoke`、`/end_session` 转发到 `IAM_BACKEND_ORIGIN`。请求通过共享 client 的相对地址发起，浏览器保持 `same-origin` cookie 语义。

更改公开入口时同步 `IAM_PUBLIC_ORIGIN`、Web 端口 10002 和后端代理地址；开发与 preview/start 不能同时占用该端口。非 localhost 环境需要受信任 HTTPS，具体启动边界见 [`AGENTS.md`](../../../app/iam/web/AGENTS.md)。

## 禁止越界

- 不在 IAM Web 中保存 OAuth client secret、access token 或另建一套前端认证会话。
- 不把 Next 改造成代理以外的业务 BFF，也不让页面直连不同 origin 绕过 HttpOnly cookie 和同源防护。
- 其他业务应用是否采用 SSR、服务端运行时或 BFF 由各自需求决定；本 ADR 不为它们规定实现。

当前 rewrite 和 CSP 配置存在于源码；本任务没有启动公开入口验证 cookie、CAP 或 OIDC 协议行为。
