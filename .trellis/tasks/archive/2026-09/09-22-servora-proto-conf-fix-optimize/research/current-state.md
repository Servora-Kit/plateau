# 当前 Proto conf 问题核查

第 1–10 节保留实施前的调查与规划依据，不代表修改后仍存在相同问题。输出中的说明标签已改为中文，输入、字段名及结果数值保持不变；实施后的验证另行记录。

本记录只描述本轮实际读取和运行所得，不继承已回滚实现的设计、验收或发布结论。路径以 Plateau 仓库根为基准。`执行确认` 指运行当前源码或生成器；`源码确认` 不代表已完成端到端验证。

## 1. 已确认问题

### F1：生成器没有正确支持原生 optional 标量（执行确认）

- `../servora/cmd/protoc-gen-servora-conf/main.go:41-44` 声明支持 proto3 optional，但 `:289-388` 按非指针标量生成默认赋值；`:503-522` 的 required 判断同样没有处理标量字段设置状态。
- 用当前插件和当前 `protoc-gen-go` 对同一 descriptor 生成完整 Go 文件后编译：`optional int32 count` 加 default 得到 `*int32 == 0` 及常量赋指针错误；`optional string name` 加 required 得到 `*string == ""` 编译错误。
- 这不是需要重写解码器的证据：通过实际 Kratos `Config.Scan` 解码原生 optional int32，`{}` 得到 `Has(count)=false`，`{"count":0}` 得到 `Has(count)=true`。
- 当前普通标量默认就是零值覆盖：直接解码 `Log_FileBackend{path:"probe.log", max_size:0}` 后调用 `ApplyConf()`，值变为 100。把默认改成“仅缺失时补”需要 schema/字段设置状态与消费者一起调整，不能只改一个判断。

### F2：级联在 repeated/map message 边界中断（执行确认 repeated；源码确认 map）

- `../servora/cmd/internal/protoreach/protoreach.go:46-61` 与 `../servora/cmd/protoc-gen-servora-conf/main.go:232-251` 都跳过 list/map message。
- `../servora/api/protos/servora/core/v1/bootstrap.proto:167-195` 中 `Log.backends` 是 repeated，内层 file.path required、file.max_size default 100、otel.endpoint required。
- 当前 `Bootstrap.ApplyConf()` 只沿 Server 级联（`../servora/api/gen/go/servora/core/v1/bootstrap.pb.servora-conf.go:14-46`）。实际输入 `obs.log.backends[0].file={}` 时返回成功；path 合法且省略 max_size 时，配置对象里的 max_size 仍是 0。
- 不能据此断言实际文件轮转必然没有默认大小：下游 lumberjack 也可能补默认。这恰好说明生成声明未执行，SDK 行为可能掩盖缺陷。
- 不是所有 oneof 都未实现：同一生成文件 `:87-123` 已对选中的 message oneof 分支调用子方法；日志链路是在其外层 repeated 边界断掉。

### F3：required 检查和默认建树互相矛盾，ApplyConf 不稳定（执行确认）

- `../servora/cmd/protoc-gen-servora-conf/main.go:269-275` 为含默认值的普通子消息自动分配对象；`:482-487` 的 required 级联却跳过 nil 子消息；`:551-560` 先检查 required，再填默认。
- 当前 `Server_HTTP{}` 第一次 `ApplyConf()` 成功并创建 Listen，第二次同样调用返回 `servora.core.v1.server.listen.addr is required`。
- 生成证据：`../servora/api/gen/go/servora/core/v1/bootstrap.pb.servora-conf.go:236-272`。
- 必须修复前后不一致；但“缺失父块永不创建”是候选目标语义，不是当前源码已承诺的契约。仅交换两步顺序也不能决定可选父块应该存在与否。

### F4：注解声明缺少生成期合法性检查（执行确认）

- 注解明确声明 default/required 互斥（`../servora/api/protos/servora/conf/v1/annotations.proto:28-40`），但 `main.go:176-193,203-224` 独立收集两者，没有拒绝冲突。
- 实际 descriptor 同时声明 default="tcp"、required=true，插件成功产出代码；运行时却先因 required 失败，无法使用声明的默认值。
- `main.go:473-477,503-522` 遇到不支持的 required 类型只输出 skipped 注释。实际 `bool enabled [(field).required=true]` 在未填写时 `ApplyConf()` 返回 nil。enum 也未在该判断中实现（仅源码确认）。
- 应支持明确承诺的类型，或在生成期拒绝不支持的声明；不能生成一个看似受约束、实际不检查的配置字段。

