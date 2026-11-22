# admin-v1：Admin Web

## 目标

基于现有 `app/admin/web`（Vben Ant Design Vue 工程）交付供唯一平台管理员使用的轻量总览与 IAM 全局用户管理界面，完成跨仓 TypeScript 传输契约统一与 Vben 工程精简，接入真实 Admin 后端与 IAM 服务并通过浏览器端全链路集成验收。

## 需求来源与职责范围

业务需求来源于 [admin-v1 主 PRD](../10-04-admin-v1/prd.md)，对应父需求的全部业务规则与验收语义均适用；以下映射划分职责归属，不缩减父需求。IAM Web 的账号自助删除与首次登录强制改密交互由已归档的 [IAM 子任务](../archive/2026-11/10-18-admin-v1-iam/prd.md) 承担。

### 业务功能范围（W1–W7）

| ID | 父需求 | 本任务交付 |
| --- | --- | --- |
| W1 | §1、§8、§9 | 接入真实 IAM OIDC 认证与 Admin 本地会话，展示当前唯一管理员信息、无权限反馈和独立退出入口；页面交互与后端自我保护及同级管理员保护规则一致。 |
| W2 | §14 | 仅保留“总览”和“用户管理”两个一级菜单；轻量总览展示当前管理员真实信息与用户管理快捷入口，移除壳层演示控件（假通知、前端锁屏、个人中心、上游外链）与租户模式偏好项，不提供统计图表或演示数据。 |
| W3 | §2、§4、§14 | `/users` 单路由双视图切换“正常用户”与“已删除用户（回收站）”，采用游标分页并支持后端已实现的筛选与排序条件；右侧抽屉承接用户详情查看与 7 个资料字段编辑，邮箱只读、头像为 URL 文本，不新增一级菜单或详情子路由。 |
| W4 | §3、§5、§6 | 弹窗承接创建用户（默认勾选且显式布尔提交“要求首次登录修改密码”，区分创建失败、发信成功与发信失败降级）、待验证用户手动补发验证邮件、独立管理设密弹窗与强制登出 IAM 确认操作，准确表达各自效果。 |
| W5 | §4、§7、§9、§14 | 封禁／解禁（不设无效原因输入）、软删除、已删除用户查看、无限期恢复与仅面向已软删除用户的显式永久删除二级确认弹窗，遵守自我保护、无限期保留和邮箱占用规则。 |
| W6 | §12 | 按结构化错误原因区分本地未登录、人的权限不足、自我/同级管理员保护、领域冲突、发信失败与下游服务故障；不泄露内部错误或服务凭据，不将下游故障误判为用户登录失效。 |
| W7 | §13 | 前端静态检查、类型检查、生产构建、启动说明，以及真实浏览器 → Admin → IAM 的全链路集成验收。 |

### 工程与契约协同范围（E1–E4）

| ID | 协同目标 | 本任务交付 |
| --- | --- | --- |
| E1 | 请求调用链与契约接入 | 保持 `app/admin/web` 独立 workspace 与 lockfile，在 `apps/web-antd` 通过 `link:` 接入 `@plateau/api` 与 `@plateau/client`，以 `createHttpClient({ service: 'admin', retry: 0 })` + `createTransport` 装配生成的 `AuthService`、`UserService` 客户端，移除 `@vben/request` Mock/Bearer 请求链，并补齐 `@plateau/client` 无请求体非安全方法（如 `DELETE`）与后端会话过滤器的 `Content-Type` 契约对齐。 |
| E2 | 跨仓传输契约单一事实源 | 在 `servora` 的 `@servora/proto-utils` 新增纯传输契约子路径导出（`TransportMeta`、`ClientTransport`、`ServerStream`、`DuplexStream` 及路径编码函数），`protoc-gen-typescript-http` 生成产物与 `@plateau/client` 统一引用，消除生成代码重复内联与手写同构类型；本地经父级 `servora-kit/pnpm-workspace.yaml` 联调。 |
| E3 | Vben 工程与插件瘦身 | 移除 `app/admin/web` 中无关 UI 框架演示应用（`web-ele`、`web-naive`、`web-tdesign`、`web-antdv-next`）、`backend-mock`、`docs`、`playground`、`@vben/request`、`nitro-mock` 构建插件、非 Antd 样式导出及 `@vben/plugins` 中未使用的 `./echarts` 与 `./tiptap`，仅保留 `apps/web-antd` 并清理未引用 `catalog` 项。 |
| E4 | `package.json` 最小化 | 统一精简工作区内各前端 `package.json`：对私有与内部 workspace 包移除冗余发布及上游元数据（`author`、`homepage`、`bugs`、`repository`、`publishConfig`、重复入口声明及失效 `scripts`），仅公开 npm 包 `@servora/proto-utils` 保留发布必需字段。 |

## 交互与技术约定

