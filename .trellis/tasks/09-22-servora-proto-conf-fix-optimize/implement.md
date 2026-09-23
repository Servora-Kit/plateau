# 实施与验收计划

## 开始门禁

用户已于 2026-09-23 在最终规划评审后明确批准“开始实施”；task.py start 已执行，任务为 in_progress。发布、提交与 BSR 推送仍须另行批准。

执行前加载 PRD、design.md、research/current-state.md 和上下文清单；按 Trellis 流程读取对应层规范。每个导出符号变更先用 LSP references 确认真实调用方；生成目录只通过生成器更新，不手改。

## 阶段 A：注解与唯一 Apply 生成契约

责任集中在一个集成人员，先固定 schema/接口再分发消费者修改，避免多个任务同时重生成同一目录。

- [x] 修改 `../servora/api/protos/servora/conf/v1/annotations.proto`：section 改 bool、删除 SectionRule/key/optional；default 能够区分未声明与显式空字符串；保留中文说明，不新增 reserved。
- [x] 同步所有 section 声明为新语法，并按实际 default/required 给标量增加 optional。Duration/message 保持已有的字段设置标记；不机械改所有字段。
- [x] 值约束按设计迁移：required 字符串的必要非空、三个 repeated 非空、Trace 的合法采样区间等；不要把 SDK/协议检查全部迁入 Proto。
- [x] 更新 `../servora/cmd/protoc-gen-servora-conf/main.go` 的收集、合法性检查、可达性分析和输出；只有 Apply，不保留五个旧方法。
- [x] 联合遍历 singular/repeated/map/oneof；跳过 map-entry 方法生成，区分 编译器为 optional 自动生成的互斥字段；缺失普通父块不建树；错误包含具体字段/元素位置。
- [x] 更新 `cmd/internal/protoreach` 的相关逻辑/测试时确认所有调用方。IsWellKnown 的 CRUD 使用保持不变，不把 conf 规则依赖灌进 CRUD。
- [x] 默认字面量解析、无法区分未设置与零值的字段规则、不支持的字段组合、default+required（含空字符串 default）在生成期失败，删除旧的静默跳过规则/repeated-default 代码路径。

### annotation 自举顺序

1. 新 annotation 语法与其使用方源 Proto 先保持一致。
2. 用临时基础生成模板，只运行标准 Go 与现有 PGV 插件，更新 `annotations.pb.go` 和 `annotations.pb.validate.go`。后者也必须更新，否则仍引用被删除的 SectionRule。
3. 编译新的 conf 插件到仓库外临时 bin；沿用已有/锁定的其他插件版本，不运行全局 `@latest` 安装。
4. 用该明确来源的插件执行完整生成；生成输出按所属仓库归位。删除失效 辅助生成文件 由生成流程负责，不留过期文件。

## 阶段 B：Servora 加载与使用入口

- [x] `core/bootstrap/scan.go`：描述符 section 标记与 snake_case key；缺段跳过，存在段/整份扫描后 Apply；移除 Section、OptionalSection、旧处理接口及兼容分支。
- [x] `core/bootstrap/config/loader.go`：根配置调用 Apply；保留来源合并顺序和失败关闭。
- [x] `core/bootstrap/bootstrap.go` / 调用方：复用现有 Close 契约，不改启动阶段；检查所有返回路径的资源责任。
- [x] `contrib/kafka/kafka.go`：BuildOpts 使用 Apply；重复的 brokers 非空检查与新声明归一，保留真实 SDK 参数转换和可选构造语义。
- [x] `contrib/db/redis/redis.go`：独立入口应用配置，Duration 默认统一到 Proto，删除框架二次零值默认；Network 传入 SDK；保留 TLS、Ping、失败 Close。
- [x] `obs/tracing/tracing.go`：配置存在性区分省略与 0，非法区间报错；保持环境默认、ParentBased 和 endpoint 关闭策略，不改协议行为。
- [x] `obs/logger`：从根配置完整执行 file/otel 规则，缺 path/endpoint 不再被加载链漏过；logger 组装入口的 Apply 责任明确，不新增每个 backend 的准备层。
- [x] `transport/server/http/cors`：默认列表集中在所属实现，构造时确定有效设置；IsEnabled、GetAllowedOrigins、Middleware 一致；删除 Proto 中三列表 default。
- [x] `transport/server/http/apidocs/apidocs.go`：clone 后调用 Apply，迁移指针字段访问；保留未启用不读文件及现有 URL/path 校验。

## 阶段 C：Plateau 配置定义与真实调用方

### 字段与配置归属

