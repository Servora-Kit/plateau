# Provider 跨端与公开入口

服务端提供身份 API、HttpOnly 登录会话、CAP 和 OIDC 协议；Web 仅提供 Provider 自有 CSR 界面，不作为 OAuth relying party 的 BFF。

- 本地通过边缘网关（Traefik：`iam.service` 经 `registry.consul.tags` 声明 API、CAP、OIDC 与 `/docs` 公开路径直转 `:10000`，`manifests/traefik/routes.yaml` 将其余页面请求兜底转发至 `iam.web:10002`，不暴露 `/metrics`、`/healthz`、`/readyz`）；`iam.web` 同时保留 Next.js 同源 rewrite 至 `IAM_BACKEND_ORIGIN` 作为脱离网关单调时的回退能力。浏览器请求统一使用相对地址和 `same-origin` cookie 语义，不直连其他 origin 绕过同源防护。
- 公开入口调整须保持 `IAM_PUBLIC_ORIGIN`、issuer、external URL 与网关/代理目标一致；本地默认使用 RFC 6761 回环域名 `http://iam.plateau.localhost:8080`，仅合法回环主机（`security.IsLoopbackHost`：IP 回环字面量、`localhost` 及非空 `*.localhost`）允许明文 `http://`，其他环境强制使用受信任 HTTPS。
- Web 不持有 OAuth client secret、access token 或另一套认证会话；其他应用的 SSR/BFF 选择不由本层规定。
- 页面与请求职责见 [前端](frontend.md)，协议与撤销边界见 [OIDC](oidc.md) 和 [会话](sessions.md)。
