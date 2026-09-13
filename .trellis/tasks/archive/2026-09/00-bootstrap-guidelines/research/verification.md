# 0 号任务实施验收

日期：2026-09-13。任务由开发者“开始执行”指令激活，沿用 `00-bootstrap-guidelines`。本文记录实际运行的检查；规范中列出的其他命令只是后续开发入口。

结论：R1–R6 及范围保护的文档验收已完成。收尾前按开发者追加要求简化六个业务包目录并同步引用；规范内容、历史去向和加载机制可用于后续开发。既有业务差异与未运行环境验证不被记为已整改或已验收。

## 交付与需求对应

| 需求 | 交付及核对 |
| --- | --- |
| R1、R2 | 沿用已确认的 11 个 package、源码映射与 `default_package: plateau`；Servora 被解析为独立 Git 仓库。配置注释补充业务包根布局，无新增配置键 |
| R3 | 80 份 spec Markdown；11 个共享子层加六个扁平业务包，所有主题均有所属索引。六个业务包的 21 份 Markdown 已移至包根，并更新来源相对路径、规范互链和历史对照。初轮移除了 74 个旧模板路径，7 个路径保留并重写 |
| R4 | 根／app AGENTS 有效结构进入平台结构与 service layout/layers；server 文件职责、biz/data 声明顺序、IAM internal 模块及差异均有正文。CRUD 串联声明、构造、字段绑定、mapper/clear、规范化 mask、作用域查询与响应处理 |
| R4 基础覆盖 | 共享错误、日志、context、配置清理、测试入口、API 演进及各 Web 实际组织有权威归属；框架与业务端通过链接组合 |
| R5 | 当前源码基线、历史逐条对照、现行规则补充和差异处置分别记录；历史覆盖最终检查见下文 |
| R6 | 中文 PRD/design/implement、任务元数据和 implement/check JSONL 同步；六条显式正文加三份任务文档接受读取检查。源码按主题只读调查，未全量塞入注入清单 |

入口：[规范索引](../../../../../spec/index.md)、[CRUD 使用](../../../../../spec/service/backend/crud.md)、[迁移映射](migration-map.md)、[差异与处置](gaps.md)。

## 已运行检查

| 检查 | 结果与边界 |
| --- | --- |
| `rtk proxy python3 -B .trellis/scripts/task.py start .trellis/tasks/archive/2026-09/00-bootstrap-guidelines` 和 `task.py current` | 真实宿主会话成功绑定并返回当前任务；没有伪造 session ID 或触发模拟 hook |
| `rtk proxy python3 -B .trellis/scripts/get_context.py --mode packages --json` | 11 个 package、11 个真实 layer；六个业务包的 `specIndex` 指向包根 index，`specLayers` 为空。默认和任务 package 均为 plateau，Servora `isGitRepo: true`，`specScope: null` |
| `rtk proxy python3 -B .trellis/scripts/task.py validate .trellis/tasks/archive/2026-09/00-bootstrap-guidelines` | implement/check 均为六条，全部路径有效 |
| Markdown 与索引扫描 | 覆盖全部 100 份任务／规范／授权导航 Markdown（含未跟踪文件），全部本地链接有效，其中规范链接 513 个；无缺末尾换行、尾随空白、未闭合围栏或未索引主题 |
| 模板扫描 | 当前 spec 无 `To be filled`、`TODO: fill`、`placeholder` 等初始化占位；guides 已去掉不适用的上游模板同步规则 |
| `rtk proxy go test ./security/... ./infra/... ./cmd/...` | 全部通过；security/infra 为已有缓存结果，两个平台安全插件测试约 1.1 秒。没有改变 Go 源码；本地测试不代表真实中间件或业务端到端验收 |
| `rtk proxy git diff --check` | 通过 |
| `rtk proxy just --fmt --check` | 通过；本仓没有 `.github/workflows`，本轮业务代码未变 |
| `rtk proxy python3 -B -m unittest discover -s .trellis/scripts/tests -v` | 8 项回归测试通过，覆盖包根、子层、混合、缺失、scope 与单仓兼容；实际工作区六个业务包在 text/JSON/compact 中均显示真实根索引 |
| 包根与旧引用复核 | 现有 SessionStart 索引收集函数发现全部六个业务包根；全仓旧业务 backend/frontend 路径搜索无匹配。此项为读取函数检查，不宣称原生 hook 事件已触发 |
| 初轮暂存区 whitespace 检查 | 曾发现初始化 `journal-1.md` 末尾多余空行；开发者授权提交收尾后已修正，随本次提交重新检查 |

