# IAM Web Next 运行与 OIDC Provider 边界

> Status: accepted

`iam/web` 是 IAM OIDC Provider 的同源交互界面，采用 Next.js 原生开发、构建和生产服务，页面业务交互保持浏览器 CSR；IAM Go service 负责 IAM API、HttpOnly 登录会话、CAP 与 OIDC 协议。Next 提供页面及资源，并通过原生路径转发维持单一公开 origin；前端与 Go 独立发布，不增加第二套认证会话或业务 BFF。

## Considered Options

- **在 IAM Go service 中嵌入 Web 资源**：不采用。前端独立运行与发布，Go service 专注 API、CAP 与 OIDC。
- **使用 Next 静态导出**：不采用。优先使用 Next 原生开发热重载、路由转发和生产预览，不围绕导出产物自建代理或启动服务。
- **让 IAM Web 直接成为 OAuth client**：不采用。IAM Web 只承载 Provider 自有登录交互，不接触 OAuth client 密钥和 token。

## Consequences

- 页面资源和 Go 接口可以独立部署，但公开入口必须保持同一个 origin，以继续使用 HttpOnly Cookie 和同源请求防护。
- IAM OIDC 协议由 Go 测试验证，Web smoke 只验证页面交互和 Provider 登录导航。
- 未来业务应用是否使用 SSR、服务端运行时或 BFF，按具体需求另行决定。
