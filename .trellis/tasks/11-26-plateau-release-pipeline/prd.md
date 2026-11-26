# 阶段四：Plateau 单一 BSR、Go 包与 NPM 自动化发布流水线

> 父任务：[11-25-servora-kit-engineering](../11-25-servora-kit-engineering/prd.md)（R4.1、R4.2）

## 目标与价值

将 `plateau` 内的平台共享契约与三大核心服务（`iam`、`admin`、`audit`）全部 `.proto` 集中至根 `api/protos/` 并声明单一 BSR 模块 `buf.build/servora/plateau`，补齐 `CGO_ENABLED=0` 构建门禁及 BSR、Go Release、`@servora/plateau-utils` NPM OIDC 三条自动化发布流水线，配合一次性交互式引导完成凭据配置与 `v0.1.0` 实发验证，并同步双仓文档与 `.trellis/spec/` 规范，使下游独立微服务仅依赖 `github.com/Servora-Kit/plateau`、`buf.build/servora/plateau` 与 `@servora/plateau-utils` 即可在 `CGO_ENABLED=0` 下构建并调用平台契约。

## 已核实的背景与约束事实

1. **Proto 目录与生成链路**：
   - [buf.yaml](../../../buf.yaml#L3-L8) 当前拆分为 4 个未命名模块：`api/protos`（17 个公共 `.proto`）、`app/iam/service/api/protos`（6 个）、`app/admin/service/api/protos`（4 个）、`app/audit/service/api/protos`（2 个），共 29 个 `.proto`。
   - [buf.go.gen.yaml](../../../buf.go.gen.yaml) 与 [buf.typescript.gen.yaml](../../../buf.typescript.gen.yaml) 均按模块相对路径（`paths=source_relative`）输出至 `api/gen/{go,ts}/**`；[app/iam/service/api/buf.openapi.gen.yaml](../../../app/iam/service/api/buf.openapi.gen.yaml#L13-L20) 与 [app/admin/service/api/buf.openapi.gen.yaml](../../../app/admin/service/api/buf.openapi.gen.yaml#L12-L18) 以仓库根 `directory: .` 解析 `buf.lock` 并按 `paths` 过滤服务目录，`app/audit/service/api/` 无 OpenAPI 模板。
2. **CGO 隔离现状**：
   - `github.com/mattn/go-sqlite3`（[go.mod](../../../go.mod#L44)）仅在 6 个 `*_test.go` 文件中被导入，生产包无 CGO 导入；[.github/workflows/ci.yml](../../../.github/workflows/ci.yml#L133-L135) 的 `go` 任务目前仅运行默认 `go build ./...`，尚未显式锁定 `CGO_ENABLED=0` 防回归。
3. **发布工作流与前端包现状**：
   - `.github/workflows/` 仅有 `ci.yml` 与 `security.yml`；`plateau` 的根 pnpm 工作区位于仓库根 [pnpm-workspace.yaml](../../../pnpm-workspace.yaml)（不同于 `../servora/web/`）。
   - [web/packages/plateau-utils/package.json](../../../web/packages/plateau-utils/package.json#L1-L37) 版本为 `0.1.0`，运行期依赖均为标准 semver（无 `workspace:*`），`prepublishOnly` 当前调用 `pnpm run clean && pnpm run build`。
4. **外部凭据与首发前置条件（用户已确认纳入实发验证）**：
   - BSR 模块 `buf.build/servora/plateau` 已存在（`2026-11-25` 创建），但 GitHub 仓库 `Servora-Kit/plateau` 尚未设置 `BUF_TOKEN` Secret。
   - `@servora/plateau-utils` 在 `registry.npmjs.org` 尚不存在（404）；npmjs OIDC Trusted Publishing 要求包名已存在后方可绑定 GitHub Actions 工作流，需先由本地一次性手动发布 `0.1.0-alpha.0` 建包并在 npmjs 后台绑定 `publish-plateau-utils.yml`，再通过 tag `plateau-utils/v0.1.0` 验证纯 OIDC `--provenance` 正式发包。
   - `plateau` 当前无任何 Git tag；`../servora` 本地 `main` 有 1 个待推送文档提交 `c12fd4fb`。

## 需求清单

- **R1（全集中式 `api/protos/` 与单一 BSR 模块）**：将 `app/{iam,admin,audit}/service/api/protos/**` 全部迁入根 `api/protos/{iam,admin,audit}/**`，删除已空的 `app/{iam,admin}/service/api/protos/` 与 `app/audit/service/api/`；在 [buf.yaml](../../../buf.yaml) 将 `modules` 合并为单一条目 `path: api/protos` + `name: buf.build/servora/plateau`；更新 IAM 与 Admin 的 `buf.openapi.gen.yaml` 过滤路径至 `api/protos/{iam,admin}/...`，确保 `GOWORK=off just gen-check` 零产物漂移。
- **R2（`CGO_ENABLED=0` 门禁与三条自动化发布工作流）**：
  - 在 [.github/workflows/ci.yml](../../../.github/workflows/ci.yml) 的 `go` 任务中将构建步骤改为 `CGO_ENABLED=0 go build ./...`；
  - 新增 `.github/workflows/buf-ci.yml`：对齐 `../servora`，在 PR、`main` push 及 `v*` tag 上执行 `bufbuild/buf-action@v1`（`push_disable_create: true`，使用 `secrets.BUF_TOKEN`）；
  - 新增 `.github/workflows/release.yml`：响应 `v*` tag，校验提交属于 `origin/main` 且该 SHA 的 `ci.yml` push 工作流成功后再生成变更说明并创建 GitHub Release；
  - 新增 `.github/workflows/publish-plateau-utils.yml` 并统一 [package.json](../../../web/packages/plateau-utils/package.json#L34) `prepublishOnly` 为 `npm run clean && npm run build`：响应 `plateau-utils/v*` tag，校验提交位于 `origin/main` 且 tag 版本号与 `package.json` 一致，安装根工作区依赖并执行 `typecheck`、`test` 后通过 OIDC `npm publish --provenance --access public` 发布。
- **R3（双仓规范与文档同步）**：同步更新 [AGENTS.md](../../../AGENTS.md)、[api/AGENTS.md](../../../api/AGENTS.md)、[app/AGENTS.md](../../../app/AGENTS.md)、[README.md](../../../README.md) 及 `.trellis/spec/` 下涉及 Proto 目录、服务布局与对外包消费说明的文档（`api/proto/{contracts,generation}.md`、`service/backend/layout.md`、`plateau/project/{structure,development}.md`、`web/plateau-utils/index.md`），保持单处权威描述无旧路径残留。
- **R4（外部凭据向导与首次实发验证）**：提供临时交互式引导脚本（不入库，验证后清理）协助配置 `Servora-Kit/plateau` 的 `BUF_TOKEN`、完成 `@servora/plateau-utils@0.1.0-alpha.0` 本地建包与 npmjs Trusted Publisher 绑定；推送 `../servora` 与 `plateau` 的 `main` 分支，待远端 `CI` 与 `Buf CI` 全绿后依次推送 `plateau-utils/v0.1.0` 与 `v0.1.0` tag，并在隔离临时目录验证外部最小消费模块在 `CGO_ENABLED=0` 下拉取 `github.com/Servora-Kit/plateau@v0.1.0`、`buf.build/servora/plateau` 与 `@servora/plateau-utils@0.1.0` 构建通过。

## 验收标准

- [x] **AC-1（对应 R1）**：`app/*/service/api/protos` 目录已不存在，29 个 `.proto` 全部位于 `api/protos/{plateau,iam,admin,audit}/**`；`buf.yaml` 仅含单一模块 `buf.build/servora/plateau`；`GOWORK=off` 下 `just lint-proto`、`just fmt-proto-check`、`just gen-check` 与 `just api-ts-check` 全部通过且 `api/gen/{go,ts}`、`openapi.yaml` 无非预期变更。
- [x] **AC-2（对应 R2）**：`CGO_ENABLED=0 GOWORK=off go build ./...` 本地与 CI 均通过；`.github/workflows/{buf-ci,release,publish-plateau-utils}.yml` 通过 YAML/prek 校验，`pnpm --filter @servora/plateau-utils test`、`typecheck` 与 `pack --dry-run` 通过。
- [x] **AC-3（对应 R3）**：`AGENTS.md`、`api/AGENTS.md`、`app/AGENTS.md`、`README.md` 与 `.trellis/spec/` 中不再将 `app/*/service/api/protos` 描述为现行目录；`GOWORK=off` 下 `plateau` 与 `../servora` 全量本地门禁（Go 单测、lint、Web 构建与单测、`govulncheck`）通过。
- [ ] **AC-4（对应 R4）**：`../servora` 与 `plateau` 的 `main` 远端 `CI` / `Buf CI` 绿灯；`buf.build/servora/plateau`、`@servora/plateau-utils@0.1.0`（含 OIDC provenance）与 `github.com/Servora-Kit/plateau` `v0.1.0` Release 均实发成功；隔离最小外部项目在无 `go.work` 且 `CGO_ENABLED=0` 条件下成功引用三者并通过 `buf generate` 与 `go build`；临时引导脚本与测试目录已清理。

## 范围外事项

- **平台脚手架与 GitHub Template**：新服务生成 CLI（`new service` / `new api`）与垂类衍生模板仓不在本任务内开发。
- **平台服务业务逻辑与容器镜像发布**：不修改 `iam`、`admin`、`audit` 的任何业务功能、数据库结构或 OpenFGA 模型；不新增 Docker 镜像（GHCR）或 Helm 发布流水线。
