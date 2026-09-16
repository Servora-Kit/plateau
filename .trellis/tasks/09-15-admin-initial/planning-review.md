# 规划审查记录

日期：2026-09-16。范围仅为本任务规划文档，不是实施验收。

## 需求收敛

- PRD 保留既有编号，原 P1–P4 均已归入已确认需求；R20/AC20 记录根 AGENTS 与应用端口配置调整，新增 R21/AC21 记录带上下文和服务身份的 gRPC 实践及按需 Servora 协同迭代。
- 最新确认已记录：两个平级菜单；权限管理直接进入管理员管理；七个现有 profile 字段可编辑，邮箱只读。
- 统一 Admin 资格、无 tenant、自我保护限于 Admin 入口、不增加密码交付/邀请/额外通知等约束未扩大。
- 已形成 `prd.md`、`design.md`、`implement.md`，任务状态仍为 planning，等待整套规划评审及实施授权。
- 后续代码与结构复核见 [设计缺口记录](research/design-gap-review.md)：G2–G5 的技术合同已补充；用户最新回复“按照你说的处理”，确认 G1 的保留账号、明确发信失败和 IAM 重发行为，已同步 R16/AC15、design 5.4 和实施/浏览器验收步骤。当前无阻塞规划的产品问题。
- 按用户意见强化 R6/AC5 的 Kratos 错误和日志合同，并新增 R22/AC22 的 Ent 便利层迁移；明确生产 CRUD 不依赖 mixin，但 Example 与测试夹具依赖，不能直接删目录。
- 本轮完成 PRD 收敛整理：已确认决定归入对应需求/验收，移除重复的临时决策段和历史问答，保留 R1–R22、AC1–AC22 及所有源码证据链接。没有把本次单项回复当作完整实施授权。
- 用户补充后，端口工作按附带配置修改处理，不独立建立迁移阶段；端口目标为 IAM 10000、Admin 10010、Example 10080、Test 10090 四个十端口段，新登记表不列 Audit/CMS。
- 用户已删除 `app/admin/service`，本轮只读确认目录不存在；工程计划由整理 IAM 复制树改为参考 Example、IAM 与 Trellis 规范重新搭建。已同步 PRD、design、S1/S4 和上下文清单；旧复制树调研仅保留为历史快照，产品范围和验收标准不变。

## 交叉审查及修正

1. 当前邮箱验证跨事务消费 token/激活用户，和新增删除存在竞态：在设计及 S2 中明确合并用户锁事务，并涵盖重新发行验证/重置 token 的状态检查。
2. 恢复 pending 用户后的验证入口：明确使用 IAM 已有手动重发流程，旧链接不复活，恢复不额外自动发送邮件。
3. 密码合同：明确传递实际密码，redact 注解用于禁止日志泄露，不把脱敏后的固定值当作 RPC 输入。
4. 首次改密 DB 成功而 SCS 失败：区分事务失败与登录交付失败，明确 reason、使用新密码登录和一次性凭证不可重复消费。
5. OpenFGA 服务授权：明确 tuple 三元组；名单读取与每请求授权分别设计。
6. cookie 按 host/path 而非端口隔离：使用 IAM/Admin 独立名称与存储，并在开发环境避免无 Secure 的 `__Host-` cookie。
7. Admin refresh/logout 并发：使用自有 login 记录与数据库锁，SCS 只保存引用，不以进程内 mutex 或旧请求快照处理跨实例轮换。
8. 首次改密与删除路径统一 user → challenge/credential 锁顺序；初始化完成记录不会因账号禁用、删除、撤权或邮箱复用而清除。
9. 核实 SDK Read 支持直接 tuple 分页和一致性参数，共享 Check adapter 仍需改造；IAM introspection/refresh 要求同一发行 client，因此 Admin 用户 token 必须使用 admin-web 凭据校验，不能用 admin-service 代替。
10. 初始化公开接口改为 UserService.GetBootstrapUser，返回持久化初始化记录指定的 User；binding 仅保留为内部持久化概念，不出现在 RPC 名称与请求中。
11. 服务 client 和 `iam.manage_users` 在部署阶段装配，Admin 据此查询初始化用户后再授予人的资格；没有人类管理员登录的前置依赖。R21 明确请求上下文、服务凭据与人类资格的区别，并要求证实通用缺口后上游修正、两侧验证；不把 metadata 透传当作可信身份委托。
12. 邮件失败决定确认后的只读交叉审查发现实施计划的 Proto 路径省略了应用前缀；已统一写成仓库根下的 `app/admin/service/api/protos/admin/**`，与 design 的生成输入一致。排除此表述和已确认邮件行为后，未发现其他阻塞产品问题。

## 文档校验

- `task.py validate` 检查 implement/check 的 spec/research 路径；条目数以最新复核输出为准，R1–R22 与 AC1–AC22 编号保持连续。
- 本任务本地 Markdown 文件链接检查通过；行号后缀按文件链接语义解析。
- 本任务文本无尾随空格；`git diff --check` 通过。
- 本轮工作限于任务目录，未修改产品代码或 spec，未启动任务实施。
- 尚未运行 Admin/IAM 产品测试、构建、数据库迁移、OpenFGA 实际装配或浏览器联调。本轮为核实 Ent 便利层，实际重跑 Servora mixin/CRUD 单测与内存 SQLite live contract，全部通过；不是 PostgreSQL 或迁移后验证。后续仍按 implement 门禁及 AC 逐项验收，不将静态规划检查或迁移前基线作为产品功能通过证据。
