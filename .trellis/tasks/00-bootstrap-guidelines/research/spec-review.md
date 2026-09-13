# 项目规范准确性审查

本次按主代理派发范围完成直接审查；唯一写入文件为本报告。spec、业务源码、生成物、配置、任务状态、hooks、暂存区和提交均未修改。以下行号取自本次读取的工作树；多人并行修订后，以问题引用的语句和源码符号复核。

结论：发现 6 项已确认事实／契约问题、1 项明确要求的覆盖遗漏，以及 2 项需要主代理裁定措辞的建议。不能据当前初稿宣告规范内容全部通过。待处理事项交主代理统一修订，本文没有执行规范修复。提交报告前复读发现 F5 已由并行协作者修正；当前剩余 6 项已确认问题／遗漏及 2 项措辞建议。

## 已确认的问题

### F1：ClearHelper 的 presence 条件写反

- 位置：`.trellis/spec/servora/framework/crud.md:13`，称只对“无 Proto presence”的 singular 字段执行清空。
- 真实依据：`../servora/contrib/db/entgo/crud/clear.go:102–125` 先跳过 list/map，随后在 `!leaf.HasPresence()` 时直接返回错误；只对支持 presence 且当前值 absent 的字段执行 override 或同名 `ClearField`。`clear_test.go:83–105` 的 `TestClearHelperRejectsUnnormalizedOrPresenceLessMask` 明确拒绝没有 presence 的 etag。
- 影响：把“字段类型没有 presence 能力”和“支持 presence 的字段当前未设置”混为一谈，会使消费者选择错误字段并在更新时报错。
- 修订：明确 `HasPresence() == true` 且当前 `Has(field) == false` 才表达 singular clear；无 presence 字段属于拒绝分支。保留 list/map 空集合由 setter 执行 replacement 的说明。

### F2：CRUD 列表调用遗漏 Ent builder，也漏掉业务过滤的实际执行点

- 位置：`.trellis/spec/service/backend/crud.md:70` 的 `entcrud.List(ctx, query, repo.listFields, scope.Fingerprint(...))`。
- 真实依据：`../servora/contrib/db/entgo/crud/list_execution.go:43–49` 的签名为 `List(ctx, builder, query, fields, scopeFingerprint)`；`app/example/service/internal/data/user.go:137–143` 的第二个实参为 `repo.data.Ent(ctx).User.Query().Where(entuser.TenantIDEQ(scope.TenantID()))`。
- 影响：当前调用形态不能编译；更重要的是没有展示真正执行 tenant 筛选的位置。scope fingerprint 只绑定分页上下文，并不替 repository 添加数据库过滤。
- 修订：补入已按业务作用域限定的 Ent builder，再传 `ListQuery`、`ListFields` 与 fingerprint。可直接使用上述 Example 调用的短片段，明确查询过滤与 token scope 校验是不同职责。

### F3：API 权威规范遗漏实际被 Example 前端消费的服务级 TS 生成链

- 位置：`.trellis/spec/api/proto/generation.md:7–16,28`。产物表只有共享 `api/gen/ts`，并概括服务命令复用根生成配置；验证段只要求根 `just gen`。
- 真实依据：`just/service.just:46–50` 的 leaf `api` 回根只生成 Go，随后使用服务自己的 TS 模板；`just/service.just:64–66` 的 `_gen` 只含 OpenAPI/Wire/Ent。根 `justfile` 的 `gen: api service::_gen` 因而不会刷新 leaf TS。`app/example/service/api/buf.typescript.gen.yaml:3–22` 将三个插件的输出写到 `app/example/web/src/api/generated` 并启用 clean；`app/example/web/src/api/userApi.ts:1–4` 实际导入这份代码。`.trellis/spec/example-web/architecture.md:7` 已准确记录该消费来源。
- 影响：修改 Example Proto 后仅按 API 正文运行根生成与共享 TS 检查，实际前端消费的生成代码仍可能过时；API 与前端主题的生成链描述不完整。
- 修订：在现有产物表补充服务级 TS 模板和独立输出，说明 root 与 leaf 输出互不清理；Example API 变更需要额外执行根入口 `just service::example::api-ts` 或在服务 leaf 执行 `just api-ts`，再检查相应前端。只修改文档时不执行生成。

### F4：service 层关于 Proto 错误的禁令与共通错误规则相互冲突

