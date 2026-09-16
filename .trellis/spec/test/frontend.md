# Test 前端规范

适用于 `app/test/web/`。该目录是 Next.js 构建验证入口，当前页面用于确认 workspace 生成 API、资源名 helper 与错误枚举可被 Next 编译和渲染；它不是业务 UI，也没有请求运行时。

## 开发前检查

- 先确认新增内容仍服务于生成 API 或 workspace 构建验证。业务交互应进入对应应用，而不是在这里建立临时产品功能。
- 保持 `@plateau/api` 作为生成代码来源，不复制类型、resource name 或 error reason。
- 当前没有 `@plateau/client` 依赖、请求 adapter、代理或服务端 API 调用；不要将它描述为已接入共享 HTTP/SSE/WebSocket client。

## 质量检查

- 在 `app/test/web` 目录运行 `pnpm run build` 或 `pnpm run lint`；该 leaf 的 [`justfile`](../../../app/test/web/justfile) 也提供同名命令。
- `package.json` 当前未定义 typecheck 或 test 脚本。新增实际行为前应先确定对应的检查入口，而不是把 Next 构建当作行为测试。

## 用途与边界

当前 Test Web 是单页 Next.js App Router 工程。[`app/page.tsx`](../../../app/test/web/app/page.tsx) 导入 Audit 与 Example 的生成 service client、Example 的 `UserName` helper 和 `UserErrorReason`，并在页面显示这些符号的运行时类型或格式化结果。这证明 workspace 依赖在此入口可以参与 Next 编译；它不发送请求，也不验证服务端可达性。

[`next.config.ts`](../../../app/test/web/next.config.ts) 仅将 `@plateau/api` 加入 `transpilePackages`，没有 rewrite 或后端 origin。`package.json` 也没有 `@plateau/client` 依赖。因此新增生成 API 构建验证应遵循同一模式：直接从 `@plateau/api` 导入真实生成符号，在页面中作无副作用的可渲染引用；不要实现 fake transport 或请求外部服务。

README 仍保留 create-next-app 的通用内容，与当前 Plateau 用途不完全一致。此事实仅记录为文档差异；本任务不修改应用 README 或代码。
