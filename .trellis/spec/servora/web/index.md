# Servora web

适用于 `../servora/web/packages/proto-utils`。当前框架共享前端能力只有 `@servora/proto-utils`；它提供跨业务仓库稳定复用的 Proto/CRUD/error 合同，不是业务 HTTP client、认证或 UI 层。

开发前读取 [proto-utils](proto-utils.md)，确认新 API 至少具有跨业务复用价值。运行 `just web-typecheck`、`just web-build`；行为变更同时运行该 package 的既有 Node 测试。生成 TypeScript 改动另读 [Proto generation](../proto/generation.md)。
