# Servora web

适用于 `../servora/web/packages/proto-utils`。`@servora/proto-utils` 提供跨应用稳定复用的 Proto/transport/CRUD/error 契约，不是应用 HTTP client、认证或 UI 层。

开发前读取 [proto-utils](proto-utils.md)，确认新 API 至少具有跨业务复用价值。Node 与 pnpm 版本由 [`../servora/mise.toml`](../../../../../servora/mise.toml) 统一固定（`web/package.json` 不再声明 `packageManager`）；[`../servora/web/pnpm-workspace.yaml`](../../../../../servora/web/pnpm-workspace.yaml) 显式启用 `minimumReleaseAge: 1440`（1 天发布冷却期）并由 `renovate.json5` 自动同步；[`../servora/web/biome.json`](../../../../../servora/web/biome.json) 排除 `src/gen`，避免格式化改写生成产物。

质量检查与发布：先通过 `just web-install`（`--frozen-lockfile`）对齐依赖，再依次运行 `just web-check`（Biome 检查）、`just web-typecheck`、`just web-test`（串联构建、测试类型检查与 Node 单元测试）；按需单独构建可执行 `just web-build`。CI 发包工作流（`publish-proto-utils.yml`）仅由 `jdx/mise-action` 读取 `mise.toml` 安装工具，不叠加 `actions/setup-node`（其 `node-version-file` 不支持 TOML，且 `registry-url` 注入的占位 `NODE_AUTH_TOKEN` 会干扰 npm OIDC Trusted Publishing），直接在 `id-token: write` 下执行 `npm publish --provenance --access public --registry=https://registry.npmjs.org`。生成 TypeScript 改动另读 [Proto generation](../proto/generation.md)。