### F5：optional section 的文档、实际行为与应用需求不是同一概念（执行确认）

- 注解 `annotations.proto:22-24` 写缺失 section 仍执行默认；实际 `../servora/core/bootstrap/scan.go:87-103` 对 optional 缺失直接返回，不调用 ApplyConf。
- 实际 `bootstrap.Scan`：输入 `{}`，CORS 默认不执行；输入 `{"cors":{}}`，补齐默认 origins/max_age；`{"cors":42}` 正确返回解码错误，没有被 optional 吞掉。
- `section.optional` 是按 key 扫描的缺失容忍，不等于“组件可禁用”。IAM 仍在 `app/iam/service/cmd/server/main.go:56-68` 无条件装配 Redis/OpenFGA 等依赖。应用的必需性不能简单由共享 message 全局 optional 标志决定。
- 用户已确认省去显式 key/optional，采用第 7 节的单标记/自动 key/扫描可缺失方案；“保留现有 section 字段”的旧建议已撤回。该简化仍不以“所有自定义配置都没有必需参数”为前提。

### F6：配置消费仍存在额外默认和未消费字段（源码确认）

- Redis：`../servora/contrib/db/redis/redis.go:45-61` 对 nil duration 补默认，`:109-130` 又对零 duration 补同样默认；Proto 已保留的 Duration 存在性在第二步丢失。`network` 在 `../servora/api/protos/servora/contrib/db/redis/v1/config.proto:17` 声明，却没有进入 Config/options（`redis.go:22-30,38-43,122-130`）。本轮未连接 Redis，未验证实际网络行为。
- Trace：`../servora/obs/tracing/tracing.go:85-111` 只采用 `(0,1]` 采样率；显式 0、负数、超过 1 都回退成环境默认。环境默认 dev/test=1、其他=0.1，不是一个可直接搬进固定 default 注解的常量。无 endpoint 时 noop（`:35-40`）应与“已启用但配置非法”分开讨论。
- 文件日志：`../servora/obs/logger/file.go:18-27` 对空 path 静默跳过，与已选 file 分支的 required 声明形成实际缺口。应先修复加载/级联边界，而非逐组件增加一层同名 Validate。
- Audit：`app/audit/service/api/protos/audit/service/conf/v1/audit_config.proto:17-19` 只有“默认 100/1s/90”的注释，没有 conf default 注解。应逐字段登记归属，不能宣称所有默认已在 Proto 中声明。

### F7：业务配置检查晚于 runtime 组件初始化（执行确认顺序；源码确认调用方退出路径）

- `../servora/core/bootstrap/config/loader.go:67-84` 合并来源、解码并检查 Bootstrap；`../servora/core/bootstrap/bootstrap.go:97-118` 随后创建 logger/tracer；服务再扫描其私有 sections。
- 真实 NewRuntime 探针通过日志 handler factory 计数；随后扫描缺失的必需 section 返回错误时，factory 已调用一次。这证明不能保证“所有配置错误都在日志/trace 初始化前暴露”。探针未启用远端 exporter、未开放监听。
- IAM `main.go:63-68` 在 Scan 失败时直接返回，尚未进入负责 Close 的 `Runtime.Run`（`bootstrap.go:133-147`）。这意味着该错误路径没有显式调用 runtime cleanup；不把进程退出后的 OS 回收混称为永久资源泄漏。
- 用户已确认本任务保持现有启动顺序，不实现“全量静态配置检查完才创建日志/追踪”的改造。上述顺序是已接受的边界，不再作为待修复缺陷；仍须保证各模块使用配置前完成 Apply，并补齐 Scan 失败后的 runtime 关闭。读取远端配置必需的连接继续按现有来源加载顺序管理。

## 2. 原对话需要修正的判断

