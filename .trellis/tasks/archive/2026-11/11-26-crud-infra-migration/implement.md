# Implement：阶段二+三 Servora 瘦身发版与 Plateau 承接升级

关联文档：[prd.md](prd.md) · [design.md](design.md) · 父任务 [11-25-servora-kit-engineering](../11-25-servora-kit-engineering/prd.md)

## 前置检查

- [x] 确认 `../servora` 位于 `main` 分支且工作区干净（`git -C ../servora status --short` 为空），记录源码提取提交 `3e2d1cdb`
- [x] 检查 `plateau` 当前未提交文件状态，不触碰与本任务无关的用户既有改动
- [x] 用户明确批准规划摘要后，运行 `python3 ./.trellis/scripts/task.py start 11-26-crud-infra-migration`

---

## 第一阶段：Servora 剥离瘦身与正式发版（R1–R3）

1. **剥离 CRUD 与非核心 `contrib` 源码（R1）**
   - [x] 在 `../servora` 删除 `core/crud/`、`conformance/`、`docs/crud.md`、`cmd/protoc-gen-servora-crud/`、`contrib/db/`、`contrib/cache/`、`api/protos/servora/{crud,example,contrib}/`（保留 `contrib/k8s/` 与 `cmd/internal/{plugintest,protoreach}/`）
   - [x] 修改 `../servora/core/bootstrap/scan_test.go`，用微内核保留的 `corev1` / `corsv1` 配置段替代 `redispb`

2. **清理 Buf / 生成器 / CI 配置并精简 `go.mod`（R2）**
   - [x] 更新 `../servora/buf.yaml`：移除 `buf.build/googleapis/googleapis` 与 `example.proto` `ignore_only` 条目，运行 `buf dep update` 刷新 `buf.lock`
   - [x] 从 `../servora/buf.go.gen.yaml`、`../servora/buf.typescript.gen.yaml` 与 `../servora/go.mod` `tool` 块移除 `protoc-gen-servora-crud` 和 `entgo.io/ent/cmd/ent`
   - [x] 删除 `../servora/justfile` 中的 `test-ent-sqlite` / `test-ent-postgres` 配方，移除 `../servora/.github/workflows/ci.yml` 中的 `conformance/**` 路径过滤
   - [x] 在 `../servora` 执行 `GOWORK=off just gen-fresh` 与 `GOWORK=off go mod tidy`，确认 `ent`、`gorm`、`go-redis`、`pgx`、`go-sqlite3`、`copier`、`aip` 已全部移出 `go.mod`

3. **收敛 `@servora/proto-utils@0.2.0` 与更新 `servora` 文档（R3）**
   - [x] 删除 `../servora/web/packages/proto-utils/src/crud/`、`test/crud.test.mjs`、`test/error-reasons.*`；清理 `tsconfig.json` 与 `tsconfig.build.json` 中的 `crud` `paths`
   - [x] 更新 `../servora/web/packages/proto-utils/package.json`：版本改为 `0.2.0`，移除 `./crud` 与 `./error-reasons/*` 导出，补充未导出子路径拒绝单测，在 `../servora/web` 同步 `pnpm-lock.yaml`
   - [x] 更新 `../servora/README.md` 与 `../servora/{.,api,cmd,contrib,core}/AGENTS.md`（删除失效子目录 `AGENTS.md`）

4. **`servora` 本地门禁验收、推送与标签发布（R3）**
   - [x] 在 `../servora` 按下方验证矩阵跑通全量本地门禁
   - [x] 提交 `../servora` 变更并执行 `SKIP=buf-breaking git -C ../servora push origin main`
   - [x] 通过 `gh run list -R Servora-Kit/servora` / `gh run watch` 等待 `main` 分支 `CI`（`ci-ok`）与 `Buf CI` 全部 `success`
   - [x] 依次创建并推送标签 `v0.12.0` 与 `proto-utils/v0.2.0`，等待 `Release`、`Buf CI`（BSR `v0.12.0`）与 `Publish @servora/proto-utils` 工作流全部成功，验证 `npm view @servora/proto-utils@0.2.0 version` 可见

---

## 第二阶段：Plateau 承接生态、合并前端包与上游升级（R4–R7）

