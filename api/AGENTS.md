# AGENTS.md - api/

<!-- Parent: ../AGENTS.md -->
<!-- Generated: 2026-03-15 | Updated: 2026-07-30 -->

## 目录职责

- 仓库级 Go 生成产物位于 `api/gen/go/`；全部 Buf workspace 模块的共享 TypeScript HTTP client、error reason 与 CRUD helper 位于 `api/gen/ts/`
  - `api/gen/package.json` 以 wildcard ESM exports 暴露 service index 与 sidecar，`api/gen/tsconfig.json` 编译到 `api/gen/dist/`；依赖安装与锁文件由仓库根 pnpm workspace 统一管理
- Go 生成模块：`api/gen/go.mod`，模块路径为 `github.com/Servora-Kit/plateau/api/gen`
