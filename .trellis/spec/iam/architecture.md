# Provider 跨端与公开入口

服务端提供身份 API、HttpOnly 登录会话、CAP 和 OIDC 协议；Web 仅提供 Provider 自有 CSR 界面，不作为 OAuth relying party 的 BFF。

- Web 将 API、CAP 与 OIDC 公开路径同源转发到 `IAM_BACKEND_ORIGIN`，请求使用相对地址和 `same-origin` cookie 语义，不直连其他 origin 绕过同源防护。
- 公开入口调整须保持 `IAM_PUBLIC_ORIGIN`、issuer、external URL 与代理目标一致；非 localhost 环境使用受信任 HTTPS。
- Web 不持有 OAuth client secret、access token 或另一套认证会话；其他应用的 SSR/BFF 选择不由本层规定。
- 页面与请求职责见 [前端](frontend.md)，协议与撤销边界见 [OIDC](oidc.md) 和 [会话](sessions.md)。
