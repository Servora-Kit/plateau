# Magpie：本地 Agent 管理、协议转换与复用评估

调研日期：2026-10-13。固定源码：[yetone/magpie@dc2f0da8203dc1c005475adc0456e1231980a82f][revision]；提交时间 2026-10-13T08:40:24Z；Go 模块解析为 `v0.1.1225-0.20261013084024-dc2f0da8203d`。这是主分支快照，不冒充稳定发行版。原六个项目沿用 2026-10-05 的报告，不视为本轮重新核验。

证据分为官方说明、源码观察、隔离验证和标记为 `[INFERENCE]` 的架构判断。本轮没有运行 Magpie 应用、读取本机账号/agent 配置、登录上游、部署 Docker、操作 GUI 或压测。隔离验证仅检查外部 Go module 的导入限制；不代表四协议运行兼容性已通过。

## 1. 结论

- **定位：本地优先的 Agent 配置管理工具与多协议 AI 网关。** 模型切换、provider/订阅账号、profiles、instructions/MCP/skills/hooks 管理与网关在同一个产品里；另支持 headless、Web、Docker、远端 Magpie。[README][readme]、[集成说明][integrating]
- **技术栈：Go + 标准库 `net/http`，Wails v3 桌面壳，原生 HTML/CSS/JavaScript 界面，Bubble Tea TUI。** 管理页面没有 React/Vue 框架、bundler 或 Wails binding generator；不要把官网或辅助工具的前端依赖当作产品 UI 栈。[go.mod][gomod]、[GUI 说明][gui]
- **四协议转换使用内部 Go 中间模型。** 请求归一到 `Request/Message/Part/Tool`，响应归一到 `Result`，流事件归一到 `Event`，由各协议 parser/builder/decoder/encoder/render 函数处理。[ir.go][ir]、[Chat][chat]、[Responses][responses]、[Anthropic][anthropic]、[Gemini][gemini]
- **当前没有可直接导入 Plateau 的独立公共转换 SDK。** 相关包在 `internal/gateway`，主要转换函数未导出；`Request`、`Event` 等虽为大写类型，所属包仍受 Go internal 导入规则限制。外部 module 的实际导入探针已被 Go 拒绝。
- **文档里的 Library 是配置资料库。** 它管理 instructions、MCP、skills、hooks、RTK 等，不代表可嵌入 Go 协议库。[Library 说明][library]
- **许可为 MIT，但易读/可抽取不等于已提供稳定 SDK。** 抽取源码需要自己处理依赖、接口、升级和兼容验证；本轮未决定引入或抽取。[LICENSE][license]

## 2. 功能、前后端与持久化

| 维度 | 核实结果 | 依据 |
|---|---|---|
| 使用场景 | 多个 coding agent 共用 API key、本地模型和订阅账号；修改各 agent 自身配置，切换 provider/model 与 profile | [README][readme]、[集成说明][integrating] |
| 后端 | `go 1.26.8`；网关使用 `net/http`、`http.NewServeMux`、`http.Client`；此调用链不需要 Gin 或 Kratos | [go.mod][gomod]、[gateway.go][gateway] |
| 桌面/浏览器 | Wails `v3.0.0-beta.24`；嵌入 HTML/CSS/JS；托盘 panel、窗口、浏览器界面复用页面及 Go JSON API | [go.mod][gomod]、[GUI 说明][gui] |
| TUI | Bubble Tea/Bubbles/Lip Gloss；与桌面、CLI、Web 共享产品核心；TUI 可在无其他实例时托管 gateway | [go.mod][gomod]、[GUI 说明][gui] |
| 数据 | `settings.json`、`providers.json`、`logins.json`、`affinity.json`、插件/资料库配置和 `usage.jsonl` 等本地文件；有进程内状态 | [账号说明][accounts]、[路由说明][routing]、[集成说明][integrating] |
| SQLite | 有 `modernc.org/sqlite` 依赖，源码用于读取/导入其他应用数据库等；不能仅凭依赖将其描述为“SQL 中转平台” | [go.mod][gomod]、[导入实现][importapps] |
| 用户访问控制 | 本机与 LAN 使用不同信任策略；LAN sharing 使用 gateway key，可约束模型、账号与日/周/月 token/cost | [集成说明][integrating]、[README][readme] |
| 计量 | token、cache read/write、reasoning、请求、上游账号/key 归因、价格估算、quota、OTLP 等；未在查阅材料中证实用户钱包、充值订单和收款闭环 | [README][readme]、[参考文档][reference] |
| 部署 | macOS/Windows/Linux、Termux、headless；Docker 使用 `-tags nogui`，默认 `serve`、`/config` 卷和容器内部 `3425` | [Dockerfile][docker]、[README][readme] |

