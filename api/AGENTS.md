# AGENTS.md - api/

<!-- Parent: ../AGENTS.md -->
<!-- Generated: 2026-03-15 | Updated: 2026-11-27 -->

## 目录职责

完整开发约定见 [api/proto](../.trellis/spec/api/proto/index.md)：[契约](../.trellis/spec/api/proto/contracts.md)、[注解](../.trellis/spec/api/proto/annotations.md) 与 [生成物归属](../.trellis/spec/api/proto/generation.md)。

- `protos/`：全集中式源 Proto 定义（单一 BSR 模块 `buf.build/servora/plateau`），全部 29 个公开 `.proto` 统一位于 `protos/plateau/{security,crud,infra,iam,admin,audit}/.../v1`（包名 `plateau.*.*.v1`，含平台共享契约及核心服务接口与私有配置）
- 仓库级 Go 生成产物位于 `api/gen/go/`；共享 TypeScript HTTP client、error reason 与 CRUD helper 位于 `api/gen/ts/`
  - `api/gen/package.json` 以 wildcard ESM exports 直接暴露 `ts/` 中的 service index 与 sidecar；当前 `api/gen/tsconfig.json` 使用 `noEmit: true`，不构建 `dist/`。依赖安装与锁文件由仓库根 pnpm workspace 统一管理
- Go 生成 package 位于根 module 的 `api/gen/go/`，import path 继续以 `github.com/Servora-Kit/plateau/api/gen/go` 开头

## Proto 约定

- gRPC 与 HTTP 共用同一个领域 Proto `service`：HTTP annotation 与 RPC 写在同一领域 Proto 内。
- 平台共享与核心服务的接口及私有配置 Proto 均位于 `protos/plateau/{security,crud,infra,iam,admin,audit}/.../v1`；OpenAPI 由仓库根级唯一 `buf.openapi.gen.yaml` 按服务路径筛选生成，服务目录不保留 `api/` 子目录。
