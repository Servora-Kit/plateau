# 技术设计

## 1. 目标与边界

需求与已确认语义以 [prd.md](prd.md) 为准；当前缺陷及已执行探针见 [research/current-state.md](research/current-state.md)。本文件确定实现方式，不授权实施或发布。

最小改动边界是：Servora conf 注解、生成器、bootstrap 加载入口，以及两仓直接受影响的配置声明、生成物和使用入口。保留现有启动顺序，不新增配置初始化框架、逐组件 prepare/Validate 包装、全局已处理标记或原始 YAML字段设置状态跟踪层。

## 2. 注解与生成物

### 2.1 section

- 将 `SectionRule` message 及其 key/optional 字段移除，message option `section` 改为 bool；扩展号维持原有分配，不新增 reserved。
- `section=true` 表示按段读取；没有该标记的 target 继续整份扫描。
- key 由 Proto message 短名转 snake_case：缩写连续词作为整体，`OIDC` 为 `oidc`，`OpenFGA` 为 `open_fga`。不按文件名推导、不裁剪 Config/Contract、不提供 override 或旧 key 别名。
- bootstrap 在加载阶段读取描述符，不再依赖生成的 SectionKey/SectionOptional 方法。命名计算仅在扫描入口进行，不增加全局注册表。
- 当前 13 个 section 与 5 个 key 迁移的权威清单在 research 第 7 节。

### 2.2 field

- `FieldRule.default` 能区分未声明默认值与显式声明 `default: ""`；不能用 GetDefault 的结果是否为空来判断。
- default 与 required 同时声明时生成失败，包括显式空字符串 default。
- 普通 proto3 标量声明 default/required 时，必须能区分未设置与显式零值；两仓按需要添加 optional，不机械修改全部字段。
- 普通消息字段的 required 检查父字段是否已设置；未设置且非必填时，不创建父对象，也不处理内部规则。
- Duration 是具有独立 default 的原子字段，nil 时可补值，不等同于创建普通父配置树。

### 2.3 唯一入口

配置处理生成文件只公开 `Apply() error`，删除 ApplyDefaults、CheckRequired、ApplyConf、SectionKey、SectionOptional；不保留转发兼容方法。标准 protobuf 方法及其他生成器的既有接口不在本次全局清理范围，业务配置处理统一使用 Apply。

Apply 直接包含本对象的字段设置状态检查、类型化默认赋值、对子消息 Apply 的调用和适用的值约束检查。避免把实际逻辑移到一个新的反射配置引擎，再让生成器仅产生空壳包装。

处理顺序：

1. 接收对象为空时，不补值、不创建对象；模块是否接受空对象由其使用入口决定。
2. 检查本对象直接声明的必填字段是否已设置；不以 GetX 返回的零值推断缺失。
3. 对缺失且声明 default 的字段进行类型化赋值。默认字面量在生成期解析，不在运行时重复解析字符串。
4. 对实际存在的普通子消息、列表中的消息元素、映射中的消息值和已选中的互斥消息分支执行 Apply。
5. 检查本对象的值约束；此时子对象的默认已经完成，错误向调用者传播。

不承诺失败时回滚已补的默认；失败对象不能继续用于初始化资源。对有效配置重复 Apply 结果稳定，不因调用次数创建新父块。

## 3. 字段形态与遍历

| 形态 | 处理方式 |
| --- | --- |
| optional string/bool/数值 | 以存在位/指针判断 default 和 required；保留明确的零值 |
| enum、bytes 的 required | 检查字段是否已设置；无法区分未设置与零值时，生成失败，不退化为“非零”检查 |
| Duration | default 仅在 nil 时补；required 与值合法性分开 |
| 普通单值消息 | required 检查 nil；存在时递归，不因子字段默认而分配父对象 |
| 列表或映射中的标量 | 不用长度冒充“是否已设置”；直接声明 conf default/required 时拒绝；非空要求改用独立值约束 |
| 列表或映射中的消息 | 遍历实际元素；不为编译器合成的映射条目生成方法；手工构造的 nil 元素报告所在位置，不补空对象冒充有效配置 |
| 业务互斥字段（oneof） | 默认值不能选择分支；仅递归已选消息。直接 default 注解拒绝；required 检查是否选中该分支，互斥分支同时必填时拒绝 |
| 编译器为 proto3 optional 自动生成的互斥字段 | 作为能记录设置状态的普通字段处理，不误当成业务互斥字段 |

