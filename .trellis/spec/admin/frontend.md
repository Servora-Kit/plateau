# Admin 前端规范

适用于 `app/admin/web`。前端选用 [Vue Vben Admin](https://github.com/vbenjs/vue-vben-admin) 的 **Ant Design Vue** 版本，承担平台资源查询、管理表单与操作反馈；应用范围见 [架构](architecture.md)。Vben 保留自己的 workspace 和 lockfile，在 `app/admin/web` 独立安装依赖；平台根 pnpm workspace 显式排除该目录，不将 Vben 内部依赖合并到根 lockfile。

## 开发前检查

- 浏览器通过 Admin 后端完成管理操作；IAM gRPC 调用凭据由后端持有，见 [后端](backend.md)。
- IAM 登录与 Admin 应用会话按实际接入设计装配；Vben 的演示登录与菜单权限不能作为 Plateau 认证授权已经接通的证据。
- 新增请求与生成 API 消费时组合 [共享 Web client 规范](../web/client/index.md)；本地 Web 预留端口为 `10052`，以根 AGENTS 的端口登记为准。

## 质量检查

- 统一入口为 `just web::admin::dev`、`just web::admin::lint`、`just web::admin::build`；分别启动 Ant Design Vue 应用、执行 Vben 统一 lint、构建 Ant Design Vue 应用。
- Vben 的 `lint` 包含 Oxfmt 检查、Oxlint、ESLint 与 Stylelint；Cspell、Publint、类型检查等独立工具按需在 `app/admin/web` 中通过 pnpm script 执行。
- 管理按钮与页面可见性不替代后端授权；联调应验证成功、未登录、无权限和服务失败时的结果。命令存在不表示这些场景已经运行验收。
