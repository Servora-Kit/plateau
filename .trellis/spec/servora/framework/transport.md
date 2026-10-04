# Transport 装配

`transport` 只提供协议、连接、endpoint 与 middleware 装配；service handler 和领域规则属于业务服务。目录与边界由 [`transport/AGENTS.md`](../../../../../servora/transport/AGENTS.md) 规定。

- client 通过 `endpoint.IndexByProtocol` 规范化 protocol 并拒绝空 service 或重复配置；gRPC/HTTP dialer 复用索引，不各自重复扫描 `Data.Client.Services`。
- client 默认 middleware 链为 recovery、可选 tracing、logging、默认 circuitbreaker、可选 metrics。
- server 顶层 `Server` 只聚合 `Start`、`Stop` 与 `Endpoint`。HTTP/gRPC 从 `corev1.Server_*` 读取 listen、timeout、TLS 和 `advertise`；后者覆盖对外注册地址，与选择注册中心后端的 `Registry` 分开。注册地址解析失败须在启动期失败。
- server middleware 固定为 recovery、可选 tracing、logging、默认 ratelimit、proto validate、可选 metrics；调用方只能在该链后追加额外 middleware。operation whitelist 不是 IP/network allowlist。
- HTTP server 通过 blank import 注册 Kratos `json` 与 `protojson` codec。Proto message 经 `json` 保持 ProtoJSON（`int64` 为字符串），非 Proto payload 使用标准 JSON；`protojson` codec 也保持可用。应用 handler 不替换全局 codec。

TLS 统一调用 [TLS 构造](tls.md)，不要在 HTTP、gRPC、tracing 中重复读取 PEM。改动 client/server chain、endpoint 或协议装配时运行 `go test ./transport/client/...` 或 `go test ./transport/server/...`，并检查关联的 Bootstrap Proto/config 及 audit middleware 挂载位置。