- 位置：`.trellis/spec/service/backend/service.md:7` 禁止将“Proto error 构造细节渗入 biz/data”，而 `coding.md:7` 明确要求 biz 输出生成的应用错误。
- 真实依据：`app/example/service/internal/biz/user.go:269–277` 的 `translateRepoError` 将 Repo 的 NotFound/AlreadyExists 转成生成 `ErrorUserErrorReason*` 并保留 cause；同文件的 etag 分支也直接返回生成业务错误。`internal/biz/AGENTS.md` 明确 biz 拥有业务错误，未禁止生成错误构造。
- 影响：按 service.md 审查会把本项目推荐的 biz 错误转换错误判为越层，或把业务错误决策移入 service。
- 修订：保留 RPC request 与 HTTP transport 类型不进入 biz/data；将 Proto/Kratos 错误构造禁令限定在 data，biz 的业务错误映射直接引用 coding.md 的权威规则。

### F5：微服务私有配置 Proto 的推荐路径不符合当前目录与源约定（已由协作者修正）

- 关闭复核：最终读取的 `layout.md:5` 已改为领域与私有配置均位于 `api/protos/`，并链接两个 IAM 真实配置源；原问题已消除。以下保留首次发现的证据，本审查未修改该规范。

- 位置：`.trellis/spec/service/backend/layout.md:5` 写为 `api/*conf.proto`。
- 真实依据：`app/AGENTS.md` 的服务结构将业务配置放在 `api/protos/` 内；当前真实例子为 `app/iam/service/api/protos/iam/conf/v1/config.proto`、`app/iam/service/api/protos/iam/oidc/conf/v1/config.proto` 与 `app/audit/service/api/protos/audit/service/conf/v1/audit_config.proto`。当前未发现 `service/api/` 根层的该类配置 Proto。
- 影响：新增服务可能按正文把源文件放到 Buf 输入目录之外，也没有保留用户要求沿用的目录层次。
- 修订：改为服务私有配置同样放在 `api/protos/<领域>/.../conf/v1/` 等与 package 对齐的版本目录，并列出 IAM 或 Audit 的实际路径，避免将不存在的通配路径写成现状。

### F6：IAM 前端把提交后清理责任归给了不会观察提交的 hook

- 位置：`.trellis/spec/iam-web/architecture.md:16` 称 `useSensitiveValue` 在卸载和提交后清除密码。
- 真实依据：`app/iam/web/hooks/use-sensitive-value.ts:15–28` 仅提供显式 `clear()` 和卸载时 ref 清理，没有提交监听或与请求 hook 的关联；`app/iam/web/app/login/page.tsx:42–49` 是页面在 `await run(...)` 后主动调用 `password.clear()`。注册、重置、安全页同样存在显式清理调用。
- 影响：新页面仅复用 hook 会误以为提交后已自动清理；这不是现有页面缺陷，而是职责说明失真。
- 修订：写为“hook 提供值、显式 clear 与卸载清理；页面在请求结束后调用 clear”，并链接现有登录页面的调用方式。

## 已确认的覆盖遗漏

### F7：CRUD 使用正文仍未交代 Mapper 与 ClearHelper 的构造契约

- 位置：`.trellis/spec/service/backend/crud.md:40–70` 已给出 Repo 字段和 ListFields 构造，但对 mapper 只有“配置 name 函数后”，对 clear 只有两个 option 名与 Apply 调用。缺少 mapper／clear 如何构造、name formatter 如何连接 PO 字段、构造错误及写入 Repo 字段的关系。
- 要求依据：`prd.md` R4 及 `design.md` 3.4 明确要求串联组件声明、构造、配置与调用，不能仅列 API 名称。这属于已确认范围内的完成度问题，不是建议增加模板数量。
- 真实依据：`app/example/service/internal/data/user.go:45–69` 展示 `NewResourceMapper` + `WithResourceName`，使用 PO 的 TenantID/ResourceID 生成 canonical name；随后构造 `NewClearHelper`，处理两类构造错误并分别赋值到 `mapper`、`clear`。
- 修订：在现有 crud.md 内补充上述短构造片段或同等明确的参数与错误说明；再明确 `PreparedUpdate.WriteMask()` 的返回值是传给 Repo 的 `*fieldmaskpb.FieldMask`，不是把外部请求 mask 原样下传。相关调用见 `app/example/service/internal/biz/user.go:150` 与框架 `core/crud/lifecycle.go:69–71`。不需要新文件、helper 或业务重构。

## 设计／措辞建议，未当作已确认业务缺陷