`3425` 是上游默认值，本轮没有给 Plateau 新增应用或分配本地端口。

### 插件运行环境必须分开

1. **内建 Go 功能插件**：进程内注册，按功能启停，例子包含 groups、Library、observability、sessions、warmups；不是动态加载 Go shared library。[插件运行时][plugins]
2. **provider/auth 社区插件**：OpenCode/pi 兼容 npm 包，通过 Bun host 登录、列模型、执行请求。[provider 插件][providers]
3. **gateway JavaScript middleware**：运行于网关进程内的 **moejs（Go 实现的 JS 引擎）**，不在 Bun host 中。`onRequest`、`onEvent`、`onResponse` 分别处理请求、SSE 事件和完整响应；运行时池与超时在内部实现。[middleware 文档][middleware]

middleware 的错误或超时默认 fail-open：保持原请求/事件并记录失败。`[INFERENCE]` 因此不能未加分析就把它作为必须拒绝请求的安全策略执行器。provider 插件可以执行本地代码，不能把“插件”一词当作操作系统沙箱保证；本轮未审计其隔离强度。

## 3. 四协议架构与 Go 数据模型

### 3.1 转换路径

跨协议请求的基本结构是：入口 JSON → 协议 parser → 内部 `Request` → 目标协议 builder → 上游。流式返回则是：上游事件 → decoder → 内部 `Event` → 客户端协议 encoder → SSE；非流式返回由 `Result` 与各协议 renderer 组织。网关包含流式/完整响应适配，不能理解为只换字段名。[ir.go][ir]、[网关实现][gateway]

| 协议 | 请求源码入口 | 流式/非流式源码入口 |
|---|---|---|
| OpenAI Chat | `parseChat`、`buildChat` | `chatDecoder`、`chatEncoder.event/finish`、`renderChat` |
| OpenAI Responses | `parseResponses`、`buildResponses` | `responsesDecoder`、`responsesEncoder.event/finish`、`renderResponses` |
| Anthropic Messages | `parseAnthropic`、`buildAnthropic` | `anthropicDecoder`、`anthropicEncoder.event/finish`、`renderAnthropic` |
| Gemini | `parseGemini`、`buildGemini` | Gemini decoder 路径、`geminiEncoder.event/finish`、`renderGemini` |

以上为源码入口核验，非逐供应商/模型的运行覆盖证明。来源为 [Chat][chat]、[Responses][responses]、[Anthropic][anthropic]、[Gemini][gemini]。

`ir.go` 的 `Part.Kind` 包含 Text、Image、File、ToolCall、ToolResult、Thinking、Search；`EventKind` 包含 KStart、KText、KThink、KToolStart、KToolArgs、KStop、KUsage、KError，以及签名/封装推理等事件。`Part.Args`、`Part.Sealed` 等仍使用 `json.RawMessage`，故设计为类型化语义与原始数据混合，不是把所有厂商数据机械转成同一个 struct。

### 3.2 同协议转发与信息保留

