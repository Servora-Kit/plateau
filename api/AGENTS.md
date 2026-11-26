# AGENTS.md - api/

<!-- Parent: ../AGENTS.md -->
<!-- Generated: 2026-03-15 | Updated: 2026-11-26 -->

## 目录职责

完整开发约定见 [api/proto](../.trellis/spec/api/proto/index.md)：[契约](../.trellis/spec/api/proto/contracts.md)、[注解](../.trellis/spec/api/proto/annotations.md) 与 [生成物归属](../.trellis/spec/api/proto/generation.md)。

- `protos/`：全集中式源 Proto 定义（单一 BSR 模块 `buf.build/servora/plateau`），包含平台共享命名空间 `plateau/<domain>/<version>` 与核心服务契约 `iam/**`、`admin/**`、`audit/**`（接口与私有配置）
- 仓库级 Go 生成产物位于 `api/gen/go/`；共享 TypeScript HTTP client、error reason 与 CRUD helper 位于 `api/gen/ts/`
  - `api/gen/package.json` 以 wildcard ESM exports 直接暴露 `ts/` 中的 service index 与 sidecar；当前 `api/gen/tsconfig.json` 使用 `noEmit: true`，不构建 `dist/`。依赖安装与锁文件由仓库根 pnpm workspace 统一管理
- Go 生成 package 位于根 module 的 `api/gen/go/`，import path 继续以 `github.com/Servora-Kit/plateau/api/gen/go` 开头

## Proto 约定

- gRPC 与 HTTP 共用同一个领域 Proto `service`：HTTP annotation 与 RPC 写在同一领域 Proto 内。
- 安全领域公共 Proto 位于 `protos/plateau/security/**`；各平台服务的接口与私有配置 Proto 位于 `protos/{iam,admin,audit}/**`，服务目录仅按需保留 `api/buf.openapi.gen.yaml`。
