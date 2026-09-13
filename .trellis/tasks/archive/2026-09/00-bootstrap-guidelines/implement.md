# 0 号任务实施清单：项目规范建设与 OpenSpec 核对

状态：规范正文、目录迁移、审查修订、历史对照和文档验收已完成。证据见 [实施验收](research/verification.md)；工作已提交，任务已归档，未推送。

需求以 [prd.md](prd.md) 的 R1–R6 为准，结构与归属以 [design.md](design.md) 为准。本任务沿用 `00-bootstrap-guidelines`，由一个负责人完成集成与验收，不要求拆分子任务。代码阅读已按主题完成并记录各组基线。

## 1. 启动与变更基线

- [x] 阅读三份规划资料和 [启动前检查记录](research/planning-review.md)，确认本轮用户指令已经允许进入执行；此前“生成产物直到执行之前”的指令不包含实施。
- [x] 刷新工作区、任务与 package 状态；保留初始化产生的 `in_progress`，不能将其当作会话已绑定或迁移已实施的证据。
- [x] 通过本地 `task.py start` 激活现有任务，随后核对 `task.py current`。若缺少真实会话标识，按工具提示使用宿主提供的标识，不伪造 ID，也不使用绕过上下文检查的选项。
- [x] 在开始修改 spec 之前，记录 Plateau／Servora 当前提交、未提交文件和暂存状态；为将修改或移除的现有规范及导航保存可恢复的逐文件副本。基线与文件清单记入任务研究资料。

以下命令在 Plateau 根目录运行；本轮已通过 `start` 激活，后续恢复先读取 `current` 与执行记录，不重复推断会话身份：

```bash
rtk proxy python3 -B .trellis/scripts/get_context.py
rtk proxy python3 -B .trellis/scripts/task.py validate .trellis/tasks/archive/2026-09/00-bootstrap-guidelines
rtk proxy python3 -B .trellis/scripts/task.py start .trellis/tasks/archive/2026-09/00-bootstrap-guidelines
rtk proxy python3 -B .trellis/scripts/task.py current
rtk proxy git status --short
rtk proxy git diff --cached --name-status
rtk proxy git -C ../servora status --short
```

本轮允许写入的实施范围：本任务目录、`.trellis/spec/`、确有必要的 `.trellis/config.yaml` 修正，以及 Plateau 中直接相关 AGENTS 的规范导航。开发者收尾前追加了业务 package 目录扁平化，范围相应包含本地 `packages_context.py` 的包根索引发现及回归测试。Servora 源码与文档、历史 OpenSpec 和其他独立仓库均作为只读来源；不改业务代码、生成物、工作区联调、无关托管脚本、skills 或 hooks。

## 2. 分组建立当前规范基线

每组都按“读取有效约定 → 按需查看实现和测试 → 写规范与索引 → 记录依据和差异”的顺序完成。主题位置见设计第 3 节，不重复在此维护完整文件表。

### 2.1 项目、母框架与 API 契约（R1、R3–R5）

- [x] 以根 `AGENTS.md` 的“目录结构”为正文基础建立 `plateau/project`，逐项核对现有路径，保留有效的端口、命令与平台边界约定。
- [x] 建立 `servora/framework`、`servora/proto`、`servora/cmd`、`servora/web`；覆盖框架开发规范、CRUD 内部契约、审计、生命周期、传输及已列出的其他能力。
- [x] 建立 `api/proto`，明确平台源 Proto、框架公共注解、生成入口和产物归属；补齐资源、查询、字段更新、错误与兼容性约束。
- [x] 按契约需要定向读取母框架实现，不为列出全部导出符号而扩大调查。当前不存在或尚未验证的能力如实说明。

### 2.2 平台共享能力（R3–R5）

- [x] 建立 `plateau/security`、`plateau/infra`、`plateau/codegen`，覆盖确认的 AuthN／AuthZ、actor、capabilities、credentials、现有基础设施与插件主题。
- [x] 分清注解声明、生成器解释与运行时执行三方职责；平台根命令进入 `plateau/codegen/cmd.md`，母框架命令进入 `servora/cmd`。
- [x] 建立 `web/client` 的架构与 HTTP／SSE／WebSocket 规范，依据实际 client 和测试说明消费边界。

### 2.3 共通微服务与 CRUD 使用（R4）