1. **required 当前不是纯字段设置状态。** `annotations.proto:29,39` 明确写了零值不接受。把 required 改成“用户提供即可”是契约变更；必须同步保留已有非空、正值等真实业务约束，不能让空地址/空密钥因语义放宽被接受。
2. **不是所有 Go 默认/校验都应删除。** 固定字面量默认、结构约束、环境策略和 SDK/外部资源检查要分开。Session 的 Cookie 标准/跨字段检查（`security/session/session.go:24-58`）、OpenFGA token 的 HTTPS/禁重定向要求（`infra/openfga/client.go:30-43`）、Trace TLS 组合检查（`tracing.go:114-138`）不能以“重复校验”为由删除。
3. **不是全面缺少字段设置状态。** Session 已用 optional bool 表达 secure/http_only/persist（`api/protos/plateau/security/session/v1/config.proto:27-31`），构造器保留 nil 与 false 的区别（`security/session/session.go:33,47,71`）。本轮真实 Config.Scan 也保留 optional int32 的显式 0。
4. **集合元素遍历不等于集合字段字段设置状态。** 本轮显式 `cors.allowed_origins: []` 仍被默认成 `["*"]`；repeated/map 没有普通 optional 标量那种原生存在位。空集合如何表示“不用默认”需要单独约定，不能承诺全部通过加 optional 解决。
5. **字段合法性约束尚未接入配置链。** 检查的 conf Proto 没有 buf.validate 规则，公共加载入口也未执行通用值约束校验；依赖中存在 protovalidate 不等于已接入。业务 API Proto 已有 buf.validate 使用，不能把这个结论扩大成“两仓所有 Proto 都没用”。

## 3. 验证方式与原始关键输出

本轮所有临时 Go 文件、插件与 fixture 都在仓库外的 `servora-conf-investigation-*` 目录。没有修改产品源代码、手改生成物、安装全局插件或连接业务中间件。

工作目录为 Servora；先从当前 checkout 构建插件：

```sh
env GOWORK=off go build -o "$PROBE_DIR/" ./cmd/protoc-gen-servora-conf google.golang.org/protobuf/cmd/protoc-gen-go
env GOWORK=off go run "$PROBE_DIR/probe.go" "$PROBE_DIR"
env GOWORK=off go run "$PROBE_DIR/scan.go" "$PROBE_DIR"
env GOWORK=off go run "$PROBE_DIR/startup.go" "$PROBE_DIR"
```

生成物 fixture 使用临时模块 `replace github.com/Servora-Kit/servora => 当前源码路径`，子进程同样 `GOWORK=off`，未依赖全局已安装的 conf 插件。以下为本轮实际输出摘录：

```text
http-empty first="<nil>" second="servora.core.v1.server.listen.addr is required" listen_created=true
log-root path="" err="<nil>" max_size=0; direct_leaf_err="servora.core.v1.log.filebackend.path is required"
log-root path="probe.log" err="<nil>" max_size=0; direct_leaf_err="<nil>"
explicit-max-size-zero after_defaults=100
consumer-optional-default: invalid operation: m.Count == 0 (mismatched types *int32 and untyped int)
consumer-optional-required: invalid operation: m.Name == "" (mismatched types *string and untyped string)
generator-default-required-conflict error="" file_count=1
consumer-default-required-conflict: ApplyConf=probe.config.name is required
generator-required-bool error="" file_count=1
consumer-required-bool: ApplyConf=<nil>
section input={} err=<nil> origins=[] max_age_nil=true
section input={"cors":{}} err=<nil> origins=["*"] max_age_nil=false
section input={"cors":{"allowed_origins":[]}} err=<nil> origins=["*"] max_age_nil=false
字段设置状态input={} err=<nil> count_present=false count=0
字段设置状态input={"count":0} err=<nil> count_present=true count=0
startup logger_factory_calls=1 business_scan_error=bootstrap: scan target[0] section "required_probe": key not found
```

以上是缺陷调查，不是修复完成后的通过报告。未运行全仓测试、完整服务启动或远端配置中心实验；Redis/Trace 参数消费结论仅来自源码。

## 4. 后续验证来源必须分开记录

