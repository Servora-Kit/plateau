# Higress 与 CLIProxyAPI 一手资料记录

调研日期：2026-10-05。只读审阅官方仓库与文档，未部署、压测或接入付费模型。以下版本为读取期间主分支状态，不代表某个稳定发行版。对架构的归纳不构成性能保证。

## Higress

主仓读取期间 HEAD：`bda81f1067e1285775e650644a20a635f51b0a6d`，提交时间 2026-10-04；[提交](https://github.com/higress-group/higress/commit/bda81f1067e1285775e650644a20a635f51b0a6d)。

### 定位与功能

云原生 API 网关，AI 转发是能力集合的一部分。官方同时覆盖 Kubernetes Ingress/Gateway API、服务发现、安全认证、LLM API、MCP 托管、限流、缓存、可观测性。支持 Docker all-in-one，也支持 Kubernetes/Helm；因此不应描述为只能在 K8s 部署。

来源：[README](https://github.com/higress-group/higress/blob/main/README.md)。README 中吞吐与采用规模是官方宣称，本研究未验证、不用于六者排名。

### 实际技术栈

- 控制面 Go：主模块 `github.com/alibaba/higress/v2`，`go 1.26.0`，依赖 Istio、Envoy go-control-plane、Kubernetes client-go/controller-runtime、gRPC。[go.mod](https://github.com/higress-group/higress/blob/main/go.mod)
- 数据面是 Envoy（C++ 生态），而非普通 Gin HTTP 应用；仓库通过子模块维护 Envoy/Istio 组件。[.gitmodules](https://github.com/higress-group/higress/blob/main/.gitmodules)、[官方组件说明](https://higress.cn/en/docs/latest/dev/architecture/)
- AI 功能通过插件扩展；`ai-proxy/main.go` 注册请求头、请求体、响应头、流式响应体等 Proxy-Wasm 回调。[插件入口](https://github.com/higress-group/higress/blob/main/plugins/wasm-go/extensions/ai-proxy/main.go)
- 控制台是独立仓库；后端 Java/Spring Boot，父 POM 当前声明 Spring Boot 2.7.18；前端 React 18、TypeScript、Ant Design 4、Ant Design Pro、ice 3。[控制台 README](https://github.com/higress-group/higress-console)、[POM](https://github.com/higress-group/higress-console/blob/main/backend/pom.xml)、[package.json](https://github.com/higress-group/higress-console/blob/main/frontend/package.json)
- 不应将控制台前端构建使用 Node 描述为 Node 业务后端。也不应因为主仓 Go 标签而忽略 Java/C++。

### 架构与调用模型

控制台/配置资源负责声明路由、提供商、消费者与插件配置；控制面经 Istio/xDS 体系配置数据面；模型请求由 Envoy 的过滤器/插件链处理并转发。管理操作与模型请求处理不在同一条业务调用链。Kubernetes 模式以其资源模型管理配置；本研究未逐项核实独立模式的所有持久化后端，因此不将它列为统一 SQL 计费数据库架构。

`ai-proxy` 文档与入口源码可见：OpenAI、Claude、Gemini 等路径识别；协议转换、模型映射、供应商适配、API token 列表、健康检测/冷却、失败重试。需要注意：文档对 `retryOnFailure` 明确限定“目前仅支持非流式请求”，不能把网关故障切换泛化为已经输出的 SSE 可透明重放。

当前插件还提供 `claudeCodeMode`，可使用 Claude Code OAuth Token。因此“只有 CLIProxyAPI/sub2api 支持 OAuth，Higress 完全不支持”并不准确；产品重心仍不同：网关侧提供认证适配，并非天然拥有完整的订阅账号运营与订单账务。

来源：[ai-proxy README](https://github.com/higress-group/higress/blob/main/plugins/wasm-go/extensions/ai-proxy/README.md)。

### 取舍

[INFERENCE] 适合已有平台治理、K8s、多协议入口需求的团队，学习价值在控制面/数据面分离、配置分发与插件扩展。若目标仅是几个 API key 轮换，它引入的技术体系更多。用户注册、余额充值、订单结算不是本轮看到的核心产品能力，不能从“消费者”和“token 计量”推导出完整计费平台。

主仓许可：[Apache-2.0](https://github.com/higress-group/higress/blob/main/LICENSE)；再分发时仍需逐组件检查许可与通知。

## CLIProxyAPI

主仓读取期间 HEAD：`a4acc9f752bd46571f737a10c04bf413656ab06b`，提交时间 2026-10-04；[提交](https://github.com/router-for-me/CLIProxyAPI/commit/a4acc9f752bd46571f737a10c04bf413656ab06b)。

### 定位与功能

将 CLI/订阅账号及常规 API key 接入兼容 API 的代理引擎。支持 Codex、Claude Code、Gemini/Antigravity 等账号认证、多账号选择、协议适配、流式和部分 WebSocket 路径、工具调用、模型映射；也支持常规 OpenAI-compatible 上游，不应简单归为“只支持订阅账号”。提供可嵌入 Go SDK。

来源：[README](https://github.com/router-for-me/CLIProxyAPI/blob/main/README.md)。

### 实际技术栈与状态

- 主模块当前为 `github.com/router-for-me/CLIProxyAPI/v8`，Go 1.26，Gin；使用 fsnotify、OAuth2、gjson/sjson、WebSocket，存储相关依赖包括 pgx、go-git、MinIO SDK。[go.mod](https://github.com/router-for-me/CLIProxyAPI/blob/main/go.mod)
- 本地文件配置/凭据为默认方案；可选 PostgreSQL、Git、对象存储。[.env.example](https://github.com/router-for-me/CLIProxyAPI/blob/main/.env.example)、[store 目录](https://github.com/router-for-me/CLIProxyAPI/tree/main/internal/store)
- 管理前端来自同组织独立仓库 CPAMC：React 19、TypeScript 6、Vite 8、Zustand、React Router 7、SCSS Modules/自有组件，使用 singlefile 插件输出 HTML，由后端托管；不是核心仓库没有 package.json 就没有 Web UI。[管理中心 README](https://github.com/router-for-me/Cli-Proxy-API-Management-Center)、[package.json](https://github.com/router-for-me/Cli-Proxy-API-Management-Center/blob/main/package.json)
- 核心配置支持自动获取/更新管理面板资源，UI 不参与实际模型流量转发。[config.example.yaml](https://github.com/router-for-me/CLIProxyAPI/blob/main/config.example.yaml)

### 架构

概念上分为兼容协议入口、认证与模型选择、Provider Executor、请求/响应 Translator、凭据刷新与持久化。源码中的 `ProviderExecutor` 明确区分 `Execute`、`ExecuteStream`、`Refresh`、`CountTokens`、`HttpRequest`，`Selector.Pick` 负责选择凭据，`Manager` 协调生命周期与执行。

来源：[conductor.go](https://github.com/router-for-me/CLIProxyAPI/blob/main/sdk/cliproxy/auth/conductor.go)、[SDK 高级文档](https://github.com/router-for-me/CLIProxyAPI/blob/main/docs/sdk-advanced.md)。注意：SDK 文档示例仍写 v6 路径，与当前 v8 源码不一致；借鉴概念可用，不能原样当最新接口文档。

当前配置列出 round-robin、weighted-round-robin、fill-first；可选会话亲和与子会话继承，支持 credential/model 冷却和跨凭据重试。订阅账号的限额、刷新、模型可用性是主要调度输入，复杂度不同于随机选一个静态 key。

来源：[配置示例](https://github.com/router-for-me/CLIProxyAPI/blob/main/config.example.yaml)、[auth 实现目录](https://github.com/router-for-me/CLIProxyAPI/tree/main/sdk/cliproxy/auth)。

### 集群和运营能力

- 当前有 Home 模式集群部署入口，`docker-compose.cluster.yml` 要求 HOME_JWT、使用 `-home-jwt`。不能说只能单机；本研究未部署 Home，也未验证跨实例凭据刷新/并发一致性保证。[cluster compose](https://github.com/router-for-me/CLIProxyAPI/blob/main/docker-compose.cluster.yml)
- 默认文件持久化或换成共享数据库，不能单独证明分布式调度完整成立。[INFERENCE]
- README 明确说明从 v6.10.0 起，CLIProxyAPI 与 CPAMC 不再内置用量统计，推荐外部统计项目。管理中心存在配额查询与日志页面，不能据此写成内置完整账单/余额扣费系统。[README Usage Statistics](https://github.com/router-for-me/CLIProxyAPI#usage-statistics)
- [INFERENCE] 适合统一个人/团队 CLI 模型调用、研究协议翻译与账号生命周期；不应直接拿它替代 new-api/sub2api 的用户运营与交易账务。

主仓许可：[MIT](https://github.com/router-for-me/CLIProxyAPI/blob/main/LICENSE)。代码许可不授予上游订阅账号转售权，账号共享、转售与认证代理仍需分别核查服务商条款。