5. **迁入 Proto 契约、CRUD 运行时、`protoc-gen-plateau-crud` 与 `infra/*`（R4–R5）**
   - [x] 从 `git -C ../servora archive 3e2d1cdb` 提取源码落位至 `plateau`：
     - `api/protos/plateau/crud/v1/{errors,page_token}.proto`（`package plateau.crud.v1`，`crudpb`）
     - `api/protos/plateau/infra/redis/v1/config.proto`（`package plateau.infra.redis.v1`，`redispb`）
     - `crud/`（含 `crud/mapper/`，更新 `nestedpb` 包路径；将跨语言共享向量与内部测试夹具 `example.proto` 收敛至 `crud/testdata/` 与 `crud/testdata/examplepb/`）、`docs/crud.md`
     - `internal/codegen/protoreach/` 与 `cmd/protoc-gen-plateau-crud/`（更新 Go 生成导入为 `plateau/api/gen/go/plateau/crud/v1`，TS 生成导入为 `@servora/plateau-utils/crud`）
     - `infra/entgo/`（`driver.go`、`tracing.go`、`crud/`，保留既有 `mixin/`）、`infra/gorm/`（含 `crud/`）、`infra/redis/`、`infra/cache/`（含 `redis/`）
   - [x] 更新 `plateau/go.mod`：将 `github.com/Servora-Kit/servora` 升级至 `v0.12.0`（含其保留的 4 款 `protoc` 插件版本），`tool` 块将 `protoc-gen-servora-crud` 替换为 `github.com/Servora-Kit/plateau/cmd/protoc-gen-plateau-crud`
   - [x] 更新 `plateau/buf.go.gen.yaml` 与 `plateau/buf.typescript.gen.yaml` 指向 `protoc-gen-plateau-crud`；执行 `buf dep update` 将 `buf.lock` 中的 `buf.build/servora/servora` 升级至最新提交
   - [x] 将 Ent 内存 SQLite 契约测试并入默认 `just test`，PostgreSQL 契约测试改为隔离 Schema 并接入 `setup-integration-env.sh` 与 `.github/workflows/ci.yml` 的 `integration` 任务；在 `.github/workflows/ci.yml` 的 `changes.go` / `changes.web` 过滤器加入 `crud/**` 与 `crud/testdata/*.json`

6. **构建单一公开工具包 `@servora/plateau-utils` 并接入生成链（R6）**
   - [x] 将 `web/packages/client` 迁移重构为 `web/packages/plateau-utils`（`src/client/*`、`src/crud/*`、`src/index.ts`、`tsconfig.json`、`tsconfig.build.json`、`package.json`），将相对导入规范为 `.js` 后缀，在根 `.gitignore` 忽略 `/web/packages/*/dist/`
   - [x] 在 `buf.typescript.gen.yaml` 通过插件级 `types` 过滤将 `CrudErrorReason` 与 `SecurityErrorReason` 输出至 `web/packages/plateau-utils/src/gen`，接入 `justfile`（`gen-check`、`_gen-clean-files`）、`.github/workflows/ci.yml`（`changes.web`）与 `prek.toml`（`exclude`）
   - [x] 迁移并合并 `web/packages/plateau-utils/test/{http,crud,error-reasons}.test.ts`；在 `.github/workflows/ci.yml` 的 `web` 任务末尾补充 `pnpm -r --if-present run test`

7. **切换 `plateau` 全仓 Go / 前端调用点并刷新生成物与锁文件（R7）**
   - [x] 更新 `plateau` 全部 15 处手写 Go 文件的 `crud`、`crud/mapper`、`crudpb`、`entdriver`、`entcrud`、`redispb`、`redis` 导入路径
   - [x] 执行 `GOWORK=off just gen-fresh`（补充 `_gen-go-pb` 引导步）、`GOWORK=off just wire` 与 `GOWORK=off go mod tidy`（并生成 `crud/testdata/examplepb` 夹具代码）
   - [x] 更新 `pnpm-workspace.yaml` 与 `renovate.json5`（豁免 `@servora/plateau-utils`），更新 `api/gen/package.json`、`app/iam/web/{package.json,next.config.ts,lib/iam-api.ts}` 与 `app/admin/web`（`pnpm-workspace.yaml`、`apps/web-antd/package.json`、`vite.config.ts`、`src/api/{client.ts,errors.ts}`、`src/api/__tests__/errors.test.ts`、`src/views/users/schemas.ts`）至 `@servora/proto-utils@^0.2.0` 与 `@servora/plateau-utils`
   - [x] 在根工作区与 `app/admin/web` 分别执行 `pnpm install` 刷新两份 `pnpm-lock.yaml`

