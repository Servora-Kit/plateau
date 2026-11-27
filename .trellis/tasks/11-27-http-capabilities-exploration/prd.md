# HTTP 现有能力与拓展探索

## 目标

系统盘点 Plateau（基于 Kratos v3 + Servora）当前的 HTTP 协议层与前后端传输能力，将围绕客户端形态适配、原生路由扩展、文件传输、多事件 SSE 与 JSON Codec 机制的全部技术讨论固定为可检索、可持续追加的专题文档，作为后续能力演进与架构选型的参考依据。

## 文档产物与主题索引

核心技术沉淀位于 [`research/http-capabilities-and-extensions.md`](research/http-capabilities-and-extensions.md)，覆盖 6 个已核查主题：

1. **Kratos HTTP 路由机制与原生 Handler 扩展**（§一）：`google.api.http` 生成的 5 步胶水、Filter/Middleware 请求流向图、`srv.Handle` vs `srv.Route` 源码级对比、`ctx.Middleware` fail-closed 规则坑点、Proto 生成产物复用矩阵。
2. **Kratos HTTP 序列化机制：`protojson` 与 `json` Codec**（§二）：Kratos v3 三种 JSON Codec 包差异、Unary 与 Stream 选 Codec 规则、Servora/Plateau 实际装配与 `kratos/v3/config` 注册竞争风险、`encoding/json` vs `protojson` 7 维度语义对比。
3. **Proto 契约优先 vs Gin / Echo 传统 HTTP 开发**（§三）：7 维度工程对比与混合模式选型判断。
4. **客户端形态适配：小程序 vs 普通 H5**（§四）：Plateau 面向小程序的 4 项可复用能力与 3 个改造硬点（Bearer JWT 认证、`ClientTransport` 适配器、`PATCH` 限制）、小程序与 H5 的 6 维度差异对比及选型判断。
5. **文件上传下载、二进制流能力与拓展方案**（§五）：`protoc-gen-go-http/v3` 与 `protoc-gen-typescript-http` 现状与缺口、不 fork 生成器的 3 点理由、中小文件流式 `srv.Route` 与大文件 S3 预签名双路线、前端薄封装建议，以及 `tx7do/go-wind-toolkit` / `go-wind-admin` 一手源码对照。
6. **Server-Sent Events（SSE）与多事件流能力及拓展方案**（§六）：Plateau Proto → Go SSE → TS `createServerStream` 全链路现状与限制、多事件 SSE 三种落地方式对比、`detachStreamContext` 断连心跳坑点。

## 范围与约束

- **文档探索型任务**：仅沉淀调研结论、源码锚点、对比分析与拓展方案，不改动产品代码、不修改生成工具链、不启动 `task.py start` 实施流程、不主动提交 Git。
- **一手证据驱动**：所有能力盘点均以当前仓库与依赖模块源码为据，并保留核查版本快照与文件路径/行号锚点；外部项目对照明确区分工具链声明与应用实际落地状态。
- **可持续追加**：后续围绕 HTTP 拓展（如小程序技术栈细化、WebSocket 双向流、文件传输 SDK 封装、POST SSE 等）的新讨论直接追加至 `research/` 文档，并在必要时拆分为独立实现任务。

## 后续可探索议题

详见 [`research/http-capabilities-and-extensions.md`](research/http-capabilities-and-extensions.md) §7.2，包括：

- 微信原生 / Taro / UniApp 适配 `@plateau/api` 与 `ClientTransport` 的细节差异；
- 独立 C 端场景按需裁剪 OpenFGA / Kafka / ClickHouse 的可行性；
- `PATCH` 兼容与 JWT 刷新机制放在网关层还是 BFF 层的权衡；
- `@servora/plateau-utils/client` 补充 `uploadMultipart` / `downloadBlob` 与 `POST`/自定义 `event:` SSE 能力；
- Kratos v3 `detachStreamContext` 断连心跳与 Traefik 长连接超时配合；
- `createDuplexStream`（WebSocket 双向流）与 Kratos v3 `client/bidi streaming` 的联调盘点。

## 验收标准

- [x] 已创建文档类型 Trellis 任务目录 `.trellis/tasks/11-27-http-capabilities-exploration/`，元数据标记 `scope: "docs"` 与 `meta.type: "docs"`，保持 `planning` 状态。
- [x] 本次对话涉及的 6 个主题（小程序与 Proto vs Gin/Echo、原生路由与 Proto 复用、小程序 vs H5、`srv.Handle` vs `srv.Route` 与生成器能力核查、多文件/大文件与 `go-wind-admin` 对照、多事件 SSE 与 `protojson`/`json` Codec）全部无损固定至 `research/http-capabilities-and-extensions.md`。
- [x] 所有仓内源码锚点已按当前目录结构核对准确，重要工程结论与坑点有清晰索引，且未改动任何业务代码或既有未提交文件。