- Plateau `go.mod:15` 固定 Servora v0.9.7，而父级 `../go.work:3-10` 会选择本地源码；本轮显式关闭 workspace。
- Plateau `buf.yaml:10-13` 从 BSR 引入 Servora schema，`buf.go.gen.yaml:62-65` 调用本地 conf 插件。Go 源码、BSR schema、插件二进制、已提交生成物是独立来源。
- 后续若变更注解或导出 API，既要验证本地一致生成，也要验证实际独立依赖消费；本轮没有授权发布、tag、BSR push 或变更其他消费仓库。

## 5. 集合契约补充核查

扫描两仓公共 Proto 及 IAM/Audit/Example 的私有 Proto，当前带 conf 注解的集合字段共六处：

| 规则 | 字段 | 来源 |
| --- | --- | --- |
| default | CORS.allowed_origins / allowed_methods / allowed_headers | `../servora/api/protos/servora/transport/http/cors/v1/config.proto:17-19` |
| required | Kafka.brokers | `../servora/api/protos/servora/contrib/kafka/v1/config.proto:17` |
| required | ClickHouse.addrs | `api/protos/plateau/infra/clickhouse/v1/config.proto:17` |
| required | OAuthClient.allowed_grant_types | `app/iam/service/api/protos/iam/oidc/conf/v1/config.proto:39` |

未发现 map default/required 声明。三个 repeated required 当前实际表示非空，不能在改为纯字段设置状态后继续偷偷用 len==0 充当存在性；如保留非空要求，应以独立集合值约束表达。

通过仓库外 `collections.go` 对实际 Kratos `Value(key).Scan` 执行探针，未调用 ApplyConf，观察解码本身：

```text
input={"cors":{"enable":true},"app":{}} origins_has=false origins_nil=true metadata_has=false metadata_nil=true
input={"cors":{"enable":true,"allowed_origins":[]},"app":{"metadata":{}}} origins_has=false origins_nil=true metadata_has=false metadata_nil=false
```

结论：repeated 的省略与显式空数组在本链路得到相同的 nil slice；map 在此次 Go 解码中保留 nil/空 map 差异，但 ProtoReflect.Has 对两者都为 false，不能把这种实现细节称为 Proto Proto 原生的字段设置标记。不能只修改 generated Go 的零值判断就同时解决这些集合语义。

CORS 中间件本身不再回填默认（`../servora/transport/server/http/cors/cors.go:11-14`）；origins 为空时不匹配来源（`:45-48,66-84`），methods/headers 为空时不输出相应 Allow 头（`:49-54`）。因此移除三项集合 default 是可观察的能力变化，不是等价代码整理，必须由用户确认；不能未经确认便转成 SDK 静默 默认处理。

当前规划方向：原生 repeated/map 的字段设置状态限制不能被 len 判断掩盖，集合非空单独约束、元素仍完整级联。此前“CORS 列表全部改为显式配置”的建议未被采纳；用户已明确确认由 CORS 运行时代码持有三项默认列表：启用时省略或空列表均用内置默认，非空列表覆盖，origins 默认 `*`，nil/enable=false 不启用。空列表不表示禁用默认。这一批准仅针对 CORS 模块策略，不把它伪装成 Proto field.default，也不推广为其他字段的静默零值回退。

## 6. 唯一 Apply 入口及调用责任

- 当前生成器声明三个配置处理方法 ApplyDefaults、CheckRequired、ApplyConf（`../servora/cmd/protoc-gen-servora-conf/main.go:5-15`）；用户要求改为唯一 `Apply() error`，内部直接完成 Proto conf 的默认、检查与递归，旧名不保留兼容包装。这是目标接口，尚未实施。
- 当前 Bootstrap 根加载已在解码后调用 ApplyConf（`../servora/core/bootstrap/config/loader.go:74-82`）；`bootstrap.Scan` 对成功加载的 target 自动调用 ConfApplier（`../servora/core/bootstrap/scan.go:75-103`）。后续直接切换到 Apply，不增加另一层配置生命周期。
- contrib/自定义配置不是独立分类：IAM 将多个自定义 target 交给 bootstrap.Scan（`app/iam/service/cmd/server/main.go:56-68`），其处理自动完成；独立入口已有显式调用，如 Kafka BuildOpts（`../servora/contrib/kafka/kafka.go:22-31`）、OpenFGA New（`infra/openfga/client.go:16-23`）、Session New（`security/session/session.go:16-23`）。
- 外部直接 Scan/解码/构造而未走 bootstrap 时，负责加载或构造的运行时代码调用同一个 Apply；不要求每个使用该配置的函数都再调用。独立模块可在其公开构造入口保证契约，重复应用必须稳定，不引入 applied 标记。
- Apply 仅处理配置数据，不加载来源、创建客户端、读取证书或启动服务。错误返回后不得继续用该配置初始化资源；本任务没有承诺失败时自动回滚已填的默认值，也没有新增事务复制层。

