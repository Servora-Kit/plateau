# Admin Web 审计视图与联调技术设计

需求来源：[本任务 PRD](prd.md) 与 [父任务 PRD](../11-23-audit-v1/prd.md)。前置契约：[IAM 与 Admin 审计接入及查询编排设计](../11-23-audit-v1-producers-bff/design.md) 与 [多服务统一审计架构调研](../11-23-audit-v1/research/multi-service-audit-architecture.md)。

## 1. 目录与文件变更结构

复用 `app/admin/web/apps/web-antd` 既有 `users` 模块的组件组织、VxeGrid 表格适配、游标翻页状态机与统一错误解析模式，不引入额外状态库或冗余封装：

```text
app/admin/web/apps/web-antd/src/
├── api/
│   ├── client.ts                                  # AdminApi 新增 audit: AuditService
│   ├── errors.ts                                  # resolveAdminApiError 新增 AdminAuditErrorReason 映射
│   └── __tests__/errors.test.ts                   # 补充审计错误码（含 503 preserveSession）单测
├── locales/langs/
│   ├── zh-CN/page.json                            # 新增 page.audit.title = "审计日志"
│   └── en-US/page.json                            # 新增 page.audit.title = "Audit Logs"
├── router/
│   ├── guard.ts                                   # generateAccess 传入 ['admin', ...authStore.permissions]
│   └── routes/modules/
│       └── audit.ts                               # 一级菜单 /audit（order: 3, authority: ['read_audit']）
├── store/
│   └── auth.ts                                    # 同步权限集合变化时触发 accessStore.setIsAccessChecked(false)
├── views/overview/
│   └── index.vue                                  # PERMISSION_CATALOG 补 read_audit + 未知权限码原文回退 + 审计快捷入口
├── views/users/
│   ├── index.vue                                  # 支持 ?userId=<id> 深链定位打开用户详情抽屉
│   └── components/user-detail-drawer.vue          # 持有 read_audit 时提供查该账号变更与操作审计双向跳转按钮
└── views/audit/
    ├── index.vue                                  # 审计日志列表页（分类快捷栏 + 折叠筛选 + 无 totalSize 游标翻页 + 503 横幅）
    ├── schemas.ts                                 # 表单 Schema、服务名归一化、请求构造与游标状态转换纯函数
    ├── __tests__/schemas.test.ts                  # 服务名归一化、请求参数构造、游标重置与 JSON 格式化纯函数单测
    └── components/
        └── audit-detail-drawer.vue                # 审计详情 + 格式化载荷 + 同 trace_id 关联链路抽屉 + 用户跳转
```

---

## 2. 路由、权限准入与总览页联动（W1）

1. **前端权限码打通与动态重算（`src/router/guard.ts` + `src/store/auth.ts`）**：
   - `guard.ts` 调用 `generateAccess` 时由硬编码 `roles: ['admin']` 改为 `roles: ['admin', ...authStore.permissions]`；既有无 `meta.authority` 的路由（`/overview`、`/users`）不受影响，声明了 `meta: { authority: ['read_audit'] }` 的 `/audit` 由 `@vben/access` 控制菜单显隐与直链准入（无权限落入 `forbidden.vue`）。
   - `auth.ts` 的 `syncUserSnapshot` 比对新旧排序后的权限集合，当在线期间管理员权限发生变化（例如撤销或授予 `read_audit`）时，调用 `accessStore.setIsAccessChecked(false)` 触发下次导航重算菜单与受保护路由。
2. **一级扁平导航（`src/router/routes/modules/audit.ts`）**：
   - `path: '/audit'`，`name: 'AuditLogs'`，`icon: 'lucide:scroll-text'`，`order: 3`（紧接 `overview` `order: 1` 与 `users` `order: 2`），不堆叠二级子菜单。
3. **总览页权限目录与快捷入口（`src/views/overview/index.vue`）**：
   - `PERMISSION_CATALOG` 补充 `read_audit: '审计日志查阅'`；并对后端返回的任意未收录新权限码提供回退渲染（直接展示原始 `code` 标签而非静默丢弃）。
   - 当会话具备 `read_audit` 权限时，在总览页渲染「审计日志」快捷跳转卡片。

---

## 3. API 客户端与错误恢复契约（W2）

1. **`src/api/client.ts`**：
   - 从 `@plateau/api/admin/audit/v1` 导入 `createAuditServiceClient` 与 `type AuditService`（含 `ListEvents`、`GetEvent`、`ListServices`），挂载至 `createAdminApi(signal)` 的 `audit` 属性。