## 上下文读取边界

用现有 `.codex/hooks/inject-subagent-context.py` 的 `get_implement_context`／`get_check_context` 读取函数预览，而非触发 hook 事件。两种角色均完整读到六条清单正文与三份任务文档，共九份；逐份全文比较无缺失、截断或 stderr 提示。

工作代理按派发范围显式读取任务资料、写作指导与相关源码；无法从读取预览推断宿主原生 `SubagentStart` hook 已触发。本次保留读取预览和代理侧实际阅读作为上下文加载证据。目录修订只调整本地 package 发现，现有 JSONL 读取器和 hook 文件不变。

## 内容审查与历史核对

初审及关闭复核见 [spec-review.md](spec-review.md)。历史 Requirement／Scenario 对照见 [openspec-crosswalk.md](openspec-crosswalk.md)，发现与处置见 [openspec-findings.md](openspec-findings.md) 和 [gaps.md](gaps.md)。数量核对不能代替语义核对，未运行的历史能力保留未验收或待确认状态。

F1–F7 与 D1/D2 全部经独立关闭复核；后续有效历史遗漏已完成定向源码核对并补入现有主题。历史表分为 69 个可正常渲染的表格，共 467 条 Requirement、974 个 Scenario；逐个核对 1,441 个源文件／行号引用，无漏项、多项或遗失文件。

最终分布为：453 条 Requirement／940 个 Scenario 有相关代码或配置但未完成该条全部场景验收；6／13 为 Actor 与 ClickHouse 旧 owner 或路径过时；8／21 为本任务范围外的 Makefile 迁移历史。原 Mail/ClickHouse 六项归属疑点已按当前 Proto 与实现关闭；有效能力保留在正确主题，没有为匹配历史路径增写业务代码。

## 变更范围保护

实际写入范围为 `.trellis/spec/`、当前任务资料，以及根、app、api、app/example/web 四份 AGENTS 的导航与直接相关目录／生成事实。追加目录修订涉及配置注释、本地 package 发现及回归测试；提交收尾涉及日志空行和 `.gitattributes` 中失效模板文档指向。根 AGENTS 的 Trellis 托管块与启动副本逐字节相同。

启动保存的 743 个跟踪文件摘要中，追加授权涉及 `.trellis/config.yaml`、`packages_context.py`、`.gitattributes` 和初始化 journal；其他变化为外部 `.gitignore` 及指向授权根 AGENTS 的 `CLAUDE.md` 别名。业务源码、生成物和 Go/pnpm/Buf/Just 联调配置未被本任务修改。Servora 保持启动提交且工作树干净；OpenSpec 所在父仓的提交及 `.trae/` 删除由外部产生，历史源规范保持原位且内容未改变。

初轮实施期间 HEAD 和暂存区发生过外部变化，因此不宣称 Git 元数据全程不变；初轮未运行暂存、提交或恢复命令。开发者之后明确要求提交收尾，按工作提交 → 归档提交 → journal 提交执行。快照与观察记录见 [执行基线](execution-baseline.md) 和 [平台基线](platform-baseline.md)。现有无关 `.gitignore` 和 `.mcp.json` 暂存修改保留，不纳入本任务提交。

收尾复核期间，根 AGENTS 的托管块位置／简介和 app AGENTS 的维护提示另有外部整理；保留当前文件内容，不恢复旧段落。托管块正文仍与启动副本一致。

## 未运行与完成边界

未运行完整产品 lint、Servora／业务服务测试、前端 typecheck/build/test、真实 OIDC/CAP/邮件或中间件集成、浏览器端到端、Proto breaking 比较、生成、模型 apply、部署或发布。文档准确性检查不将这些未运行项写成已通过。

开发者已授权提交和 Trellis 收尾。工作已提交为 `e516191`，随后通过 `task.py archive --no-commit` 完成归档与会话解绑；更新归档相对链接、JSONL 自引用和 task 元数据，检查通过后形成独立归档提交，再记录 journal。未请求推送。

## 提交前最终检查

包根与实际子层索引完整，旧业务层路径无残留；100 份 Markdown 的 5077 个本地链接全部有效。实现／检查角色均完整读到九份文档（各 82226 bytes），无缺失、截断或 stderr；8 项 Python 回归测试、语法检查、Just 格式检查、工作区和暂存区 whitespace 检查通过。归档后重新检查 5077 个链接和六条 JSONL，全数通过；两种角色完整读取各 82416 bytes，当前会话不再绑定该任务。
