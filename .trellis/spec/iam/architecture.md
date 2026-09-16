# IAM 应用架构与跨端边界

IAM 是身份目录、认证会话与 OIDC Provider 应用；它验证并投影身份，不拥有业务资源授权。Go service 提供 IAM API、HttpOnly 登录会话、CAP 与 OIDC 协议；Next Web 提供 Provider 自有的 CSR 页面和静态资源。两端经同源公开入口协作，Web 不作为 OAuth relying party 的 BFF，也不保存 OAuth client secret、access token 或另一套前端认证会话。

## 当前同源实现

[`next.config.ts`](../../../app/iam/web/next.config.ts) 的 `rewrites()` 把 `/v1`、`/cap`、`/.well-known`、`/keys`、`/authorize`、`/oauth`、`/userinfo`、`/revoke`、`/end_session` 转发到 `IAM_BACKEND_ORIGIN`。请求通过共享 client 的相对地址发起，浏览器保持 `same-origin` cookie 语义。

更改公开入口时同步 `IAM_PUBLIC_ORIGIN`、Web 端口 10002 和后端代理地址；开发与 preview/start 不能同时占用该端口。非 localhost 环境需要受信任 HTTPS，具体启动边界见 [`AGENTS.md`](../../../app/iam/web/AGENTS.md)。

## 禁止越界

- 不在 IAM Web 中保存 OAuth client secret、access token 或另建一套前端认证会话。
- 不把 Next 改造成代理以外的业务 BFF，也不让页面直连不同 origin 绕过 HttpOnly cookie 和同源防护。
- 其他业务应用是否采用 SSR、服务端运行时或 BFF 由各自需求和实现决定；IAM 不为它们规定实现。

## 跨端契约与证据

- [`configs/local/oidc.yaml`](../../../app/iam/service/configs/local/oidc.yaml) 和 [`configs/local/bootstrap.yaml`](../../../app/iam/service/configs/local/bootstrap.yaml) 都以 `IAM_PUBLIC_ORIGIN` 声明本地公开 issuer 与 external URL；服务端 HTTP 组合与 OIDC Provider 注册见 [`internal/server/http.go`](../../../app/iam/service/internal/server/http.go) 和 [`internal/oidc/provider.go`](../../../app/iam/service/internal/oidc/provider.go)。
- Web 页面目录、生成 API 装配与状态边界见 [前端](frontend.md)；会话撤销和应用退出的现状见 [会话](sessions.md)。

当前 rewrite、配置与服务注册是源码证据；本任务未启动公开入口验证 cookie、CAP 或 OIDC 协议行为。
