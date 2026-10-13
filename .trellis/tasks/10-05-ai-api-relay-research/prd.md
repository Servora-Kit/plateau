# 七款 AI API 中转项目技术调研

## 目标

对 Higress、Floway、gpt-load、CLIProxyAPI、new-api、sub2api、Magpie 进行一手资料技术调研，帮助 HoronLee 理解功能、前后端技术与架构差异，尤其关注 Go 的实际职责、协议转换模块的复用条件和工程学习价值。

## 范围

1. Higress：https://github.com/higress-group/higress
2. Floway：https://github.com/Menci/Floway
3. gpt-load：https://github.com/tbphp/gpt-load
4. CLIProxyAPI：https://github.com/router-for-me/CLIProxyAPI
5. new-api：https://github.com/QuantumNous/new-api
6. sub2api：https://github.com/Wei-Shaw/sub2api
7. Magpie：https://github.com/yetone/magpie

每项覆盖：产品定位、主要功能、Go 与其他语言的职责、后端框架、前端框架及 UI 库、数据存储、请求处理与扩展模式、密钥或账号调度、计量计费、多实例部署条件、许可及使用风险。

### 2026-10-13 扩展要求：Magpie

- 核实本地 agent 配置管理、桌面/终端/Web 入口与 AI 网关之间的关系，避免将桌面入口或语言标签当成完整后端架构。
- 核实 OpenAI Chat Completions、OpenAI Responses、Anthropic Messages、Gemini 的请求、非流式响应和流式转换实现；明确数据类型、跨事件状态、同协议处理及转换损耗。
- 核实公开 Go 库可以独立复用到什么程度，与 RelayKit、CLIProxyAPI SDK 的职责和许可证差异；不预设将外部协议全部重写成 Proto。
- 核实订阅账号发现、上游调用、Claude CLI/MCP 桥接、插件运行时和本地数据的安全影响。
- 保存固定提交的来源、可执行的隔离验证结果与未验证事项；原六项保留 2026-10-05 的历史快照，不冒充同日重新核验。

## 约束

- 只读调研；允许任务目录中保存报告，不修改业务代码、不部署服务、不提交 Git。
- 本轮停留在规划中的 research 阶段，不启动实现任务。
- 以官方 README、依赖清单、部署文件和关键实现为依据，重要结论附可访问来源。
- 区分文档宣称、源码证据、推断及未验证项；不将语言标签视为整个技术栈。
- 不以未经同条件实测的性能数字进行项目排名。
- 保留现有任务和用户未提交修改。
- 不读取本机真实账号、密钥或 agent 配置，不启动会自动发现凭据的 Magpie 应用；运行验证仅使用临时目录、虚构输入及回环地址，清理临时产物。

## 验收标准

- 七个项目各有功能、前后端技术、系统架构说明，Magpie 纳入所有适用的横向对比维度。
- 横向比较清晰区分网关、密钥调度、CLI/OAuth 代理、运营计费平台和本地 agent 配置管理工具。
- 明确 Higress 数据面等多语言组成以及 Floway 的 TypeScript 后端，避免将所有项目都视为纯 Go 程序。
- 提供按场景划分的选型建议及 Go 工程阅读顺序，而非单一排名。
- 保存中文调研报告及来源，交付对话版主要结论。
- Magpie 独立研究记录覆盖协议/流式、公开库、账号调度、部署/状态、许可与风险；与 RelayKit、CLIProxyAPI 的复用比较有源码依据。
- PRD、任务元数据、比较报告和来源索引一致；不启动实现任务，不将本次调研视为采用某个库的决定。
