# Admin Web 实施计划

需求：[PRD](prd.md) · 技术合同：[设计](design.md)。本计划在用户审阅确认并通过 `task.py start` 激活任务后按序执行，规划阶段不修改产品代码。

---

## 阶段 1：跨仓传输契约提取与 `@plateau/client` 演进（E1、E2）

先打通底层共享包与代码生成器，保证上层应用直接建立在最终契约之上：

- [x] **1.1 `@servora/proto-utils` 新增 `./transport` 子路径（`../servora`）**
  - 新增 `servora/web/packages/proto-utils/src/transport.ts`，导出 `TransportMeta`、`ClientTransport`、`ServerStream`、`DuplexStream`、`encodePathSegment`、`encodeMultiSegmentPath`，移除空置 `src/client/.gitkeep`；
  - 更新 `servora/web/packages/proto-utils/package.json`（新增 `"./transport"` 导出，保留公开 npm 包发布必需元数据）与 `tsconfig.build.json`；
  - 新增 `servora/web/packages/proto-utils/test/transport.test.mjs` 覆盖单段/多段路径编码（含 RFC 3986 `[!'()*]` 转义）。
- [x] **1.2 `protoc-gen-typescript-http` 按需导入改造与双仓重生成**
  - 修改 `servora/cmd/protoc-gen-typescript-http/internal/plugin/{transportgen.go,packagegen.go}` 及 `generate_test.go`：移除内联的 38 行传输类型与编码函数，改为按文件实际包含的 `service`、流式方法与路径变量按需从 `@servora/proto-utils/transport` 导入；
  - 在 `servora` 执行 `go test ./cmd/protoc-gen-typescript-http/...`、`just web-build`、`just web-test`，本地 `go install ./cmd/protoc-gen-typescript-http` 后执行 `servora` 的 `just gen-ts`；
  - 更新父级 `../pnpm-workspace.yaml`（补入 `- "!plateau/app/admin/web"`），在 `plateau` 执行 `just api-ts` 与 `just api-ts-check` 验证全部生成 TS 产物零错误。
- [x] **1.3 `@plateau/client` 类型复用与无请求体写请求 `Content-Type` 对齐**
  - 更新 `web/packages/client/src/{types.ts,index.ts,http.ts}`（合并原 `http-client.ts` 与冗余 `http.ts`）：复用 `@servora/proto-utils/transport` 类型；`createTransport` 对所有非安全 unary 方法（含 `body === null` 的 `DELETE` 与无请求体 `POST`）统一设置 `Content-Type: application/json`，`http.ts` 将 `body: null` 归一为空报文发送且不误删非安全方法的 `Content-Type`；
  - 在 `web/packages/client/test/http.test.ts` 补充 `DELETE`（`body = null`）与 `GET` 请求头契约单测，运行 `pnpm --filter @plateau/client test` 与 `pnpm --filter @plateau/client typecheck`；
  - 精简 `plateau` 内前端私有包 `package.json`（根 `package.json`、`api/gen/package.json`、`web/packages/client/package.json`、`app/{iam,example,test}/web/package.json`）。

---

## 阶段 2：Vben 工程瘦身、`package.json` 最小化与契约接入（E1、E3、E4）

清理无关模板资产并将 `apps/web-antd` 接入 `@plateau/api` 与 `@plateau/client`：

- [x] **2.1 移除冗余应用、插件与 Mock 包（`app/admin/web`）**
  - 删除 `apps/{web-ele,web-naive,web-tdesign,web-antdv-next,backend-mock}`、`docs`、`playground`、`.changeset`、`packages/effects/request`；
  - 删除 `packages/effects/plugins/src/{echarts,tiptap}` 并更新 `@vben/plugins/package.json`（仅保留 `./vxe-table`、`./motion`）；
  - 删除 `packages/styles/src/{ele,naive,antdv-next}` 并更新 `@vben/styles/package.json`（仅保留 `.`、`./antd`、`./global`）；
  - 移除 `internal/vite-config/src/plugins/nitro-mock.ts` 及其在 `plugins/index.ts`、`typing.ts`、`config/application.ts` 中的挂载与 `nitropack` 依赖。