本轮在仓库外运行真实扫描入口探针（工作目录为 Servora，`GOWORK=off`），使用 CORS 已有 Duration default 检查实际效果。输出：

```text
raw Scan: max_age_present=false
bootstrap.Scan: max_age=24h0m0s
raw Scan + ApplyConf: max_age=24h0m0s
```

探针验证了“原始扫描仅解码、bootstrap 自动应用、手动入口显式应用”的现有行为，未测试尚未实现的 Apply 新名称，也未修改产品代码。

## 7. section 自动命名与统一可缺失的精简评估

本轮从两仓公共与三个 Go 服务的私有 Proto 读取实际声明，计算并经用户确认固定 snake_case 规则：先分开缩写词与后续单词，再分开小写/数字与大写边界，最后转小写；不裁剪 Config/Contract 后缀。共 13 个 section，无实际 dotted key；5 个 key 会改变。下表是已批准的迁移目标，产品 schema 尚未修改。

| Message | 当前 key | 已批准自动 key |
| --- | --- | --- |
| Redis | redis | redis |
| Kafka | kafka | kafka |
| AuditContract | audit | audit_contract |
| CORS | cors | cors |
| ClickHouse | clickhouse | click_house |
| Mail | mail | mail |
| OpenFGA | openfga | open_fga |
| JwtAuthnConfig | jwt | jwt_authn_config |
| CAP | cap | cap |
| Session | session | session |
| IAM | iam | iam |
| OIDC | oidc | oidc |
| AuditConsumerConfig | audit_consumer | audit_consumer_config |

变更 key 的来源：Servora `api/protos/servora/obs/audit/v1/config.proto:9-10`；Plateau `api/protos/plateau/infra/clickhouse/v1/config.proto:11-15`、`api/protos/plateau/infra/openfga/v1/config.proto:10-14`、`api/protos/plateau/security/authn/jwt/v1/config.proto:11-15`、`app/audit/service/api/protos/audit/service/conf/v1/audit_config.proto:11-15`。文件名不参与 key 推导；新增后缀剥离和逐类型例外会重新引入隐藏映射，不作为推荐。

需要保留的两个区别：

1. **按段扫描与整份扫描不同。** 当前 `bootstrap.Scan` 在 target 实现 Section 时按 key 读取，否则整份读取（`../servora/core/bootstrap/scan.go:62-70`）；Bootstrap 根加载也是直接整份 Scan（`../servora/core/bootstrap/config/loader.go:74-82`）。不能删掉所有区分后把 Bootstrap 错读成 `bootstrap` 键。建议只留 `section=true` 布尔标记，由 bootstrap 从描述符识别，生成物无需再暴露 SectionKey/SectionOptional。
2. **加载可缺失与使用时不需参数不同。** 当前 AuditContract、CAP、Session、IAM、OIDC 的 section optional 为 false（含默认 false）。其中 OIDC 的 issuer/signing_key_path/crypto_key_path 明确 required（`app/iam/service/api/protos/iam/oidc/conf/v1/config.proto:17-22`），CAP.signing_secret required（`api/protos/plateau/security/cap/v1/config.proto:18-21`），Session.cookie 与 cookie.name required（`api/protos/plateau/security/session/v1/config.proto:17-24`）。IAM 确实扫描并将这些对象交给 wireApp（`app/iam/service/cmd/server/main.go:56-68`）。允许省略整个段可以成为新加载策略，但不能因此取消实际使用入口的 Apply 或必需参数检查。

已确认的简化契约：仅保留 section 布尔标记；标记存在时自动派生 key，缺段跳过、不执行 Apply、不创建配置树；段存在则解码并 Apply，错误正常返回。实际模块若仍使用未加载段的空配置，必须在对应使用/构造入口通过同一个 Apply 执行必要检查；不能借“默认可选”放行。应用聚合根已明确声明的字段 required 不受 section 加载策略影响。

