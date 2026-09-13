# Example Web 前端

适用于 `app/example/web/`。这是 `example.service.v1.User` 的 Vue 参考客户端，用来直接练习公开 HTTP 外观、生成 TypeScript API 和 `@servora/proto-utils`；它不是第二份 API 契约来源。

## 开发前检查

- 修改目录、页面或 Pinia 状态前读取 [架构](architecture.md)。
- 修改请求、生成代码消费或错误映射前读取 [请求](requests.md)；表单、加载与可访问性改动读取 [表单](forms.md)。
- 资源名 `example.servora.dev/User` 与 `tenants/{tenant}/users/{user}` 必须以生成 helper 和 service Proto 为准，不手写平行契约。

## 质量检查

- `just web::example::lint`、`just web::example::test-unit`、`pnpm --dir app/example/web run type-check`。
- `just web::example::build` 检查生产构建；Vite 的 dev/preview 端口都是 10032，不要并行启动。
- 真实请求验证需要先运行 `just service::example::run`，再用 `just web::example::dev` 在浏览器执行 CRUD。当前任务未运行此联调，不能表述为已验收。

## 主题索引

| 主题 | 内容 |
| --- | --- |
| [架构](architecture.md) | Vue、路由、Pinia 和生成 API 的目录职责 |
| [请求](requests.md) | 应用自有 transport、ProtoJSON、CRUD helper 与错误分类 |
| [表单](forms.md) | 提交、加载、错误与键盘可操作性 |

来源：[`app/example/web/AGENTS.md`](../../../app/example/web/AGENTS.md)、[`package.json`](../../../app/example/web/package.json)。