- [x] **2.2 `package.json` 与 `pnpm-workspace.yaml` 精简对齐**
  - 清理 `app/admin/web/pnpm-workspace.yaml`：移除 `docs`、`playground` 工作区项及全部未引用 `catalog:` 条目，补齐 `@servora/proto-utils` 本地联调映射；
  - 批量精简 `app/admin/web` 根与 `apps/web-antd`、`packages/**`、`internal/**`、`scripts/**` 各 `package.json` 中的上游作者/仓库/发布残留字段与失效脚本；
  - 在 `apps/web-antd/package.json` 移除 `@vben/request`，通过 `link:` 引入 `@plateau/api`、`@plateau/client` 及 `@servora/proto-utils`，在 `apps/web-antd/tsconfig.json` 开启 `"allowImportingTsExtensions": true`；
  - 配置 `apps/web-antd/{vite.config.ts,.env,.env.development,.env.production}`（固定 `10012` 端口、`/v1 -> http://127.0.0.1:10010` 同源代理、`resolve.dedupe: ['@servora/proto-utils']`）与 `app/admin/web/justfile`；
  - 在 `app/admin/web` 执行 `pnpm install` 重建 `pnpm-lock.yaml` 与内部构建 stub，验证依赖解析正常。

---

## 阶段 3：认证会话守卫、壳层裁剪与轻量总览页（W1、W2、W6）

建立真实 OIDC 会话状态机并替换模板演示页面：

- [x] **3.1 API 组合根与结构化错误映射器**
  - 新增 `apps/web-antd/src/api/{client.ts,errors.ts,index.ts}`，删除旧 `src/api/request.ts` 与 `src/api/core/*`；
  - 实现 `createAdminApi(signal?)`、`buildOidcLoginUrl(returnTo?)` 与完整覆盖设计 §6 错误矩阵的 `resolveAdminApiError`（严禁透出内部 `cause` 或敏感载荷）。
- [x] **3.2 会话 Store、OIDC 登录页、故障回退页与路由守卫**
  - 重写 `src/store/auth.ts`：实现内存态 `sessionStatus`、`currentUser`、`permissions`、`ensureSession()` 单飞探针与仅销毁本地会话的 `logout()`；
  - 重写 `src/views/_core/authentication/login.vue` 与 `src/layouts/auth.vue`：仅保留“使用 Plateau IAM 登录”按钮及 `?error=access_denied|service_unavailable|login_failed` 精确分类提示；
  - 完善 `src/views/_core/fallback/{forbidden.vue,service-unavailable.vue}`，重写 `src/router/{guard.ts,access.ts}` 与 `src/router/routes/core.ts`，确保 `401` 跳登录、`403` 展示无权限页、`503`/网络错误展示依赖故障页且保留本地 Cookie。
- [x] **3.3 壳层伪控件清理与轻量总览页（`/overview`）**
  - 更新 `src/preferences.ts` 与 `src/layouts/basic.vue`：移除假通知、本地锁屏、个人中心、外链菜单与 `tenantMode` 等扩展偏好；
  - 删除 `views/dashboard/*`、`views/demos/*`、`views/_core/{about,profile}` 与旧路由模块 `router/routes/modules/{dashboard,demos,vben}.ts`；
  - 新增 `router/routes/modules/{overview.ts,users.ts}` 与 `views/overview/index.vue`（展示当前唯一管理员真实身份、权限码徽标与用户管理快捷入口，无任何统计图表）。

---

## 阶段 4：用户管理全生命周期界面与单元测试（W3、W4、W5、W6）

交付 `/users` 双视图列表、详情编辑抽屉及各项独立管理动作：

- [x] **4.1 `/users` 单路由双视图与游标分页列表（`views/users/index.vue` + `schemas.ts`）**
  - 实现「正常用户」与「已删除用户（回收站）」`Segmented` 切换、基于 `buildFilter(UserFields, ...)` / `buildOrderBy(UserFields, ...)` 的查询栏，以及带 `AbortController` 请求取消和尾页空结果自动回退的 `cursorStack` 游标分页栏；
  - 按设计 §5.3 渲染生命周期徽标、`requiredActions` 标签、已删除记录“无限期保留 · 邮箱占用中”提示（无倒计时）以及受权限码与本人自我保护约束的行操作按钮。
- [x] **4.2 右侧详情与 7 字段资料编辑抽屉（`user-detail-drawer.vue`）**
  - `GetUser` 加载最新快照与 `etag`，只读展示完整身份元数据（邮箱只读）；
  - `useVbenForm` 编辑 7 个 `profile.*` 字段（头像为纯 URL 文本输入），计算真实变更集并通过 `makeUpdateMask(UserUpdateFields, ...)` + `etag` 提交 `UpdateUser`，处理 `ETAG_MISMATCH` 冲突刷新与管理员本人顶栏资料同步。
