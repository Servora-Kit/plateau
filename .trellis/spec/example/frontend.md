# Example 前端规范

适用于 `app/example/web/`。这是 `example.service.v1.User` 的 Vue 参考客户端，用来直接练习公开 HTTP 外观、生成 TypeScript API 和 `@servora/proto-utils`；它不是第二份 API 契约来源。

## 开发前检查

- 修改目录、页面或 Pinia 状态前阅读本文件的目录职责与状态责任。
- 修改请求、生成代码消费或错误映射前读取 [请求](requests.md)；表单、加载与可访问性改动读取 [表单](forms.md)。
- 资源名 `example.servora.dev/User` 与 `tenants/{tenant}/users/{user}` 必须以生成 helper 和 service Proto 为准，不手写平行契约。

## 质量检查

- `just web::example::lint`；单元测试和类型检查分别使用 `pnpm --dir app/example/web run test:unit`、`pnpm --dir app/example/web run type-check`。
- `just web::example::build` 检查生产构建；Vite 的 dev/preview 端口都是 10032，不要并行启动。
- 真实请求验证需要先运行 `just service::example::run`，再用 `just web::example::dev` 在浏览器执行 CRUD。当前任务未运行此联调，不能表述为已验收。

## 目录职责

Example 使用 Vue 3、Vue Router 和 Pinia。入口 [`src/main.ts`](../../../app/example/web/src/main.ts) 安装 Pinia 与 router；[`src/router/index.ts`](../../../app/example/web/src/router/index.ts) 当前只有 `/` 到 `HomeView` 的路由；[`src/App.vue`](../../../app/example/web/src/App.vue) 只渲染 `RouterView`。新增页面先扩展 router，再将页面交互保持在 `views/`。

- `src/api/generated/` 是服务 leaf 生成的 TypeScript API，只能消费，不能手改或抄写类型。
- `src/api/transport.ts` 是本应用的 `ClientTransport` adapter；`src/api/userApi.ts` 在该 adapter 上创建生成的 User client。
- `src/stores/users.ts` 负责跨控件共享的查询、选择、忙碌、状态和 CRUD 编排；页面从 store 读取状态并调用动作。
- `src/views/HomeView.vue` 负责表单和列表 UI、局部编辑值与可访问输出；`assets/` 负责样式和静态资源。

当前 Example 不依赖或导入 `@plateau/client`，而是按应用自身 adapter 发送请求。不要为了和 IAM 一致而改变这个已验证 transport 边界；共享 client 的能力与其他应用差异见 [Web client 架构](../web/client/architecture.md)。

## 状态责任

Pinia store 区分资源数据（`users`、`selected`、`pager`）、查询输入（tenant、filter、showDeleted）和操作状态（`busy`、`listState`、`error`、`status`）。`HomeView` 的创建、编辑字段是页面局部 reactive/ref 状态；选中 User 变化时用 watcher 同步编辑初值。不要把所有表单输入提升到 store，也不要让视图自行绕过 store 请求 API。

来源：[`main.ts`](../../../app/example/web/src/main.ts)、[`stores/users.ts`](../../../app/example/web/src/stores/users.ts)、[`views/HomeView.vue`](../../../app/example/web/src/views/HomeView.vue)。
