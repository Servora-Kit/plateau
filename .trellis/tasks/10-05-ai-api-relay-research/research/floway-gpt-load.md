# Floway 与 GPT-Load：源码核对对比

调研日期：2026-10-05。只读官方 GitHub 仓库；未构建、部署或运行测试。版本快照：Floway `main` @ [`c7e4d782763b010e44ba243588f2c83c90c97335`](https://github.com/Menci/Floway/commit/c7e4d782763b010e44ba243588f2c83c90c97335)（提交时间 2026-10-03）；GPT-Load `main` @ [`5c6ec0f257b3983e507fb88e1ac1d1841365a71d`](https://github.com/tbphp/gpt-load/commit/5c6ec0f257b3983e507fb88e1ac1d1841365a71d)（提交时间 2026-10-05）。本文将 README/文档宣称与 manifest、源码观测分开；“未见”不等于证明不存在。

## 一眼比较

| | Floway | GPT-Load |
|---|---|---|
| 产品重心 | 自托管、面向 coding agent 的多协议 LLM 网关；把订阅账号和 API-key provider 放进一个网关。 | 自托管多渠道/多凭据网关；核心面向统一管理渠道、凭据、分组、访问密钥、调度、故障恢复与使用统计。 |
| 真正后端 | TypeScript，不是 Go。Hono 路由；Cloudflare Workers 或 Node.js；README 的 Docker 部署是 Node.js。没有 Go 版本、Gin/Echo router 或 Go ORM 可列。 | Go 1.27.0；Gin 1.12.0；GORM 1.31.2，含 SQLite/MySQL/PostgreSQL 驱动。 |
| 协议策略 | 明确具有跨协议翻译层：OpenAI Chat/Responses、Anthropic Messages、Gemini 等入口，可按上游能力翻译；并以 interceptor/shim 适配 coding-agent 的 Responses 语义。 | 对外提供多种原生接口与渠道/协议适配；README 明确采用 Bifrost Core 做 provider auth、转换、流式及 usage normalization，GPT-Load 自己实现账号/凭据选择、重试、健康、日志和计量。 |
| 前端 | React 19、React Router 8、Fluent UI v9，UnoCSS；Vite 8；pnpm 10。 | Vue 3 + Vue Router 5、Vue Query、Reka UI、Tailwind CSS 4；Vite 8、TypeScript 6、pnpm 11。内嵌管理 UI。 |
| 数据/计费 | Node 部署的 SQLite 与文件存储；Cloudflare 部署用 D1 等平台绑定。源码/README 支持 telemetry 与 token/usage 观测；不像面向终端客户的预付费/充值平台。 | 默认本地 SQLite；可配置 MySQL/PostgreSQL。usage、tokens、缓存命中和成本估算、AccessKey 限额/配额与 RPM 管控，定位偏运营台；所读资料未见在线支付/充值。 |
| 更有辨识度的差异 | 跨 API 形状适配、Responses state/opaque blobs、客户端 agent 工作流；平台可选 serverless Workers。 | 凭据池调度和可配置失败处理更突出：凭据权重、路由优先级、隔离/冷却/黑名单、亲和性、可配置数据库与管理运营。 |

## Floway

### 定位与能力：文档宣称

README 将其定位为 self-hosted LLM API gateway，专为 coding agents/API clients；支持 GitHub Copilot、ChatGPT/Codex 和 Claude.ai 订阅，以及 Azure、Ollama、自定义 HTTP provider。对外接收 OpenAI（含 Responses、Embeddings、Images、Transcriptions）、Anthropic Messages、Gemini、Cohere/Jina/Voyage rerank 等接口；可发现部分 provider 模型目录、配置模型别名/上游顺序/API key、为 Claude Code/Codex 生成配置。文档宣称 Responses state 与 opaque blobs 可跨兼容模型保留/路由。[README](https://github.com/Menci/Floway/blob/c7e4d782763b010e44ba243588f2c83c90c97335/README.md)

重要区分：这里的“订阅”是由 Floway 持有/使用用户授权的上游 Copilot/ChatGPT/Claude 账号（OAuth 或凭据），与仅保存商业 API token 的上游都支持；它并不等同于向 Floway 的最终 API 用户出售订阅套餐。

### 实际技术栈（manifest / 源码）

- 官方仓库快照是 pnpm TypeScript workspace；根 [package.json](https://github.com/Menci/Floway/blob/c7e4d782763b010e44ba243588f2c83c90c97335/package.json) 定义 pnpm 10.24.0、TypeScript 5.9.3、Wrangler 4.x。GitHub 仓库元数据显示主语言 TypeScript。仓库根文件清单和部署目标均指向 TS/Node/Workers；未找到 Go module，因此 Go 版本、Go router/ORM：**不适用/未见 Go 后端**，不能将其标成 Go 项目。
- 后端 HTTP 层为 Hono：[`packages/gateway/package.json`](https://github.com/Menci/Floway/blob/c7e4d782763b010e44ba243588f2c83c90c97335/packages/gateway/package.json) 依赖 `hono`；[`packages/gateway/src/data-plane/routes.ts`](https://github.com/Menci/Floway/blob/c7e4d782763b010e44ba243588f2c83c90c97335/packages/gateway/src/data-plane/routes.ts) 将各协议的 chat/codex/models/audio/images/rerank route 挂载到 Hono app。没有看到 ORM 依赖/Go ORM：SQLite repo 以 `SqlRepo` 抽象接入；更细 SQL 实现/ORM 行为本次未逐行审计，不推断其使用某个 ORM。
- Dashboard manifest [`apps/web/package.json`](https://github.com/Menci/Floway/blob/c7e4d782763b010e44ba243588f2c83c90c97335/apps/web/package.json)：React 19.2、React Router 8.3、Fluent UI React 9、Vite 8、UnoCSS、pnpm workspace；不是 Vue/Vben。

### 请求、路由、状态与扩展

- 数据面 API 路由按协议/功能分组注册；分组的 `chat/routes.ts` 和上层 `data-plane/routes.ts` 源码显示将 Chat Completions、Responses、Anthropic、Gemini 等入口接到处理逻辑，而不是单纯通用反向代理。[routes.ts](https://github.com/Menci/Floway/blob/c7e4d782763b010e44ba243588f2c83c90c97335/packages/gateway/src/data-plane/routes.ts)
- README 明确宣称按模型配置、公开模型目录、provider upstream/routing order 分发；不同 provider 有自有适配器包（Copilot、Codex、Claude Code、Azure、Custom、Ollama）。实现拆分见 [`packages/gateway/package.json`](https://github.com/Menci/Floway/blob/c7e4d782763b010e44ba243588f2c83c90c97335/packages/gateway/package.json) 的 workspace provider/translate/protocols 依赖。其核心特色是翻译/语义适配：README 文档区分 native API 与不同上游协议，并说只在需要时跨协议翻译；Responses compact shim、opaque state 携带兼容性身份等功能依赖更高层的 agent-aware 处理。
- 源码实例：[`packages/gateway/src/execution/handler.ts`](https://github.com/Menci/Floway/blob/c7e4d782763b010e44ba243588f2c83c90c97335/packages/gateway/src/execution/handler.ts) 处理 provider 模型目录刷新任务，并将配置错误/上游不可用映射为 JSON 错误。它不是唯一数据面 handler，切勿把该文件误作全部转发实现。
- README 列出 Responses HTTP 与 WebSocket、SSE 兼容接口和 Stateful Responses；数据面 route 分拆为 Responses/Chat 等模块。README 表明音频转录目前为缓冲 multipart passthrough，但 chat/Responses 等可以流式。由于本轮未逐个读取 provider streaming 迭代器，不把每种 provider/协议的具体 flush/backpressure 细节当成已验证事实。
- Provider access 混合 OAuth/订阅账号和 API tokens；Dashboard 管理上游、排序、别名和下游 API keys。文档说模型可 live discover 或手工定义，Custom 可指定协议端点和 header 规则。[README](https://github.com/Menci/Floway/blob/c7e4d782763b010e44ba243588f2c83c90c97335/README.md)
- 持久化：README 自报 Docker/Node 使用 SQLite、文件 dump body 与 Responses 大型 state payload；Docker Compose 将 `/data/floway.db` 和 `/data/files` 放在 `floway-data` 卷。[docker-compose.yml](https://github.com/Menci/Floway/blob/c7e4d782763b010e44ba243588f2c83c90c97335/docker/docker-compose.yml)。README 的 Workers 方案使用 D1 migration；具体 Worker state/backend binding 及多实例一致性未在本轮源码中核验。Node 启动代码以单进程 `SqlRepo` 和 `InProcessExecutionCellNamespace` 注册执行环境，见 [`apps/platform-node/entry.ts`](https://github.com/Menci/Floway/blob/c7e4d782763b010e44ba243588f2c83c90c97335/apps/platform-node/entry.ts)。**推断**：本地 Node + SQLite/文件卷主要是单实例部署形态；多个副本共享数据库/文件时的并发/锁/Responses 状态协调保证，现有检查不能确认，不应假设自动横向扩展。

### 运营、部署、许可

README 的仪表盘面向自身 gateway 管理及 telemetry/usage 观察、API key/upstream 运维。源码职责和 README 未显示终端租户的钱包、充值、支付、发票体系，故它不是已证实的商业计费 SaaS（不排除未核验功能）。默认 Docker Compose 是一个 Node server 单服务、8788 端口+持久卷；另支持本地 Node 与 Cloudflare Workers。[README](https://github.com/Menci/Floway/blob/c7e4d782763b010e44ba243588f2c83c90c97335/README.md)、[compose](https://github.com/Menci/Floway/blob/c7e4d782763b010e44ba243588f2c83c90c97335/docker/docker-compose.yml)。根 [LICENSE](https://github.com/Menci/Floway/blob/c7e4d782763b010e44ba243588f2c83c90c97335/LICENSE) 为 MIT。

## GPT-Load

### 定位与能力：文档宣称

README 将 GPT-Load 2.0 定位为 self-hosted AI gateway for multi-channel, multi-credential setups：将 API keys 与 Codex/Claude/Antigravity/Grok 等订阅 OAuth 凭据放到统一的 channel、group 与 AccessKey 管理中；为 OpenAI/Responses、Anthropic、Gemini、Mistral 等 client protocols 提供统一入口。README 宣称支持多凭据调度、weights、重试、cooldown、blacklist、session affinity、request logs 和 usage/cost estimates。[README](https://github.com/tbphp/gpt-load/blob/5c6ec0f257b3983e507fb88e1ac1d1841365a71d/README.md)

### 实际技术栈（manifest / 源码）

- [`go.mod`](https://github.com/tbphp/gpt-load/blob/5c6ec0f257b3983e507fb88e1ac1d1841365a71d/go.mod) 是 `module gpt-load`、`go 1.27.0`；显式依赖 Gin 1.12.0、GORM 1.31.2、GORM SQLite driver 1.11.0、MySQL driver 1.6.0、Postgres driver 1.6.3。ORM/router 均有 manifest 证据。
- 前端不是 Go template 粗略页面：[`web/package.json`](https://github.com/tbphp/gpt-load/blob/5c6ec0f257b3983e507fb88e1ac1d1841365a71d/web/package.json) 明确 Vue 3.5、Vue Router 5、Vue Query、Reka UI、Tailwind 4、Vite 8.3、TypeScript 6、pnpm 11；Docker/README 描述自带管理 console。
- HTTP 实际核心为 Go Gin [`internal/gateway/handler.go`](https://github.com/tbphp/gpt-load/blob/5c6ec0f257b3983e507fb88e1ac1d1841365a71d/internal/gateway/handler.go)：`Handler.Handle` 做请求上下文/认证检查、速率与配额检查、协议 dialect 选择、请求日志，并调用执行尝试。README 对转换/流的依赖说明与 `go.mod` 中 Bifrost Core 1.11.0、CLIProxyAPI/v8 8.0.8 依赖相符；GPT-Load 自己所有权明确包括 credential storage, account selection, scheduling, retry, health, affinity, logging, usage policy。

### 请求路径、调度与协议

- [`internal/scheduler/scheduler.go`](https://github.com/tbphp/gpt-load/blob/5c6ec0f257b3983e507fb88e1ac1d1841365a71d/internal/scheduler/scheduler.go) 的 `New` 按配置快照筛选 execution targets，将 native/converted route mode 分层；WeightedMix 设置允许 native 与 converted 同层。`Iterator` 获取当前可用凭据候选，依据访问密钥可用组/凭据、冷却和黑名单状态、组/凭据权重、模型/目标、跳过组与已尝试候选来产出 Selection。不是随机挑单一 API key 的简单代理。
- [`internal/gateway/handler.go`](https://github.com/tbphp/gpt-load/blob/5c6ec0f257b3983e507fb88e1ac1d1841365a71d/internal/gateway/handler.go) `Handle` 建 request recorder，评估 AccessKey usage quota/RPM，解析 protocol dialect。`executeAttempts` 通过 scheduler iterator 取 credential/group/model 选择；解密、规范化 credential，执行参数覆盖后交给 `Forward` / `ForwardStream`。失败按 health decision 更新 cooldown/blacklist；可重试时推进候选优先级或跳过故障 group；subscription credential 的特定认证错误可触发一次 refresh/replay。只有尚未提交响应时能对备用凭据重试；已提交流结果则记录并结束，不能透明切换重放已送出的响应。源码证据：同一文件 898 行起的 `executeAttempts`，特别是选择 `iterator.Next`、流分支 `ForwardStream`、`result.Committed` 与失败 retry 分支。
- README 定义多种对外原生路由形状；protocol dialect/route conversion 显示其不只是纯粹透明透传。项目把通用 provider auth、转换、流与用量规整委托给 Bifrost Core；订阅 OAuth/执行路径还依赖 CLIProxyAPI 的适配。由此与 Floway 的区别不是“是否翻译”，而在主要设计中心：Floway 把 coding-agent API 兼容、Responses state/多协议转换作为产品特色；GPT-Load 将成熟转换层复用为执行部件，自己强化面向运营的凭据池调度/健康、计量和访问控制。
- Stream 路径有实质不同于普通 buffer proxy 的代码路径；`executeAttempts` 将 stream 请求派给 `ForwardStream`，记录 first response 与 stream completion；已提交后不再作未发送式 failover。WebSocket/Responses 与实时接口也在 README scope 中列出。

### 存储、配额、部署、多实例与许可

- README 声称默认应用管理的 SQLite 保存 database、`auth.key`、`encryption.key`；可接 MySQL/PostgreSQL。README 强调 encryption key 必须与 DB 同备份，否则已存 provider credentials 无法解密；项目注明当前版本无主密钥 rotation。[README](https://github.com/tbphp/gpt-load/blob/5c6ec0f257b3983e507fb88e1ac1d1841365a71d/README.md)；驱动/ORM 证据见 [`go.mod`](https://github.com/tbphp/gpt-load/blob/5c6ec0f257b3983e507fb88e1ac1d1841365a71d/go.mod)。
- Usage 运营能力比单纯 telemetry 更完整：README 列出用量、请求日志、tokens/cache hit 与 cost estimates；Handler 源码冻结 per-attempt pricing、在调用结束后写 usage recorder，并对访问密钥执行 cost quota/RPM/concurrency 控制。注意这证明的是成本核算/配额策略，并不等于现金结算或支付系统。
- 官方 Compose 默认一容器、loopback host 映射、named data volume，需固定 OAuth callback 端口；README 指明这些固定端口意味着每台主机只能运行一个默认 Compose instance。也支持 native binary 和 Windows service。[docker-compose.yml](https://github.com/tbphp/gpt-load/blob/5c6ec0f257b3983e507fb88e1ac1d1841365a71d/docker-compose.yml)、[README](https://github.com/tbphp/gpt-load/blob/5c6ec0f257b3983e507fb88e1ac1d1841365a71d/README.md)。外部 SQL 支持为持久配置/数据提供共享 DB 选项，但不应由此直接推断完整多副本运行时一致性：源码将 affinity cache、Responses bindings、scheduler progress 实例化在进程内（如 `NewHandler`）；所读代码未见共享 Redis/分布式租约或跨副本状态协调。**推断**：多副本下需特别验证亲和性、实时凭据健康/冷却、session 等运行状态；README 未承诺所有状态的 HA 语义。
- [LICENSE](https://github.com/tbphp/gpt-load/blob/5c6ec0f257b3983e507fb88e1ac1d1841365a71d/LICENSE) 为 MIT；README 列出第三方 Bifrost Core Apache-2.0、CLIProxyAPI MIT，并提供 [`THIRD_PARTY_NOTICES.md`](https://github.com/tbphp/gpt-load/blob/5c6ec0f257b3983e507fb88e1ac1d1841365a71d/THIRD_PARTY_NOTICES.md) 与各 license 文件。商业/品牌服务渠道的推广或赞助展示不等于项目本身收费授权。

## 适合的阅读入口与未验证项

- 想读 TypeScript 多协议/Responses agent 语义：Floway 的 `packages/gateway/src/data-plane/routes.ts`、`packages/gateway/src/execution/handler.ts`、`packages/gateway/src/data-plane/chat/` 和 provider/translate workspace。
- 想读 Go 凭据选择/失败流：GPT-Load `internal/scheduler/scheduler.go` → `internal/gateway/handler.go` 的 `Handle`/`executeAttempts`；协议/上游适配可再看 README 指向的 Bifrost、CLIProxyAPI。
- 未做运行时验证：实际 upstream API 兼容性、压测/吞吐和延迟、公有部署行为、跨多实例的所有状态同步语义、长期使用平台订阅账号的条款合规性。不能据本报告给性能排名。两项目都自称 MIT，但第三方依赖另有许可义务；上线前应按具体分发方式复核全部依赖与上游 provider 的服务条款。
