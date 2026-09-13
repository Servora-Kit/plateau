# 0 号任务规划基线与调查入口

本文件保留规范建立前确认的归属、来源和调查顺序，继续作为任务背景；现行规范、实际调查结果和完成证据以 [PRD](../prd.md)、[design](../design.md)、[implement](../implement.md) 及 [实施验收](verification.md) 为准。规范已建立，本文件中的调查入口不表示仍有待实施事项。

## 已确定的任务边界

- 在 Plateau 的同一个 0 号任务中建设项目规范，并核对历史 OpenSpec。开发者与 AI 共同阅读和维护这些资料。
- 五个共享／框架 package：`plateau`、`api`、`web`、`service`、`servora`；六个业务端 package：`iam-service`、`iam-web`、`example-service`、`example-web`、`audit-service`、`test-web`。默认 `plateau`。
- 正文按 `.trellis/spec/<package>/<layer>/<topic>.md` 组织。各层有 `index.md`；命名依据职责，不强制 frontend/backend。索引链接不会自动展开，任务需显式加载必要正文。
- 根命令与共享生成逻辑归 `plateau/codegen`；母框架命令归 `servora/cmd`。不再设独立 `client`、`gen` 或 `cmd` package。
- `servora.git: true` 只提供独立仓库 Git 上下文，不改变规范加载、依赖联调或发布权限。
- 只修改规范、必要配置、任务资料及直接相关导航；工作区联调、业务代码、生成物、Trellis 托管机制、历史 OpenSpec 与其他独立仓库维持原状。

## 用户明确约定的权威入口

- 根 `AGENTS.md` 的“目录结构”是 `plateau/project/structure.md` 的正文基础；目录事实实施时核对，旧文档不存在的路径不能视为当前实现。
- `app/AGENTS.md` 的“服务结构”分别进入 `service/backend/layout.md`、`layers.md` 与各层编码正文。
- server 生产文件按服务端职责组织：`server.go`、`grpc.go`、`http.go` 及必要的 `sse.go`、`asynq.go` 等。
- biz 顺序：import → 必要业务常量／变量 → 一组或多组 `XxxRepo` 接口 → `XxxUsecase` → `NewXxxUsecase` → 方法。data 顺序：import → 私有 `xxxRepo` → `NewXxxRepo` → 实现业务 Repo 接口的方法。
- 应用专有 `oidc`、`authn`、`mail`、`startup` 等模块沿 IAM 习惯放在服务 `internal/` 下；模块归属不能由通用四层强行替代。
- CRUD 以 `app/example/service` 为主要参考，记录完整协作流程与推荐写法；用户列出的字段和调用入口保存在设计 3.4，实施已核对具体语义、顺序和 `entcrud.Bind`，结果进入共通 CRUD 正文。
- CRUD 消费流程归 `service/backend/crud.md`，内部契约归 `servora/framework/crud.md`，Example 业务细节归 `example-service/user-crud.md`。
- 基础覆盖包含测试、错误／日志／context／配置／清理、API 契约与兼容性、业务前端基本组织。具体归属见设计 3.5，不为各应用机械创建同一套文件。

用户已确认规范本身是依据。现有实现不同应记录差异，不能反向削弱要求，也不能把代码整改写成已完成。

## 实施时的来源入口

表中路径均相对 Plateau 根目录，是定向调查入口，不是已经核验完毕的实现结论，也不预登记为 JSONL 源码注入。

| 归属 | 首先读取的入口 | 调查重点 |
| --- | --- | --- |
| Plateau 总体 | `AGENTS.md`、`README.md`、`justfile`、`docs/adr/` | 结构、平台边界、端口、命令与有效决策 |
| 平台能力 | `security/`、`infra/`、`cmd/`、`internal/codegen/` | 安全、基础设施、生成职责与对应测试 |
| 平台 API | `api/AGENTS.md`、`api/protos/`、`buf.yaml`、`buf.go.gen.yaml`、`buf.typescript.gen.yaml` | 源定义、注解、生成流程与消费契约 |
| 共通服务与 CRUD | `app/AGENTS.md`、`app/example/service/`、`app/iam/service/` | 分层、编码风格、完整 CRUD 使用及差异 |
| 共享前端 | `web/packages/client/` | client 架构、协议和测试依据 |
| 母框架 | `../servora/AGENTS.md`、`../servora/core/`、`../servora/api/`、`../servora/cmd/`、`../servora/transport/`、`../servora/obs/`、`../servora/contrib/`、`../servora/security/`、`../servora/web/` | 框架内部约束及 Plateau 消费的真实契约 |
| 业务端 | 配置中六个业务 package 的源码路径、各自有效 AGENTS／README 和相关 ADR | 应用规则、前端基本组织、维护状态与检查入口 |
| 历史核对 | `../openspec/specs/`，必要时对应归档 | 在当前基线之后逐项核对 Requirement／Scenario |

## 检查时特别注意

- 初始 spec 模板是待整理对象，不是已经建立的项目权威规范；其中可能包含上游项目专有段落。
- IAM 是主要业务实践，内部 Example 是有效参考；Audit 的维护限制须保留，不能把它默认为新增服务范例。
- 不把一次实现细节直接推广为通用规范；优先使用确认约定、稳定契约和有适用范围的真实例子。
- `task.json.relatedFiles` 记录实际和计划的变更范围；JSONL 只引用真实、稳定且有用的规范／研究正文，两者不互相替代。
- 当前任务的 `in_progress` 来自初始化。只有实际激活和会话核对才证明任务绑定；加载函数预览成功不等于宿主 hook 已触发。
- 业务源码深读、CRUD 契约核对和历史覆盖安排在实施阶段。规划期间不以这些尚未执行的工作为由继续扩大调查或修改产品代码。
