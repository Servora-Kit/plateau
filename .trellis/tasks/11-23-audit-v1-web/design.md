# Admin Web 审计视图与联调技术设计

需求来源：[本任务 PRD](prd.md) 与 [父任务 PRD](../11-23-audit-v1/prd.md)。前置契约：[IAM 与 Admin 审计接入及查询编排设计](../11-23-audit-v1-producers-bff/design.md)。

## 1. 目录与文件变更结构

完全复用 `app/admin/web/apps/web-antd` 既有 `users` 模块的组件组织、VxeGrid 表格适配、游标翻页状态机与统一错误解析模式，不引入额外状态库或冗余封装：

```text
app/admin/web/apps/web-antd/src/
├── api/
│   ├── client.ts                           # AdminApi 新增 audit: AuditService
│   ├── errors.ts                           # resolveAdminApiError 新增 AdminAuditErrorReason 映射
│   └── __tests__/errors.test.ts            # 补充审计错误码（含 503 preserveSession）单测
├── locales/langs/
│   ├── zh-CN/page.json                     # 新增 page.audit.title = "审计日志"
│   └── en-US/page.json                     # 新增 page.audit.title = "Audit Logs"
├── router/
│   ├── guard.ts                            # generateAccess 传入 ['admin', ...authStore.permissions]
│   └── routes/modules/
│       └── audit.ts                        # 一级菜单 /audit（order: 3, authority: ['read_audit']）
└── views/audit/
    ├── index.vue                           # 审计日志列表页（分类快捷栏 + 多维筛选 + 游标分页）
    ├── schemas.ts                          # 表单 Schema、表格列定义、请求构造与游标状态转换纯函数
    ├── __tests__/schemas.test.ts           # 请求参数构造、游标重置与 JSON 格式化纯函数单测
    └── components/
        └── audit-detail-drawer.vue         # 审计详情 + 格式化载荷 + 同 trace_id 关联链路抽屉
```

---

## 2. 路由、菜单与权限准入设计（W1）

1. **前端权限码打通（`src/router/guard.ts`）**：
   - 现有 `guard.ts:90-94` 调用 `generateAccess` 时硬编码了 `roles: ['admin']`，未把会话接口返回的细粒度 `permissions` 传入路由过滤；
   - 改为传入 `roles: ['admin', ...authStore.permissions]`：既有无 `meta.authority` 的路由（`/overview`、`/users`）行为保持不变，新增声明了 `meta: { authority: ['read_audit'] }` 的 `/audit` 路由由 `@vben/access` 原生按 `GetSessionResponse.permissions` 控制菜单显隐与直链准入（无权限访问 `/audit` 自动落入 `forbidden.vue`）。
2. **一级扁平导航（`src/router/routes/modules/audit.ts`）**：
   - `path: '/audit'`，`name: 'AuditLogs'`，`icon: 'lucide:scroll-text'`，`order: 3`（紧接 `overview` `order: 1` 与 `users` `order: 2`），不挂任何二级子路由。

---

## 3. API 客户端与错误恢复契约（W2）

1. **`src/api/client.ts`**：
   - 从 `@plateau/api/admin/audit/v1` 导入 `createAuditServiceClient` 与 `type AuditService`，挂载到 `createAdminApi(signal)` 返回对象的 `audit` 属性，支持传入 `AbortSignal` 在快速切换筛选条件或关闭抽屉时取消在途请求。
2. **`src/api/errors.ts`**：
   - 接入 `@plateau/api/admin/audit/v1/audit.errors` 的 `AdminAuditErrorReason` 与 `isAdminAuditErrorReason`：
     - `ADMIN_AUDIT_ERROR_REASON_EVENT_NOT_FOUND` → `category: 'not_found'`（提示记录不存在或已过期清理）；
     - `ADMIN_AUDIT_ERROR_REASON_INVALID_PAGE_TOKEN` → `category: 'invalid_argument'`（列表页捕获后自动清空游标栈并回到首页重新加载）；
     - `ADMIN_AUDIT_ERROR_REASON_INVALID_ARGUMENT` → `category: 'invalid_argument'`（如起止时间倒置提示）；
     - `ADMIN_AUDIT_ERROR_REASON_DOWNSTREAM_UNAVAILABLE` → `category: 'unavailable'`，**显式标记 `preserveSession: true`**，前端展示服务暂不可用横幅/提示并提供重试按钮，绝不触发 `authStore.markUnauthenticated()` 踢出当前管理员。

---

## 4. 审计日志列表页交互与状态设计（`views/audit/index.vue` & `schemas.ts`）

### 4.1 顶部快捷视图与多维筛选表单

- **顶部 `Segmented` 类别快捷切换**（切换时自动重置游标栈至第 1 页并触发查询）：
  - `全部`（不传 `category`）
  - `认证与会话`（`AUDIT_CATEGORY_AUTHN`）
  - `资源变更`（`AUDIT_CATEGORY_RESOURCE`）
  - `授权拒绝`（`AUDIT_CATEGORY_AUTHZ`）
- **`useVbenVxeGrid` 搜索栏字段**：
  1. **时间范围（`timeRange`）**：`RangePicker`（支持时分秒，提供「近 1 小时 / 近 24 小时 / 近 7 天 / 近 30 天」快捷预设），转换为 UTC RFC3339 `startTime` / `endTime`；校验 `startTime <= endTime`。
  2. **主体类型（`actorType`）**：全部 / 人类（`human`）/ 服务（`service`）/ 匿名（`anonymous`）。
  3. **执行结果（`outcome`）**：全部（`success: undefined`）/ 成功（`success: true`）/ 失败（`success: false`）。
  4. **生产服务（`service`）**：支持下拉选择常用控制面服务（`iam.service`、`admin.service`、`audit.service`）或清空查全部。
  5. **主体 ID（`actorId`）**、**目标资源（`resourceName`，如 `users/019...`）**、**操作名（`operation`）**、**链路 `traceId`**：文本精确匹配输入（自动 `trim`），并支持从 URL Query（如 `?traceId=...`、`?actorId=...`、`?resourceName=...`）初始化回填。