- 当目标模型支持客户端 API 时有 `passthrough` 路径；模型 API 选择、Claude/Responses 特定优先级、搜索和其他策略也会影响是否转换。[路由说明][routing]
- 同协议透传不保证原字节不变：模型/字段修正、middleware、redaction、provider 兼容处理可能介入。不能把该路径写成“绝不修改”。
- `Part.Sealed/SealedBy` 保留 Anthropic redacted thinking 或 Responses encrypted reasoning 等原始结构，并限制由对应 API 的 builder/encoder/render 写出；跨协议不能把厂商签名凭空转换成另一家的有效签名。[ir.go][ir]
- Chat 入站使用私有 DTO 与部分 RawMessage 字段；任意未知扩展字段不因此获得无损保证。本轮没有发现 RelayKit 式公共 `Quality/Steps/Diagnostics` 转换结果 API，不代表全仓不存在任何内部诊断。

### 3.3 SSE、协议限制与失败处理

- 每个协议有自己的 encoder 状态和结束处理，处理 Chat `[DONE]`、Responses item/sequence、Anthropic content block、Gemini parts/usage；这些职责在 Go HTTP 网关内部，没有依赖 Proto/gRPC HTTP 注解生成器。[协议源文件][responses]
- **Responses WebSocket 不提供。** `responsesOverHTTP` 对 WebSocket upgrade 返回 426 并明确提示使用 POST HTTP/SSE，普通 GET 返回 405。不能把“支持 Responses”扩大为全传输方式覆盖。[gateway.go][gateway]
- 官方约束明确：仅在尚未向下游发送回复时切换候选，不能拼接两个上游的半条回答。思考内容暂存、失败分类和队列/RPM 等有另外的策略；实际行为依路径而定。[路由说明][routing]
- 本轮未验证真实模型的工具往返、未知字段、流中断、背压、取消或长会话兼容性，不能从存在测试文件或内部模型推导全部通过。

## 4. 账号、路由、状态与隐私

### 4.1 账号与 Claude bridge

账号发现默认读取受支持 agent 的登录文件和系统凭据存储；可通过 Privacy 配置、`magpie accounts discovery off` 或本次运行的 `MAGPIE_NO_ACCOUNTS=1` 禁用。`[INFERENCE]` 这是面向用户本机的产品行为，不能不经授权搬到平台服务扫描宿主账号。[README][readme]、[账号说明][accounts]

Claude 订阅路径通过真实本地 `claude` 进程与 MCP tool bridge 工作；必须区别于“直接拿 OAuth token 发标准 Messages HTTP 请求”。普通 Anthropic API-key provider 是另一条路径。部署这种订阅桥接还涉及 CLI 安装、身份、子进程与工具结果的生命周期。[Claude bridge][claudebridge]

### 4.2 路由与扩展性

README 列出 smart、order、rotate、usage、pace；同提交更详细的路由说明还列出 group `weight`（成员级平滑加权轮询），不能把 README 的五项当作完整枚举。[路由说明][routing]

- smart/usage/pace 综合账号 allowance、重置时间或使用情况；支持嵌套组和 intent routing。
- affinity 尽量让对话留在先前回答的账号以利用 prompt cache；有 auto/session/turn/off/strict，部分状态保存在 `affinity.json`。
- key/account 并发、等待队列与 RPM 有限制；部分候选降序、队列、失败状态在进程内。
- WebDAV/S3/GitHub 备份同步与 Remote Magpie 连接能力不等于 active-active 集群调度；没有在本轮找到分布式亲和、并发槽和全局 RPM 的完整一致性保证。

来源：[路由说明][routing]、[同步说明][sync]。以上没有做多实例实测。

## 5. 与 RelayKit / CLIProxyAPI 的复用比较

RelayKit 与 CLIProxyAPI 行沿用本任务前续源码核验，仅用于说明接口形态；不构成本轮对两者最新版本的完整复审。

