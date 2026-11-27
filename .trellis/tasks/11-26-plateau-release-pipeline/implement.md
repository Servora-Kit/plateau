# Implement：Plateau 单一 BSR 统一命名空间、CGO 门禁与原子发布流水线

需求与技术方案见 [prd.md](prd.md) 与 [design.md](design.md)。

---

## 1. 有序执行清单

### 步骤一：清理工作区 `v1alpha1` 残留并迁移 12 个服务 `.proto` 至 `plateau.{iam,admin,audit}.*.v1`（R1、R3）
- [x] 回退此前中止的 `v1alpha1` 改动（恢复 17 个平台公共 `.proto`、`cmd/protoc-gen-plateau-*`、`crud/`、`infra/`、`transport/`、`buf.typescript.gen.yaml`、`web/packages/plateau-utils`、`/v1/` HTTP 路由与 OIDC 配置至 `HEAD`，清理所有未跟踪 `v1alpha1` 目录）。
- [x] 按 [design.md §1.2](design.md) 映射表，将 `api/protos/{iam,admin,audit}/.../v1/` 下共 12 个 `.proto` 迁至 `api/protos/plateau/{iam,admin,audit}/.../v1/`，删除裸顶层 `api/protos/{iam,admin,audit}/`。
- [x] 更新这 12 个 `.proto` 的 `package` 为 `plateau.{iam,admin,audit}.*.v1`，`option go_package` 为 `"github.com/Servora-Kit/plateau/api/gen/go/plateau/<subpath>/v1;<alias>v1"`，同步内部 `import` 与类型引用，保持 `google.api.http` `/v1/...` 路由不变。
- [x] 保留根目录唯一 `buf.openapi.gen.yaml`（`out: internal/assets`），删除 `app/{iam,admin}/service/api/` 目录，更新 `just/service.just` 为单行 `buf generate --template buf.openapi.gen.yaml --path "api/protos/plateau/{{ SERVICE_NAME }}" -o "{{ SERVICE_MODULE }}"`，运行 `just gen-clean && GOWORK=off just gen`（含生成 `app/audit/service/internal/assets/openapi.yaml`）。

### 步骤二：Go 后端与前端 `@plateau/api` 导入联动更新（R1）
- [x] 更新 `app/{iam,admin,audit}/service` 与 `security/{authn/session,oidc,audit/kafka}` 中对 `api/gen/go/plateau/{iam,admin,audit}/.../v1` 的导入路径及单测 gRPC 全限定方法名断言（`/plateau.{iam,admin,audit}.*.v1.<Service>/<Method>`），运行 `GOWORK=off just wire`。
- [x] 更新 `app/iam/web` 与 `app/admin/web` 中对 `@plateau/api/plateau/{iam,admin,audit}/.../v1` 的导入路径（保持 `/v1/...` HTTP 路由与 `@servora/plateau-utils@0.1.0` 不变）。

### 步骤三：`0.x` 关闭自动 `breaking`、三处 `CGO_ENABLED=0` 与原子 Tag 发布工作流（R2、R3、R4、R5）
- [x] 修改 `prek.toml`：在 `pre-push` 新增 `go-build-nocgo`（`CGO_ENABLED=0 GOWORK=off go build ./...`），移除 `buf-breaking` 钩子并补充 `0.x` 阶段注释。
- [x] 修改 `.github/workflows/ci.yml`：移除 `changes` `proto` 过滤器中的 `'app/*/service/api/buf.*.gen.yaml'`，移除 `proto` 任务中的 `Check breaking changes` 步骤。
- [x] 修改 `.github/workflows/buf-ci.yml` 与 `.github/workflows/release.yml`：`buf-ci.yml` 移除 `tags: ["v*"]` 并显式设置 `breaking: false`；`release.yml` 在 `Verify tag commit passed CI on main` 后插入 `bufbuild/buf-action@v1` 推送 BSR `v*` 标签步骤。
- [x] 确认 `just/service.just` 的 Unix / Windows `_build` 默认启用 `CGO_ENABLED := env("CGO_ENABLED", "0")`。

