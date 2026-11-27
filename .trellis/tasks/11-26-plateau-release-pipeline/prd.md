# 阶段四：Plateau 单一 BSR 统一命名空间、CGO 门禁与原子发布流水线

> 父任务：[11-25-servora-kit-engineering](../11-25-servora-kit-engineering/prd.md)（R4.1、R4.2）

## 目标与价值

将 `plateau` 三大核心服务（`iam`、`admin`、`audit`）共 12 个公开 `.proto`（含 4 个服务 `conf.proto`）补齐顶层产品前缀迁至 `api/protos/plateau/{iam,admin,audit}/.../v1/`，与既有 17 个平台契约共同归属单一 BSR 模块 `buf.build/servora/plateau`（全仓 29 个公开 `.proto` 统一为 `plateau.*.*.v1`）；保持全栈 `/v1/...` HTTP 路由、OIDC 回调、Traefik 规则与 `@servora/plateau-utils@0.1.0` 不变；在 `0.x` 演进阶段关闭三处自动 `breaking` 拦截（待 `v1.0.0` 发布时恢复）；归拢仓库根级唯一 `buf.openapi.gen.yaml`、补齐本地与 CI 三处 `CGO_ENABLED=0` 构建门禁及原子化 Tag 发布门禁；在不添加任何 `retract` / `deprecated` 标记的前提下完成 `v0.1.1`（Go Module + BSR）外部消费验证并同步全仓文档与 `.trellis/spec/` 规范。

## 已核实的背景与约束事实