| 负责模块 | 必须覆盖的变化 |
| --- | --- |
| Servora core | Listen.network/addr、FileBackend.path/max_size、OtelBackend.endpoint；Trace.sampling_ratio字段设置状态；缺失父块不自动创建 |
| Servora Kafka/Redis | Kafka 默认字段与 brokers 非空；Redis timeouts/default 与 Network 消费 |
| Servora AuditContract/CORS/APIDocs | 现有 default 的字段设置状态；CORS 列表默认撤出 Proto；APIDocs 原有静态默认与直接字段访问 |
| Plateau OpenFGA | api_url/store_id字段设置状态与非空，保留 token URL 安全规则 |
| Plateau ClickHouse | addrs 非空，timeout/pool/lifetime/compression 等已有默认的字段设置状态；保留未启用语义 |
| Plateau Session | cookie 父字段 required、name 非空、path/same_site default；现有 optional bool 的显式 false 不被覆盖 |
| Plateau CAP | signing_secret字段设置状态/值检查；TTL/count/size/difficulty/prefix 默认；删除两阶段手动调用 |
| IAM/OIDC/Mail | IAM 管理员 email；OIDC issuer/密钥路径与 OAuthClient 必需字段；grant types 非空；SMTP port 默认和实际使用入口 |
| AuditConsumerConfig | 仅接口/key 对齐，保留数据层现有 100/1s/90 策略，不新增第二份默认 |

- [x] 更新 `infra/openfga/client.go`、`infra/clickhouse/clickhouse.go`、`security/session/session.go`、`security/cap/cap.go` 的旧方法调用；保留 SDK/安全检查。
- [x] 核对 `app/iam/service/internal/oidc/provider.go`、`internal/biz/user_initializer.go`、`internal/mail/smtp.go` 及 OIDC 的其他真实密钥/初始化入口：段缺失允许跳过后，实际使用前仍必须经过 Apply。已有错误返回入口直接接入，不再包 prepare。
- [x] `app/iam/service/cmd/server/main.go`、`app/audit/service/cmd/server/main.go`：保持现有 Scan/装配顺序，补 Scan 失败的 runtime 关闭。
- [x] Example 的 `Scan(rt)` 无 target，不把它误称为又扫描了一遍 Bootstrap；其核心配置已经在 NewRuntime 内处理。
- [x] 迁移手写配置字面量和直接字段访问，使用合适的指针值/GetX；不通过断言或静默 默认处理 掩盖类型变化。

### 自动 key 的具体配置位置

五个旧/新 key 的唯一映射表见 research 第 7 节。必须检查：

- `app/audit/service/configs/{local,docker}/audit.yaml`
- `app/audit/service/configs/{local,docker}/audit_consumer.yaml`
- `app/iam/service/configs/{local,docker}/openfga.yaml`
- 相关示例、测试输入、部署引用和直接 Value(key).Scan 使用点。

只改 section 键，不全局替换同名字符串：`http://openfga:8080`、`clickhouse:9000` 是容器地址，`servora_audit` 是数据库名，`servora.audit.events` 是 topic；FGA_* 是既有环境变量，不因 section 改名就改变量名。JwtAuthnConfig 当前未找到生产 YAML，不能凭 `jwt` 字符串扩大到全部 JWT 业务。

## 阶段 D：两仓一致生成与本地消费验证

### 已核实的现有入口

- Servora：`just gen` 是 Go 生成，`just gen-ts` 是 TS 生成；需要清除过期生成文件时才使用 `just gen-fresh`。
- Plateau：`just api-go` / `just api-ts` 是对应 API 生成；`just gen` 还执行服务生成，不等于“仅 Proto”；`just wire` 用于受影响装配。
- Plateau `just plugin` 默认安装已发布 Servora v0.9.7。不能用它覆盖本次临时编译的新插件。

### 本地路径

- [x] 仓库外建立临时 Buf workspace：当前 Servora Proto 为本地同名 module，另纳入 Plateau 四个输入根；外部依赖沿用锁定的 googleapis/protovalidate。此方法已对当前 schema 验证，新 schema 实施后重验。
- [x] 新 annotation、新 conf 插件和临时 workspace 同步生成。Plateau 输出只归 Plateau，自身依赖的 Servora 输出归 Servora，不能把两仓生成物混写。
- [x] Go 本地验证使用 GOWORK=off 和临时 modfile/replace；不提交本机路径、不新增仓库 go.work/嵌套 module。
- [x] 记录标准插件版本、新 conf binary 来源、Buf 输入、Go module 来源；只有确定在使用新来源后才能把结果列为新契约验证。

典型命令入口如下；临时模板、modfile 和输出目录在实施时于仓库外准备，不能原样使用仓库默认 BSR 输入冒充本地新 schema：