迁移风险：旧 key 与自动 key 不同时，漏改的配置可能被当成“段缺失”而跳过，尤其只有默认值的模块未必立即报错。因此必须同步两仓 YAML/环境覆盖、文档、生成物和调用方并做真实加载验证，而不只重新生成 Go。按已确认的干净切换原则不保留旧 key 别名；具体实施仍等待完整规划评审批准。

## 8. 单一 Apply 的值校验技术验证

现有依赖为 `buf.build/go/protovalidate v1.2.0`。核对了该版本的 `filter.go:19-29`、`option.go:88-92`、`validator.go:29-40,105-135`：提供全局 validator 和公开 FilterFunc/WithFilter，可限制当前对象的值规则，不需要每个配置新建 validator 或组件校验包装。

在仓库外编译临时 Proto（optional 字符串 min_len、repeated min_items、普通/repeated/map 子消息），执行实际库的局部及完整校验。关键行为结果：

```text
absent optional value: local_error=false
explicit empty string: local_error=true
empty collection: local_error=true
child-only violations: local_error=false
child entry: error=true; full parent validation: error=true
```

这支持 design.md 中的实现选择：生成的 Apply 负责类型化 default/字段设置状态/递归，值规则使用现有 validator 的当前对象过滤；子对象通过自己的 Apply 执行规则，避免父子重复执行同一子树的值规则。探针仅证明该库接口对这些规则可用，未实现新 conf 生成器，也不代表任意高级 ignore/CEL 组合已经受支持。

## 9. 两仓本地 schema 联调与自举证据

- Buf CLI 实测为 1.72.0。临时 workspace 在仓库外复制五个输入根：Servora、Plateau 公共 Proto、IAM、Audit、Example；本地 Servora module 名保持 `buf.build/servora/servora`，依赖只保留已锁定的 googleapis/protovalidate。
- 两仓当前外部 schema lock 相同，因此临时 workspace 可直接使用 Servora 的原始 buf.lock，不需要发布或改动仓库 lock。
- 已运行 `buf build --as-file-descriptor-set`，得到临时 descriptor；随后通过 Go protobuf 实际解析，验证五个输入域的已知文件均存在：

```text
combined local schema: files=47, required inputs present=5
```

- 这证明当前两仓 schema 能通过本地多 module workspace 一致编译；没有把父级 go.work 当成 BSR override。新 bool section/新插件仍须实施后重新验证。
- 额外确认 `../servora/api/gen/go/servora/conf/v1/` 同时存在 `annotations.pb.go` 和 `annotations.pb.validate.go`。删除 SectionRule 后，自举时必须以 Go + PGV 基础插件同步刷新这两个文件，再编译新 conf 插件；“只生成 annotations.pb.go”会留下引用旧类型的 PGV 校验生成文件。

以上探针均在仓库外，Go 使用 GOWORK=off、GOFLAGS=-mod=readonly；没有修改两仓产品代码、安装全局插件、更新 BSR/Go 版本或运行发布。

## 10. 迁移清单核对补充

- 非生成旧方法调用除 bootstrap 外还包括 Kafka BuildOpts、OpenFGA New、ClickHouse NewConnOptional、Session New、CAP New、APIDocs NewHandler；APIDocs 的默认字段直接访问在 optional 化后也要改为适当 getter。
- IAM NewIAMProvider 在 `app/iam/service/internal/oidc/provider.go:44-78` 读取 issuer/crypto key 并创建协议 provider；NewUserInitializer 在 `internal/biz/user_initializer.go:30-44` 读取管理员 email；NewSender 在 `internal/mail/smtp.go:20-66` 消费 SMTP 配置且有 port=0→587 分支。缺段统一跳过后，这些真实使用入口不能只依赖过去 section.optional=false 的保护。
- ClickHouse 的可选入口在 `infra/clickhouse/clickhouse.go:35-43` 对 nil/无地址返回未启用，其语义不能和“已配置但错误”混淆。Example main 的 Scan(rt) 没有 target，不重复执行整份扫描；核心配置已由 NewRuntime 的 loader 处理。
- 5 个 key 的 YAML 位置及 service DNS/topic/数据库名区别已经纳入 implement.md，禁止全局文本替换。
- 当前 Plateau 仍固定 Servora v0.9.7 与旧 BSR commit。临时 workspace/modfile 能验证本地源代码集成，但不能替代真实新版本的独立消费验证；发布需另行授权，未完成该门禁不能标记整个任务完成。

