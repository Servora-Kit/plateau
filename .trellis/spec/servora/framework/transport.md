# Transport 装配

`transport` 只提供协议、连接、endpoint 与 middleware 装配；service handler 和领域规则属于业务服务。目录与边界由 [`transport/AGENTS.md`](../../../../../servora/transport/AGENTS.md) 规定。

- client 通过 `endpoint.IndexByProtocol` 建立 service/protocol 配置索引；[`client/endpoint/endpoint.go`](../../../../../servora/transport/client/endpoint/endpoint.go) 负责规范化 protocol 并拒绝空 service 或重复配置。gRPC/HTTP dialer 复用此索引，不能各自重复扫描 `Data.Client.Services`。
- client middleware 的默认链是 recovery、可选 tracing、logging、默认 circuitbreaker、可选 metrics，来源为 [`client/middleware/chain.go`](../../../../../servora/transport/client/middleware/chain.go)。
- server 顶层 `Server` 只聚合 `Start`、`Stop` 与 `Endpoint`。HTTP/gRPC 从 `corev1.Server_*` 读取 listen、timeout、TLS 和 `advertise`；`advertise` 是对外注册地址覆盖，与选择注册中心后端的 `Registry` 配置分开。注册地址解析失败应在启动期失败，依据 [`HTTP server`](../../../../../servora/transport/server/http/server.go) 与 [`gRPC server`](../../../../../servora/transport/server/grpc/server.go)。
- server middleware 固定为 recovery、可选 tracing、logging、默认 ratelimit、proto validate、可选 metrics；调用方只能在该链后追加额外 middleware。operation whitelist 不是 IP/network allowlist。
- HTTP server 通过 blank import 注册 Kratos `json` 与 `protojson` codec；[`server_test.go`](../../../../../servora/transport/server/http/server_test.go) 验证 Proto message 经 `json` 保持 ProtoJSON（`int64` 为字符串），普通非 Proto payload 仍使用标准 JSON，`protojson` codec 也保持可用。不要在业务 handler 另行替换全局 codec。

TLS 统一调用 [TLS 构造](tls.md)，不要在 HTTP、gRPC、tracing 中重复读取 PEM。改动 client/server chain、endpoint 或协议装配时运行 `go test ./transport/client/...` 或 `go test ./transport/server/...`，并检查关联的 Bootstrap Proto/config 及 audit middleware 挂载位置。