### 步骤四：文档与 `.trellis/spec/` 同步及本地全量门禁验证（R6）
- [x] 更新 `AGENTS.md`、`api/AGENTS.md`、`app/AGENTS.md`、`README.md` 及 `.trellis/spec/` 相关规范，反映全仓 29 个 `plateau.*.*.v1` 契约、根级 `buf.openapi.gen.yaml`、三处 `CGO_ENABLED=0`、`0.x` 关闭自动 `breaking`（`v1.0.0` 恢复）与 `v0.1.1` 发布约定。
- [x] 依次执行 §2 全部本地门禁、预提交钩子与 GitNexus 图变更分析，确认 100% 绿灯。

### 步骤五：确认后提交推送、`v0.1.1` 实发与外部隔离消费者验收（R6）
- [ ] 经用户确认后提交并推送 `origin/main`，等待远端 `CI`（`ci-ok`）与 `Buf CI` 全部 `success`。
- [ ] 推送 `v0.1.1` 标签，等待 `Release` 工作流完成 BSR `v0.1.1` 标签推送与 GitHub Release 创建。
- [ ] 在仓库外隔离临时目录（`GOWORK=off`、`CGO_ENABLED=0`）验证引用 `github.com/Servora-Kit/plateau@v0.1.1`、`buf.build/servora/plateau:v0.1.1` 与 `@servora/plateau-utils@0.1.0` 通过 `buf generate`、`go build` 与 TS 类型检查，清理临时目录并勾选本任务与父任务验收项。

---

## 2. 验证命令

```bash
# 1. Proto 格式、Lint 与生成零漂移校验
GOWORK=off just lint-proto
GOWORK=off just fmt-proto-check
GOWORK=off just gen-check
GOWORK=off just api-ts-check

# 2. 残留裸包路径与临时 v1alpha1 扫描（均应为空）
fd v1alpha1 api web
rg -n 'api/gen/go/(iam|admin|audit)/|@plateau/api/(iam|admin|audit)/' app cmd crud infra internal security transport web

# 3. Go 零 CGO 编译、服务构建、静态检查与竞态单测
CGO_ENABLED=0 GOWORK=off go build ./...
GOWORK=off just service::_build
GOWORK=off just lint-go
GOWORK=off just test

# 4. 前端工具包与全栈 Web 校验
pnpm --filter @servora/plateau-utils typecheck
pnpm --filter @servora/plateau-utils test
pnpm --filter @servora/plateau-utils pack --dry-run
just web::lint
just web::test
just web::build

# 5. 本地钩子与 GitNexus 图变更分析
prek run --all-files --stage pre-commit --skip app/admin/web/
node .gitnexus/run.cjs analyze --index-only
mcp__gitnexus_detect_changes -j '{"scope":"all","repo":"."}'
```

---

## 3. 高关注文件与回滚检查点

- **高关注文件**：
  - `buf.openapi.gen.yaml`、`just/service.just`、`prek.toml`
  - `.github/workflows/{ci,buf-ci,release}.yml`
  - `api/protos/plateau/{iam,admin,audit}/**/v1/*.proto`
  - `security/authn/session/user.go`、`security/oidc/rp.go`、`security/audit/kafka/kafka.go`
- **回滚检查点**：
  1. 步骤一完成重生成后，立即通过 `GOWORK=off just lint-proto` 与 `GOWORK=off just fmt-proto-check`，确保 29 个 `plateau.*.*.v1` 包名、目录与 `go_package` 零违规后再推进步骤二；
  2. 步骤四本地所有门禁与集成测试 100% 通过且经用户确认前不推送远端 `main`；远端 `main` 的 `CI` 与 `Buf CI` 全部绿灯前绝不推送 `v0.1.1` 标签。