## 11. 实施后的验证记录（2026-09-23）

用户已批准本地实施。两仓源码、生成物、真实消费者和当前说明均已切换；以下是实施后的实际结果，不复用前述调查结论充当通过记录。

### 生成来源

- 先用标准 Go 与 PGV 插件生成新的注解类型，再从当前源码构建全部自定义插件到仓库外临时目录；没有安装全局插件。
- Buf 1.72.0；标准 Go 插件 v1.36.11、gRPC 插件 v1.6.2、PGV v1.3.3。自定义 Go、错误、脱敏、CRUD、审计、配置、TypeScript、认证和授权插件均来自当前两个仓库。
- 五个本地 Proto 输入根合并在仓库外 Buf 工作区，外部依赖沿用锁文件；两仓 Go、共享 TypeScript、Example 独立 TypeScript 和 OpenAPI 已重新生成。生成目录未手改。

### 已通过检查

| 检查 | 实际结果 |
| --- | --- |
| Servora `GOWORK=off go test -short ./...` | 753 项通过，74 个包 |
| Servora `GOWORK=off go build ./...` | 通过 |
| Servora `GOWORK=off just lint-go` | 0 问题 |
| Plateau `GOWORK=off go test -short -modfile=临时文件 ./...` | 257 项通过，90 个包 |
| Plateau `GOWORK=off go build -modfile=临时文件 ./...` | 通过 |
| Plateau Go 静态检查 | 仓库外源码副本采用同一临时依赖配置，通过，无问题 |
| Servora 原仓库及 Plateau 四个本地输入模块的 Buf 检查 | 使用各自规则，通过 |
| Servora `just web-typecheck` | 通过 |
| Plateau `just api-ts-check` | 通过 |
| Example Web `pnpm --dir app/example/web type-check` | 通过 |

Plateau 编译和测试使用仓库外临时依赖文件，把 Servora 指向当前源码，明确关闭父级工作区。现有静态检查工具不能正确加载命令行临时依赖文件，因此静态检查在仓库外相同 Go 源码副本中，把已验证的依赖内容作为该副本的 go.mod；原仓库 go.mod、go.sum 和 buf.lock 未改。尝试临时工作区时还遇到旧 cloud.google.com/go 的重复包来源，未靠调整产品依赖绕过；该工作区不作为通过依据。

### 实际运行

- 原始 Scan 不填默认值；同一 CORS 输入经 bootstrap.Scan 后与手动 Apply 后，max_age 均为 24h。输出分别为 `max_age_present=false`、`max_age=24h0m0s`、`max_age=24h0m0s`。
- CORS 使用真实本地 HTTP 服务和请求验证：空配置、禁用、省略列表、空列表、自定义列表、来源不匹配六种场景通过。默认允许任意来源，响应保持原有的来源回显方式，不改成固定输出星号。
- Example 按现有配置在 10030/10031 启动；GET `/v1/tenants/proto-conf-smoke/users` 返回 `{"users":[],"nextPageToken":""}`。检查完成后已停止本次启动的进程。
- IAM/Audit 的 local、docker 四组真实配置均完成核心与私有段扫描。提供了仅用于本地检查的 FGA_STORE_ID、IAM_CAP_SIGNING_SECRET；没有连接或修改对应外部服务。未提供必要环境值时，空 store_id/signing_secret 被配置校验拒绝。
- 未进行 IAM/Audit 带全部中间件的完整服务启动；配置扫描通过不等同于全栈可用。

### 回归与审查