- [x] 将 `app/AGENTS.md` 的“服务结构”整理为 `service/backend/layout.md`、`layers.md` 和各层编码正文。
- [x] 落实确认的 server 文件边界、biz/data 声明顺序与命名、应用专有 internal 模块位置。对 IAM 中尚未遵循的命名记录差异，不在本任务中重命名代码。
- [x] 以 `app/example/service` 梳理 Servora CRUD 的完整使用流程，写入 `service/backend/crud.md`：串联 ResourcePlan／ListPreparer／ResourceNameMatcher、ListFields／ResourceMapper／ClearHelper 的声明、构造、字段配置与绑定、映射和响应调用。
- [x] 逐项核对开发者指出的 `service.plan.ToResponse`、`entcrud.NewListFields`、`entcrud.Columns`、字段绑定（`bind`）和 `repo.mapper.ToDTO`；明确实际符号、参数、调用顺序、适用条件及错误处理，避免把用户片段直接当作已验证代码。
- [x] 补齐 `coding.md`、`bootstrap.md`、`testing.md` 中的错误、日志、context、配置、清理、测试与检查入口；共通规则和应用差异分别归属。

### 2.4 业务端规范（R2、R4）

- [x] 建立 `iam-service`、`example-service`、`audit-service` 的业务正文，引用已有共享规范，仅补充各自领域和装配差异。
- [x] 建立或调整 `iam-web`、`example-web`、`test-web`：覆盖业务交互，以及实际适用的目录、路由、组件／状态、请求、加载与错误状态和测试方式。
- [x] IAM、Example 前端以 `architecture.md` 承载基本组织；Test 规模较小时并入 `purpose.md`，不创建空主题。
- [x] 保留 Audit 的维护状态和适用限制；不为占位 Admin/CMS 目录虚构实现，不把独立 `../servora-example` 纳入扩展。

阶段检查：每个主题有明确责任、实际来源和适用边界；每个分组有有效 `index.md`。正文需要更多调查时在任务资料记录，不以空模板宣告该组完成。

## 3. 历史 OpenSpec 与实现差异（R5）

- [x] 当前规范基线形成后，盘点 `../openspec/specs/` 的范围内主题，再按需要查看归档历史。
- [x] 建立 `research/openspec-crosswalk.md`，按设计第 7 节记录历史 Requirement／Scenario、当前归属、证据、状态与处理结果。范围外主题显式标记，不能悄悄遗漏。
- [x] 建立 `research/gaps.md`，记录规范与实现差异、历史未实现／过时能力及待处理事实。没有发现问题时记录检查范围和“未发现”，不留空模板。
- [x] 将仍然有效但初稿遗漏的约束补回权威正文；冲突时分别表达确认要求与当前行为，不用历史文档覆盖用户已确认的设计。
- [x] 所有差异均有处置结论：纳入规范、标明维护限制、等待必要决策或另行处理。不得为了消除差异直接修改业务实现。

## 4. 索引、旧模板与导航（R1–R4、R6）

- [x] 按设计第 6 节逐项核对旧规范去向，保留有价值的已有内容；新正文和引用确认后再移除对应旧模板。
- [x] 清理 `client`、`gen` 等已取消 package 的旧规范路径及失效引用，整理旧 backend/frontend 分组。不能仅改目录名就视为迁移完成。
- [x] 保留并检查 `guides` 的适用内容；发现 Trellis 上游专有段落或重复内容时按本项目职责整理，不将其当作 Plateau 的强制规则。
- [x] 检查全部层级索引、跨主题链接、共享与业务引用；仅按需要补充 Plateau AGENTS 导航，保留原有有效约定与托管块。
- [x] 按开发者追加要求，将六个业务 package 的 21 份 Markdown 移到包根并移除重复的 backend/frontend 目录；同步相对链接、历史对照、配置说明和规划资料。
- [x] 本地 package 发现增加包根索引支持，保留真实 `specLayers`，并通过扁平、分层、混合、缺失、scope 和单仓场景回归检查。
- [x] 按实际最终文件更新 `task.json.relatedFiles`。稳定正文需要参与后续任务检查时，显式加入对应 JSONL；源码和正在重写的文件由执行者按需读，不用目录条目注入全部内容。

## 5. 验证与完整审阅（R1–R6）