2. **`src/api/errors.ts`**：
   - 接入 `@plateau/api/admin/audit/v1/audit.errors` 的 `AdminAuditErrorReason` 与 `isAdminAuditErrorReason`：
     - `ADMIN_AUDIT_ERROR_REASON_EVENT_NOT_FOUND` → `category: 'not_found'`（提示记录不存在或已超过保留期清理）；
     - `ADMIN_AUDIT_ERROR_REASON_INVALID_PAGE_TOKEN` → `category: 'invalid_argument'`（列表页捕获后清空游标栈回到第 1 页重新加载）；
     - `ADMIN_AUDIT_ERROR_REASON_INVALID_ARGUMENT` → `category: 'invalid_argument'`；
     - `ADMIN_AUDIT_ERROR_REASON_DOWNSTREAM_UNAVAILABLE` → `category: 'unavailable'`，**显式标记 `preserveSession: true`**，保留登录态并在表格上方渲染常驻 `Alert` 错误横幅与「重试」按钮；
     - 当审计请求返回 `403`（`SECURITY_ERROR_REASON_PERMISSION_DENIED`）时，调用 `authStore.ensureSession(true)` 刷新在线权限快照，而不清空有效登录会话。

---

## 4. 审计日志列表页交互与状态设计（`views/audit/index.vue` & `schemas.ts`）

### 4.1 顶部快捷视图、折叠筛选栏与服务自动发现

- **顶部 `Segmented` 类别快捷切换**（切换时重置游标栈至第 1 页并触发查询）：
  - `全部`（不传 `category`）
  - `认证与会话`（`AUDIT_CATEGORY_AUTHN`）
  - `资源变更`（`AUDIT_CATEGORY_RESOURCE`）
  - `授权拒绝`（`AUDIT_CATEGORY_AUTHZ`）
- **两行可控折叠搜索栏（`showCollapseButton: true`）**：
  - **首行常驻 4 项**：
    1. **时间范围（`timeRange`）**：`RangePicker`（支持时分秒与「近 1 小时 / 近 24 小时 / 近 7 天 / 近 30 天」预设）。**重要不变量**：预设选定或页面初始化时必须写入固化的起止时间戳，翻页过程中严禁动态重算 `now()`（确保 `audit.service` 查询指纹与 `pageToken` 一致）；当 URL 未携带精确过滤参数（`traceId`、`actorId`、`resourceName`、`operation`、`event`）时，默认初始化为固化的「近 7 天」，同时允许一键清空查询全量 180 天；若带精确 URL 参数进入则默认不加时间限制以免漏查早期记录。
    2. **主体类型（`actorType`）**：全部 / 人类（`human`）/ 服务（`service`）/ 匿名（`anonymous`）。
    3. **执行结果（`outcome`）**：全部 / 成功（`true`）/ 失败（`false`）。
    4. **生产服务（`service`）**：`AutoComplete` 组件。候选列表合并三部分并按字典序去重：① 页面挂载时异步调用 `audit.ListServices({})` 返回的已观测服务；② 前端内置三个平台基准服务保底（`//iam.service`、`//admin.service`、`//audit.service`，保障空库冷启动可用）；③ 当前列表页事件中出现的新服务。下拉展示与表格主视图统一剥离前导 `//` 显示友好名称（如 `iam.service`），发起查询时 `normalizeServiceFilter` 自动为未带 `//` 的非空输入补齐 `//` 前缀以精确匹配后端 `WHERE service = ?`。
  - **折叠区 4 项**（当 URL Query 命中任一折叠字段时自动展开）：
    5. **主体 ID（`actorId`）**
    6. **目标资源（`resourceName`，如 `users/019...`）**
    7. **操作名（`operation`）**
    8. **链路 `traceId`**

### 4.2 表格列与无 `totalSize` 游标栈翻页

- **表格列定义**：
  1. `发生时间`（`occurredTime`，本地格式化精确到秒，悬浮展示毫秒与 UTC）；
  2. `类别`（彩色 `Tag`：认证与会话 / 资源变更 / 授权拒绝）；
  3. `生产服务 & 操作`（主行显示简短方法名 + 去 `//` 服务名徽标，Tooltip 展示完整原始 `service` + `operation`）；
  4. `操作主体`（`actorType` 标签 + 等宽字体 `actorId`，匿名显示「匿名请求」）；
  5. `目标资源`（`resourceName`，空时显示 `-`）；
  6. `结果`（成功绿标 / 失败红标 + `errorCode` 摘要）；
  7. `来源 IP`（`clientIp`）；
  8. `操作`（固定右侧「详情」按钮）。
