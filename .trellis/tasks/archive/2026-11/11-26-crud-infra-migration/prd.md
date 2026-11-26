# 阶段二+三：Servora 瘦身发版与 Plateau 承接 CRUD / 基础设施 / 前端工具包

## 目标

在同一子任务内按单向无死锁顺序一次性完成「`servora` 剥离发版 → `plateau` 承接升级」全流程：先从 `../servora` 剥离 AIP CRUD 全栈生态及数据库/缓存 `contrib` 适配并发布微内核版本 `servora v0.12.0`（含 `buf.build/servora/servora`）与 `@servora/proto-utils@0.2.0`，随后在 `plateau` 完整落位 `crud/`、`cmd/protoc-gen-plateau-crud`、`infra/{entgo,gorm,redis,cache}` 与单一前端工具包 `@servora/plateau-utils`（`web/packages/plateau-utils`），同时将 `plateau` 升级至瘦身后的上游版本，使两仓在 `GOWORK=off` 独立门禁及本地 `../go.work` 联调下均零回归通过。

## 背景与已核实事实

来源为父任务 [11-25-servora-kit-engineering/prd.md](../11-25-servora-kit-engineering/prd.md) §阶段二–三（R2.1–R3.3）与 [design.md](../11-25-servora-kit-engineering/design.md) §3–§5。基于两仓源码实证核查确认：

1. **`servora` 待剥离目录与微内核保留范围**：
   - `../servora` 当前 `main` HEAD 为 `3e2d1cdb`（上一正式标签为 `v0.11.0` 与 `proto-utils/v0.1.11`）。
   - 待迁出或移除资产包括：`../servora/core/crud/`、`../servora/conformance/`、`../servora/docs/crud.md`、`../servora/cmd/protoc-gen-servora-crud/`、`../servora/contrib/db/{entgo,gorm,redis}`、`../servora/contrib/cache/`、`../servora/api/protos/servora/{crud,example,contrib}/` 及 `../servora/web/packages/proto-utils/{src/crud,test/crud.test.mjs,test/error-reasons.*}`。
   - `../servora/contrib/k8s/`（被 `../servora/core/registry/kubernetes.go` 直接调用）与 `../servora/cmd/internal/{plugintest,protoreach}/`（被 `protoc-gen-servora-conf` 及保留插件单测消费）属于微内核组成部分，继续保留在 `servora`；仅 `../servora/core/bootstrap/scan_test.go:10,66-76` 引用了 `servora/contrib/db/redis/v1`（`redispb`），需替换为微内核保留的配置消息测例。
   - 剥离后 `servora` 保留的 7 份 `.proto`（`conf/v1`、`core/v1`、`errors/v1`、`redact/v3`、`security/tls/v1`、`transport/http/{apidocs,cors}/v1`）仅依赖 `google/protobuf/*` 与 `buf.build/bufbuild/protovalidate`（`../servora/api/protos/servora/core/v1/bootstrap.proto:5`），不再依赖 `buf.build/googleapis/googleapis`，且无 `*.errors.ts` 产物；`../servora/go.mod:23,36-41,45,60` 中的 `entgo.io/ent`、`gorm.io/gorm`、`github.com/redis/go-redis/v9`、`github.com/jackc/pgx/v5`、`github.com/mattn/go-sqlite3`、`github.com/jinzhu/copier` 与 `go.einride.tech/aip` 均可随代码移除。
   - `../servora` 远端工作流要求：`ci.yml:108-115` 与 `buf-ci.yml:29-33` 仅在 `pull_request` 执行 `breaking`，`main` 推送直接放行并更新 BSR；`release.yml:36-48` 要求目标提交在 `origin/main` 的 `CI`（`push`）通过后方可推送 `v0.12.0` 标签；`publish-proto-utils.yml:3-31` 由 `proto-utils/v0.2.0` 标签触发 OIDC 发布。
2. **`plateau` 承接影响面与测试夹具约束**：
   - `plateau` 当前共 15 处手写 Go 文件引用待迁移包：`app/iam/service/cmd/server/{main.go,wire.go}`、`app/iam/service/internal/data/{data.go,data_test.go,user.go,authn.go,lifecycle_credential_test.go}`、`app/iam/service/internal/biz/{user.go,account_test.go}`、`app/iam/service/internal/service/{user.go,service_test.go}`、`app/admin/service/internal/data/data.go`、`app/audit/service/internal/biz/page_token.go` 与 `security/cap/cap_integration_test.go`。
   - 迁自 `servora` 的 25+ 个 CRUD / Ent / 插件单测及 `resource_names.json` 依赖多段资源契约 `tenants/{tenant}/users/{user}`（原 `servora/example/v1/example.proto`）。为防止演示契约污染 `plateau/api/protos/` 与后续公开 BSR，需仿照既有 `crud/mapper/testdata/nestedpb/` 将跨语言向量与该契约一并收敛至 `crud/testdata/` 及 `crud/testdata/examplepb/`。
   - `infra/entgo/driver.go` 与 `plateau` 既有代码中 `github.com/mattn/go-sqlite3` 仅出现在 `*_test.go`，生产包已满足 `CGO_ENABLED=0` 编译要求。