本轮默认值字面量支持现有使用所需的字符串、布尔值、数值和 Duration；未实现的类型或组合必须在生成期报错，不能用注释代替实现或输出不可编译代码。enum/bytes 的默认值不通过隐式转换假装支持。

统一分析配置之间的引用关系，不再分别维护默认和必填两条遍历。覆盖单值、列表、映射和互斥字段，处理递归和共享子消息；跳过编译器合成的映射条目与 Protobuf 内置类型的内部结构。`cmd/internal/protoreach` 的 IsWellKnown 也被 CRUD 使用，保持其原有语义，不顺带改变 CRUD 展开规则。

错误路径使用 Proto 字段名；父调用只在失败时补上下文，包含列表索引、映射键和选中分支。映射键应正确转义，并按确定顺序遍历，避免同一输入随机报告不同首错。成功路径不为每个节点构造完整错误路径字符串。

## 4. 值约束：复用现有能力，不再建一套 DSL

两仓已经依赖 `buf.build/go/protovalidate v1.2.0`。配置所需的简单非空、集合非空、已核实的范围约束使用既有 `buf.validate` 表达，不为 conf 再发明 non_empty/min/max 选项，也不把全仓验证库迁移绑定进来。

- 旧 required 字符串迁为 optional + conf required，并以 `string.min_len` 保留明确的非空要求；空白规范化、URL/证书/SDK 的实际合法性仍由所属模块处理。
- Kafka.brokers、ClickHouse.addrs、OAuthClient.allowed_grant_types 的旧 required 实际要求集合非空，迁为 `repeated.min_items`，不再把集合长度当作是否已设置的依据。
- 配置图以 Servora conf 注解和其可达关系确定；不因为普通 RPC 使用 buf.validate 就给所有业务消息批量增加配置 Apply。
- 生成的 Apply 复用 protovalidate 全局校验器，通过 WithFilter/FilterFunc 将值规则限定到当前对象；子对象由自己的 Apply 校验，避免重复检查整棵子树，不为每份配置创建校验器。
- 已在临时动态 Proto 探针验证：optional 缺失可跳过 min_len；显式空串失败；空集合的 min_items 失败；父对象的局部校验不执行子对象规则，而子对象自身入口仍失败。
- 不在本次引入通过 ignore 规则跳过配置子树的能力；与统一递归 Apply 冲突的配置 ignore 声明须在生成期明确拒绝，不能由过滤器静默改变含义。既有业务 API 的验证规则不受影响。

SDK 转换、跨字段协议约束、文件读取和连接错误继续在运行时拥有。例如 Session Cookie 安全前缀、OpenFGA token 的 HTTPS/禁重定向、TLS 组合、SMTP 成对凭据等，不为减少函数数量而删除或搬成重复规则。

## 5. 加载和使用责任

### bootstrap

- 根配置仍由现有 loader 合并来源、解码并 Apply；保留读取远端配置必需连接的现有顺序。
- Scan 从 descriptor 判断 section 标记；缺段直接跳过，不把调用方预分配对象改造成“已配置”，不运行默认。
- 存在段或整份 target 解码成功后调用 Apply；错误带 target/section 上下文。optional 策略不得吞掉存在段的类型错误。
- 保留 nil runtime、nil config、nil/typed-nil target 的防护，不保留旧方法适配链。

### 独立使用入口

原始 Scan、其他解码器和手工构造不会自动执行配置契约。由能够处理错误的真实加载/构造入口调用 Apply；不是每个 getter/provider 都调用一遍，也不是给每个模块新增 prepare 包装。

特别核对 section 现在允许缺失后的实际使用点：IAM 的 OIDC provider、UserInitializer、SMTP sender，以及已有 Kafka/OpenFGA/Session/CAP/ClickHouse 等入口。仍要使用未加载配置的模块，必须在访问其必要参数或创建相应资源前检查，不得依赖过去的 section.optional=false。

已有明确的禁用/可选构造语义保留，例如 nil 或未配置地址时返回未启用的 Optional 构造器；不为了执行 Apply 反而启用模块。无错误返回的底层组装函数不机械增加包装，在上游真实错误边界保证 Apply 前置。

### 失败清理

