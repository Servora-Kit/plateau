# 实施中止、问题清单与拆分交接

日期：2026-10-02。本文记录一次已中止并回退的实施，不是当前代码能力清单，也不是本任务的验收报告。

## 1. 用户最新决定与回退边界

- 停止本次整体实施，撤销 Plateau、Servora 的本轮代码、配置、生成物、测试和 spec 修改；仅保留本任务的交接与规划文档。
- Plateau 已回到 `main`；本轮自动创建的 `feat/admin-initial` 无提交，已删除。以后默认直接在 `main` 迭代，只有用户事先明确要求才开分支。
- 任务退回 `planning`，清除本会话的活动任务绑定，不归档为完成，不自动创建或启动子任务。
- 用户计划将任务拆成多个小任务。Admin 后端的新工程起点改为**完整复制当时的 IAM 后端，再在副本上改造成 Admin**，不继续采用本轮从零拼装的实现；本次尚未执行复制。
- 原 PRD 的产品范围继续作为拆分输入；原 design/implement 中“从 Example 新建 Admin、不得恢复 IAM 复制树”的工程步骤已被上述决定替代，不能继续机械执行原 S1–S6。
- 本轮验证进程与 `plateau-admin-check` Compose 容器、网络已停止并移除；隔离测试卷保留，未删除数据卷。临时 RPC smoke 脚本及运行密钥文件已删除。
- 已安装的依赖、工具和忽略的本地缓存不属于 Git 代码交付，不作系统级卸载。回退后的源码不包含下文描述的临时修正。

## 2. 用户审查指出的结构问题