3. **前端工具包整合与构建链路事实**：
   - `plateau` 现通过私有源码包 `web/packages/client`（`@plateau/client`，`"exports": { ".": "./src/index.ts" }`）及 `"@servora/proto-utils": "^0.1.11"` 服务 `api/gen`（`package.json:24`）、`app/iam/web`（`package.json:19-20`、`next.config.ts:28`、`lib/iam-api.ts:4`）与 `app/admin/web`（`pnpm-workspace.yaml:47`、`apps/web-antd/package.json:17-18`、`vite.config.ts:23`、`src/api/{client.ts,errors.ts}`、`src/views/users/schemas.ts`）。
   - 阶段四将采用 `npm publish --provenance --access public` 发布 `@servora/plateau-utils`，而 `npm publish` 仅读取顶层 `exports`；同时 `.github/workflows/ci.yml:292-295,331-345` 在 `just api-ts-check` 与 `admin-web` 任务中先执行根 `pnpm install --frozen-lockfile` 再直接运行类型检查。因此 `web/packages/plateau-utils` 需输出 `dist/*.{js,d.ts}` 并配置 `"prepare": "pnpm run build"`，保证安装依赖后 `dist/` 即刻可用。

## 需求清单

### 第一部分：Servora 瘦身与发版（对应父任务阶段二 R2.1–R2.2）

- **R1 剥离 CRUD 全栈与数据/缓存 `contrib`**：在 `../servora` 移除 `core/crud/`、`conformance/`、`docs/crud.md`、`cmd/protoc-gen-servora-crud/`、`contrib/db/`、`contrib/cache/`、`api/protos/servora/{crud,example,contrib}/` 及 `web/packages/proto-utils/src/crud/`；保留 `contrib/k8s/`、`cmd/internal/{plugintest,protoreach}/` 及 `conf`、`go-errors`、`redact`、`typescript-http` 4 款插件，并将 `core/bootstrap/scan_test.go` 改为使用微内核保留的配置消息测例。
- **R2 精简 `servora` 生成配置、依赖与构建脚本**：移除 `../servora/buf.yaml` 中的 `buf.build/googleapis/googleapis` 与 `example.proto` 豁免并更新 `buf.lock`；从 `buf.go.gen.yaml`、`buf.typescript.gen.yaml` 与 `go.mod` `tool` 块移除 `protoc-gen-servora-crud` 和 `entgo.io/ent/cmd/ent`；运行 `just gen-fresh` 与 `GOWORK=off go mod tidy` 清退 ORM / Redis / AIP 依赖；清理 `justfile` 中的 `test-ent-*` 配方与 `ci.yml` 中的 `conformance/**` 路径过滤。
- **R3 发布 `servora v0.12.0` 与 `@servora/proto-utils@0.2.0`**：将 `../servora/web/packages/proto-utils/package.json` 升至 `0.2.0`，`exports` 收敛为 `.`、`./transport`、`./errors`、`./proto/*`，清理 CRUD / error-reasons 测试并补充未导出路径拒绝断言，更新 `../servora` 仓内 `README.md` 与 `AGENTS.md`；在 `GOWORK=off` 全套门禁通过后推送 `servora/main`，待远端 `CI`（`ci-ok`）与 `Buf CI` 成功后推送标签 `v0.12.0` 与 `proto-utils/v0.2.0`，完成 GitHub Release、BSR 与 npm 正式发布。

### 第二部分：Plateau 承接、合并与上游升级（对应父任务阶段三 R3.1–R3.3）

