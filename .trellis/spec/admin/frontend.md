# 管理界面与工程边界

适用于 `app/admin/web`，采用 Vben 的 Ant Design Vue 版本，承担平台资源查询、管理表单和操作反馈；应用范围见 [架构](architecture.md)。

- 保留独立 workspace 和 lockfile，在本目录安装依赖；根 pnpm workspace 排除该目录，不合并内部依赖。
- 浏览器通过管理后端操作，服务调用凭据仅由后端持有，见 [后端](backend.md)。
- 演示登录、菜单权限和按钮可见性不替代真实认证、应用会话或服务端授权。