1. **现行仓库状态与待修正项**：
   - [buf.yaml](../../../buf.yaml#L3-L5) 已合并为单一命名模块 `buf.build/servora/plateau`（`path: api/protos`），`Servora-Kit/plateau` 的 `BUF_TOKEN` 与 npmjs `@servora/plateau-utils@0.1.0`（含 OIDC provenance）已完成配置与发布。
   - 当前 `HEAD` 仍待补齐 5 项工程化改进：
     1. `iam`、`admin`、`audit` 的 12 个 `.proto` 位于裸顶层 `api/protos/{iam,admin,audit}/.../v1/`，缺少 `plateau.` 命名空间前缀，易在单一 BSR 模块导出时与下游业务仓命名冲突；
     2. 仍保留 `app/iam/service/api/buf.openapi.gen.yaml` 与 `app/admin/service/api/buf.openapi.gen.yaml` 两份子目录模板及 `just/service.just` 的存在性分支；
     3. `just/service.just` 与 `prek.toml` `pre-push` 尚未默认强制 `CGO_ENABLED=0`；
     4. `0.x` 早期演进阶段无需将包后缀降级为 `v1alpha1`（避免 `v1.0.0` 时二次全栈重命名），直接保留 `.v1` 与 `/v1/...` 路由并关闭 `prek.toml`、`ci.yml`、`buf-ci.yml` 三处自动 `breaking` 拦截即可（保留 `buf.yaml` 的 `breaking: use: [FILE]` 供手动运行 `just breaking`）；
     5. `buf-ci.yml` 此前直接监听 `v*` 标签，需改为由 `release.yml` 在 `origin/main` 祖先校验与 `ci.yml == success` 通过后原子推送 BSR 标签再创建 GitHub Release。
2. **外部注册中心版本号策略**：
   - `@servora/plateau-utils@0.1.0` 已在 `HEAD`（`a73e5e53`）发布至 npmjs，保留 `.v1` 后 `web/packages/plateau-utils` 代码与生成产物零差异，直接沿用 `0.1.0`，无需空发新版本。
   - `github.com/Servora-Kit/plateau@v0.1.0` 已写入 Go Checksum DB；采用早期开发无标记演进策略（不写 `go.mod retract`、不标 `.proto deprecated`），合入后按补丁修订号发布 `v0.1.1`（同时推送 BSR `v0.1.1` 标签）。
3. **保持不变的契约与路由**：
   - 17 个平台公共 `.proto`（`api/protos/plateau/{security,crud,infra}/.../v1/`）、4 款 `protoc-gen-plateau-*` 插件、`buf.typescript.gen.yaml` 及 `@servora/plateau-utils` 错误原因子路径保持不变；
   - 全栈 HTTP `/v1/iam/...` 与 `/v1/admin/...` 路由、Admin OIDC `redirect_uri`、Traefik 规则、会话 `JSONPrefixes`、前后端开发反代、RFC 标准无版本端点及测试夹具均保持不变。

## 需求清单

- **R1（12 个服务 `.proto` 补齐 `plateau.` 顶层命名空间，全仓统一 `plateau.*.*.v1`）**：
  - 将 `api/protos/{iam,admin,audit}/.../v1/*.proto`（共 12 个文件，含 4 个服务 `conf/v1/config.proto`）迁移至 `api/protos/plateau/{iam,admin,audit}/.../v1/`，删除裸顶层 `api/protos/{iam,admin,audit}/`；
  - 更新这 12 个 `.proto` 的 `package` 为 `plateau.{iam,admin,audit}.*.v1`，`option go_package` 对齐 `github.com/Servora-Kit/plateau/api/gen/go/plateau/.../v1;<alias>v1`，同步内部 `import` 与类型引用，保持 `google.api.http` `/v1/...` 路由不变；
  - 联动更新 `app/{iam,admin,audit}/service`、`security/{authn/session,oidc,audit}`（5 个关联文件）及 `app/{iam,admin}/web` 中的导入路径与单测 gRPC 全限定方法名断言（如 `/plateau.iam.user.v1.UserService/...`）。
- **R2（`0.x` 阶段关闭自动 `breaking` 拦截）**：
  - 保留 [buf.yaml](../../../buf.yaml#L12-L16) 中的 `breaking` 规则以支持本地手动执行 `just breaking`；
  - 移除 [prek.toml](../../../prek.toml) `pre-push` 中的 `buf-breaking` 钩子、移除 [.github/workflows/ci.yml](../../../.github/workflows/ci.yml) `proto` 任务的 `Check breaking changes` 步骤、在 [.github/workflows/buf-ci.yml](../../../.github/workflows/buf-ci.yml) 的 `bufbuild/buf-action@v1` 中显式设置 `breaking: false`（并在配置注释与规范文档中注明 `v1.0.0` 发布前恢复）。
- **R3（根级唯一 `buf.openapi.gen.yaml` 与无分支 `just openapi`）**：
  - 在仓库根目录保留唯一 [buf.openapi.gen.yaml](../../../buf.openapi.gen.yaml)（`out: internal/assets`），删除 `app/{iam,admin}/service/api/` 目录及 `ci.yml` 中的旧子目录过滤项；
  - 将 [just/service.just](../../../just/service.just) 的 `openapi` 配方精简为单行 `buf generate --template buf.openapi.gen.yaml --path "api/protos/plateau/{{ SERVICE_NAME }}" -o "{{ SERVICE_MODULE }}"`，将 `app/audit/service/internal/assets/openapi.yaml` 纳入版本控制与 `just gen-check`。
- **R4（三处统一强制 `CGO_ENABLED=0` 构建门禁）**：
  - 在 [just/service.just](../../../just/service.just) 的 `_build`（Unix 与 Windows）默认启用 `CGO_ENABLED=0`（支持环境变量覆盖），在 [prek.toml](../../../prek.toml) `pre-push` 新增 `go-build-nocgo`（`CGO_ENABLED=0 GOWORK=off go build ./...`），与 [.github/workflows/ci.yml](../../../.github/workflows/ci.yml) 保持三处一致；`just test` 维持默认 CGO 环境支持 `-race` 与 `go-sqlite3` 单测。
- **R5（Tag 发布工作流原子门禁修正）**：
  - [.github/workflows/buf-ci.yml](../../../.github/workflows/buf-ci.yml) 移除 `tags: ["v*"]`（仅处理 PR 校验与 `main` 推送）；
  - [.github/workflows/release.yml](../../../.github/workflows/release.yml) 在 `origin/main` 祖先校验与 `ci.yml == success` 校验通过后，先通过 `bufbuild/buf-action@v1` 推送 BSR `v*` 版本标签，再创建 GitHub Release；保留 [.github/workflows/publish-plateau-utils.yml](../../../.github/workflows/publish-plateau-utils.yml) 不变。
- **R6（文档规范同步与 `v0.1.1` 实发验证）**：
  - 同步更新 [AGENTS.md](../../../AGENTS.md)、[api/AGENTS.md](../../../api/AGENTS.md)、[app/AGENTS.md](../../../app/AGENTS.md)、[README.md](../../../README.md) 及 `.trellis/spec/` 中关于 `plateau.*.*.v1` 目录、根级 `buf.openapi.gen.yaml`、三处 `CGO_ENABLED=0`、`0.x` 关闭自动 `breaking` 拦截及发版流程的描述；
  - 推送 `main` 待远端 `CI` 与 `Buf CI` 全绿后推送 `v0.1.1` 标签，并在仓库外隔离目录（`GOWORK=off`、`CGO_ENABLED=0`）验证外部最小消费者同时引用 `github.com/Servora-Kit/plateau@v0.1.1`、`buf.build/servora/plateau:v0.1.1` 与 `@servora/plateau-utils@0.1.0` 构建通过。

## 验收标准

- [x] **AC-1（对应 R1）**：29 个公开 `.proto` 全部位于 `api/protos/plateau/{security,crud,infra,iam,admin,audit}/**/v1/`，裸顶层 `api/protos/{iam,admin,audit}/` 及任何临时 `v1alpha1` 目录均已清理；HTTP 路由与 OpenAPI 路径保持 `/v1/...`；`GOWORK=off` 下 `just lint-proto`、`just fmt-proto-check`、`just gen-check` 与 `just api-ts-check` 全部通过。
- [x] **AC-2（对应 R2、R3）**：`prek.toml`、`ci.yml`、`buf-ci.yml` 已关闭自动 `breaking` 拦截且保留 `just breaking` 手动入口；`app/*/service/api/` 已删除，根级唯一 `buf.openapi.gen.yaml` 与单行 `just openapi` 工作正常，`app/{iam,admin,audit}/service/internal/assets/openapi.yaml` 零漂移。
- [x] **AC-3（对应 R4、R5）**：`just/service.just`、`prek.toml` `pre-push` 与 `ci.yml` 三处强制 `CGO_ENABLED=0` 构建通过；`buf-ci.yml` 不再监听 `v*` 标签，`release.yml` 在 `ci.yml == success` 门禁后串行执行 BSR 标签推送与 GitHub Release 创建，YAML/TOML 通过 `prek` 校验。
- [x] **AC-4（对应 R1、R6）**：`AGENTS.md`、`api/AGENTS.md`、`app/AGENTS.md`、`README.md` 与 `.trellis/spec/` 全部对齐 `plateau.*.*.v1`、`/v1/`、根级 `buf.openapi.gen.yaml` 与 `0.x` `breaking` 策略；`GOWORK=off` 下全量 Go 竞态单测/集成测试、Lint、IAM/Admin Web 构建与单测、`@servora/plateau-utils@0.1.0` 单测与打包校验全部通过。
- [x] **AC-5（对应 R6）**：远端 `main` 的 `CI` 与 `Buf CI` 绿灯，`buf.build/servora/plateau:v0.1.1` 与 `github.com/Servora-Kit/plateau@v0.1.1` 实发成功；仓库外隔离目录在 `GOWORK=off`、`CGO_ENABLED=0` 下引用 `plateau@v0.1.1`、`buf.build/servora/plateau:v0.1.1` 与 `@servora/plateau-utils@0.1.0` 通过 `buf generate`、`go build` 与 TS 类型检查，临时目录已清理。

## 范围外事项

- **平台脚手架与 GitHub Template**：新服务生成 CLI（`new service` / `new api`）与垂类衍生模板仓不在本任务内开发。
- **平台服务业务逻辑与容器镜像发布**：不改动 `iam`、`admin`、`audit` 的业务规则、数据库表结构或 OpenFGA 授权模型；不新增 Docker 镜像（GHCR）或 Helm 发布流水线。