### D1：server 对浏览器会话的禁令应区分 HTTP 装配与领域管理

- 位置：`.trellis/spec/service/backend/server.md:11` 的“不要在 server 中管理……浏览器会话”。
- 依据：`app/iam/service/internal/server/http.go:43,61–78` 在 HTTP 层装配 `LoadAndSave`；`.trellis/spec/plateau/security/authN.md` 也要求先在 HTTP 边界装配会话生命周期。领域会话创建、解析、注销仍在 IAM biz/authn。
- 判断：若“管理”仅指领域状态，本条意图合理；但当前表述可能被解读成禁止必要的 HTTP filter 接线，需主代理决定准确措辞。
- 建议：明确 server 允许装配已有 SessionManager/filter，中间件不承载登录、撤销及存储业务。无需移动现有代码。

### D2：context.Background 的单例许可没有足够共同规则依据

- 位置：`.trellis/spec/service/backend/coding.md:5` 的“唯一可接受的本地初始化例子”。
- 依据：`app/iam/service/internal/authn/jwt.go:40–47` 的 `NewServiceAuthenticator` 也在构造期使用 Background 并注入已有 verifier；共享 `security/authn/jwt/authn.go` 的 injected verifier 分支跳过 discovery。当前正文未解释为何这个初始化用法违规。
- 判断：请求 context 必须贯穿调用链的规则成立；把可接受初始化限定为一个文件属于额外设计决定，不能从单个 Example 样例自然推导。
- 建议：改为“构造期使用独立 context 的一个现有例子”，或明确初始化 context 的生命周期标准；若确实决定禁止 IAM 写法，再将它作为待整改差异记录。当前不改业务代码。

## 其他核对结果

- 初审时 11 个配置 package 均被 `get_context.py --mode packages --json` 发现；默认 plateau，Servora 显示独立 Git 仓库。收尾前按开发者修订改为 11 个共享子层加六个扁平业务包，最终发现与索引结果见 [实施验收](verification.md)。
- 扫描 80 份 spec Markdown：443 个非代码区本地 Markdown 链接均可解析，无缺末尾换行、尾随空白或未闭合代码围栏。首次简单正则误将 Go 泛型调用识别为链接，排除围栏与行内代码后已消除误报。
- server 生产文件边界、biz/data 声明顺序、IAM 专有 internal 模块、AuthN/AuthZ/CAP/审计主题均有对应正文；框架审计与 Audit 服务维护限制已区分。Mapper/Clear 构造缺口见 F7。
- JWT 验签工具与 AuthN claims policy、Session manager 装载归属与 ContextExtender 的 Actor 不变约束、生成期整体替换与运行时 provider 覆盖、OpenFGA 动态目标与直接 subject／批量返回校验，经当前实现定向对照未发现新增矛盾。
- IAM Provider UI、Example 自有 fetch transport、Test 仅构建验证的差异已明确；没有要求业务端统一接入共享 client。敏感值 hook 责任问题见 F6。
- 没有重复读取完整历史 OpenSpec；覆盖表与最终 JSONL／hook 加载、任务验收状态由主代理和历史核对作者完成，本报告不替代其最终证据。

## 验证与限制

- 文档格式检查：通过上述针对 Markdown 的检查；内容准确性发现 F1–F7，其中 F5 已由协作者修正，其余待主代理修订后复核。
- 产品 Lint：本审查未运行。仅修改审查 Markdown，未改 Go/TS/Proto，不要求文档任务运行全部产品 lint。
- TypeCheck：本审查无新增可执行类型产物，不适用；未将静态源码阅读记为编译通过。
- Tests：未重复运行。派发记录提供主代理已通过 `go test ./security/... ./infra/... ./cmd/...`，本报告按该来源转述；CRUD 结论来自现行签名、调用和测试断言的静态核验，不宣称运行过相邻 Servora 或业务集成测试。
- 未执行生成、模型 apply、部署、提交、暂存、恢复外部改动或修改托管机制。


## 修订后关闭复核（2026-09-13）

本轮只复核 F1–F7、D1–D2，以及主代理指定的 Example leaf TS、CAP wire/legacy、OpenFGA model/PEP 新增说明；未展开全量规范或历史覆盖审查。前文为初审历史，保留不改；本节记录这些事项的最新状态。

### 原有九项全部关闭