- **组件范式**：保持 Vben 5 默认范式，采用 `Page` + `useVbenVxeGrid`（内置 `formOptions` 查询栏 + `#slot` 渲染 `ant-design-vue` 原子控件）+ `useVbenDrawer` / `useVbenModal` + `useVbenForm`（Zod 校验）+ Pinia。
- **同源开发与代理**：`apps/web-antd` 本地开发与预览固定监听 `10012`（`strictPort: true`），Vite 仅将 `/v1` 同源代理至 Admin HTTP 服务（默认 `http://127.0.0.1:10010`），Cookie 凭据按 `same-origin` 自动携带，浏览器不直连 IAM 管理接口且不持有服务凭据。
- **状态与缓存约束**：会话身份与实时权限码（`access`、`manage_users`、`expunge_users`）仅驻留内存 Pinia Store，不写入 `localStorage` / `sessionStorage`。

## 验收标准

- [x] AC-W1（W1）：真实管理员经 IAM OIDC 登录进入 Admin 工作区，普通 IAM 用户被拒绝并展示明确无权限提示；退出 Admin 后旧本地会话不可继续访问，IAM 登录会话不被主动终止；有效 Admin 会话不因 IAM 单独登出而被页面主动清除。
- [x] AC-W2（W2、E3、E4）：仅展示“总览”与“用户管理”两个一级菜单，无演示登录表单、演示菜单、壳层伪功能（假通知、前端锁屏、个人中心、上游文档外链、租户模式偏好）或虚构统计；总览显示真实当前管理员信息并可直达用户管理；无关演示包与冗余依赖已移除。
- [x] AC-W3（W3）：`/users` 正常用户与已删除用户双视图、游标翻页、条件筛选排序及右侧详情抽屉均反映 IAM 真实数据；7 个允许的 `profile.*` 字段可通过 `FieldMask` + `etag` 编辑保存，邮箱只读，头像为 URL 文本且无上传控件；变更后重新查询结果一致。
- [x] AC-W4（W4）：创建用户弹窗默认勾选“要求首次登录修改密码”，提交时始终显式传递最终布尔值（取消勾选显式传 `false`）；准确区分创建失败、创建且发信成功、创建成功但发信失败三种结果——发信失败时关闭创建表单、展示已创建身份与非重复创建提示，并支持对待验证用户手动补发验证邮件及反馈 `VERIFICATION_EMAIL_FAILED` 投递结果。
- [x] AC-W5（W4）：管理设密与强制登出 IAM 为独立操作且文案与真实效果一致；不暗示会终止 Admin 或其他应用本地会话，不附带密码邮件通知、邀请或再次强制改密语义。
- [x] AC-W6（W5）：封禁／解禁（携带 `etag`）、软删除／恢复（不要求页面 `etag`）结果与真实状态一致；重复软删除遇到 `USER_NOT_FOUND` 时提示目标不存在或已删除并刷新列表，不提示本次删除成功；回收站视图支持无限期恢复与受 `expunge_users` 权限控制的显式永久删除二级确认，不展示自动清理倒计时，不对正常态用户暴露永久删除入口；页面禁止对当前管理员自身发起封禁、软删除与永久删除。
- [x] AC-W7（W6）：人的未登录（401）、权限不足或保护拦截（403）、领域冲突（409）与下游服务/邮件依赖故障（503）展示清晰区分；下游 IAM 鉴权或网络故障转换的 `DOWNSTREAM_UNAVAILABLE` 绝不被误判为当前管理员登录失效或清除本地会话；任何失败均不提示操作已完成，不泄露密码、token、密钥或内部错误堆栈。
- [x] AC-W8（W7、E1、E2）：`servora`、`plateau` 与 `app/admin/web` 的代码生成、类型检查、依赖/循环检查、Lint、单测及生产构建全部通过；使用真实 IAM、Admin、PostgreSQL 与 OpenFGA 在真实浏览器中完成登录、查询、全生命周期变更、无权限拦截与依赖故障验收，提供可复现命令与实测结果，不以 Mock 替代。

## 依赖与交付协作

- 依赖已归档的 [Admin 后端](../archive/2026-11/10-18-admin-v1-backend/prd.md) 提供 OIDC BFF 握手、SCS 本地会话、`AuthService` 与 `UserService` HTTP JSON 接口。
- 依赖已归档的 [IAM 能力补齐](../archive/2026-11/10-18-admin-v1-iam/prd.md) 提供完整用户生命周期 RPC、OIDC 授权码交互及首次登录强制改密承接页面。
- 本任务承担父需求 §13 的最终真实浏览器端到端链路验证；验证通过后交由父任务 [10-04-admin-v1](../10-04-admin-v1/prd.md) 完成整体归档评审。

## 约束与不做

- 沿用父任务全部产品约束和排除项；前端路由守卫、菜单与按钮显隐仅改善交互体验，不替代后端认证与授权校验。
- 不新增管理员管理、权限分配界面、组织、租户、Audit 页面、个人账号中心或统计接口。
- 不新增前端请求自动重试、审计接入、跨应用退出联动、密码邮件通知或管理员身份接管能力。