| 问题 | 本轮实际情况 | 后续约束 |
| --- | --- | --- |
| 任务过大、整体把关缺失 | 同时实施 IAM 生命周期、首次改密、Admin BFF、两个 Web；局部修补多于整体规范检查 | 先拆任务、固定文件所有者和跨任务接口，每个小任务独立过结构与运行门禁，再集成；不把代理交付或生成成功当成质量通过 |
| 数据层绕开现有 Ent 写法 | Admin 用 Ent 建表，却在业务 repository 内另写多段原生 SQL 和另一套事务组织 | 以 IAM/Example 的 Ent repository、行锁和事务模式为基线；有确切适配缺口再定位所属层，不为方便写第二套持久化惯例 |
| gRPC 没有沿用既有示例 | 初版把 token 缓存、自写完成日志、配置拼装、错误映射混在一起；出现具体类型强转；后续仍重复调用 `BuildClientConfigIndex` | 参考 [worker_client.go](https://github.com/Servora-Kit/servora-example/blob/main/app/master/service/internal/data/worker_client.go)，使用 `iam_client.go`、`const iamServiceName = "iam.service"`、标准 `NewChainBuilder`、Dialer、repo 构造与薄 RPC 适配。连接复用和 cleanup 要明确，不重复扫描框架配置，也不机械重复每请求建连 |
| 工作流约定 | 助手未经要求创建 feature 分支 | 默认 `main`，不开分支、不提交、不发布，除非用户明确要求 |

上述架构问题在中止时尚未全部纠正。临时进行过 repo/文件重排、Ent 改写和配置调用方迁移，但没有完成全部 Wire/测试联动；这些半完成修改已一并回退，不能在后续任务中当作存在的基线。

## 3. 已复现的工程与测试问题

### 3.1 Vben 导入基线不完整

- `pnpm --dir app/admin/web install --frozen-lockfile` 的 postinstall 在 `packages/@core/base/shared/src/cache/index.ts` 缺失处失败；preferences、utils、表格模块仍消费该模块。
- 原根 `.gitignore` 的全局 `cache/`、`bin/` 规则误忽略源码：除 shared/cache 外，`scripts/vsh/bin/vsh.mjs`、`scripts/turbo-run/bin/turbo-run.mjs` 也缺失，导致 `vsh lint` 找不到模块。
- 中止前曾恢复六个 cache 源文件和两个 CLI 入口，随后随整体实施回退。2026-10-03 已在独立小事务中重新恢复；根 `.gitignore` 已精简并限定服务产物路径，移除误伤源码的宽泛规则，Vben 子目录无需维护额外例外。安装、显式 postinstall/stub 构建与两个 CLI 入口验证通过，浏览器已正常显示登录页。该缺失源码问题不再作为 Admin 子任务前置修复项，不代表 IAM 接入或完整前端验收已完成。
- cache 来源为上游提交 `f2b3b1255389e69313463bee69c15e0d871132a7`；CLI 入口 blob 为 `407754d4e0a4fa566727971baf0acd606462e476`。这是可重取的来源证据，不要求后续盲目覆盖更高版本。
- 独立 Vben workspace 需沿用 catalog，不能新增裸版本依赖后留下未使用 catalog 项；共享源码包使用 `.ts` 导入时，消费端需正确配置 `allowImportingTsExtensions`。
- Admin Web 曾完成生产构建和四条错误分类测试，但最后 lint 仍有三条 Vue closing-bracket 格式错误；不能把“构建成功”写成全部前端门禁通过。安装期间还有既有 peer/deprecation、可选 sharp 构建提示，不等同于新增业务失败。

### 3.2 Proto 配置与枚举

- `conf.field` 的默认值输入可以使用 `720h`、`1h`、`15m`；YAML/env 经 protobuf JSON Duration 解码时，需使用秒值：`2592000s`、`3600s`、`900s`。不能混淆两种语法。
- IAM `TestDevelopmentConfigScan` 的 local/docker 曾因 `15m` 解码失败；修正加载值后该包通过。Admin 后续复制和新增配置也必须做真实 Scan，不只看 Proto 生成是否成功。
- 固定默认值和字段范围归 Proto conf/validate 注解；保留 optional 对显式 `false`、`0` 的区分，不在 Go 包装层重新填默认掩盖非法输入。
- 新枚举需遵守 Buf STANDARD 的零值命名要求；公开字段号与已发布枚举值不能被随意重排。
- 邮件部分完成、首次改密已提交等 reason 以生成的完整枚举字符串为准，例如 `USER_ERROR_REASON_USER_CREATED_VERIFICATION_EMAIL_FAILED`；不得在 HTTP 层另造未生成的短名协议。安全 metadata 只传允许的 `resource_name`，不能把未知超时推断成已创建。

### 3.3 IAM 回归夹具与编译门禁

- 新增的登录资格检查使旧 OIDC fixture 失效：它只建 User/Identifier，没有真实 Authenticator/PasswordAuthenticator，四个协议测试在初始化时得到 `record not found`。
- 临时修正改为普通用户聚合创建、真实邮箱验证和密码认证，不放宽生产首次改密规则、不清 flag 绕过测试；修正后 OIDC 包通过。后续测试应建立符合真实领域不变量的数据。
- `data/user.go` 曾出现事务内 `:=` 重声明导致编译失败，修正后才执行到数据库/transport 测试。Wire/Ent 生成成功不能替代完整编译。

## 4. 本轮实际验证与明确未完成项

以下仅是**已回退临时代码**的历史结果，不可作为重新实施的验收结果。

| 范围 | 实际结果 |
| --- | --- |
| IAM biz/authn/authz/mail/service/startup、共享 security | 包测试曾通过 |
| IAM data、server | 设置隔离 PostgreSQL、Redis、OpenFGA 后包测试通过，包含新增数据库与 HTTP/gRPC 场景 |
| IAM OIDC、cmd/server | 最初失败；上述 fixture/Duration 修正后两包测试通过 |
| IAM Web | lint/typecheck/build 通过；浏览器仅验证了后端不可用时首次改密页的错误提示和 GET 状态重查，没有完整登录/改密端到端验收 |
| OpenFGA | 模型 4/4 tests、12/12 checks 通过；隔离实例已写模型及 `admin-service` 服务关系，不代表 Admin 人类资格闭环通过 |
| Admin Web | 4 条错误结果测试、一次生产构建通过；typecheck 和最终完整 lint 未通过收尾 |
| Admin 后端 | 未完成统一 build/test/runtime 验收，停止时仍在结构重整；不存在可宣称完成的 Admin/OIDC/Consul 管理闭环 |
| Jaeger/日志 | 仅核对既有源码与用户历史运行观察；本轮没有实际 Admin → IAM 的 Jaeger 结果，也没有证实 obs 缺陷 |

未来优先保留的验证场景：PostgreSQL user-first 锁顺序、删除/恢复/purge/注册竞争及回滚；受限首次改密不能换普通登录/OIDC；DB 改密成功但 SCS 失败的部分完成结果；refresh/logout 跨实例竞争；初始化完成后不重新授予；下游服务身份失败不清人的登录态；后台自封禁/自删除/自撤权拒绝；邮件失败与未知超时明确区分。

## 5. 拆分输入（尚未创建子任务）

建议按可独立验收的交付物拆，而不是把原 S1–S6 直接改名：

1. **工程与 Web 基线**：按用户决定复制 IAM 后端作为 Admin 起点，核对 Data/Repo/Wire/生成、Proto namespace、独立数据库。复制完成前不得用原 IAM 配置启动 Admin 副本，避免连到 IAM 数据库或执行 IAM seed/provider 初始化。
2. **IAM 账号生命周期**：管理设密/强制登出、软删除/恢复/purge、自删与原子验证邮件流程；以真实 PostgreSQL 验证。
3. **IAM 首次改密与稳定初始化**：seed/管理创建 flag、受限会话、Web/OIDC 衔接、失败交付与稳定 binding。
4. **Admin 身份接入与权限**：标准 IAM client/Consul、OIDC BFF、共享 SCS、两层授权、初始化资格与本地退出。
5. **Admin 管理界面与联合验收**：两个菜单、生命周期操作、资格操作、部分完成、冲突与依赖失败，逐项覆盖父任务 AC。

具体子任务粒度和依赖仍待用户下一轮决定。复制 IAM 是工程起点，不改变“用户身份与凭据归 IAM、Admin 只拥有管理资格和自己的应用会话”的领域边界；复制后的 IAM 特有 provider、seed、身份仓储与 transport 暴露必须在 Admin 改造任务中显式处理，不能因为代码存在就把它们启动成第二个 IAM。