### 4.2 表格列与游标栈翻页

- **表格列定义**：
  1. `发生时间`（`occurredTime`，本地格式化精确到秒，悬浮展示毫秒与 UTC）；
  2. `类别`（`category` 彩色 `Tag`：认证与会话 / 资源变更 / 授权拒绝）；
  3. `生产服务 & 操作`（主行显示简短方法名，副行/Tooltip 展示完整 `service` + RPC `operation`）；
  4. `操作主体`（`actorType` 徽标 + 等宽字体 `actorId`，匿名显示「匿名请求」）；
  5. `目标资源`（`resourceName`，空时显示 `-`）；
  6. `结果`（成功绿标 / 失败红标 + `errorCode` 摘要）；
  7. `来源 IP`（`clientIp`）；
  8. `操作`（固定右侧「详情」按钮，点击打开 `AuditDetailDrawer`）。
- **游标分页状态机**：
  - 与 `views/users/schemas.ts` 保持一致：`cursorStack: string[]` 记录历史页 `pageToken`，提供「上一页 / 下一页」与每页条数（`20` / `50` / `100`）切换；
  - 计算规范化查询签名 `querySignature`：当任意筛选字段、类别快捷项或 `pageSize` 变化时，强制重置 `cursorStack = ['']`、`currentPageIndex = 0`，避免将旧游标带入新过滤条件触发 400 错误。

---

## 5. 审计详情与同 `trace_id` 跨服务链路抽屉（`audit-detail-drawer.vue`）

抽屉接收 `{ resourceName: string, initialEvent?: AuditEvent }`，打开时调用 `GetAuditEvent({ name: resourceName })` 获取权威记录，并当 `traceId` 非空时并发调用 `ListAuditEvents({ traceId, pageSize: 50 })` 拉取同一调用链上的全部关联审计记录：

```text
┌──────────────────────────────────────────────────────────────┐
│ 审计事件详情                                           [关闭] │
├──────────────────────────────────────────────────────────────┤
│ [成功/失败徽标]  /admin.user.v1.UserService/DisableUser      │
│ 1. 核心问责属性 (Descriptions)                                │
│   - 事件 ID / 发生时间 / 类别 / 事件类型                      │
│   - 生产服务: admin.service   操作主体: [人类] 0195f3...     │
│   - 目标资源: iam.plateau.dev/User · users/0195f4...         │
│   - 来源上下文: IP 127.0.0.1 · UA Mozilla/...                │
│   - 追踪标识: TraceID 4bf92f... [复制] · RequestID ...       │
│   - 错误详情 (仅失败显示): error_code + error_message        │
├──────────────────────────────────────────────────────────────┤
│ 2. 同链路关联审计记录 (trace_id 串联，按 occurred_time 正序)  │
│   ● 14:02:11.104 [admin.service] human:0195f3...             │
│     /admin.user.v1.UserService/DisableUser (当前查看)        │
│   ○ 14:02:11.112 [iam.service]   service:admin   [查看此跳]  │
│     /iam.user.v1.UserService/DisableUser                     │
├──────────────────────────────────────────────────────────────┤
│ 3. 脱敏载荷摘要 (detail_json)                      [复制JSON]│
│   { "name": "users/0195f4..." }                              │
└──────────────────────────────────────────────────────────────┘
```

- **抽屉内单跳切换**：点击「同链路关联审计记录」时间线中的任意其他节点（如从 Admin 的 `human` 记录切到 IAM 的 `service:admin` 记录），直接在当前抽屉内切换加载该 `event.name` 的详情并更新高亮节点，无需关闭抽屉回到列表二次搜索。
- **并发与卸载清理**：主详情与关联 `traceId` 查询共享同一个 `AbortController`；快速切换记录或关闭抽屉时立即中止旧请求；若关联 `traceId` 查询单独失败，仅在关联区块内展示局部提示，不阻断主详情与 `detail_json` 渲染。

---

## 6. 全链路真实联调方案（W4）

在本地基础设施与三个后端服务 + 两个前端全部真实运行下执行端到端验证：
1. **基础设施与模型就绪**：启动 PostgreSQL、Redis、OpenFGA、Kafka、ClickHouse；执行 `just openfga-model-apply` 并确认 `service:admin` 对 `iam:global`（`manage_users`、`expunge_users`）与 `audit:global`（`read_events`）元组已写入。
2. **核心场景实测闭环**：
   - **场景 A（认证成败与脱敏）**：在 IAM Web 故意输入错误密码触发登录失败，随后正确登录，再登出；在 Admin Web「审计日志 → 认证与会话」核对失败记录（`anonymous` + `AUTHN_ERROR_REASON_INVALID_CREDENTIALS` + `detail_json` 无密码明文）与成功记录（`human:<uid>`）。
   - **场景 B（跨服务管理变更与双跳关联）**：管理员在 Admin Web 创建用户并执行封禁/解封；在「审计日志 → 资源变更」点开对应记录抽屉，验证同一 `trace_id` 时间线下同时呈现 `admin.service`（`human:<admin_uid>`）与 `iam.service`（`service:admin`）两跳记录且可一键切换。
   - **场景 C（403 授权拒绝）**：使用未授予管理员权限的普通 IAM 账号尝试登录 Admin Web（触发回调 `?error=access_denied`），在「审计日志 → 授权拒绝」核对 `SECURITY_ERROR_REASON_PERMISSION_DENIED` 记录。
