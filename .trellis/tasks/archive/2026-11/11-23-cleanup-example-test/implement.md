# 执行计划：清理 example 与 test 示例服务

## 实施清单（按序执行）

### Step 1：解除保留模块对 example / test 的代码与配置耦合
- [x] 修改 `web/packages/client/test/kratos_stream_fixture.go`：移除 `examplepb` 导入，改用 `structpb.Struct` 解析 `name`、`etag`、`allowMissing`。
- [x] 修改 `app/iam/service/configs/local/oidc.yaml` 与 `app/iam/service/configs/docker/oidc.yaml`：删除注释掉的 `test-web` 客户端块。
- [x] 修改 `app/iam/service/cmd/server/config_test.go`：删除无用的 `t.Setenv("IAM_TEST_WEB_CLIENT_SECRET", ...)`。

### Step 2：移除目录与更新构建编排配置
- [x] 删除目录：`app/example/`、`app/test/`、`.trellis/spec/example/`、`.trellis/spec/test/`。
- [x] 更新 `buf.yaml`：移除 `app/example/service/api/protos` 与 `app/test/service/api/protos`。
- [x] 更新 `just/services.just`：移除 `example`、`test` 模块声明及所有聚合目标引用。
- [x] 更新 `just/webs.just`：移除 `example`、`test` 模块声明及 `build`/`lint` 引用。
- [x] 更新根 `justfile`：`compose-build` 移除 `example`，保留 `audit`。
- [x] 更新 `just/service.just`：移除已无消费者的 `TYPESCRIPT_TEMPLATE` 与服务级 `api-ts` 分支。

### Step 3：重建生成产物与收敛依赖锁文件
- [x] 执行 `just gen-fresh`：清空并重建 `api/gen/go`、`api/gen/ts` 及保留服务的 OpenAPI / Wire / Ent，同时清除未跟踪的旧 `api/gen/dist`。
- [x] 执行 `GOWORK=off go mod tidy`：收敛 `go.mod` 与 `go.sum`。
- [x] 执行 `pnpm install`（仓库根目录）：刷新 `pnpm-lock.yaml`，移除 `@plateau/example-web` 与 `@plateau/test-web`。

### Step 4：同步文档与 Trellis 注册表
- [x] 更新 `.trellis/config.yaml`：移除 `example`、`test` package 注册。
- [x] 更新 `.trellis/spec/api/proto/generation.md`：更新模块清单为 `iam`、`admin`、`audit`。
- [x] 更新 `AGENTS.md`、`app/AGENTS.md`、`api/AGENTS.md`：移除 `example`/`test` 端口表、目录说明、命令示例及服务级 TS 生成说明。

### Step 5：全量门禁与残留检查
- [x] 验证 Just 语法与命令列表：`just --list --list-submodules`（如配置有 `just-lsp` 同步检查 `just/*.just` 与 `justfile`）。
- [x] 验证 Proto 与 Go 质量门禁：`just lint-proto`、`just api-ts-check`、`GOWORK=off just lint-go`、`GOWORK=off go test ./...`。
- [x] 验证流式夹具独立编译：`GOWORK=off go build -o /dev/null ./web/packages/client/test/kratos_stream_fixture.go`。
- [x] 验证前端包与保留前端：`pnpm --filter @plateau/client test`、`pnpm --filter @plateau/client typecheck`、`just web::lint`、`just web::build`。
- [x] 验证锁文件稳定：`pnpm install --frozen-lockfile` 与二次 `GOWORK=off go mod tidy` 后无新增 diff。
- [x] 验证 Trellis 与残留检索：`python3 ./.trellis/scripts/get_context.py --mode packages`；全仓检索非归档文件中无 `app/example`、`app/test`、`examplepb`、`10080`、`10090`、`IAM_TEST_WEB` 残留。
- [x] 运行 GitNexus 变更分析（`detect_changes`）并记录结果。

## 风险文件与回滚点

- 高关注文件：`web/packages/client/test/kratos_stream_fixture.go`、`buf.yaml`、`just/service.just`、`just/services.just`、`just/webs.just`、`pnpm-lock.yaml`、`go.mod`。
- 执行前确认 `git status` 干净；若中途需回滚，可在未提交状态下按 `git checkout -- .` 恢复已删跟踪文件。

## 启动前检查（`task.py start` 之前）

- [x] `prd.md`、`design.md`、`implement.md` 已完成并通过收敛自检。
- [x] `implement.jsonl` 与 `check.jsonl` 已配置真实规范与研究路径并通过 `task.py validate`。
- [x] 用户已显式审阅并批准最终规划摘要。
