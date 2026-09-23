# servora-proto-conf-fix-optimize

## 目标

修复 Servora Proto conf 从声明、生成、加载到消费的断点，以唯一 `Apply() error` 提供可预测的配置契约，并同步 Plateau 现有消费者。用户不必编排检查与默认的内部步骤，也不需要新的组件配置包装层。

## 背景

旧任务 `09-16-proto-conf-contract` 及其实现已被撤回；本任务不继承旧设计、验收或发布批准。用户已逐项确认下列语义与范围，完整实施仍须在最终规划评审后单独批准。

当前源码证据、文件行号和实际探针输出集中保存在 [research/current-state.md](research/current-state.md)：F1 optional 标量生成失败，F2 集合级联断点，F3 默认建树与必填检查不稳定，F4 声明冲突/不支持规则未拒绝，F5 section 文档与行为不一致，F6 消费端默认/字段归属问题，F7 启动顺序及失败清理边界。F7 的现有启动顺序已被接受，本任务修其失败清理，不重排阶段。

## 需求

| ID | 已确认要求 | 证据/归属 |
| --- | --- | --- |
| R1 | 生成器支持承诺的字段形态；非法字面量/组合和不支持的声明在生成期报错。default 与 required 互斥，包括显式空字符串 default；不得静默跳过规则或输出不可编译代码。 | F1、F4；Servora conf 注解与生成器 |
| R2 | 对实际存在的普通子消息、repeated message、map value、已选 oneof 分支执行同一规则。普通父块缺失且未 required 时保持缺失，不执行内部规则；显式空对象逐层处理；required 父块缺失报错。不因后代 default 建树，Duration 等自身有 default 的原子字段仍补值。重复应用稳定，错误指出具体字段/集合位置。 | F2、F3 |
| R3 | default 只补缺失，不覆盖明确的 0/false/空字符串；required 只要求明确提供，不再等同非零。必要标量使用 Proto 原生的设置标记，不机械全 optional 化。非空、范围等要求独立表达并保留已有必要限制；集合长度不能冒充“是否已设置”。 | F1、F3、F5 |
| R4 | 明确 Proto 默认/约束、组件策略、SDK 转换与资源检查各自的唯一负责维护的位置；修复已证实的重复默认、静默忽略及未消费字段，不普遍新增 Validate/prepareConfig。保留必要的 Cookie/TLS/HTTPS 等协议和安全检查，不统一强塞进生成器。 | F6；具体归属见 design.md 第 6 节 |
| R5 | 保持现有启动顺序：核心配置应用后创建日志/追踪，再扫描私有配置和装配业务。对应模块使用配置/创建资源前完成 Apply；错误阻止后续启动并释放已创建资源，补齐 Scan 失败路径。允许私有配置错误晚于基础 runtime 初始化。远端来源连接保留现有加载顺序并明确关闭责任。 | 用户确认；F7 |
| R6 | 采用破坏性干净切换，同步两仓受影响的 Proto、生成物、真实调用方、配置和当前示例。覆盖 IAM、Audit、Example 与平台共享配置；不保留兼容层、deprecated 别名、旧 key 或占位字段，不新增 reserved，不额外重排/复用字段号。旧配置/生成物不保证兼容。分别验证本地源码与真实独立依赖，不只报告 go.work 成功。 | 用户确认；F1–F7 |
| R7 | 配置处理生成文件只公开 `Apply() error`，内部直接完成检查、默认和递归；删除 ApplyDefaults、CheckRequired、ApplyConf、SectionKey、SectionOptional，不留转发包装。确有复用或递归需要才使用私有辅助函数。 | 用户明确要求；`../servora/cmd/protoc-gen-servora-conf/main.go` |
| R8 | bootstrap 自动调用交给它的配置对象的 Apply。底层 Scan、自行解码或手工构造的独立入口，在使用配置前调用同一方法；不按 contrib/私有 Proto 目录区别待遇，不在每个 getter/provider 重复插入。Apply 只处理配置数据，不加载来源、读取证书、创建 SDK 客户端或启动服务；不引入 applied 标记。 | `../servora/core/bootstrap/scan.go:75-103`；Kafka/OpenFGA/Session 现有入口 |
| R9 | CORS 的 allowed_origins/methods/headers 默认只在 `transport/server/http/cors/cors.go` 所属实现持有，移除重复 Proto 列表 default。显式启用时省略/空列表使用内置默认，非空覆盖；origins 默认 *，允许任意来源。nil/enable=false 不启用；空列表不表示禁用默认。构造时确定有效设置，不在每请求分配默认。这是 CORS 策略，不推广成其他字段的零值回退。 | 用户确认；`../servora/transport/server/http/cors/cors.go:11-34,45-63` |
| R10 | section 仅保留 bool 标记。section=true 时以 message 短名转 snake_case 作为 key，缩写按词分组，不裁剪后缀、不提供 override/别名；无标记整份扫描。缺段跳过、不建树、不 Apply；存在段解码并 Apply，错误正常返回。bootstrap 从描述符识别标记，不生成定位方法。加载可选不取消字段 required、应用根字段 required 和实际使用前的必要参数检查。13 个段中 5 个 key 直接迁移。 | 用户确认；research 第 7 节的命名表 |

