# TLS 配置构造

`security/tls` 是 server/client TLS 配置的唯一构造边界。transport、tracing 和 provider 只消费它返回的 `*crypto/tls.Config`，不得重复读取 PEM、组装证书池或分叉校验规则。公共 schema 是 `servora.security.tls.v1.TLS`，实现和完整 API 在 [`security/tls/tls.go`](../../../../../servora/security/tls/tls.go)、[`builder.go`](../../../../../servora/security/tls/builder.go)。

`NewServerConfig` 要求 cert/key 成对；`NewClientConfig` 对 mTLS 同样要求成对，CA 非空时加载 RootCAs。默认最低版本 TLS 1.2，提升版本要显式指定，不能降低默认值。`BuildServerTLS`、`BuildClientTLS` 在 nil 或 `enable=false` 时返回 `(nil, nil)`；启动期才可使用会 panic 的 `MustServerConfig` 或 `MustBuildServerTLS`。

本包接收 Proto 或 options，不读取业务配置文件，也不承担动态证书轮转、secret manager、ACME 或发现。改动时运行 `go test ./security/tls`，覆盖缺失文件、mTLS 配对、CA/PEM 和默认值。
