# Go 1.27.3 多模块升级与语法优化

## 目标

将 Plateau、Servora、servora-transport/server/tcp 三个本地 Go 模块统一到 Go 1.27.3，将不再维护的 servora-example 四个模块移出父级工作区，同时采用少量行为等价、确实更清晰的语法。

## 背景与证据

- 用户明确排除不再维护的 servora-example 仓库。父级工作区由七模块缩为三个，保留 Plateau、Servora、servora-transport/server/tcp；清单与移除可行性证据见 [upgrade-inventory.md](research/upgrade-inventory.md)。Plateau 内的 `app/example` 服务仍在范围内。
- 本机已安装 Go 1.27.3，官方标记该版本为 stable。当前父级工作区声明低于 Plateau/Servora 最低版本，实际 `go list -m` 因此失败；此次统一版本需要同时解除该冲突。
- 两仓已有 `new(expr)`、`errors.AsType` 使用先例。官方语义、采纳项的具体函数/行号、不采纳项与已完成的语言 smoke 见 [go-language.md](research/go-language.md)。

## 需求

- **R1 工作区精简与版本统一**：从父级 `go.work` 删除 servora-example 的根模块、api/gen、master、worker 四个 `use` 条目；其余三个 `go.mod` 与父级 `go.work` 声明 Go 1.27.3，保留模块路径不变。不删除示例仓库，不修改其自有工作区或任何文件。
- **R2 有效入口一致**：同步 Servora Conf 测试临时模块及 Plateau、Servora 现有 README 中框架/平台的 Go 要求；Servora CI 继续从根 `go.mod` 读取版本。保持现有工具链管理方式，不修改全局 mise 配置；不升级 servora-example 的 Docker 镜像或文档。
- **R3 保守语法优化**：仅采纳 `new(expr)` 的中间变量/四个私有测试 helper 清理、Example 单个 Kratos 错误的 `errors.AsType` 判断、Audit 单个 `strings.CutPrefix` 分支；迁移全部 helper 调用并删除旧实现。其余候选及拒绝原因保留在研究中。
- **R4 可观察验证**：逐一检查三个保留模块的独立构建，验证精简后的父级工作区，运行当前 CLI/插件和语义 smoke，分别记录成功、实际失败及环境限制。工作区通过不能替代独立检查。OrbStack 已由用户启动，不再将其列为未解决问题；旧示例镜像不属于验收项。
- **R5 分阶段授权**：实施在规划批准后开始；用户在验证完成后明确追加授权提交、归档和 push。仍不打标签或发布版本。
- **R6 用户追加修复**：修复 Servora `core/registry/etcd.go:211` 的 errcheck 发现，保持原有心跳注册失败后的重试、超时和取消语义，不增加忽略规则。

## 约束

- 公开 API、业务行为、错误身份/包装关系、nil/零值、显式标量类型与指针独立性不变；`int32Pointer` 调用保留 `*int32`。
- 不增加无必要的分配、复制或计算；保留现有 map key 预分配、排序与空切片形态。
- 代码修改前完成 GitNexus upstream impact 与需要的 LSP references；没有索引须先建索引，UNKNOWN 不能作为低风险结论。
- servora-example 的旧依赖、构建、测试及镜像均排除；移除四个工作区条目不影响其仓库文件与自有工作区。对保留模块仍须执行真实检查，避免成员减少后依赖选择变化造成回归。

## 验收标准

- [x] **AC1 → R1**：三个保留模块及父级工作区均为 Go 1.27.3；父级 `use` 仅包含这三个模块，无 servora-example 四个条目，原版本声明冲突消失；示例仓库文件未修改。
- [x] **AC2 → R2**：实际执行工具链为 go1.27.3；Conf fixture、平台/框架 README 要求一致，CI 仍依据 `go.mod` 选择版本；不触碰已排除的镜像和示例自有工作区。
- [x] **AC3 → R3**：全部采纳项完成且无遗留 helper 调用，指针类型、nil、原错误返回与 source 投影结果保持；未采纳项不被顺带改写。
- [x] **AC4 → R4**：三个模块各有独立 build/短测试结果；Plateau、Servora 增加竞争检查和 Go lint，精简后的父级工作区有实际解析/构建记录。此次改动引起的失败全部解决；Servora 原有 errcheck 已按用户追加要求修复，两仓 lint 均通过。
- [x] **AC5 → R4**：当前 CLI/插件与所改行为具有真实 smoke 证据；未运行项不标记为通过，不要求构建或测试 servora-example。
- [x] **AC6 → R5**：规划资料齐备并经用户明确批准后才进入实施；任务记录只包含实际执行的检查结论。
- [x] **AC7 → R6**：心跳失败通知改为容量为 1 的信号通道，注册失败仍触发重试；Servora 全仓构建、竞争测试、lint 通过，故障注入运行验证失败后恢复及取消退出。

## 不在范围

- servora-example 整仓排除，不修改其四份 go.mod、自有 go.work、README、Dockerfile 或源代码；只删除父级 go.work 中对应的四个成员。
- 不升级 Go 以外的业务依赖，不调整公开 API，不迁移历史框架接口，不修改端口或基础设施策略。
- 不修改第三方或生成代码来追求新语法，不运行全仓写入式现代化、格式化或无必要的生成。
- 不为采用 Go 1.27 泛型方法重构模块；不为风格统一改写复杂错误条件或排序结构。
- 不打标签、不发布版本，不提交父级本机工作区；提交、归档与 push 按用户后续明确授权执行。

## 方案资料

- [技术设计](design.md)
- [实施与验证计划](implement.md)
- `implement.jsonl`、`check.jsonl`：规范与研究上下文清单。

实施与验证已完成，具体命令、环境处理、最小修复和交付信息见 [实施记录](implement.md#执行记录2026-11-09)。用户已授权提交、归档和 push；本任务没有关联 PR，按直接 main 交付归档。