8. **文档与 `.trellis/spec/` 同步及全量验收（R7、AC-R1～AC-R7）**
   - [x] 更新 `plateau/AGENTS.md`（移除迁入目录冗余子 `AGENTS.md` 并将核心约束写回 Go 包注释，精准同步 `README.md` 相关条目）
   - [x] 同步更新 `.trellis/spec/servora/`（移除已迁出的 `framework/crud.md` 并更新索引）与 `.trellis/spec/{plateau,api,service,web}/`（补充 `crud`、`infra/{entgo,gorm,redis,cache}`、`protoc-gen-plateau-crud` 与 `@servora/plateau-utils` 规范）
   - [x] 按下方验证矩阵完成 `plateau` 在 `GOWORK=off` 及默认 `../go.work` 下的全量校验

---

## 验证命令矩阵

### A. `../servora` 验证（第一阶段提交发版前）

| 检查项 | 命令（在 `../servora` 下） | 预期结果 |
| --- | --- | --- |
| 生成幂等 | `GOWORK=off just gen-check && GOWORK=off just gen-check` | 两次均 0 退出码，无差异或未跟踪文件 |
| Go 构建 / Lint / 单测 | `CGO_ENABLED=0 GOWORK=off go build ./... && GOWORK=off just lint-go && GOWORK=off just test` | 全绿，0 lint 问题，无重型 ORM/Redis/AIP 依赖 |
| Proto 校验 | `just lint-proto && just fmt-proto-check` | 0 错误 |
| 前端包校验 | `just web-install && just web-check && just web-typecheck && just web-test` | Biome、tsc、构建与含未导出断言的单测全过 |
| 安全与钩子 | `prek run --all-files --stage pre-commit && GOWORK=off go run golang.org/x/vuln/cmd/govulncheck@latest ./...` | 全绿，0 可触发漏洞 |

### B. `plateau` 验证（第二阶段承接升级后）

| 检查项 | 命令（在 `plateau` 下） | 预期结果 |
| --- | --- | --- |
| 纯 Go 编译校验 | `CGO_ENABLED=0 GOWORK=off go build ./...` | 0 错误，生产包无 CGO 依赖 |
| 生成幂等 | `GOWORK=off just gen-check && GOWORK=off just gen-check` | `api/gen/{go,ts}`、`web/packages/plateau-utils/src/gen`、`wire_gen.go`、Ent、OpenAPI 均零漂移 |
| Go Lint / 竞态单测 / 一致性向量 | `GOWORK=off just lint && GOWORK=off just test` | 全部通过（含 `crud/testdata` 一致性向量与内存 SQLite live contract） |
| Proto 破坏性与格式校验 | `just fmt-proto-check && just breaking` | 相对 `origin/main` 新增包零破坏性违规 |
| `@servora/plateau-utils` 包校验 | `pnpm --filter @servora/plateau-utils typecheck && pnpm --filter @servora/plateau-utils test && pnpm --filter @servora/plateau-utils pack --dry-run` | 单测通过，打包清单含 `dist/` 且导出路径准确 |
| 全栈前端校验 | `pnpm install --frozen-lockfile && just api-ts-check && just web::install && just web::lint && just web::build && just web::test` | IAM Web（Next.js）与 Admin Web（Vben）lint / typecheck / build / unit test 全绿 |
| 本地 `../go.work` 联调校验 | `go build ./...`（不带 `GOWORK=off`） | 跨仓本地工作区恢复正常编译 |
| Prek 与 GitNexus 变更检查 | `prek run --all-files --stage pre-commit --skip app/admin/web/` | 0 格式与配置错误 |

## 风险文件与回滚点

- **回滚点 1（`servora` 推送前）**：检查 `../servora/api/gen/go/servora` 与 `../servora/web/packages/proto-utils/src/gen/servora` 仅剩 7 个内建包，避免误发多余契约。
- **回滚点 2（`servora` 打标后、`plateau` 提交前）**：确认 `go list -m github.com/Servora-Kit/servora@v0.12.0` 与 `npm view @servora/proto-utils@0.2.0` 均已就绪后再更新 `plateau` 锁文件。
- **注意保护用户工作区**：`plateau` 若存在未提交的文档或测试改动（如 `README.md`、`docs/iam.md`、`app/admin/service/internal/server/audit_integration_test.go`），编辑时只触碰与本次迁移直接相关的行，提交时使用 `commit` 命名空间仅纳入本任务变更。
