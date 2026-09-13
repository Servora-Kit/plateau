# AGENTS.md - api/

<!-- Parent: ../AGENTS.md -->
<!-- Generated: 2026-03-15 | Updated: 2026-07-30 -->

## 目录职责

完整开发约定见 [api/proto](../.trellis/spec/api/proto/index.md)：[契约](../.trellis/spec/api/proto/contracts.md)、[注解](../.trellis/spec/api/proto/annotations.md) 与 [生成物归属](../.trellis/spec/api/proto/generation.md)。

- `protos/`：源 Proto 定义，领域命名空间 `plateau/<domain>/<version>`
- 仓库级 Go 生成产物位于 `api/gen/go/`；全部 Buf workspace 模块的共享 TypeScript HTTP client、error reason 与 CRUD helper 位于 `api/gen/ts/`
  - `api/gen/package.json` 以 wildcard ESM exports 直接暴露 `ts/` 中的 service index 与 sidecar；当前 `api/gen/tsconfig.json` 使用 `noEmit: true`，不构建 `dist/`。依赖安装与锁文件由仓库根 pnpm workspace 统一管理
- Go 生成模块：`api/gen/go.mod`，模块路径为 `github.com/Servora-Kit/plateau/api/gen`
- Example 服务的 TS 模板独立生成 `app/example/web/src/api/generated`；该前端当前消费这份输出，根 `just gen` 不刷新它，需额外运行 `just service::example::api-ts`

## Proto 约定

- gRPC 与 HTTP 共用同一个领域 Proto `service`：HTTP annotation 与 RPC 写在同一领域 Proto 内。
- 安全领域 Proto 位于 `protos/plateau/security/**`。
