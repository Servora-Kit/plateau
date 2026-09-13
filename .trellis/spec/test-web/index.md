# Test Web 前端

适用于 `app/test/web/`。该目录是 Next.js 构建验证入口，当前页面用于确认 workspace 生成 API、资源名 helper 与错误枚举可被 Next 编译和渲染；它不是业务 UI，也没有请求运行时。

## 开发前检查

- 先确认新增内容仍服务于生成 API 或 workspace 构建验证。业务交互应进入对应应用，而不是在这里建立临时产品功能。
- 保持 `@plateau/api` 作为生成代码来源，不复制类型、resource name 或 error reason。
- 当前没有 `@plateau/client` 依赖、请求 adapter、代理或服务端 API 调用；不要将它描述为已接入共享 HTTP/SSE/WebSocket client。

## 质量检查

- 在 `app/test/web` 目录运行 `pnpm run build` 或 `pnpm run lint`；该 leaf 的 [`justfile`](../../../app/test/web/justfile) 也提供同名命令。
- `package.json` 当前未定义 typecheck 或 test 脚本。新增实际行为前应先确定对应的检查入口，而不是把 Next 构建当作行为测试。

## 主题索引

| 主题 | 内容 |
| --- | --- |
| [用途与边界](purpose.md) | 当前页面、依赖、路由和验证范围 |

来源：[`app/test/web/package.json`](../../../app/test/web/package.json)、[`app/page.tsx`](../../../app/test/web/app/page.tsx)、[`next.config.ts`](../../../app/test/web/next.config.ts)。