| 编号 | 修订后的规范位置 | 关闭依据 |
| --- | --- | --- |
| F1 | `.trellis/spec/servora/framework/crud.md:15` | 已明确 singular 字段必须支持 presence 且当前 absent 才清空，无 presence 直接报错；与初审引用的 ClearHelper 实现及拒绝测试一致。 |
| F2 | `.trellis/spec/service/backend/crud.md:99–106` | 已补齐 `List(ctx, builder, query, fields, fingerprint)`，明确 tenant 条件在 Ent builder 中执行，fingerprint 只用于分页上下文校验。 |
| F3 | `.trellis/spec/api/proto/generation.md:7–17,31` | 已列出 Example 独立 TS 输出、root/leaf 生成差异及 clean 边界，并补充 `just service::example::api-ts` 和 Example Web 类型检查入口。与 `just/service.just`、leaf 模板和真实导入一致。 |
| F4 | `.trellis/spec/service/backend/service.md:7` | 已将 Proto/Kratos 错误构造禁令限定在 data，允许 biz 依据 coding.md 映射生成业务错误，消除了共同规则冲突。 |
| F5 | `.trellis/spec/service/backend/layout.md:5` | 再次确认领域与私有配置均在 `api/protos/`，两个 IAM 真实配置路径保留有效。 |
| F6 | `.trellis/spec/iam-web/architecture.md:16` | 已区分 hook 的显式 clear／卸载清理与页面在 `await run(...)` 后主动清理，并链接真实提交页面。 |
| F7 | `.trellis/spec/service/backend/crud.md:70–99` | 已补齐 NewResourceMapper、WithResourceName、NewClearHelper、构造错误及 Repo 字段赋值；明确 PreparedUpdate.WriteMask 返回规范化 FieldMask，并禁止原始 RPC mask 原样下传。 |
| D1 | `.trellis/spec/service/backend/server.md:11` | 已明确 HTTP server 可以装配 SessionManager 的 load/save filter，中间件装配与登录／撤销／存储领域业务分开。 |
| D2 | `.trellis/spec/service/backend/coding.md:5` | 已取消对单个初始化样例的唯一许可，列出 Example 与 IAM 两个构造期例子，并保留请求 context 传递和后台生命周期责任。 |

上述九项在本轮复核范围内无遗留问题；关闭的是文档事实、契约与措辞问题，不表示执行了业务代码整改或新增运行验收。

### 指定新增说明的定向核验

- **Example leaf TS：通过。** `generation.md:7–17,31` 与 `just/service.just:46–50,64–66`、Example 的 `api/buf.typescript.gen.yaml` 及 `web/src/api/userApi.ts` 一致。根生成不刷新 leaf TS，独立输出和消费者检查的描述准确。
- **CAP wire/legacy：通过。** `.trellis/spec/plateau/security/capabilities.md:17–19` 的路由、请求／响应字段、operation 常量和内部错误隐藏与 `security/cap/cap.go:514–552` 一致；legacy 不读取及协议范围与该文件开头注释一致。`cap_integration_test.go:234–258` 验证歧义 JSON 拒绝，`:260–332` 的 `TestCapWidgetV0157HTTPInterop` 覆盖基本 wire、未知 instr 拒绝及旧路由 404。这里的“本地 HTTP 互操作测试”是 Go helper 按 widget 0.1.57 算法构造请求（`:49–64`），没有运行实际 widget 或浏览器；正文保留了不等同真实浏览器／部署验收的限制，未发现新增事实错误。
- **OpenFGA model／PEP：通过。** `.trellis/spec/plateau/infra/openfga.md:13–17` 与 `manifests/openfga/fga.mod`、`iam.fga`、`admin.fga` 一致：schema 1.2、IAM 的 service 主体、Admin 的 user 关系派生正确。`justfile:159–179` 保证 validate/test 在 apply 之前；`openfga.sh:148–158,198–203` 写模型后更新所选 env 的 FGA_MODEL_ID，init 的 store 创建分支见 `:183–194`。`.trellis/spec/plateau/security/authZ.md:25` 对 PEP/PDP/PAP 的划分与已核验运行时一致，没有将 model apply、身份认证或测试 tuple 当作接收服务已完成业务授权的证据。

### 本轮验证边界

仅追加本节并检查追加内容格式、原报告字节前缀保留情况及本轮涉及规范的本地链接。未修改 spec、源码、生成物或 Git，未运行产品 lint/typecheck、已有测试、生成及模型命令；源码未变，无需重复此前测试。本轮没有发现需要新增登记的问题。