- 回归覆盖显式零值、空默认值、缺失父块、集合/互斥分支、重复应用、实际日志根配置、采样比例、自动段名及失败关闭。
- 跨目录生成分别验证外部有配置规则、只有值规则和纯数据子树；处理方法不存在时不生成无效调用，外部集合的空元素使用私有类型化递归检查，互斥分支的空包装不会引发崩溃。
- 值规则错误按路径稳定排序并保留可识别类型；局部校验按对象身份过滤，避免同类型递归消息被重复校验。
- loader 在各失败路径及成功退出时，先停止监听器，再关闭自己创建的可关闭远端来源，合并错误且重复关闭安全。
- 删除了四个受类型变化影响、却只断言服务器非空的 HTTP/gRPC 测试；保留端点、TLS、HTTP 响应和错误行为断言。

### 未完成的正式依赖门禁

- 尚未提交、推送、打标签、发布 Go 版本、推送 BSR 或部署。
- Plateau 仍固定旧 Servora v0.9.7 与旧 BSR 引用；无临时覆盖的独立消费验证未完成，不能把本地源码通过标成完整任务完成。
- `logger.New` 改为三个返回值。LSP 发现第三仓 `servora-example/app/master/service/internal/server/tcp_config_scan_test.go:28` 仍使用两个返回值；该仓不在本次范围，没有修改，发布前需安排其迁移。

## 12. 正式发布与独立接入验证

用户已明确授权提交两仓、发布 Servora v0.9.9、更新 Plateau 正式依赖并按 Trellis 归档；本节关闭第 11 节保留的发布门禁。

### 提交与发布

- Servora 实现提交：`fe93c3d397cb228d9e3213711ef227f83ae9cc08`，`fix(conf)!: 统一配置应用契约`，已推送 main。
- Plateau 实现提交：`0cf0293`，`refactor(conf)!: 接入统一配置应用契约`。
- main 的 [CI](https://github.com/Servora-Kit/servora/actions/runs/35855905124)、Buf CI 和代码质量检查均通过后，推送 v0.9.9 标签。
- [Release 工作流](https://github.com/Servora-Kit/servora/actions/runs/35856408227) 和 [版本标签的 Buf CI](https://github.com/Servora-Kit/servora/actions/runs/35856408305) 均成功。
- [GitHub Release v0.9.9](https://github.com/Servora-Kit/servora/releases/tag/v0.9.9) 已公开，不是草稿或预发布版本。
- BSR 的 main 和 v0.9.9 标签均指向 `66efd9fc6c25423eb504dcc1834f1552`。按用户要求，buf.yaml 保留 `buf.build/servora/servora`，具体提交由 buf.lock 固定。

### 正式依赖来源

- go.mod 和 Just 的 SERVORA_VERSION 均为 v0.9.9；go.sum 已更新，protovalidate 因生成代码直接使用而转为直接依赖。
- `go list -m -json github.com/Servora-Kit/servora` 显示真实 v0.9.9、模块缓存路径及校验和，没有 Replace 字段。
- 本机镜像对新版本的校验记录暂时返回 404，改用官方 Go 代理和 sum.golang.org 后成功；未关闭校验。
- 从发布版本安装全部 Servora 生成插件到仓库外临时目录；配置插件的 `go version -m` 明确显示 v0.9.9。Plateau 插件从当前仓库构建。
- 使用正式 BSR 依赖执行 `just gen` 和 Example 独立 TypeScript 生成；生成产物与实现阶段一致，没有新增生成文件差异。
- buf dep update 同时刷新了默认引用的 protovalidate 定义，Google APIs 锁定提交不变。

### 独立验证结果

以下 Go 检查均设置 GOWORK=off 并清空 GOFLAGS，没有临时依赖文件或本地源码覆盖：

- 连续执行 go mod tidy，第二次 go.mod/go.sum 的 SHA-256 不变。
- `go list ./...`、`go build ./...`：通过。
- `go test -short ./...`：257 项通过，90 个包。
- `just lint`、`just service::lint`：全部通过，不再需要隔离源码副本。
- `just service::_build`：Audit、Example、IAM 独立构建入口通过。
- Example Web 类型检查通过；共享 TypeScript 与 Proto 检查由根 lint 覆盖。
- Example 使用正式依赖启动，GET `/v1/tenants/servora-v099-smoke/users` 返回 `{"users":[],"nextPageToken":""}`；完成后已停止本次进程。

没有发布前端 npm 包、部署业务服务或修改第三仓。第三仓旧 logger.New 调用仍由其自身升级负责，不影响本次两仓验收。
