# 七款 AI API 中转项目：功能、技术栈与架构比较

原六项调研日期：2026-10-05；Magpie 补充日期：2026-10-13。依据官方 README、依赖清单、部署配置和关键源码；没有部署这些系统、调用真实上游或压测。各项对应各自记录的主分支快照，不保证与旧版本/稳定发行版相同，也不代表七项在同一天重新核验。架构取舍与选型建议标记为 [INFERENCE]。Magpie 另完成外部 Go module 导入限制的隔离验证，见[详细记录](magpie.md#6-隔离验证普通外部-go-module-能否导入转换所在包)；这不构成协议运行兼容性证明。

## 1. 先修正分类

**七者并非都是 Go 程序，也不是同一种产品。**

- **Higress**：云原生 API 网关，AI 是其重点能力；Go 控制面、Envoy 数据面、Java 控制台后端并存。
- **Floway**：TypeScript/Hono 的 agent-oriented LLM 网关，关注协议、Responses 状态与客户端工作流兼容。
- **gpt-load**：当前为 2.x 多渠道、多凭据管理与调度网关；不再适合仅概括为 API key 轮换器。
- **CLIProxyAPI**：面向 CLI/订阅账号的认证、协议转换与执行引擎，同时支持普通 API key。
- **new-api**：多供应商渠道聚合、统一接口、用户权限与计费运营平台。
- **sub2api**：以 OAuth/订阅账号池调度和额度分发为重心，同时带用户、余额与支付管理。
- **Magpie**：本地 Agent 配置管理与四协议网关一体化工具；支持桌面/终端/Web/headless，转换代码是内部实现，尚无独立公开 Go converter SDK。

这是产品重心分类，不是互斥能力表。例如 Higress 的 ai-proxy 也有 Claude Code OAuth Token 适配，gpt-load 2.x 也接订阅账号，new-api 也做会话亲和。应比较实现深度、数据模型和运维成本，而不是只打“支持/不支持”勾。

## 2. 前后端技术栈

版本是 manifests 声明，不是已安装依赖锁定值或生产运行版本。

| 项目 | 后端/执行核心 | 前端 | 持久化与典型部署状态 |
|---|---|---|---|
| Higress | Go 1.26 控制面、Istio/xDS；Envoy C++ 数据面；Go 等语言编写 Wasm 插件；独立控制台 Java/Spring Boot | React 18、TypeScript、Ant Design 4/Pro、ice 3 | K8s 资源/配置模型；支持 Helm 及 Docker all-in-one；本轮未核实所有独立模式存储实现，不应当作 SQL 钱包平台 |
| Floway | TypeScript、Hono；Node.js 或 Cloudflare Workers | React 19、React Router、Fluent UI v9、UnoCSS、Vite 8 | Node：SQLite + 文件；Workers：D1 等平台资源，部署形态不同 |
| gpt-load | Go 1.27、Gin、GORM；Bifrost Core + CLIProxyAPI 作为执行适配依赖 | Vue 3、TypeScript、Vue Query、Reka UI、Tailwind 4、Vite 8 | 默认 SQLite，可选 MySQL/PostgreSQL；本地加密凭据与密钥文件 |
| CLIProxyAPI | Go 1.26、Gin；Executor、Translator、Auth Manager/Selector；当前主模块 v8 | 独立 CPAMC：React 19、TypeScript、Zustand、Vite 8、SCSS Modules；构建单 HTML 后由后端托管 | 默认 YAML/凭据文件；可选 PostgreSQL、Git、对象存储；另有 Home 集群模式 |
| new-api | Go 1.25.1、Gin、GORM；provider adaptor、relaykit | React 19、TypeScript、Rsbuild 2、TanStack、Base UI、Tailwind 4 | SQLite/MySQL/PostgreSQL；Redis、内存缓存；支持独立日志库/ClickHouse |
| sub2api | Go 1.27、Gin、Ent、Wire | Vue 3、TypeScript、Pinia、Vue Router、Tailwind 3、Vite 5 | 推荐 PostgreSQL + Redis；调度缓存、并发状态、outbox 等 |
| Magpie | Go 1.26.8、标准库 net/http；内部 Request/Result/Event 中间模型；Wails v3 beta 桌面壳 | 原生 HTML/CSS/JavaScript，嵌入资源；无 UI framework/bundler；Bubble Tea TUI | 本地 JSON/JSONL 配置与日志、进程内调度状态；SQLite 用于其他应用数据集成等；Bun provider 插件与进程内 moejs middleware 分开 |

证据：[Higress Go](https://github.com/higress-group/higress/blob/main/go.mod)、[控制台 POM](https://github.com/higress-group/higress-console/blob/main/backend/pom.xml)、[控制台前端](https://github.com/higress-group/higress-console/blob/main/frontend/package.json)；[Floway gateway](https://github.com/Menci/Floway/blob/c7e4d782763b010e44ba243588f2c83c90c97335/packages/gateway/package.json)、[Floway web](https://github.com/Menci/Floway/blob/c7e4d782763b010e44ba243588f2c83c90c97335/apps/web/package.json)；[gpt-load Go](https://github.com/tbphp/gpt-load/blob/5c6ec0f257b3983e507fb88e1ac1d1841365a71d/go.mod)、[web](https://github.com/tbphp/gpt-load/blob/5c6ec0f257b3983e507fb88e1ac1d1841365a71d/web/package.json)；[CLIProxyAPI Go](https://github.com/router-for-me/CLIProxyAPI/blob/main/go.mod)、[CPAMC web](https://github.com/router-for-me/Cli-Proxy-API-Management-Center/blob/main/package.json)；[new-api Go](https://github.com/QuantumNous/new-api/blob/1a4166d8e8ba9802d2ca56fe8ecf0ed5404e80d5/go.mod)、[web](https://github.com/QuantumNous/new-api/blob/1a4166d8e8ba9802d2ca56fe8ecf0ed5404e80d5/web/package.json)；[sub2api Go](https://github.com/Wei-Shaw/sub2api/blob/b8dece9000c68815a5b867ca5a1e6f236e173905/backend/go.mod)、[web](https://github.com/Wei-Shaw/sub2api/blob/b8dece9000c68815a5b867ca5a1e6f236e173905/frontend/package.json)。

## 3. 架构差异：复杂度放在哪里

### Higress：流量基础设施

- 控制台/资源配置声明路由、服务、消费者、插件；控制面经 Istio/xDS 体系将配置下发到数据面。
- 模型请求由 Envoy 与插件链执行，不需要经过 Java 控制台服务。
- ai-proxy 插件处理供应商协议、模型映射、token 选择、响应/流式转换；认证、限流、缓存、可观测性及 MCP 能力可组合。
- [INFERENCE] 价值在统一企业入口、配置更新与扩展治理，不在用户钱包、订单和收款。部署 all-in-one 不会改变内部多组件的职责划分。

来源：[官方组件说明](https://higress.cn/en/docs/latest/dev/architecture/)、[插件入口](https://github.com/higress-group/higress/blob/main/plugins/wasm-go/extensions/ai-proxy/main.go)、[插件配置](https://github.com/higress-group/higress/blob/main/plugins/wasm-go/extensions/ai-proxy/README.md)。

### Floway：协议与 agent 会话语义

- 同一 gateway 进程提供逻辑上的 control-plane 与 data-plane；不是 Higress 式独立 Envoy 数据面。
- workspace 拆分 gateway、provider、protocols、translate、interceptor、platform；Node/Workers 是平台适配选择。
- 请求先解析模型与目标，再按客户端 API 形状和上游能力进行必要转换。
- 对 Responses state、compact、reasoning signatures/opaque blobs 的兼容性单独建模。README 明确在不兼容目标上去除可选状态、拒绝不可兼容的必需状态，而不是任意模型随意拼接。
- [INFERENCE] 适合研究“看起来兼容 OpenAI”之外的真实 agent 行为；即便不选 TypeScript，也值得读它的协议设计。

来源：[README](https://github.com/Menci/Floway/blob/c7e4d782763b010e44ba243588f2c83c90c97335/README.md)、[API 路由](https://github.com/Menci/Floway/blob/c7e4d782763b010e44ba243588f2c83c90c97335/packages/gateway/src/data-plane/routes.ts)。

### gpt-load：把协议引擎与调度策略分开

- 核心配置围绕 Channel、Credential、Group、AccessKey；下游 Key 限定可访问组和协议。
- Gin Handler 执行鉴权、配额/RPM 检查、请求记录，再由 Scheduler 按模型、优先级、权重、健康与已尝试候选选择凭据。
- 通用供应商转换/流式/usage 规整复用 Bifrost Core；订阅执行路径也复用 CLIProxyAPI。凭据管理、调度、重试、冷却、黑名单、亲和与运营策略归 gpt-load 自己。
- 失败不是机械重试相同请求：健康结果决定隔离、冷却、切换候选或组；流结果一旦提交，就不能按“未发送”路径透明换账号重放。
- [INFERENCE] 适合研究 Go 网关的调度与资源状态，以及如何组合现有执行引擎而不重写所有 provider。

来源：[README](https://github.com/tbphp/gpt-load/blob/5c6ec0f257b3983e507fb88e1ac1d1841365a71d/README.md)、[Scheduler](https://github.com/tbphp/gpt-load/blob/5c6ec0f257b3983e507fb88e1ac1d1841365a71d/internal/scheduler/scheduler.go)、[Handler](https://github.com/tbphp/gpt-load/blob/5c6ec0f257b3983e507fb88e1ac1d1841365a71d/internal/gateway/handler.go)。

### CLIProxyAPI：可嵌入的账号与协议执行引擎

- 兼容协议入口、模型注册、Auth Manager/Selector、Provider Executor、Translator、凭据持久化与热更新共同组成核心。
- Executor 区分普通执行、流执行、刷新、token 计数；Selector 负责账号选择。模型和凭据的可用状态共同决定路由。
- 当前配置包含轮询、加权轮询、fill-first、会话亲和、冷却与重试；OAuth 刷新不是外围脚本，而是账号生命周期的一部分。
- Go SDK 可嵌入其他程序，gpt-load 的直接依赖是一个实际复用例子。
- [INFERENCE] 更适合个人/团队统一 CLI 模型访问，以及研究 OAuth 与协议适配；账务和收款需另行解决。

来源：[Executor/Selector/Manager](https://github.com/router-for-me/CLIProxyAPI/blob/main/sdk/cliproxy/auth/conductor.go)、[配置示例](https://github.com/router-for-me/CLIProxyAPI/blob/main/config.example.yaml)。注意官方 SDK 高级文档仍含旧 v6 示例，当前主源码是 v8，应以版本匹配的接口为准。

### new-api：渠道聚合与账务耦合的应用平台

- 核心实体包含用户、下游 Token、分组、Channel、模型与定价/额度；渠道适配器处理供应商差异。
- 请求处理不仅选渠道，还包括预扣、分组定价准备、实际用量结算、错误退款及日志。
- 支持多协议、优先级/权重、自动组与会话亲和；relaykit 提取协议 DTO 和转换逻辑；另有异步任务插件扩展。
- [INFERENCE] 与纯代理相比，它必须同时维护“请求成功与否”和“用户应扣多少”的一致性，应用复杂度更多来自产品运营，而不是 HTTP 转发本身。

来源：[Relay controller](https://github.com/QuantumNous/new-api/blob/1a4166d8e8ba9802d2ca56fe8ecf0ed5404e80d5/controller/relay.go)、[渠道选择](https://github.com/QuantumNous/new-api/blob/1a4166d8e8ba9802d2ca56fe8ecf0ed5404e80d5/service/channel_select.go)、[relaykit](https://github.com/QuantumNous/new-api/blob/1a4166d8e8ba9802d2ca56fe8ecf0ed5404e80d5/relaykit/README.md)。

### sub2api：账号资源调度与下游额度平台

- Go 应用采用 handler/service/repository 等职责分层，Ent 管数据、Wire 组装依赖；不应因这些分层就称为微服务系统。
- 核心上游对象是 Account，OAuth/订阅凭据及常规 API Key 都可形成调度资源。
- 请求入口检查下游用户、Key、并发、额度/订阅与在途余额，再生成会话识别信息并选择账号；各平台的 handler/service 负责协议适配。
- Redis 调度缓存/并发状态、PostgreSQL 持久化以及 outbox/快照同步共同支撑账号状态传播；比单进程随机轮询更重。
- [INFERENCE] 值得研究账号并发、黏性会话、限额恢复与账务的组合；系统维护成本也来自这些状态之间的一致性。

来源：[Gateway handler](https://github.com/Wei-Shaw/sub2api/blob/b8dece9000c68815a5b867ca5a1e6f236e173905/backend/internal/handler/gateway_handler.go)、[调度缓存](https://github.com/Wei-Shaw/sub2api/blob/b8dece9000c68815a5b867ca5a1e6f236e173905/backend/internal/repository/scheduler_cache.go)、[Compose](https://github.com/Wei-Shaw/sub2api/blob/b8dece9000c68815a5b867ca5a1e6f236e173905/deploy/docker-compose.local.yml)。

### Magpie：本地 Agent 控制与四协议网关

- 同时管理 agent 自身的模型配置、profiles、instructions/MCP/skills/hooks，以及 API key/订阅账号；桌面、TUI、Web、CLI 是同一产品的多种入口。
- `net/http` gateway 在需要跨协议时，将请求转成内部 `Request/Message/Part/Tool`，响应/流转成 `Result/Event`，再由目标协议 codec 输出；同协议有 passthrough，但策略仍可能修改请求。
- 转换位于 `internal/gateway`，核心函数未导出；即使 `Request/Event` 是大写类型，也不能由普通外部 module 导入其包。实际导入探针返回 `use of internal package ... not allowed`。文档中的 Library 是 agent 配置资料库，不能当成协议 SDK。
- Claude 订阅路径使用真实 `claude` 进程与 MCP tool bridge；与普通 Anthropic API-key HTTP 路径分别考虑。账号发现默认读本机登录信息，需要明确关闭或授权。
- provider 插件使用 Bun；gateway JS middleware 使用进程内 moejs，异常/超时默认保留原输入。Responses 提供 HTTP/SSE，WebSocket upgrade 返回 426。
- [INFERENCE] 主要学习价值是内部语义模型、逐协议事件状态、缓存亲和与 agent 产品集成；它暂不能替代 RelayKit 或 CLIProxyAPI 的公开包直接嵌入 Plateau。

固定提交与来源：[Magpie 专项记录](magpie.md)、[go.mod](https://github.com/yetone/magpie/blob/dc2f0da8203dc1c005475adc0456e1231980a82f/go.mod)、[IR](https://github.com/yetone/magpie/blob/dc2f0da8203dc1c005475adc0456e1231980a82f/internal/gateway/ir.go)、[GUI](https://github.com/yetone/magpie/blob/dc2f0da8203dc1c005475adc0456e1231980a82f/docs/subsystems/gui-shell.md)、[middleware](https://github.com/yetone/magpie/blob/dc2f0da8203dc1c005475adc0456e1231980a82f/docs/subsystems/gateway-middleware.md)。

## 4. 功能横向比较

| 维度 | Higress | Floway | gpt-load 2.x | CLIProxyAPI | new-api | sub2api | Magpie |
|---|---|---|---|---|---|---|---|
| 最鲜明能力 | API/AI/MCP 流量治理 | agent 多协议和状态兼容 | 多渠道凭据池调度/运维 | CLI/OAuth 执行与转换 | 多供应商、用户与计费 | 订阅账号池、配额分发 | 本地 agent 配置与四协议网关一体化 |
| 上游来源 | 模型 API 为主，部分 OAuth 适配 | 订阅账号 + API key | API key + 订阅账号 | CLI/OAuth + API key | provider channel/API 凭证为主 | OAuth 账号 + API key | API key、本地模型、订阅/CLI bridge、插件 |
| 故障处理重心 | 网关策略、token 健康与 failover | 路由顺序、模型/状态兼容 | 权重、冷却、隔离、亲和 | 账号/模型冷却、刷新、切换 | 渠道重试与计费退款 | 账号切换、并发槽、会话状态 | 额度/窗口感知、组路由、缓存亲和、输出前切换 |
| 用户钱包/支付 | 不是产品核心 | 本轮未见完整体系 | 成本估算/配额，不应当成支付平台 | 核心不提供完整体系 | 有用户、额度、订阅及支付集成 | 有用户余额、充值及支付集成 | 有 usage/cost 与访问 key 限额，未证实充值支付闭环 |
| Web UI 作用 | 平台运维控制台 | 网关/agent 配置与 telemetry | 凭据/渠道/访问 Key 运营台 | 独立管理中心 | 管理端 + 用户自助功能 | 管理端 + 用户自助功能 | 托盘/窗口/Web 共享管理页面，另有 TUI/CLI |

重要限定：

1. **计量、计价、扣费、支付是四个层次。** 能算 token 或预估美元成本，不证明已经有余额一致性、退款、支付订单和 webhook 幂等。
2. CLIProxyAPI README 明确：自 v6.10.0 起不再内置用量统计，推荐外部项目；CPAMC 配额/日志不能当作内置账单系统。[依据](https://github.com/router-for-me/CLIProxyAPI#usage-statistics)
3. sub2api 内置支付指南当前迁移表注明订阅套餐暂不支持；不能从其“订阅账号”定位推导出支付端已经支持所有周期套餐。该限制针对内置支付，不是宣称系统没有任何订阅/额度模型。[依据](https://github.com/Wei-Shaw/sub2api/blob/b8dece9000c68815a5b867ca5a1e6f236e173905/docs/PAYMENT_CN.md)
4. Higress 插件文档中 `retryOnFailure` 限于非流式；gpt-load 源码区分流是否已提交。任何项目都应分别核查连接失败、首 token 前、已输出 token 后的行为，而不是把“支持重试”理解成无损恢复。

## 5. 部署、扩展与风险

### 状态分布

- Higress：控制面与数据面可独立部署；平台组件和插件治理带来更多运维对象。
- Floway：Node 单进程/SQLite/文件与 Workers 平台资源是不同运行模型；不能从 Workers 可扩展反推本地 SQLite 方案同样具备全部多实例保证。
- gpt-load：支持外部 SQL，仍有进程内亲和、Responses binding 等状态；共享数据库并不自动完成跨副本调度。
- CLIProxyAPI：本地文件是默认，另有远程存储及 Home 集群入口；本研究没有验证 Home 的多节点一致性保证。
- new-api：共享数据库、认证 secrets 和按需共享 Redis 是多实例部署要点；内存缓存与限流状态需按文档配置。
- sub2api：PostgreSQL + Redis、并发槽与调度状态传播显式进入架构；这说明系统在处理分布式状态，不构成任何规模的吞吐保证。
- Magpie：本地配置与日志、`affinity.json`、进程内候选/队列/RPM；Remote Magpie 和 WebDAV/S3/GitHub 同步是连接与备份能力，不能据此推导分布式调度一致性。账号扫描、agent 配置修改和本地信任策略不宜未经调整用于多租户平台。

各项目部署源码与配置来源见四份详细记录。本轮没有统一硬件、统一上游、相同流式负载的实测，因此不做性能或稳定性排名。

### 许可与账号风险

| 项目 | 调研时主仓许可 |
|---|---|
| Higress | Apache-2.0 |
| Floway | MIT |
| gpt-load | MIT |
| CLIProxyAPI | MIT |
| new-api | AGPL-3.0 |
| sub2api | LGPL-3.0 |
| Magpie | MIT |

许可源文件见各项目详细记录；第三方组件另有义务。AGPL/LGPL 不应统一概括为“不能商用”，也不能统一当作 MIT；需按修改、组合、分发、网络服务的实际方式复核。sub2api README 还包含用途与商业化免责声明，应与 LICENSE 分开审阅，不能凭一句 README 文案做最终许可结论。

软件许可与上游使用授权分别审查：消费者订阅不自动授予 API 转售或共享权；OAuth token 的保管、账号封禁、协议变化、请求日志中的用户数据均是重要风险。此处不是法律意见，也未逐家审阅提供商条款。

## 6. 选型与 Go 阅读建议

以下均为 [INFERENCE]，依需求选择，不是通用质量排名：

| 目标 | 优先研究 |
|---|---|
| 统一企业/K8s 入口，并管理 AI、传统 API、MCP | Higress |
| 改善 Codex/Claude Code 等 agent 的跨协议和上下文兼容 | Floway；Go 路线同时对照 CLIProxyAPI |
| 多渠道、API key/OAuth 凭据池、健康和访问配额管理 | gpt-load 2.x |
| 将已有 CLI/订阅账号提供给兼容 API 客户端，或嵌入 Go 应用 | CLIProxyAPI |
| 用户 Key、渠道、价格、余额、订阅与支付的完整运营产品 | new-api |
| 账号池并发、会话亲和、订阅额度分发与用户运营 | sub2api |
| 多个本地 coding agent 的模型/账号、配置资料库、桌面和终端统一管理 | Magpie |

如果重点是 **Go 工程与架构学习**：

1. **gpt-load**：先读 Scheduler 与 Handler，理解候选选择、健康反馈、失败切换、流提交后的约束；知道转换引擎被复用后，不会误以为所有 provider 都在本仓。
2. **CLIProxyAPI**：看 Executor/Translator/Auth Manager 如何拆分，进一步研究 token 刷新、协议流事件与账号状态。
3. **sub2api**：读 Ent/Wire、handler/service/repository、Redis 调度状态与额度/并发控制；对采用 Go + Ent + Wire 的项目有直接参考价值，但不建议连同业务复杂度一起照搬。
4. **new-api**：研究渠道适配与计费生命周期，重点看预扣、结算、退款和日志如何随重试变化。
5. **Higress**：最后进入 Envoy/Istio/xDS/Wasm，学习平台级流量基础设施，而不是把它当成普通 Go Web 项目模板。
6. **Magpie**：读 `internal/gateway/ir.go` 与各协议 codec，再看路由、亲和、Claude CLI/MCP bridge；用于学习和评估抽取，不能当作已经公开的可导入 Go SDK。

Floway 虽非 Go，仍可作为协议语义的对照样本。语言相同帮助阅读与复用，真正决定架构是否适合的是：上游凭证种类、下游用户模型、状态管理与是否需要交易账务。

### 协议转换复用：2026-10-13 补充

| 候选 | 可复用形态 | 对 Plateau 的含义 |
|---|---|---|
| RelayKit | 独立 Go module、DTO、请求/响应/流转换；AGPL-3.0 | 可评估直接使用，宿主仍负责 HTTP/SSE 收发，先核查许可与转换损耗 |
| CLIProxyAPI | `sdk/translator` 主要处理 JSON 字节；完整 SDK 还管理账号、执行与代理生命周期；MIT | 区分只复用转换与嵌入整套运行系统，避免重复管理状态 |
| Magpie | 内部统一 IR 与各协议 codec，`internal/gateway` 不允许普通外部 module 导入；MIT | 可接其 HTTP gateway、研究或评估抽取；当前没有 RelayKit 式公开转换 SDK |

详细来源与导入验证见 [Magpie 专项记录](magpie.md#5-与-relaykit--cliproxyapi-的复用比较)。[INFERENCE] 三者都不要求 Plateau 先将四套厂商数据全部重写成 Proto；是否设计内部 Proto 取决于真实 RPC 需求。Magpie 缺少公开转换包只影响直接嵌入方式，不构成必须新增 Proto 的理由。本轮没有决定采用、fork 或抽取任何一个项目。

## 7. 版本与详细证据

| 项目 | 读取期间主分支 SHA |
|---|---|
| Higress | `bda81f1067e1285775e650644a20a635f51b0a6d` |
| Floway | `c7e4d782763b010e44ba243588f2c83c90c97335` |
| gpt-load | `5c6ec0f257b3983e507fb88e1ac1d1841365a71d` |
| CLIProxyAPI | `a4acc9f752bd46571f737a10c04bf413656ab06b` |
| new-api | `1a4166d8e8ba9802d2ca56fe8ecf0ed5404e80d5` |
| sub2api | `b8dece9000c68815a5b867ca5a1e6f236e173905` |
| Magpie | `dc2f0da8203dc1c005475adc0456e1231980a82f`（2026-10-13 补充） |

Higress、CLIProxyAPI 及其独立控制台的部分链接为 live main，SHA 用于记录读取期间主仓状态；Floway、gpt-load、new-api、sub2api 主要使用历史 commit 固定链接，Magpie 使用本轮固定 commit。两轮调研时间分别保留。

- [Higress / CLIProxyAPI 详细证据](higress-cliproxyapi.md)
- [Floway / gpt-load 详细证据](floway-gpt-load.md)
- [new-api / sub2api 详细证据](new-api-sub2api.md)
- [Magpie：本地 Agent 管理、转换与复用证据](magpie.md)

任务保持规划调研范围：已完成本轮比较，没有业务代码实现或部署，未修改已有 Admin 规划任务与用户工作区变更。
