# Plateau 统一 Security Wire Error 边界

> Status: accepted

Plateau 不为 AuthN/AuthZ 或 JWT、mTLS、OIDC、OpenFGA、Casbin 等具体实现分别暴露 provider-specific errors proto。具体实现保留自己的 Go 错误语义，middleware 统一包装为 `plateau/security/errors/v1/errors.proto` 定义的最小标准类别，再映射到 HTTP/gRPC 状态；这样客户端依赖安全语义而不是当前 provider。

## Considered Options

- **每个实现维护 errors proto**：拒绝。会把 JWT/OpenFGA/Casbin 细节泄露到 wire contract，并迫使客户端按 provider 分支。
- **AuthN 与 AuthZ 各自维护一套 errors proto**：拒绝。两套枚举重复表达标准状态，且旧 runtime 合同已经删除。
- **只返回普通 Go error**：拒绝。跨 HTTP/gRPC 传输仍需要稳定的客户端状态映射。

## Consequences

统一 proto 只保留 `UNAUTHENTICATED`、`PERMISSION_DENIED`、`INVALID_ARGUMENT`、`UNAVAILABLE`、`INTERNAL` 五类标准错误。组件内部可以拥有更细的 `errors.go`，但不得把这些细节提升为跨服务协议；未来需要机器可读补充信息时，应扩展通用 security error detail，而不是新建 provider 错误枚举。

