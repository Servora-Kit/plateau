# IAM 认证流程

所有认证页面保持相同的顺序：读取必要 URL 状态或输入值，先做客户端校验，以 `useSingleFlight` 提交带 signal 的生成 API 调用，按成功、已知业务错误、未认证和基础设施错误分别更新页面状态。表单应使用已有字段组件的 label、错误关联、`aria-busy` 和禁用状态。

## 已实现流程

- 登录页调用 Authn 登录接口，成功后导航；注册页处理重复账户和密码错误，见 [`login/page.tsx`](../../../app/iam/web/app/login/page.tsx) 与 [`register/page.tsx`](../../../app/iam/web/app/register/page.tsx)。
- 忘记密码、重置密码和邮箱验证使用 [`useOneTimeFragment`](../../../app/iam/web/hooks/use-one-time-url.ts) 或 `useOneTimeRequestId` 读取一次性值，并立刻用 `history.replaceState` 从地址栏移除。不能把 token 放入 React 可长期复用状态、日志或错误文本。
- `useProfile` 收到未认证结果时转到 `/login/`；账户资料与安全页在 `loading`、`error`、`ready` 状态分别显示可访问的加载、错误重试和已登录内容。
- 登出只有在 service 成功确认后跳回登录页，具体模式见 [`account/security/page.tsx`](../../../app/iam/web/app/account/security/page.tsx)。

## 错误和加载

对 `ApiError` 使用 [`lib/errors.ts`](../../../app/iam/web/lib/errors.ts) 的 `isUnauthenticated`、`isInvalidPassword`、`isInvalidOneTimeToken`、`isAlreadyRegistered` 与 `requestErrorMessage`。已知业务错误给出对应操作提示；网络、超时、403、5xx 使用安全的通用文案。`busy` 或 `pending` 期间禁用会产生副作用的提交，不以重新发请求恢复 UI。

当前页面实现是 CSR 页面交互的证据；未在本轮运行真实 IAM service、CAP 或邮件/OIDC 回调，所以这些流程存在源码与配置，尚未由本任务进行端到端验收。