- **无 `totalSize` 游标分页栏**：
  - `ListEventsResponse` 不返回全表 `COUNT(*) FINAL` 总数；底部自定义翻页状态栏展示「当前第 X 页 · 本页 Y 条」，提供 `20` / `50` / `100` 每页条数切换与「上一页 / 下一页」按钮（状态纯函数独立维护于 `views/audit/schemas.ts`）；
  - 当规范化查询签名 `querySignature`（全部筛选条件 + 快捷类别 + `pageSize`）变化时，自动重置 `cursorStack = ['']`、`currentPageIndex = 0`。

---

## 5. 审计详情抽屉与跨页双向跳转（`audit-detail-drawer.vue`）

1. **零冗余加载与深链直达**：
   - `ListEvents` 返回的 `Event` 已包含全部 19 项字段，从列表行点击或抽屉内同链路节点切换时**直接渲染内存已有 `Event`**，不重复发 `GetEvent`；
   - 仅当用户点击抽屉右上角「刷新」按钮，或通过 URL `?event=events/<id>` 深链直达（内存尚无该对象）时调用 `GetEvent`。
2. **同 `trace_id` 跨服务问责链路串联**：
   - 当 `traceId` 非空时并发调用 `ListEvents({ traceId, pageSize: 200 })`（对齐后端 `maxPageSize = 200`），按 `occurredTime` 正序排列展示跨服务各跳记录（如 `admin.service` `human:<uid>` → `iam.service` `service:admin`），支持在抽屉内一键切换高亮节点；
   - 若响应 `nextPageToken` 非空（单条链路超过 200 条记录），展示截断提示横幅并提供「在列表中按此 TraceID 查看全部」按钮（回填筛选栏并关闭抽屉）。
3. **用户管理页 ↔ 审计日志页双向跳转闭环**：
   - `user-detail-drawer.vue`：当管理员持有 `read_audit` 权限时，展示「查看账号变更审计」（跳转 `/audit?resourceName=users/<userId>`）与「查看该账号操作审计」（跳转 `/audit?actorId=<userId>`）；
   - `audit-detail-drawer.vue`：当记录 `resourceName` 形如 `users/<userId>` 或 `actorType === 'human'` 且 `actorId` 非空时，提供跳转至 `/users?userId=<id>` 查看对应用户详情的快捷链接；`views/users/index.vue` 支持识别 `?userId=<id>` 自动拉取并打开该用户详情抽屉。

---

## 6. 全链路真实联调与人工验收驻留方案（W4）

在本地基础设施与三个后端服务 + 两个前端全部真实运行下执行端到端浏览器验证，并**在验收通过后保持全部服务在线供人工验收**：
1. **基础设施与模型就绪**：确认 Docker 中 PostgreSQL、Redis、OpenFGA、Kafka、ClickHouse 健康运行；执行 `just openfga-model-apply` 并确认 `service:admin` 对 `iam:global`（`manage_users`、`expunge_users`）与 `audit:global`（`read_events`）元组已就绪。
2. **核心场景实测闭环**：
   - **场景 A（认证成败与脱敏）**：在 IAM Web 输入错误密码触发登录失败，随后正确登录；在 Admin Web「审计日志 → 认证与会话」核对失败记录（`AUTHN_ERROR_REASON_INVALID_CREDENTIALS`、`detail_json` 无密码明文）与成功记录，以及 `ListServices` 动态返回的服务候选。
   - **场景 B（跨服务管理变更、双跳关联与用户页互跳）**：管理员在 Admin Web 创建/禁用/启用用户，从用户详情抽屉一键跳至「审计日志」查看该账号变更审计，点开详情抽屉验证同一 `trace_id` 下同时呈现 `admin.service`（`human:<admin_uid>`）与 `iam.service`（`service:admin`）两跳记录且可无额外请求切换，再从审计抽屉反向跳回用户详情。
   - **场景 C（403 授权拒绝）**：使用无管理权限的普通 IAM 账号访问 Admin Web 触发拒绝，在「审计日志 → 授权拒绝」核对 `SECURITY_ERROR_REASON_PERMISSION_DENIED` 记录。
