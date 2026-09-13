# Test Web 用途与边界

当前 Test Web 是单页 Next.js App Router 工程。[`app/page.tsx`](../../../app/test/web/app/page.tsx) 导入 Audit 与 Example 的生成 service client、Example 的 `UserName` helper 和 `UserErrorReason`，并在页面显示这些符号的运行时类型或格式化结果。这证明 workspace 依赖在此入口可以参与 Next 编译；它不发送请求，也不验证服务端可达性。

[`next.config.ts`](../../../app/test/web/next.config.ts) 仅将 `@plateau/api` 加入 `transpilePackages`，没有 rewrite 或后端 origin。`package.json` 也没有 `@plateau/client` 依赖。因此新增生成 API 构建验证应遵循同一模式：直接从 `@plateau/api` 导入真实生成符号，在页面中作无副作用的可渲染引用；不要实现 fake transport 或请求外部服务。

README 仍保留 create-next-app 的通用内容，与当前 Plateau 用途不完全一致。此事实仅记录为文档差异；本任务不修改应用 README 或代码。
