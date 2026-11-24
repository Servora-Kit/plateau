# audit-v1：Admin Web 审计视图与联调

## 目标

在 `app/admin/web`（Vue Vben Admin）落地「审计日志」管理页面与同 `trace_id` 调用链关联详情抽屉，并完成跨前后端与消息/列式存储的端到端真实联调验收。

## 需求来源与范围

来源为 [audit-v1 主 PRD](../11-23-audit-v1/prd.md) §7、§8。涉及 `app/admin/web/**` 及跨服务联调验证记录。前置依赖 [IAM 与 Admin 审计接入及查询编排](../11-23-audit-v1-producers-bff/prd.md)。

| ID | 父需求 | 本任务交付 |
| --- | --- | --- |
| W1 | §7 | 在 Admin Web 导航新增与「总览 / 用户管理」同级的单个一级菜单「审计日志」（`/audit`，`order: 3`），打通 `router/guard.ts` 将会话 `permissions` 注入前端路由准入校验，按 `read_audit` 权限码控制菜单与页面访问，不堆叠二级子菜单。 |
| W2 | §7 | 实现审计日志列表页：扩展 `AdminApi.audit` 与 `resolveAdminApiError`（确保下游 503 故障保留登录态），提供类别快捷切换（全部 / 认证与会话 / 资源变更 / 授权拒绝）、时间范围、`actor_type`（人类 / 服务 / 匿名）、服务名、执行结果、`actor_id` / 目标资源 / `operation` / `trace_id` 过滤与游标栈翻页（过滤条件变更自动重置游标）。 |
| W3 | §7 | 实现审计事件详情抽屉：通过 `GetEvent` 展示完整标准字段（发生时间、服务与操作、主体、目标资源、客户端 IP/UA、结果与错误码、`trace_id`/`request_id`）、格式化 `detail_json`，并按同一 `trace_id` 自动拉取并按时间正序展示跨服务关联问责时间线（支持抽屉内切换查看任一跳记录）。 |
| W4 | §8 | 完成本地完整环境（PostgreSQL、Redis、OpenFGA、Kafka、ClickHouse、IAM、Admin、Audit）下的端到端浏览器联调验收。 |

## 验收标准

- [ ] AC-W1（W1、W2）：`just web::build` 与 `just web::lint` 通过；审计页面筛选、重置、翻页及空态/错误态反馈与后端真实数据一致，不使用任何前端硬编码演示数据。
- [ ] AC-W2（W3）：点开任意审计记录可查看结构化详情与 `detail` 载荷；对于管理员在 Admin Web 发起的用户变更操作，详情抽屉内可直观看到同一 `trace_id` 下 Admin（`human`）与 IAM（`service`）两跳完整问责记录。
- [ ] AC-W3（W4）：浏览器实测覆盖 IAM 登录失败/成功、Admin 创建/封禁/恢复用户、非管理员访问拒绝等核心路径，事件经 Kafka 入 ClickHouse 后可在 Admin Web 准确检索与核对。

## 依赖与交付协作

- 前置依赖：`11-23-audit-v1-producers-bff`。
- 页面组件拆分、状态流转与联调脚本方案见 [技术设计](design.md)；待子任务 3 完成后，结合生成的 `@plateau/api` 实际契约编写本任务的 `implement.md` 再启动实施。