- [x] **4.3 创建用户、补发邮件、管理设密、强制登出、状态变更与显式永久删除**
  - 实现 `create-user-modal.vue`：默认勾选首次改密且显式传布尔值 `requirePasswordChange`，区分创建失败、发信成功与 `verificationEmailSent === false`（关闭创建表单防重提 + 展示已创建账号 + 一键补发验证邮件）；
  - 实现待验证用户 `SendUserVerificationEmail`、`set-password-modal.vue`（`SetUserPassword` 独立设密）、`RevokeUserSessions` 独立强退确认框、`DisableUser`/`EnableUser`（带 `etag`，无原因输入）、`DeleteUser`（处理重复删除 `404 USER_NOT_FOUND` 不报成功并刷新）、`UndeleteUser` 与 `expunge-user-modal.vue`（仅回收站且具 `expunge_users` 可用，核对完整邮箱后调用 `ExpungeUser`）。
- [x] **4.4 前端单元测试与全量静态检查**
  - 编写 Vitest 单测覆盖 `resolveAdminApiError` 错误分流与脱敏、查询过滤/更新掩码/显式布尔载荷构造、游标栈翻页与会话状态机；
  - 通过全部静态与构建门禁命令。

---

## 阶段 5：真实浏览器端到端集成验收与规范同步（W7）

禁止以 Mock 替代，拉起真实全栈服务并在真实浏览器中逐项验证 `AC-W1`～`AC-W8`：

- [x] **5.1 本地全栈环境就绪**
  - 确认 PostgreSQL（`plateau_iam`、`plateau_admin`）、OpenFGA（应用模型并确认 `service:admin` 对 `iam:global` 的 `manage_users`、`expunge_users` 元组）、本地邮件捕获/SMTP、IAM 服务（`10000`/`10001`）+ IAM Web（`10002`）、Admin 服务（`10010`）与 Admin Web（`10012`）全部真实运行。
- [x] **5.2 浏览器端逐条验收与证据留存**
  - 覆盖：管理员 OIDC 登录与总览展示、普通 IAM 用户 `access_denied` 拦截、创建用户（默认勾选与取消勾选首次改密 + 发信成功与 SMTP 关闭降级补发）及新用户在 IAM 首次改密、列表筛选与游标翻页、7 字段资料编辑与邮箱只读、独立设密与独立强制登出 IAM、自我保护禁用、软删除/重复删除语义/回收站查看/无限期恢复/显式永久删除释放邮箱、Admin 与 IAM 独立登出互不误伤、以及停止/恢复下游 IAM 或模拟服务鉴权异常时的 `503` 会话保留行为。
- [x] **5.3 规范文档与工程说明更新**
  - 同步更新 `.trellis/spec/servora/web/proto-utils.md`、`.trellis/spec/servora/proto/generation.md`、`.trellis/spec/web/client/{architecture.md,http.md}`、`.trellis/spec/admin/frontend.md`、根 `AGENTS.md`、`app/AGENTS.md` 及 `app/admin/web/README.md`。

---

## 验证门禁命令清单

```bash
# 1. Servora 跨仓插件与 @servora/proto-utils 验证
just --justfile ../servora/justfile test
just --justfile ../servora/justfile web-test
just --justfile ../servora/justfile gen-ts

# 2. Plateau API 生成与共享客户端验证
just api-ts
just api-ts-check
pnpm --filter @plateau/client typecheck
pnpm --filter @plateau/client test

# 3. Admin Web 静态检查、单测与生产构建验证
pnpm --dir app/admin/web run lint
pnpm --dir app/admin/web run check
pnpm --dir app/admin/web run test:unit
just web::lint
just web::build
```

## 风险点与回滚预案

- **跨工作区 `link:` 类型解析与依赖校验**：`app/admin/web` 独立启用 `vue-tsc`、`eslint-plugin-pnpm` 与 `knip`（`vsh check-dep`）；阶段 2 完成瘦身与 `link:` 挂载后立即跑通 `pnpm run lint && pnpm run check`，确认无游离 catalog 项、无循环依赖、无 `.ts` 扩展名解析报错后再进入业务页面编码。
- **下游故障与会话隔离回归**：阶段 5 在真实浏览器中断开 IAM gRPC 或注入异常服务凭据，实测验证 `GetSession` 与 `ListUsers` 返回 `503` 时本地 `admin_session` Cookie 不被清除，恢复连接后点击刷新直接恢复访问。
