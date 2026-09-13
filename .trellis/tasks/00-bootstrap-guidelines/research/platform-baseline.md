# Plateau 与 API 当前基线

调查基于启动时的 Plateau d1fa919 与只读源码，先建立以下当前规范，再由独立历史核对对照 OpenSpec。

| 范围 | 实际依据 | 提炼结果 |
| --- | --- | --- |
| 总体结构与边界 | 根 AGENTS、README、go.work、pnpm-workspace.yaml、ADR 0001/0002/0003 | 根 AGENTS 目录章节作为结构基础；IAM 全局身份池，tenant/membership 属业务；共享 API 和手写 client 分离 |
| AuthN 与 Actor | security/actor.go、authn/rules.go、authn/jwt、authn/session 及相应测试 | Actor Type/ID 不变量；PUBLIC 写 anonymous；具体 JWT/Session profile；缺规则失败；业务 mapper 持有身份语义 |
| AuthZ | security/authz/openfga/authz.go、middleware.go 及测试 | 使用官方 SDK；具体 subject mapper；BatchCheck 顺序/完整性；ListAllowed 裸 ID；Proto 目标校验与公开错误分类 |
| 密码、JWT、会话 | security/password/password.go、jwt/jwt.go、jwt/jwks.go、session/session.go | 密码升级需匹配成功；验签与 claims policy 分离；应用独立 SCS manager、Store 生命周期与 manager 装载标记 |
| CAP | security/cap/cap.go、cap_test.go、cap_integration_test.go | HS256 签名 challenge，Redis 管理一次性 nonce/token，scope 从服务端确定；不是通用权限模型 |
| 基础设施 | infra/openfga/client.go、infra/clickhouse/clickhouse.go 及 tests | SDK 配置适配与业务授权分开；带 token 禁止明文URL/redirect；ClickHouse 三态返回及 Close 责任 |
| 安全 codegen | 两个 cmd 插件、internal/codegen/optionmerge、ruleplan、plugintest | 方法非零 mode 整体替换；显式声明先校验；按 Go 输出目录和 operation 确定性分组；生成函数返回克隆 |
| API | api/AGENTS、Example User Proto、平台安全 Proto、Buf templates、api/gen/package.json/tsconfig.json、Just入口 | gRPC/HTTP 统一源；显式字段/presence/CRUD查询；共享 Go/TS 产物；当前 TS 直接导出源且 noEmit |

已核对的旧文档差异：根 AGENTS 缺少 security/session、web/packages，并残留不存在的 errors/entgo 目录；api/AGENTS 把当前 noEmit 写成构建 dist。已在允许的导航文档中同步实际目录与输出事实。security/cap/AGENTS 的 Redis challenge 概括未在本任务范围内改动，新 capabilities 正文明确签名 challenge 与一次性状态的区别；security/AGENTS 的 errors 手写目录残留同样由新结构规范澄清。

## 已运行检查

从 Plateau 根运行 `rtk proxy go test ./security/... ./infra/... ./cmd/...`，全部通过。security 和 infra 使用缓存结果；两个平台插件测试约 1.1 秒。测试覆盖的是列明包的单元／本地契约，不代表真实 IAM、OpenFGA、ClickHouse、CAP widget 或业务前端端到端验收。

当前所有 API/backend、API/frontend、client、gen 旧文件均与保存的初始化模板副本相同，没有新用户正文。32 个主代理负责的重复模板已归并/删除，空目录移除；其他组的清理见各自 baseline。guides 保留跨层和复用主题，去掉不适用的 Trellis 上游脚本同步与模板注册说明。

## 执行期间 Git 状态变化

执行途中观察到 HEAD 从 d1fa919 移至 fc6996e，暂存区出现初始化与部分正在编写的规范；本任务及助手未执行 reset/add/commit。保留该外部状态，不恢复其暂存项。范围外内容相比启动摘要的差异为 .gitignore，以及指向根 AGENTS 的 CLAUDE.md 符号链接（后者随允许的导航编辑变化）。源码保护检查按启动文件摘要执行，暂存内容另取观察时快照作对照。

后续检查又观察到三份前端文档的暂存 blob 更新；继续保留，没有用早先的 index 快照覆盖。OpenSpec HEAD 也变为 e31fd7634d2432362e5c483a8ad4a203d1d12591，但与启动提交 759a482 的 tree diff 为空，源规范内容未改变，工作树干净。最终状态在 verification 中按最后观察结果记录，不能声称整个执行期间 Git 元数据静止。
