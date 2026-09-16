# IAM Web 前端

适用于 `app/iam/web/`。IAM Web 是 IAM OIDC Provider 的同源交互界面：页面交互在浏览器 CSR 执行，IAM Go service 提供 API、HttpOnly 登录会话、CAP 与 OIDC 协议。它不是 OAuth client，也不保存 client secret 或 token。

## 开发前检查

- 读取本页的“前端目录与状态”确认 App Router 目录、组件和 hook 的职责。
- 认证、注册、账户或一次性链接变更读取 [认证流程](auth-flows.md)；涉及公开入口、rewrite 或 OIDC 路径读取 [IAM 架构](architecture.md)。
- 请求改动同时读取共享 [Web client](../web/client/index.md) 的 HTTP 规则，并检查生成 API 与 `lib/iam-api.ts` 的装配。

## 质量检查

- 静态检查：`just web::iam::lint`；类型检查按需使用 `pnpm --filter @plateau/iam-web run typecheck`。
- 构建：`just web::iam::build`；本地预览按需使用 `pnpm --filter @plateau/iam-web run preview`。它和 dev 都使用 10002，不能同时启动。
- IAM 页面当前未发现前端单元测试文件。认证或路由改动需要以本地运行的 IAM service 做页面 smoke；OIDC 协议是否正确仍由 Go 侧测试验证，不能仅凭页面构建断言端到端完成。

## 前端目录与状态

IAM Web 使用 Next.js App Router。页面位于 [`app/`](../../../app/iam/web/app)，共享页面骨架和表单部件位于 [`components/`](../../../app/iam/web/components)，状态性复用逻辑位于 [`hooks/`](../../../app/iam/web/hooks)，请求装配、错误映射与输入校验位于 [`lib/`](../../../app/iam/web/lib)。新增代码应放在承担同一职责的位置，避免将 transport、认证判断或敏感值处理复制进页面。

### 路由与组件

- `app/page.tsx` 重定向到 `/login/`；认证流程的页面包括 login、register、forgot-password、reset-password、verify-email。
- `app/account/page.tsx` 与 `app/account/security/page.tsx` 是已登录账户页，复用 `AccountShell` 与 `useProfile`。
- `AuthShell`、`AccountShell`、`StatusMessage` 和 `form-controls.tsx` 负责布局、可访问表单字段和统一状态呈现；页面负责提交动作、局部字段错误和导航。
- 需要浏览器 API、React state 或事件处理的页面与 hook 用 `"use client"`。不要把未验证的 SSR 数据获取模式混入现有 CSR 流程。

### 请求与状态

[`lib/iam-api.ts`](../../../app/iam/web/lib/iam-api.ts) 是生成 Account、Authn、Session client 的统一入口，使用 workspace `@plateau/client`。页面通过 `createIamApi(signal)` 调用它，不直接在页面实现 `fetch` 或另造生成 client。

`useSingleFlight` 用 `pending` 和 `AbortController` 阻止重复提交，并在卸载时取消活动请求；`useProfile` 管理 `loading`、`ready`、`error`、`unauthenticated` 四种账户状态；`useSensitiveValue` 只提供 `clear()` 与卸载时清空 ref 的保护。页面在 `await run(...)` 返回后显式调用密码实例的 `clear()`，示例见 [`account/security/page.tsx`](../../../app/iam/web/app/account/security/page.tsx)、[`login/page.tsx`](../../../app/iam/web/app/login/page.tsx)、[`register/page.tsx`](../../../app/iam/web/app/register/page.tsx) 与 [`reset-password/page.tsx`](../../../app/iam/web/app/reset-password/page.tsx)。新增交互应选择与生命周期相符的现有 hook，不能只靠按钮禁用掩盖并发请求。

[`lib/errors.ts`](../../../app/iam/web/lib/errors.ts) 将 `ApiError`、HTTP 状态和 Kratos reason 转为认证分支或安全的用户文案。业务 reason 与页面反馈仍由 IAM Web 决定；不要把后端原始错误、凭据或 URL fragment 直接呈现。

来源：[`use-profile.ts`](../../../app/iam/web/hooks/use-profile.ts)、[`use-single-flight.ts`](../../../app/iam/web/hooks/use-single-flight.ts)、[`app/account/security/page.tsx`](../../../app/iam/web/app/account/security/page.tsx)。
