# 管理界面与工程边界

适用于 `app/admin/web`（唯一应用为 `apps/web-antd`，基于 Vben 5、Ant Design Vue、Pinia 与 VxeTable），承担平台总览与 IAM 全局用户管理界面；应用范围见 [架构](architecture.md)。

- 保留独立 pnpm workspace 和 lockfile，根与父级 `pnpm-workspace.yaml` 均排除 `app/admin/web`，依赖由 `just web::admin::install`（已纳入根 `just init`）统一安装；`apps/web-antd` 通过 `link:` 引用 `@plateau/api` 与 `@servora/plateau-utils`，通过工作区 `catalog:` 消费正式发布的 `@servora/proto-utils`，Vite `resolve.dedupe` 固定 `['@servora/plateau-utils', '@servora/proto-utils']`。
- 本地默认经 Traefik 网关以 `http://admin.plateau.localhost:8080` 访问（网关将 `/v1` 与 `/docs` 直转 `admin.service:10010`、其余页面转 `admin.web:10012`）；`admin.web` 自身固定监听 `10012`（`strictPort: true`）并保留 `/v1` 至 `http://127.0.0.1:10010` 的同源代理作为单调回退能力，Cookie 按 `same-origin` 自动携带；浏览器不直连 IAM 管理接口，服务调用凭据仅由后端持有（见 [后端](backend.md)）。
- API 客户端在 `src/api/client.ts` 以 `createHttpClient({ service: 'admin', retry: 0 })` + `createTransport` 统一装配，不使用前端请求自动重试；结构化错误经 `resolveAdminApiError` 按 HTTP 状态与业务 reason 分流，下游 `503` 保持本地会话不清除，不透出内部 `cause`。
- 当前管理员身份与权限码（`access`、`manage_users`、`expunge_users`、`manage_clients`、`read_audit`）仅驻留内存 Pinia Store，不写入 `localStorage` / `sessionStorage`；OAuth 客户端管理页（`/oauth-clients`）提供 4 类应用预设向导、协议联动校验、一次性明文密钥弹窗、多密钥零停机轮换抽屉与 `STATIC` 客户端只读锁定提示；登录页区分 `login_cancelled`、`access_denied`、`service_unavailable` 与 `login_failed` 四类回调提示，前端路由守卫、菜单和按钮显隐不替代服务端认证、本地会话或授权校验。
- 修改源码后执行 `just web::admin::lint`、`just web::admin::test` 与 `just web::admin::build`。