- **R4 承接 AIP CRUD 运行时、Proto 契约与 `protoc-gen-plateau-crud`**：在 `plateau` 新增 `api/protos/plateau/crud/v1/{errors,page_token}.proto`（`package plateau.crud.v1`，`crudpb`）、`crud/`（含 `crud/mapper/`、`crud/testdata/` 共享向量及内部测试夹具 `crud/testdata/examplepb/`）、`docs/crud.md`、`internal/codegen/protoreach/` 与 `cmd/protoc-gen-plateau-crud/`（Go 代码引用 `plateau/crud` 与 `plateau/api/gen/go/plateau/crud/v1`，TS 代码引用 `@servora/plateau-utils/crud`）；同步更新 `go.mod` `tool`、`buf.go.gen.yaml`、`buf.typescript.gen.yaml` 及 `.github/workflows/ci.yml` 的 `go` / `web` 路径过滤规则。
- **R5 统一 `infra/` 基础设施与双 ORM 适配**：新增 `api/protos/plateau/infra/redis/v1/config.proto`（`package plateau.infra.redis.v1`，`redispb`）；将 `entgo`（通用 `driver.go`、`tracing.go`、`crud/`，与既有 `infra/entgo/mixin/` 同目录共存）、`gorm`（含 `crud/`）、`redis`、`cache`（含 `cache/redis`）整合至 `plateau/infra/`，保留 Ent 与 GORM 双套 CRUD 适配，并将 Ent 内存 SQLite 契约并入默认 `just test`、PostgreSQL 契约接入隔离 Schema 与 CI `integration` 任务。
- **R6 整合单一公开前端工具包 `@servora/plateau-utils`**：将 `web/packages/client` 迁移重构为 `web/packages/plateau-utils`（包名 `@servora/plateau-utils`，版本 `0.1.0`，开启 `"sideEffects": false` 与 `dist/` 构建 + `prepare` 脚本），提供 `.` / `./client`、`./crud` 与 `./error-reasons/*`（在 `buf.typescript.gen.yaml` 通过插件级 `types` 过滤生成 `plateau/crud/v1` 与 `plateau/security/errors/v1` 错误枚举并纳入 `gen-check` / `prek.toml`）子路径导出；补充根 `ci.yml` `web` 任务单测步骤。
- **R7 升级 `plateau` 上游依赖、切换全仓调用并同步规范**：将 `plateau` 升级至 `github.com/Servora-Kit/servora v0.12.0`、`buf.build/servora/servora` 最新提交与 `"@servora/proto-utils": "^0.2.0"`，将 `pnpm-workspace.yaml` 与 `renovate.json5` 的冷却豁免扩展至 `@servora/plateau-utils`；切换全仓 15 处 Go 调用点及 `api/gen`、`app/iam/web`、`app/admin/web` 前端引用，重新执行 `just gen-fresh` 与 `just wire`；同步更新两仓相关文档及 `.trellis/spec/{servora,plateau,api,service,web}/` 规范。

## 验收标准

- [x] **AC-R1 & AC-R2（Servora 净身）**：`../servora` 已移除 `core/crud`、`conformance`、`docs/crud.md`、`cmd/protoc-gen-servora-crud`、`contrib/{db,cache}`、`api/protos/servora/{crud,example,contrib}` 与 `web/packages/proto-utils/src/crud`；`go.mod` 不再包含 `ent`、`gorm`、`go-redis`、`pgx`、`go-sqlite3`、`copier`、`aip`；连续两次 `GOWORK=off just gen-check`、`just lint`、`just test`、`just web-test` 与 `govulncheck` 全部通过。
- [x] **AC-R3（Servora 正式发版）**：`Servora-Kit/servora` 远端 `main` 的 `ci-ok` 与 `Buf CI` 为绿色；`v0.12.0` Release 与 BSR `buf.build/servora/servora:v0.12.0` 已就绪，npmjs 已可拉取 `@servora/proto-utils@0.2.0`（仅导出 `.`、`./transport`、`./errors`、`./proto/*`，导入 `./crud` 抛出 `ERR_PACKAGE_PATH_NOT_EXPORTED`）。
- [x] **AC-R4 & AC-R5（Plateau 后端承接）**：`plateau` 已落位 `crud/`（含 `crud/testdata`）、`cmd/protoc-gen-plateau-crud`、`api/protos/plateau/{crud/v1,infra/redis/v1}` 及 `infra/{entgo,gorm,redis,cache}`，且 `api/protos/` 不含任何 `example` 演示契约；`CGO_ENABLED=0 GOWORK=off go build ./...` 通过，`GOWORK=off go test -race -count=1 ./...`（含一致性向量与内存 SQLite 契约）全部通过。
- [x] **AC-R6（`@servora/plateau-utils` 单包收敛）**：`web/packages/client` 已替换为 `web/packages/plateau-utils`（`@servora/plateau-utils@0.1.0`），导出 `.`、`./client`、`./crud`、`./error-reasons/*`；`pnpm --filter @servora/plateau-utils test`、`pnpm pack --dry-run` 校验产物与导出路径完整无缺失。
- [x] **AC-R7（全链路升级与零回归）**：`plateau` 已升级至 `servora v0.12.0` 与 `@servora/proto-utils@^0.2.0`，全仓无残留 `servora/core/crud`、`servora/contrib/{db,cache}`、`protoc-gen-servora-crud`、`@plateau/client` 或 `@servora/proto-utils/crud` 代码引用；在 `GOWORK=off` 下 `just gen-check`、`just lint`、`just test`、`just web::lint`、`just web::build`、`just web::test` 全部通过，本地 `../go.work` 模式下 `go build ./...` 亦正常通过，且 `.trellis/spec/` 规范已同步无断链。

## 范围外事项

- **全集中式 `api/protos/` 与 `buf.build/servora/plateau` 命名模块**：`app/{iam,admin,audit}/service/api/protos/**` 迁入根 `api/protos/` 及服务 OpenAPI 生成模板改造归属阶段四独立子任务。
- **`plateau` 自动化发布流水线与首次外部发包**：`plateau` 的 `buf-ci.yml`、`release.yml`、`publish-plateau-utils.yml` 工作流及 `buf.build/servora/plateau` / `@servora/plateau-utils` 首次正式打标发布归属阶段四独立子任务。
- **既有平台服务业务逻辑改动**：不更改 `app/iam`、`app/admin`、`app/audit` 的业务接口行为、数据库 Schema 或 OpenFGA 授权模型。