### 5.1 结构与上下文

```bash
rtk proxy python3 -B .trellis/scripts/get_context.py --mode packages --json
rtk proxy python3 -B .trellis/scripts/task.py validate .trellis/tasks/archive/2026-09/00-bootstrap-guidelines
rtk proxy python3 -B .trellis/scripts/task.py list-context .trellis/tasks/archive/2026-09/00-bootstrap-guidelines
rtk proxy rg --files --hidden .trellis/spec
rtk proxy rg -n 'To be filled|TODO: fill|placeholder' .trellis/spec
rtk proxy git diff --check
```

- [x] package 输出覆盖配置中的 11 个 package；实际分组与设计一致或有合理调整记录，每个被发现的分组都有真实索引。
- [x] 对本任务所有新增和修改的 Markdown 检查末尾换行、尾随空白、代码围栏、索引与链接；未跟踪文件也必须检查，不能只依赖 `git diff --check`。
- [x] 人工复核关键词命中；示例文字不等于模板残留。`rg` 无匹配时退出码为 1，不能误判为工具故障。
- [x] 两份 JSONL 都有真实条目，路径可解析、原因具体、无 `_example` 占位或源码全量注入。
- [x] 使用现有上下文读取器预览 implement／check 两种角色，核对三份规划和清单正文实际出现，检查截断与遗漏；再检查实际执行会话收到的内容。预览成功与宿主 hook 真正触发分别记录。

下列命令只调用本地读取函数，不触发 hook 事件、不设置会话绑定，输出读取规模与异常提示。启动前已按相同方式检查，实施后需要按最终上下文再核对：

```bash
rtk proxy python3 -B - <<'PY'
import importlib.util
from pathlib import Path
root = Path.cwd()
spec = importlib.util.spec_from_file_location('task00_context', root / '.codex/hooks/inject-subagent-context.py')
module = importlib.util.module_from_spec(spec)
spec.loader.exec_module(module)
task = '.trellis/tasks/archive/2026-09/00-bootstrap-guidelines'
for role in ('implement', 'check'):
    context = getattr(module, f'get_{role}_context')(str(root), task)
    print(f'{role}: {len(context.encode())} bytes')
    for line in context.splitlines():
        if line.startswith('[Trellis]'):
            print(line)
PY
```

### 5.2 内容、来源与范围

- [x] 对照 PRD 每一项验收检查正文与来源，特别检查开发者明确的结构／分层／文件顺序和 CRUD 流程，以及新增的基础工程覆盖。
- [x] 审阅 API、codegen、安全运行时、框架 CRUD 与消费方之间的契约引用，确保同一规则由一处权威正文维护。
- [x] 核对历史覆盖表与差异表，区分静态存在、测试覆盖和运行验收；规范中列出的命令不算已执行证据。
- [x] 复核全部文档变更，包括未跟踪文件和删除内容；对照基线确认业务代码、生成物、工作区联调、Servora 和历史 OpenSpec 未被本任务改变；本地发现兼容改动与提交前后的 Git 操作单独记录。
- [x] 在 `research/verification.md` 记录最终命令、结果、未运行检查与仍需另行处理的差异；依据真实结果更新 PRD 验收勾选。

本任务的必要验证是文档、引用、规范发现、上下文加载及包根发现的 Python 回归测试。业务测试或构建只在确有契约疑点、且可在既有环境中无额外业务改动地验证时定向执行；记录结果与限制。`just gen`、OpenFGA apply、部署、发布、依赖升级和工作区重建均不属于本次验证。

## 6. 整理失败与完成边界

- 旧文件归并或导航失败：使用实施开始前保存的逐文件副本，仅恢复本任务改动；不得用整仓重置、清理或模板覆盖处理。
- 发现工具无法加载布局：核对包根索引、实际子层和 JSONL，按本次设计修复包根发现兼容；不扩大到无关运行时机制。
- 发现业务实现不符合规范：保留规范及差异记录，业务整改另行处理。
- 规范建设通过验收后，报告完成范围和证据。提交、推送、归档及会话收尾按届时用户指令处理，不因写完文档自动进行。

本清单中的实施项全部完成并通过 PRD 验收，才表示本任务的规范建设内容完成。提交、推送、归档与会话收尾仍遵守本节边界。