```sh
# Servora：编译当前源码的插件到临时 bin
env GOWORK=off go build -o "$BIN_DIR/protoc-gen-servora-conf" ./cmd/protoc-gen-servora-conf

# 临时 Buf workspace：明确采用本地 schema 与指定模板
buf build --as-file-descriptor-set -o "$TMP_DIR/schema.binpb"
buf generate --template "$GO_TEMPLATE"

# Plateau：临时 modfile 只用于本地源代码集成验证
env GOWORK=off go test -modfile="$PLATEAU_MODFILE" ./infra/... ./security/... ./app/iam/service/... ./app/audit/service/... ./app/example/service/...
```

上述命令展示验证方式；已执行结果及未完成的正式版本验证见 research/current-state.md 的实施记录。

## 阶段 E：行为验证与质量门禁

所有共同编辑结束、生成物一致后，由集成人员统一执行格式化、静态检查和测试；并行执行者不在共享工作区各自跑全仓验证。

### 应保留的回归场景

- [x] 真实标准 Go 生成物与配置处理生成文件一起编译：optional 标量、required 的显式 false/0、空字符串 default 及冲突；不是伪造同名 Go struct 验证模板。
- [x] 普通父块缺失/空对象/required 和重复 Apply；跨包子配置、repeated、map、oneof；合成 map-entry 不生成非法方法接收对象。
- [x] 从实际根配置触达日志 file/otel 规则；错误定位到元素位置。
- [x] section 的缩写/复合词自动命名、缺段跳过、存在段类型错误与 Apply 错误；整份根配置路径不变。
- [x] CORS 的启用、禁用、省略、空列表和自定义列表，验证真实 HTTP 响应头。
- [x] Trace 明确 0/非法比例，Redis/SMTP 等消费者不再次覆盖已提供的值；断言 SDK 的真实语义，不将 SDK 原生默认误判为 Proto 覆盖。
- [x] 现有 Runtime Close 顺序/幂等检查与服务失败路径验证；不为了测一个调用次数新增 wrapper。

旧的自动创建缺失父块断言已替换；回归使用真实生成物检查数据、错误与边界，不固定生成文本或第三方错误文案。临时运行程序仅用于实际入口验证，结果见研究记录第 11 节。

### 命令与实际运行

Servora 定向入口：

```sh
env GOWORK=off go test ./cmd/protoc-gen-servora-conf ./cmd/internal/protoreach ./core/bootstrap/...
env GOWORK=off go test ./contrib/db/redis ./contrib/kafka ./obs/logger ./obs/tracing ./transport/server/http/...
env GOWORK=off go test -short ./...
just lint-proto
just web-typecheck
```

Plateau 定向入口见阶段 D；完成真实依赖切换后再执行根门禁：

```sh
env GOWORK=off go list ./...
env GOWORK=off go build ./...
env GOWORK=off go test -short ./...
just api-ts-check
just lint
```

- [x] 启动隔离配置的 Example，按固定端口 10030/10031 验证真实 HTTP。已有 ListUsers 路由是 `/v1/{parent=tenants/*}/users`（`app/example/service/api/protos/example/service/v1/user.proto:25-28`），使用隔离测试 tenant 发出实际 GET；不臆造 health URL。
- [x] IAM/Audit 至少验证真实 local/docker 配置扫描和受影响构造路径；真实依赖就绪后再记录完整启动结果。缺依赖时明确阻塞，不把配置加载通过称为服务可用。
- [x] 用实际 CORS middleware 启动临时 HTTP 服务发请求；直接构造与 bootstrap 处理后的配置均覆盖已批准策略。
- [x] 分别记录已执行、未执行、失败和外部阻塞，不把本轮规划探针当成修复后验收。

## 阶段 F：真实依赖与发布门禁

本地源码验证不等于发布后的独立消费。当前 Plateau 固定旧 Servora Go 版本和 BSR commit；新 API 无法靠这些旧版本完成真实独立构建。

- [ ] 在源码验证完成后请求单独的发布授权；未授权不 commit/push/tag/BSR push，不捏造新版本。
- [ ] 真实新 Servora 版本/schema 可用后，同步 Plateau go.mod/go.sum、SERVORA_VERSION 和 buf.lock；不留下本机 replace。
- [ ] 重新生成并在无父 workspace、无临时 modfile、无旧 PATH 插件掩盖的环境完成根门禁。
- [x] 正式依赖门禁未完成，任务保持 in_progress 并明确标记等待发布授权；本地源码验证不冒充独立发布版本接入完成。

## 收尾与最终审查

仅在实际 smoke 通过后处理收尾：更新现有用法、示例、注释及受影响规范；删除失效方法/注释/生成文件和本轮临时联调脚本。直接描述新用法，不创建独立迁移指南。检查没有旧 key 别名、旧 Apply 方法包装、新增 reserved、意外字段号重排或 Admin 业务改动。

回滚按同一逻辑变更组还原源 Proto、生成器、生成物和消费者，不能只回退某一层并继续运行。任何发布回滚需独立确认，不在本地实现操作中自动执行。
