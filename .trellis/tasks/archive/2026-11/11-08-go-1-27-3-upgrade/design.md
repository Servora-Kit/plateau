# Go 1.27.3 升级技术设计

## 版本契约

- Plateau、Servora、servora-transport/server/tcp 三个模块的 `go` 声明统一为 `1.27.3`；模块与旧版本的逐项证据以 [升级清单](research/upgrade-inventory.md#工作区与模块) 为准。
- 父级 `../go.work` 声明改为 `1.27.3`，移除 servora-example 根模块、api/gen、master、worker 四个 `use` 条目；只保留上述三个模块。不提交父级工作区，不修改或删除 servora-example 的任何文件及自有工作区。
- 版本策略沿用现有 `go` 声明与 `GOTOOLCHAIN=auto`，不增加 `toolchain` 指令，不新增或修改用户全局 mise 配置。独立验证使用 `GOTOOLCHAIN=local`，使实际执行版本可核实。
- Servora CI 继续通过 `go-version-file: go.mod` 选择版本，保留 `GOWORK=off`。servora-example 的 Docker builder 与镜像构建已排除，不新增其他镜像升级。
- Servora Conf 行为测试创建的两个临时模块也声明 `go 1.27.3`，继续通过本地 replace 测试当前仓库，不修改依赖版本或行为断言。
- 在 Plateau、Servora 的现有 README 中明确平台/框架要求 Go 1.27.3+；Servora 对旧示例仓库的介绍不改成其已经升级的说法，不修改 servora-example README，不新增专用升级说明或 changelog 文件。

## 语法优化

仅采纳 [官方与代码研究](research/go-language.md#计划采纳) 中 A/B/C 三类已核实改写：

1. 使用 `new(expr)` 消除仅用于取址的中间变量与四个私有测试 helper，迁移所有调用并删除 helper。
2. Example User usecase 的单个 Kratos 错误判断使用 `errors.AsType[*kerrors.Error]`，继续返回原始错误，保留包装关系。
3. Audit `serviceFromSource` 的 `//` 分支使用 `strings.CutPrefix`，保留其他解析逻辑。

保持以下不变量：

- nil、零值、显式标量类型、指针独立性不变；`int32Pointer(0)` 必须变为 `new(int32(0))`。
- 错误树匹配、nil、包装错误、自定义 As 语义不变；不拆分或提前执行多项 `||` 条件。
- 前缀解析包括空串和仅 `//` 的结果不变。
- 不增加分配、复制、转换次数或额外业务计算；预分配 map key 收集、排序与非 nil 空切片形态不变。
- 不调整公开 API，不动生成物，不为了泛型方法或嵌入字段新写法改造模块结构。

## 影响分析与实施顺序

调研时 `gitnexus list` 没有索引；实施前已按下列顺序建立两个仓库的索引并完成目标符号分析，未将索引说明或空调用结果当作完整影响证据：

1. 先完成 Go 版本元数据更新，解除父级工作区 1.27.0 低于根模块 1.27.1 的解析错误。
2. 使用现有 GitNexus 的 `analyze --index-only` 为 Plateau 与 Servora 建立索引，不注入上下文文件。对 `hashTemporaryPassword`、`translateRepoError`、`serviceFromSource`、四个待删除 helper，以及被修改 fixture 所属测试函数逐项执行 upstream impact。使用 LSP references 验证全部 helper 调用，再做源代码改写。

HIGH/CRITICAL 先报告真实调用与执行流程风险；UNKNOWN 继续核实动态/同包调用，不按安全结论处理。索引或 LSP 失败时先排查工具或工作区问题，不跳过前置检查直接修改函数。

## 兼容与依赖控制

- 保持三个保留模块的 module path、`require` 版本不变；工作区仅移除用户已排除的四个示例成员，不全量升级业务依赖，不新增兼容包装层。
- 不运行写入式全仓 `go fix`，不默认执行 `go work sync` 或 `go get -u`。现代化分析器仅用于 `-diff` 预览，实际修改精确落在采纳项。
- 如 Go 工具确实要求整理 module 元数据，只接受与版本升级有因果关系的差异，禁止顺便刷新历史依赖。
- servora-example 不再维护，用户明确排除整仓；其旧 API 与依赖不迁移、不验证。Plateau 内 `app/example` 服务继续作为平台模块的一部分参加升级和语法优化。

## 验证设计

1. 静态核对三个 `go.mod`、父级 `go.work` 的成员/版本、Conf fixture、平台/框架 README，确认有效入口一致且旧示例文件未修改。
2. 三个保留模块分别执行 `GOWORK=off` 构建与短测试，Plateau、Servora 增加竞争检查与 Go lint。
3. 两仓目标包测试覆盖已改的密码处理、账户行为、CRUD filter、Audit source 解析及 Conf 插件生成编译行为。新测试只用于发现现有测试未覆盖且确有不确定性的可见行为，不为语法或文本写断言。
4. 在父级工作区显式覆盖三个保留模块的包模式，确认实际主模块与执行版本；不再验证 servora-example 自有工作区。
5. 运行实际二进制/插件 smoke，并用一次性程序比较改写前后的指针类型与独立性、错误身份、包装/自定义 As 和前缀极端输入。所有临时文件放仓库外并删除。
6. 用户已说明 OrbStack 启动，旧 Docker 未启动记录仅作历史证据。精简范围后没有 Go builder 镜像改动，不为容器可用而额外增加旧示例构建；保留模块如需已有中间件则按对应测试约定使用。

## 回退与记录

- 按仓库/文件保留可独立审阅的版本声明和语法修改；提交、归档与推送在用户明确授权后执行，不打标签或发布版本。
- 如某语法候选的真实调用或 smoke 表明行为/分配变化，保留原实现并记录证据，不通过额外兼容层勉强迁移。
- 实施与验证已完成，结果记录在 `implement.md`；用户已追加授权提交、归档和 push，按直接 main 交付处理。
