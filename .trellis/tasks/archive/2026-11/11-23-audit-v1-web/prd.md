# audit-v1：Admin Web 审计视图与联调

## 目标

在 `app/admin/web`（Vue Vben Admin）落地「审计日志」管理页面与同 `trace_id` 调用链关联详情抽屉，并完成跨前后端与消息/列式存储的端到端真实联调验收。

## 需求来源与范围

来源为 [audit-v1 主 PRD](../11-23-audit-v1/prd.md) §7、§8。涉及 `app/admin/web/**` 及跨服务联调验证记录。前置依赖 [IAM 与 Admin 审计接入及查询编排](../11-23-audit-v1-producers-bff/prd.md)。

| ID | 父需求 | 本任务交付 |
| --- | --- | --- |
| W1 | §7 | 在 Admin Web 导航新增与「总览 / 用户管理」同级的单个一级菜单「审计日志」（`/audit`，`order: 3`），打通 `router/guard.ts` 将会话 `permissions` 注入前端路由准入并在会话权限刷新变化时触发路由访问重算，按 `read_audit` 控制菜单与页面准入；同步在「总览」页补齐 `read_audit` 权限文案、未知权限码原文回退与审计快捷入口卡片。 |
| W2 | §7 | 实现审计日志列表页：扩展 `AdminApi.audit`（`ListEvents`、`GetEvent`、`ListServices`）与 `resolveAdminApiError`（`503` 保留登录态并展示常驻重试横幅，`403` 动态刷新会话权限），提供类别快捷切换、8 项折叠筛选栏（无精确 URL 参数时默认固化「近 7 天」确定时间区间，支持清空查全量 180 天；生产服务结合 `ListServices`、前端三基准服务保底、当前列表未知项与 `AutoComplete` 自由输入并自动归一化 `//` 前缀）及无 `totalSize` 游标栈翻页。 |
| W3 | §7 | 实现审计事件详情抽屉与跨页跳转闭环：列表与同链路切换直接复用内存完整 `Event`（手动刷新或 `?event=events/<id>` 深链直达时调 `GetEvent`），展示完整问责字段与格式化 `detail_json`，按同一 `trace_id` 拉取最多 `200` 条关联记录按时间正序串联（超限时展示截断提示与跳回列表按钮），并打通用户详情抽屉与审计日志页的双向深链跳转。 |
| W4 | §8 | 完成本地完整环境（PostgreSQL、Redis、OpenFGA、Kafka、ClickHouse、IAM、Admin、Audit）下的端到端浏览器联调验收。 |

## 验收标准

- [x] AC-W1（W1、W2）：`just web::build` 与 `just web::lint` 通过；审计页面筛选、重置、翻页及空态/错误态反馈与后端真实数据一致，不使用任何前端硬编码演示数据。
- [x] AC-W2（W3）：点开任意审计记录可查看结构化详情与 `detail` 载荷；对于管理员在 Admin Web 发起的用户变更操作，详情抽屉内可直观看到同一 `trace_id` 下 Admin（`human`）与 IAM（`service`）两跳完整问责记录。
- [x] AC-W3（W4）：浏览器实测覆盖 IAM 登录失败/成功、Admin 创建/封禁/恢复用户、非管理员访问拒绝等核心路径，事件经 Kafka 入 ClickHouse 后可在 Admin Web 准确检索与核对。

## 依赖与交付协作

- 前置依赖：`11-23-audit-v1-producers-bff`（以及为动态服务发现补充的 `ListServices` 只读接口）。
- 页面组件拆分、状态流转与联调脚本方案见 [技术设计](design.md)，执行步骤与验证计划见 [实施清单](implement.md)。