| 维度 | RelayKit | CLIProxyAPI | Magpie（本次固定提交） |
|---|---|---|---|
| 主要职责 | 独立 DTO、请求/响应/流事件转换 | Translator + 可嵌入代理、账号与执行生命周期 | 完整本地 Agent 管理与网关产品 |
| 数据形态 | 已知 Go DTO，转换结果带诊断/路径 | Translator 主要接收/返回 JSON 字节，流转换携带状态 | 协议私有 DTO + 内部统一 Request/Result/Event + RawMessage |
| 公开复用入口 | `relaykit/relayconvert` 独立 module | `sdk/translator`、`sdk/cliproxy` 等 | 官方集成方式以 HTTP gateway、agent adapter、plugin 为主；无独立公共 converter 包 |
| HTTP/SSE | 宿主实现收发与 framing | 完整 SDK 含运行时；单独 Translator 职责较窄 | 网关自身实现路由、解析、转换、SSE、调度与记录 |
| 许可 | AGPL-3.0，需核查引入方式 | MIT | MIT；保留版权及许可，依赖/插件另审 |
| 对 Plateau 的主要价值 | 评估直接复用转换能力 | 评估转换/账号执行 SDK | 学习 IR、事件状态和本地 Agent 产品；作为外部 HTTP 上游或评估后抽取 |

