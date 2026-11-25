# Admin Web 审计视图与联调实施计划

需求：[PRD](prd.md) · 技术契约：[设计](design.md)。

## 1. 变更范围声明

- **现状与目标差距**：`app/audit/service` 与 `app/admin/service` 已完成审计入库、`GetEvent`/`ListEvents`/`ListServices` 只读查询编排及 `@plateau/api` TypeScript 客户端生成，但 `app/admin/web` 尚未挂载 `AdminApi.audit` 与审计错误码解析，前端路由守卫尚未把细粒度 `permissions` 注入菜单准入，缺少「审计日志」列表页、同 `trace_id` 详情抽屉以及与用户详情抽屉的双向跳转闭环，且全栈真实联调尚未执行。
- **改动文件清单**：
  - 前置只读发现增量（方案 A）：`app/audit/service/api/protos/audit/event/v1/event.proto`、`app/admin/service/api/protos/admin/audit/v1/audit.proto`、`api/gen/{go,ts}/**`、`app/admin/service/internal/assets/openapi.yaml`、`app/audit/service/internal/{biz,data,service,server}/*`、`app/admin/service/internal/{biz,data,service}/*`；
  - API 与错误恢复：`app/admin/web/apps/web-antd/src/api/{client.ts,errors.ts,__tests__/errors.test.ts}`；
  - 路由、权限状态与国际化：`app/admin/web/apps/web-antd/src/router/{guard.ts,routes/modules/audit.ts}`、`app/admin/web/apps/web-antd/src/store/auth.ts`、`app/admin/web/apps/web-antd/src/locales/langs/{zh-CN,en-US}/page.json`；
  - 总览页与用户页联动：`app/admin/web/apps/web-antd/src/views/overview/index.vue`、`app/admin/web/apps/web-antd/src/views/users/{index.vue,components/user-detail-drawer.vue}`；
  - 审计日志视图与单测：`app/admin/web/apps/web-antd/src/views/audit/{index.vue,schemas.ts,__tests__/schemas.test.ts,components/audit-detail-drawer.vue}`。
- **明确不做事项**：不在前端引入任何硬编码演示数据，不发起全表 `COUNT(*) FINAL` 统计请求，不在列表点击已有记录时重复请求 `GetEvent`，不在验证结束后关闭本地前后端服务（保留在线供人工验收）。

## 2. 执行步骤

- [x] **Step 1：全平台生产服务动态发现接口 `ListServices` 与架构规范同步**
  - 在 `audit.event.v1.EventService` 与 `admin.audit.v1.AuditService`（`GET /v1/admin/audit/services`）新增只读 `ListServices` 接口并执行 `just gen`；
  - 在 `audit.service` 基于无 `FINAL` 的 `SELECT DISTINCT service FROM events WHERE service != '' ORDER BY service ASC` 实现毫秒级服务名枚举，在 `admin.service` 完成编排与 `503` 错误隔离，补齐分层单测与 gRPC 准入断言；
  - 同步修订 `11-23-audit-v1/prd.md`、`.trellis/spec/audit/backend.md`、`.trellis/spec/plateau/security/audit.md` 及本任务规划文档。

- [x] **Step 2：Admin Web API 客户端、错误恢复、权限守卫与总览页联动（W1、W2）**
  - 在 `src/api/client.ts` 接入 `createAuditServiceClient`；在 `src/api/errors.ts` 映射 `AdminAuditErrorReason`（`503 DOWNSTREAM_UNAVAILABLE` 标记 `preserveSession: true`）并补充 `errors.test.ts` 单测；
  - 在 `src/router/guard.ts` 将 `['admin', ...authStore.permissions]` 传入 `generateAccess`，在 `src/store/auth.ts` 检测权限变更触发 `accessStore.setIsAccessChecked(false)`，新增一级路由 `src/router/routes/modules/audit.ts` 与中英文菜单标题；
  - 在 `src/views/overview/index.vue` 补齐 `read_audit` 权限说明、未收录新权限码原文回退渲染及审计日志快捷入口卡片。

- [x] **Step 3：审计日志列表页、详情抽屉与用户页双向跳转实现（W2、W3）**
  - 实现 `src/views/audit/schemas.ts` 与 `__tests__/schemas.test.ts`：覆盖服务名 `//` 前缀自动归一化与去前缀展示、固化时间窗口请求构造、查询签名变更自动重置游标栈、无 `totalSize` 翻页状态计算及 `detail_json` 安全格式化；
  - 实现 `src/views/audit/index.vue`：顶部类别 `Segmented`、首行 4 项 + 折叠 4 项筛选栏（合并 `ListServices` + 三基准服务保底 + 当前列表服务 + `AutoComplete` 自由输入，无精确 URL 参数默认固化近 7 天）、`503` 常驻重试 `Alert` 横幅、`403` 动态刷新权限与底部游标分页栏；
  - 实现 `src/views/audit/components/audit-detail-drawer.vue`：直接复用内存完整 `Event`（支持手动刷新与 `?event=events/<id>` 直达 `GetEvent`）、同 `trace_id` 关联查询（`pageSize: 200` + 超限截断提示与回填列表按钮）、JSON 一键复制，并打通与 `user-detail-drawer.vue` / `views/users/index.vue`（`?userId=<id>`）的双向深链跳转。

- [x] **Step 4：前端静态检查、全栈真实环境浏览器联调验收与驻留运行（W4，AC-W1 ~ AC-W3）**
  - 运行 `app/admin/web` 单元测试、`just web::lint` 与 `just web::build` 确保类型、规范与构建零警告通过；
  - 启动本地 Docker 基础设施与 `iam.service`（`10000`/`10001`）、`admin.service`（`10010`）、`audit.service`（`10020`/`10021`）、IAM Web（`10002`）、Admin Web（`10012`）；
  - 使用内置浏览器工具完整实测场景 A（登录成败与脱敏）、场景 B（跨服务用户变更双跳 `trace_id` 串联与双向跳转）、场景 C（403 授权拒绝），核对无控制台报错且真实数据完全一致，最后保持全部服务运行供人工验收。

## 3. 风险点与验证保障

- **翻页过程中时间窗口漂移导致 `INVALID_PAGE_TOKEN`**：预设时间（如「近 7 天」）必须在选中或初始化时落地为固定的起止时间戳写入表单值，严禁在每次 `ListEvents` 请求构造时动态取 `Date.now()`；在 `schemas.test.ts` 中显式断言翻页前后请求时间戳恒定。
- **下游故障误踢管理员会话**：在 `errors.test.ts` 与列表/抽屉异常处理逻辑中确保 `503` 保持当前会话并呈现页内重试入口，`403` 触发 `ensureSession(true)` 动态同步权限而非登出。
