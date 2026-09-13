# IAM Web 前端

适用于 `app/iam/web/`。IAM Web 是 IAM OIDC Provider 的同源交互界面：页面交互在浏览器 CSR 执行，IAM Go service 提供 API、HttpOnly 登录会话、CAP 与 OIDC 协议。它不是 OAuth client，也不保存 client secret 或 token。

## 开发前检查

- 读取 [架构](architecture.md) 确认 App Router 目录、组件和 hook 的职责。
- 认证、注册、账户或一次性链接变更读取 [认证流程](auth-flows.md)；涉及公开入口、rewrite 或 OIDC 路径读取 [同源边界](bff.md)。
- 请求改动同时读取共享 [Web client](../web/client/index.md) 的 HTTP 规则，并检查生成 API 与 `lib/iam-api.ts` 的装配。

## 质量检查

- 静态检查：`just web::iam::typecheck` 与 `just web::iam::lint`。
- 构建：`just web::iam::build`；本地预览用 `just web::iam::preview`。它和 dev 都使用 10002，不能同时启动。
- IAM 页面当前未发现前端单元测试文件。认证或路由改动需要以本地运行的 IAM service 做页面 smoke；OIDC 协议是否正确仍由 Go 侧测试验证，不能仅凭页面构建断言端到端完成。

## 主题索引

| 主题 | 内容 |
| --- | --- |
| [架构](architecture.md) | App Router、CSR 页面、组件、hook、状态和生成 API |
| [认证流程](auth-flows.md) | 登录、注册、账户、重置和验证的生命周期 |
| [同源边界](bff.md) | Provider UI、Go service、Next rewrite 与未来业务 BFF 的边界 |

来源：[`app/iam/web/AGENTS.md`](../../../app/iam/web/AGENTS.md)、[`docs/adr/0004-separate-iam-op-ui-from-oidc-rp-bff.md`](../../../docs/adr/0004-separate-iam-op-ui-from-oidc-rp-bff.md)。
