# Implement：Plateau 单一 BSR、Go 包与 NPM 自动化发布流水线

需求与技术方案见 [prd.md](prd.md) 与 [design.md](design.md)。

---

## 1. 有序执行清单

### 步骤一：全集中式 `api/protos/` 与单一 BSR 模块改造（R1）
- [x] 将 `app/iam/service/api/protos/iam/**`（6 个文件）、`app/admin/service/api/protos/admin/**`（4 个文件）、`app/audit/service/api/protos/audit/**`（2 个文件）移动至 `api/protos/{iam,admin,audit}/**`。
- [x] 删除已空的 `app/iam/service/api/protos/`、`app/admin/service/api/protos/` 与整个 `app/audit/service/api/` 目录。
- [x] 修改 `buf.yaml`：将 `modules` 合并为单一条目 `path: api/protos` + `name: buf.build/servora/plateau`，执行 `buf dep update`。
- [x] 修改 `app/iam/service/api/buf.openapi.gen.yaml` 与 `app/admin/service/api/buf.openapi.gen.yaml`：保留 `directory: .`，将 `paths` 更新为 `api/protos/iam/...` 与 `api/protos/admin/...`。

### 步骤二：CI `CGO_ENABLED=0` 门禁与三条发布工作流落地（R2）
- [x] 修改 `.github/workflows/ci.yml`：在 `go` 任务的 `Build` 步骤加入 `env: { CGO_ENABLED: "0" }`。
- [x] 修改 `web/packages/plateau-utils/package.json`：将 `prepublishOnly` 更新为 `npm run clean && npm run build`。
- [x] 新增 `.github/workflows/buf-ci.yml`：配置 `pull_request`（按 `.proto`/`buf.yaml`/`buf.lock` 过滤）与 `push`（`branches: [main]`、`tags: ["v*"]`）执行 `bufbuild/buf-action@v1`（`token: ${{ secrets.BUF_TOKEN }}`、`push_disable_create: true`）。
- [x] 新增 `.github/workflows/release.yml`：响应 `v*` tag，校验 `origin/main` 祖先关系与 `ci.yml` 成功记录后通过 `softprops/action-gh-release@v3` 发布 GitHub Release。
- [x] 新增 `.github/workflows/publish-plateau-utils.yml`：响应 `plateau-utils/v*` tag，声明 `id-token: write`，校验 `origin/main` 祖先关系及 tag 与 `package.json` 版本一致，在根工作区安装依赖、在 `web/packages/plateau-utils` 执行 `typecheck` 与 `test`，最后执行 `npm publish --provenance --access public --registry=https://registry.npmjs.org`。

### 步骤三：双仓文档、规范同步与本地全量门禁验收（R3）
- [x] 更新 `AGENTS.md`、`api/AGENTS.md`、`app/AGENTS.md`、`README.md`，修正集中式 `api/protos/` 目录结构说明，并补充 BSR（`buf.build/servora/plateau`）、Go 模块与 `@servora/plateau-utils` 的版本发布与外部引用说明。
- [x] 同步更新 `.trellis/spec/` 相关规范：
  - `.trellis/spec/api/proto/contracts.md`、`.trellis/spec/api/proto/generation.md`
  - `.trellis/spec/service/backend/layout.md`
  - `.trellis/spec/plateau/project/structure.md`
  - `.trellis/spec/web/plateau-utils/index.md`（补充发布校验说明）
- [x] 运行本地全量门禁（见 §2 验证命令），运行 GitNexus 图变更检测，通过后使用 `commit` 命名空间分批提交本地变更。
### 步骤四：交互式凭据引导、远端推送与 `v0.1.0` 实发验收（R4）
- [ ] 基于 `wizard` 模板在会话 scratch 目录生成临时引导脚本，交由用户执行完成两项外部操作：
  1. 配置 `Servora-Kit/plateau` 仓库 Secret `BUF_TOKEN`；
  2. 本地临时以 `0.1.0-alpha.0` 执行 `npm publish --access public --tag alpha` 初始化 `@servora/plateau-utils` 包名（立即还原 `package.json` 为 `0.1.0`），并在 npmjs 包设置页绑定 GitHub Actions Trusted Publisher（`Servora-Kit/plateau` + `publish-plateau-utils.yml`）。
- [ ] 推送 `../servora` `main`（含 `c12fd4fb`）并确认远端 CI 绿灯；推送 `plateau` `main`（首次模块合并推送使用 `SKIP=buf-breaking`，推送后立即运行 `just breaking` 确认对新 `origin/main` 零破坏性差异），等待远端 `CI` 与 `Buf CI` 全部成功。
- [ ] 推送 `plateau-utils/v0.1.0` tag 并等待 `Publish @servora/plateau-utils` 成功；推送 `v0.1.0` tag 并等待 `Release` 与 `Buf CI` 成功。
- [ ] 在临时隔离目录（无 `go.work`、`CGO_ENABLED=0`）执行外部最小消费者构建验证（`buf build` + `go get github.com/Servora-Kit/plateau@v0.1.0` + `npm view @servora/plateau-utils@0.1.0`），通过后删除临时目录与 scratch 向导脚本，勾选父任务 `11-25-servora-kit-engineering` 全部剩余验收项。

---

## 2. 验证命令

```bash
# 1. Proto 格式、Lint 与生成零漂移校验（连续校验）
GOWORK=off just lint-proto
GOWORK=off just fmt-proto-check
GOWORK=off just gen-check
GOWORK=off just api-ts-check

# 2. Go 零 CGO 编译、静态检查、竞态单测与漏洞扫描
CGO_ENABLED=0 GOWORK=off go build ./...
GOWORK=off just lint-go
GOWORK=off just test
GOWORK=off go run golang.org/x/vuln/cmd/govulncheck@latest ./...

# 3. 前端工具包与全栈 Web 校验
pnpm --filter @servora/plateau-utils typecheck
pnpm --filter @servora/plateau-utils test
pnpm --filter @servora/plateau-utils pack --dry-run
just web::lint
just web::test
just web::build

# 4. 本地钩子与图变更分析
prek run --all-files --stage pre-commit --skip app/admin/web/
node .gitnexus/run.cjs analyze --index-only
mcp__gitnexus_detect_changes -j '{"scope":"all","repo":"."}'
```

---

## 3. 高关注文件与回滚检查点

- **高关注文件**：
  - `buf.yaml`、`buf.lock`、`app/{iam,admin}/service/api/buf.openapi.gen.yaml`
  - `.github/workflows/{ci,buf-ci,release,publish-plateau-utils}.yml`
  - `web/packages/plateau-utils/package.json`
  - `AGENTS.md`、`api/AGENTS.md`、`app/AGENTS.md`、`README.md`
- **回滚检查点**：
  1. 步骤一完成后必须立即通过 `GOWORK=off just gen-check`，若 `api/gen/{go,ts}` 或 `openapi.yaml` 出现任何 diff，说明模块相对路径或 OpenAPI `paths` 有误，修正前不得进入步骤二；
  2. 步骤四推送 `main` 前必须确认 `gh secret list -R Servora-Kit/plateau` 已包含 `BUF_TOKEN`；推送 `plateau-utils/v0.1.0` 前必须确认 `main` CI 已绿且 npmjs Trusted Publisher 已保存绑定。
