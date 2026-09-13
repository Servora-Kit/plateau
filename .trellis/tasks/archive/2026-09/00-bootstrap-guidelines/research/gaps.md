# 规范与实现差异及处置

本记录区分开发者确认的目标规范、现有实现和本轮验证结果。规范建设不要求先完成所有业务整改；下列范围外事项保留证据，未修改其实现。历史 Requirement／Scenario 的逐条核对另见 `openspec-crosswalk.md`，历史发现另见 `openspec-findings.md`。

## 当前基线与确认规范

| 项目 | 依据与现状 | 本轮处置与后续边界 |
| --- | --- | --- |
| data Repo 命名 | 开发者确认私有 `xxxRepo`／`NewXxxRepo`；[Example](../../../../../../app/example/service/internal/data/user.go) 符合，[IAM](../../../../../../app/iam/service/internal/data/user.go) 当前为 `userRepository`／`NewUserRepository` | [data 规范](../../../../../spec/service/backend/data.md) 保留确认的推荐命名；IAM 代码未重命名，后续修改相关模块时单独整理 |
| 目录与 TS 输出描述过时 | 根 AGENTS 残留不存在的目录并遗漏 session／web/packages；api AGENTS 将 [noEmit 配置](../../../../../../api/gen/tsconfig.json) 描述为生成 dist | 已在四份授权 AGENTS 的导航及直接相关事实范围内修正；共享 TS 为源文件 exports，Example leaf TS 为独立生成输出，见 [生成规范](../../../../../spec/api/proto/generation.md) |
| 安全下级 AGENTS 的旧概括 | [security/AGENTS](../../../../../../security/AGENTS.md) 的 errors 目录描述与现有生成错误归属不符；[CAP AGENTS](../../../../../../security/cap/AGENTS.md) 的 Redis challenge 概括没有区分签名 challenge 与 nonce/token | 两份文件不在本次导航编辑范围；新 [结构规范](../../../../../spec/plateau/project/structure.md) 和 [CAP 规范](../../../../../spec/plateau/security/capabilities.md) 已明确当前契约。后续维护这些来源文档时同步，不按旧描述改代码 |
| 应用请求实现尚未统一 | IAM 使用共享 client；Example 使用自己的 fetch adapter；Test 仅导入生成 API 标识，见 [前端基线](frontend-baseline.md) | 各应用规范分别记录真实消费方式；没有把共享 client 设计方向写成所有应用已接入，也未进行前端重构 |
| 流接口存在但应用使用未确认 | [共享 client](../../../../../../web/packages/client/src) 有 SSE／WebSocket；本轮搜索未发现业务应用消费或完整流交互验收 | 保留组件契约和验证要求；不声明应用端已支持或已验收 |
| Audit 维护状态 | [app/AGENTS](../../../../../../app/AGENTS.md) 将 Audit 标为不再更新和参考、等待重构 | [audit-service](../../../../../spec/audit-service/status.md) 明确保留维护限制；框架 [audit](../../../../../spec/servora/framework/audit.md) 独立维护，不用旧业务服务写法限制框架能力 |
| Audit 的无 ClickHouse 路径 | 当前 [BatchWriter](../../../../../../app/audit/service/internal/data/batch_writer.go) 在 ClickHouse 为 nil 时跳过存储并提交 batch | [摄取规范](../../../../../spec/audit-service/ingestion.md) 如实记录该维护限制；不能当作审计已持久化，也不作为新增服务推荐。未在本轮改为其他投递策略 |
| 生成器版本与本地源码联调 | 当前 checkout 安装 Plateau 插件，Servora 插件由 Just 安装配置版本；go.work 只决定源码模块解析 | 规范要求变更生成器时核对实际二进制来源；本轮未重装工具、重建产物或调整联调配置 |
| 运行验收尚未进行 | IAM 公开入口、真实 OIDC/邮件/CAP、OpenFGA 模型与 tuples、Kafka／ClickHouse 及业务 Web 端到端链路未在本轮运行 | 源码／配置存在与运行验收分开记录；已运行的 Go 定向测试只覆盖其包和测试边界，不抵销这些未运行项目 |

## 历史规则核对结果

69 份源 spec、467 条 Requirement、974 个 Scenario 均有来源与去向，共 1,441 个文件／行号引用。最终按 Requirement 记录 453 条代码或配置存在但未验收、6 条历史 owner／路径过时、8 条 Makefile 迁移历史处于本任务范围外；未把定向 Go 测试扩展成全部历史 Scenario 已验收。

最初的六项归属疑点已关闭：Mail 的三项本就只要求平台共享配置，发送行为在 IAM；ClickHouse 的三项对应能力已在 Plateau，Servora 路径过时。其余有效遗漏已补入现行规范，详情见 [历史发现与处置](openspec-findings.md)。没有需要本任务补写业务代码才能关闭的规划决策；运行环境验证和既有实现差异仍按本文件边界后续处理。

## 审查发现的规范问题

独立审查见 [spec-review.md](spec-review.md)。F1–F7 及 D1/D2 均属于规范准确性／完整性修订，不是本次发现并修复了业务缺陷。关闭结果和最终检查以 `verification.md` 为准，不能把初稿问题直接当作当前源码缺陷。

## 执行期间的外部 Git 状态

启动时 Plateau 提交为 `d1fa919`，工作树和暂存区为空；随后观察到 HEAD 变为 `fc6996e`，初始化与部分正文进入暂存区，同时 `.gitignore` 被外部修改。本任务未执行暂存、重置或提交，已保留观察到的状态；详细快照位置见 [执行基线](execution-baseline.md) 与 [平台基线](platform-baseline.md)。

`git diff --cached --check` 报告 `.trellis/workspace/HoronLee/journal-1.md:7` 的末尾新空行。已逐字节确认暂存文件与启动时 `d1fa919` 中该文件相同，属于既有初始化内容；它不在本次写入范围，未修改或重新暂存。范围保护检查也识别到 `CLAUDE.md` 是根 `AGENTS.md` 的符号链接，其内容变化来自授权的导航编辑，不是另行修改文件。