## 范围与限制

- 只改当前配置契约及其真实消费链。Admin 首期任务不变；OAuth/账号功能建设、端口调整、Audit 业务重构、热重载和无关业务 API 不在范围。
- 注释和文档使用易读中文，不夹杂难懂的英文概念；实际 API 名称、标识符、配置键保留。直接更新当前用法和示例，不新建独立迁移指南；不删除无关已有 reserved。
- 不引入全局原始配置字段设置状态追踪、通用配置事务/回滚层或全量配置前置检查阶段。Apply 失败后不得继续使用对象，但不承诺回滚已补默认。
- 不把“只公开一个 conf 处理方法”扩大成全仓 protobuf/验证生成器迁移。技术方案复用现有值约束能力，不为各组件新建验证框架。
- 不混淆配置字段与 SDK 参数语义。显式值不被框架重写，不等于 SDK 自身的零值语义自动改变；运行时策略的保留/归一清单见设计。
- 发布、commit/push/tag、BSR push、部署和外部通信均需另行明确授权；不伪造版本，不提交临时本地 replace/workspace。

## 验收标准

- [x] AC1（R1）：标准 Go 生成物与配置处理生成文件一起编译并执行；optional 标量正确，冲突/不支持声明在生成期失败且包含字段路径。
- [x] AC2（R2）：从真实根配置/section 入口触达日志集合等规则；必填叶子缺失被拒绝，默认生效；map 与选中 oneof 分支遵守相同契约，错误指出元素位置。
- [x] AC3（R2、R3）：缺失的非 required 父块保持缺失，required 父块缺失失败，显式空父块逐层处理；Duration 自身 default 不受误伤；重复 Apply 不产生新父块或不同结果。
- [x] AC4（R3）：真实来源合并/解码后，缺失触发 Proto default，显式 0/false/空字符串保持原值；required 以字段设置状态判断，内容约束独立生效；集合非空不冒充字段设置状态，CORS 策略按 AC12 单独验收。
- [x] AC5（R4）：受影响默认/校验有唯一负责维护的位置；必要 SDK/安全检查保留，已选模块的非法配置不被静默忽略；保留的字段实际被消费，不暗改业务策略/连接目标/TTL。
- [x] AC6（R5）：核心配置错误在核心资源初始化前失败；私有配置错误在对应模块使用前失败，runtime 的 Scan 错误路径关闭资源且不进入后续业务启动。不要求私有配置错误早于日志/追踪初始化。
- [x] AC7（R6）：两仓生成物、示例和入口一致。Servora v0.9.9 已正式发布，Plateau 已使用真实 Go 版本、对应 BSR 锁定提交和 v0.9.9 插件完成独立验证，没有本地依赖替换。
- [x] AC8（全部）：当前探针是调查证据，不是修复通过。完成 design.md、implement.md、上下文清单与最终规划评审；用户随后批准才进入实施。最终按实际执行结果记录各项验收。
- [x] AC9（R6）：旧字段/方法/key 直接移除，无新增 reserved、占位字段、deprecated 别名或兼容包装；真实消费者同步切换，不重排无关字段号。
- [x] AC10（R7）：配置处理生成文件仅公开 Apply，bootstrap 和受影响调用方使用同一入口，调用方无需编排内部步骤。
- [x] AC11（R8）：同一有效/无效配置经 bootstrap 与独立解码后调用 Apply，得到一致数据或错误；原始 Scan 不被误称为已应用配置，直接构造后的独立入口可用。
- [x] AC12（R9）：通过实际 HTTP 请求验证 CORS：nil/禁用不添加响应头；启用时省略与空列表使用内置默认，非空列表按配置匹配来源及输出方法/请求头，默认 origins 为 *。默认无重复来源。
- [x] AC13（R10）：自动 key 覆盖缩写/复合 message 名，整份根配置不被误当同名段；缺段跳过，存在段的解码/应用错误不吞掉。真实使用缺少必要参数仍失败；变更 key 的 YAML、环境覆盖、文档与调用方同步迁移，无旧别名。

## 当前状态

用户拥有的语义与范围决策已收敛，没有待回答的产品语义问题。

- [design.md](design.md)：生成/加载/使用职责、值约束方案、默认值的负责位置、生成自举和发布边界。
- [implement.md](implement.md)：实施顺序、文件清单、回归场景、命令及真实依赖门禁。
- [research/current-state.md](research/current-state.md)：F1–F7、实际扫描/生成探针、13 个段与 5 个 key、值校验和本地 Buf 实验。

本地实施、Servora v0.9.9 发布和 Plateau 正式依赖接入均已完成，验收结果见 research/current-state.md 第 11、12 节。按用户授权完成提交后，使用 Trellis 归档并记录会话；没有部署业务服务。