依据：[RelayKit README（前续核验提交）](https://github.com/QuantumNous/new-api/blob/0f38a14a5e8e322ea741478b0e49878be3130e1d/relaykit/README.md)、[CLIProxyAPI Translator（历史快照）](https://github.com/router-for-me/CLIProxyAPI/blob/a4acc9f752bd46571f737a10c04bf413656ab06b/sdk/translator/types.go)、[Magpie 集成方式][integrating]、[MIT 许可][license]。

### 对 Plateau 的判断

- `[INFERENCE]` Magpie 证明“Go 中间语义模型 + 每协议 codec”是可研究的实现路径；它不要求把四套厂商 JSON 都重写成 Proto。
- `[INFERENCE]` 没有公开 SDK，只说明不能直接按 RelayKit 的使用方式 import；不应据此推导 Plateau 必须先建立完整 Proto 转换模型。内部 Proto 的范围仍由真实 RPC 需求决定。
- `[INFERENCE]` 直接接 Magpie HTTP 是现成集成方式，但会引入独立应用的生命周期与状态；抽取转换代码可依据 MIT 评估，仍需拆除 provider/settings/usage 等依赖并自负兼容维护。两者均未获采用决定。
- `[INFERENCE]` 本地账号发现、修改 agent 配置、单实例文件状态与 fail-open middleware 不应无条件照搬到多租户平台。

## 6. 隔离验证：普通外部 Go module 能否导入转换所在包

目的仅为验证“是否能像公共 SDK 一样直接导入”，不运行任何 Magpie 代码或账号发现。

在临时目录创建 `module example.com/magpie-import-probe`，探针为：

```go
package main

import "github.com/yetone/magpie/internal/gateway"

func main() {
    println(gateway.DefaultAddr)
}
```

实际执行：

```text
GOWORK=off go get github.com/yetone/magpie@dc2f0da8203dc1c005475adc0456e1231980a82f
GOWORK=off go run -mod=readonly -tags nogui .
```

第一步解析为 `v0.1.1225-0.20261013084024-dc2f0da8203d`；第二步以退出码 1 返回预期限制：

```text
package example.com/magpie-import-probe
    main.go:3:8: use of internal package github.com/yetone/magpie/internal/gateway not allowed
```

这是 **Go 导入可用性验证**，不是上游故障，也不是转换功能 smoke 成功。程序未进入 `main`，未启动网关、后台保活或读取账号；依赖仅进入 Go module cache，探针目录清理，Plateau/Servora 的 go.mod/go.sum 未改。

未采用伪装为上游命名空间来绕过 internal 限制的方法，也未将私有函数复制成一个“已可复用 SDK”冒充验证。

## 7. 未验证项与资料索引

尚未验证：真实供应商/订阅登录、四协议所有字段与转换组合、SSE 中断/取消、GUI、Docker、支付能力的全仓否定、插件沙箱、跨节点一致性、性能及账号条款。源码/文档中的上游自测数字不作为本报告性能证据。

后续若选型，首先决定使用外部 HTTP gateway、采用已公开转换库，还是维护自己的协议实现；然后按真实客户端、工具调用、reasoning、usage 和错误场景做独立验证。本轮只完成调研，不修改平台实现规范或认定某个库已被采用。

[revision]: https://github.com/yetone/magpie/tree/dc2f0da8203dc1c005475adc0456e1231980a82f
[readme]: https://github.com/yetone/magpie/blob/dc2f0da8203dc1c005475adc0456e1231980a82f/README.md
[gomod]: https://github.com/yetone/magpie/blob/dc2f0da8203dc1c005475adc0456e1231980a82f/go.mod
[license]: https://github.com/yetone/magpie/blob/dc2f0da8203dc1c005475adc0456e1231980a82f/LICENSE
[integrating]: https://github.com/yetone/magpie/blob/dc2f0da8203dc1c005475adc0456e1231980a82f/docs/integrating.md
[reference]: https://github.com/yetone/magpie/blob/dc2f0da8203dc1c005475adc0456e1231980a82f/docs/reference.md
[gui]: https://github.com/yetone/magpie/blob/dc2f0da8203dc1c005475adc0456e1231980a82f/docs/subsystems/gui-shell.md
[library]: https://github.com/yetone/magpie/blob/dc2f0da8203dc1c005475adc0456e1231980a82f/docs/subsystems/library.md
[accounts]: https://github.com/yetone/magpie/blob/dc2f0da8203dc1c005475adc0456e1231980a82f/docs/subsystems/providers-accounts.md
[routing]: https://github.com/yetone/magpie/blob/dc2f0da8203dc1c005475adc0456e1231980a82f/docs/subsystems/gateway-routing.md
[plugins]: https://github.com/yetone/magpie/blob/dc2f0da8203dc1c005475adc0456e1231980a82f/docs/subsystems/plugin-runtime.md
[providers]: https://github.com/yetone/magpie/blob/dc2f0da8203dc1c005475adc0456e1231980a82f/docs/subsystems/provider-plugins.md
[middleware]: https://github.com/yetone/magpie/blob/dc2f0da8203dc1c005475adc0456e1231980a82f/docs/subsystems/gateway-middleware.md
[sync]: https://github.com/yetone/magpie/blob/dc2f0da8203dc1c005475adc0456e1231980a82f/docs/subsystems/sync-backup.md
[claudebridge]: https://github.com/yetone/magpie/blob/dc2f0da8203dc1c005475adc0456e1231980a82f/docs/subsystems/claude-subscription-bridge.md
[docker]: https://github.com/yetone/magpie/blob/dc2f0da8203dc1c005475adc0456e1231980a82f/Dockerfile
[importapps]: https://github.com/yetone/magpie/blob/dc2f0da8203dc1c005475adc0456e1231980a82f/internal/provider/importapps.go
[gateway]: https://github.com/yetone/magpie/blob/dc2f0da8203dc1c005475adc0456e1231980a82f/internal/gateway/gateway.go
[ir]: https://github.com/yetone/magpie/blob/dc2f0da8203dc1c005475adc0456e1231980a82f/internal/gateway/ir.go
[chat]: https://github.com/yetone/magpie/blob/dc2f0da8203dc1c005475adc0456e1231980a82f/internal/gateway/chat.go
[responses]: https://github.com/yetone/magpie/blob/dc2f0da8203dc1c005475adc0456e1231980a82f/internal/gateway/responses.go
[anthropic]: https://github.com/yetone/magpie/blob/dc2f0da8203dc1c005475adc0456e1231980a82f/internal/gateway/anthropic.go
[gemini]: https://github.com/yetone/magpie/blob/dc2f0da8203dc1c005475adc0456e1231980a82f/internal/gateway/gemini.go