保持现有核心配置、日志/追踪、私有 Scan、业务装配的先后关系。IAM/Audit 在 Scan 失败时关闭已经创建的 Runtime；复用既有 Close 幂等/LIFO 契约，不增加新的资源管理层。业务 cleanup 仍先于 runtime cleanup。

## 6. 默认策略的具体归属

| 对象 | 决定 |
| --- | --- |
| 已声明的标量/Duration default | Proto 为权威来源；消费者不再补同一个字面量默认 |
| Redis timeout | nil 默认迁到 Proto，删除框架二次零值回退；显式零值原样传入 SDK，遵循 SDK 本身的参数语义，不擅自将 0 改成“无限超时” |
| Redis network | 保留字段并传入实际 go-redis options，不继续忽略；SDK 的默认/网络选择语义不另造一份 |
| Trace sampling_ratio | 用字段设置状态区分省略与 0；省略继续用现有环境策略，明确 0 表示根 trace 采样率为 0，保留 ParentBased 对父 span 的继承语义；非法区间报错，不静默回退 |
| SMTP port | 固定默认 587 在 Proto 表达，消费者改为 Apply 后读取；移除“0 就补 587”的重复分支，保留合法端口范围检查 |
| CORS 三项列表 | 只在 cors 实现持有已批准的默认；启用时空/省略取默认，非空覆盖；max_age 的 Proto default 独立保留 |
| AuditConsumerConfig 的 100/1s/90 | 保留现有数据层的运行时策略及业务行为，不在 Proto 再复制一份默认；本任务只做接口/key 兼容，不重构 Audit 批处理与保留期策略 |
| SDK 自身的零值语义 | 明确区分传入配置与 SDK 的有效参数；不能宣称保留配置中的 0 就会取消 SDK 原有默认行为 |

CORS 在创建中间件时确定有效设置、预计算可复用响应头；不在每个请求复制默认列表。IsEnabled、日志用的 origins 和实际 middleware 必须使用一致的有效策略，不能保留“原列表为空所以禁用”的旧门槛。保留既有来源匹配、credentials、exposed_headers 等行为，不扩展成 CORS 安全策略重设计。

## 7. 两仓生成与依赖一致性

来源必须分别识别：源 Proto、annotation 的标准 Go 生成物、新 conf 插件、业务生成物、Go module 版本/BSR lock。父级 go.work 不能替代任何一个来源检查。

自举顺序：先修改 annotation 与使用其新语法的源 Proto；以基础插件（protoc-gen-go 与现有 protoc-gen-validate）的临时模板一起更新 annotation 的 Go API 和 PGV 校验生成文件；再编译新 conf 插件；最后生成其余 Go/TS 辅助生成文件 和必要 Wire 产物。当前 annotation 目录同时有 annotations.pb.go 与 annotations.pb.validate.go，只更新前者会让旧 PGV 文件继续引用已删除的 SectionRule。不能让旧 conf 插件读取新 bool section schema，也不能手改生成物绕过自举。

规划阶段先在仓库外临时 Buf 工作区验证五个输入域，共 47 个 Proto 文件。实施阶段用当前源码自举注解和新插件；发布阶段再使用 Servora v0.9.9 的 Go module、BSR 定义和版本化插件独立重新生成，实际结果见研究记录第 12 节。

本地编译使用临时 modfile/replace 指向当前 Servora、GOWORK=off；不把临时覆盖提交到仓库，也不把它称为真实发布依赖验收。最终 Plateau 的 go.mod、插件版本和 buf.lock 必须切到真实、可解析的新 Servora 版本/schema 后再独立检查。

发布、tag、push、BSR push 需另行明确授权；未获授权时停止在这个门禁，报告已完成的本地验证和未完成的真实版本消费验证，不伪造版本、不把整体任务标记完成。

## 8. 风险与非目标

- 自动 key 改名漏迁可能被当成缺段，不能只看代码编译；必须实际扫描真实配置。
- 将“必须设置”与“值不能为空”分开后，可能放宽原有错误输入；必须同步迁移值约束和使用入口。
- 新生成类型的指针字段影响手写字面量及直接字段访问，用 LSP 定位真实调用者后迁移；不手改生成物。
- 不新增热重载、跨请求配置缓存、全量前置校验、兼容别名、reserved 或通用配置事务层。
- 用户已批准的破坏性切换不意味着可以跳过依赖来源、真实调用和错误路径验证。
