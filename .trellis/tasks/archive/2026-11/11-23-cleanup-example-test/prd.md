# 清理 example 与 test 示例服务

## Goal

移除 Plateau 仓库中已不再承担参考价值的 `app/example` 与 `app/test` 示例服务（含其 Proto、生成产物、Just 注册、依赖项、文档和规范），降低多模块维护、代码生成与依赖锁文件负担，使仓库聚焦真实运行的 `iam`、`admin` 以及显式保留的 `audit` 服务。

## Background & Confirmed Facts

- 当前 `app/` 目录包含 `iam`、`admin`、`audit`、`example`、`test` 五个服务；其中 `iam` + `admin` 已构成完整的微服务与前后端调用流，`audit` 按用户要求保留。
- `app/example`（`service` + `web`）、`app/test`（`service` + `web`）、`api/gen/{go,ts}/{example,test}` 与 `.trellis/spec/{example,test}` 合计包含 144 个 Git 跟踪文件。
- 实测发现三类跨模块耦合必须在删除前或同步处理：
  1. `web/packages/client/test/kratos_stream_fixture.go:16,161-173` 导入 `api/gen/go/example/service/v1` 的 `DeleteUserRequest`，移除 `example` 生成物后会导致该文件无法编译。
  2. `app/iam/service/configs/{local,docker}/oidc.yaml:6-16` 与 `app/iam/service/cmd/server/config_test.go:20` 残留 `app/test/web`（`test-web` / 端口 `10092`）的配置注释与环境变量设置。
  3. `app/example/service/api/buf.typescript.gen.yaml` 是全仓唯一的服务级 TS 生成模板；移除后 `just/service.just:16,46-50` 中的 `TYPESCRIPT_TEMPLATE` / `api-ts` 成为无消费者死分支。

## Requirements

- **REQ-1 移除示例服务目录与规范目录**
  - 删除 `app/example/`、`app/test/`、`.trellis/spec/example/` 与 `.trellis/spec/test/`。
- **REQ-2 清理 Proto 模块注册并重建共享生成产物**
  - 从 `buf.yaml:6-7` 移除 `app/example/service/api/protos` 与 `app/test/service/api/protos`。
  - 重建 `api/gen/go` 与 `api/gen/ts`（并清除未跟踪的旧 `api/gen/dist`），确保不再存在 `example`、`test` 生成目录，同时 `iam`、`admin`、`audit`、`plateau` 产物保持完整有效。
- **REQ-3 清理 Just 构建编排与服务级死分支**
  - 从 `just/services.just` 与 `just/webs.just` 移除 `example`、`test` 模块声明及所有聚合目标引用。
  - 从根 `justfile:137` 的 `compose-build` 移除 `example`，保留 `audit`。
  - 从 `just/service.just:16,46-50` 移除已无任何服务使用的 `TYPESCRIPT_TEMPLATE` 与服务级 `api-ts` 分支。
- **REQ-4 解除共享包与保留服务对示例服务的残留耦合**
  - 重构 `web/packages/client/test/kratos_stream_fixture.go`，移除 `examplepb` 导入，改用 `structpb.Struct` 保持原有 WebSocket JSON 字段（`name`、`etag`、`allowMissing`）解析行为。
  - 清理 `app/iam/service/configs/{local,docker}/oidc.yaml:6-16` 的 `test-web` 注释客户端以及 `app/iam/service/cmd/server/config_test.go:20` 的 `IAM_TEST_WEB_CLIENT_SECRET` 残留设置。
- **REQ-5 同步依赖锁文件**
  - 在 `GOWORK=off` 下运行 `go mod tidy`，刷新 `go.mod` 与 `go.sum`。
  - 在仓库根目录运行 `pnpm install`，刷新 `pnpm-lock.yaml`，移除 `@plateau/example-web` 与 `@plateau/test-web` 的 workspace 依赖记录。
- **REQ-6 同步仓库文档与 Trellis 注册信息**
  - 从 `.trellis/config.yaml:179-184` 移除 `example`、`test` package 注册。
  - 更新 `.trellis/spec/api/proto/generation.md:7`，仅列出当前保留的模块（`iam`、`admin`、`audit`）。
  - 更新 `AGENTS.md`、`app/AGENTS.md`、`api/AGENTS.md`，移除 `example`/`test` 的端口表项、命令示例、目录说明与服务级 TS 生成说明，并将新增服务参考指向现有服务与后端规范。

## Out of Scope

- 不删除或改动 `app/audit/`、[`docker-compose.apps.yaml`](../../../docker-compose.apps.yaml)、`infra/clickhouse/` 及 `.trellis/spec/audit/`。
- 不修改 `app/iam/` 与 `app/admin/` 的业务行为，不改动 `app/iam/service/internal/oidc/provider_test.go:50` 的独立单测常量。
- 跨仓补充：应用户要求同步更新 `../servora/docs/crud.md`，将参考应用从已删除的 `plateau/app/example` 切换至 `plateau/app/iam` 与 `plateau/app/admin`。
- 不修改插件单元测试（`cmd/protoc-gen-plateau-{authn,authz}/main_test.go`）中虚构的 `example.com` / `examplev1` 测试包名，也不改文档中的 RFC 示例域名（`example.com`）。

## Acceptance Criteria

- [x] **AC-1（对应 REQ-1）**：`app/example/`、`app/test/`、`.trellis/spec/example/`、`.trellis/spec/test/` 已不存在；`app/iam/`、`app/admin/`、`app/audit/` 保持完整。
- [x] **AC-2（对应 REQ-2）**：`buf.yaml` 不再包含 `example`/`test` 模块；执行 `just gen-fresh` 后 `api/gen/go`、`api/gen/ts` 下无 `example`/`test` 目录，且 `just lint-proto` 与 `just api-ts-check` 全部通过。
- [x] **AC-3（对应 REQ-3）**：`just/services.just`、`just/webs.just`、`justfile`、`just/service.just` 不再引用 `example`/`test` 或服务级 `api-ts`；`just --list --list-submodules` 正常列出剩余任务，且 `just lint`、`just web::lint`、`just web::build` 通过。
- [x] **AC-4（对应 REQ-4）**：`web/packages/client/test/kratos_stream_fixture.go` 不再导入 `examplepb`，可在 `GOWORK=off` 下正常编译，且 `pnpm --filter @plateau/client test` 与 `typecheck` 通过；`app/iam/service/configs/{local,docker}/oidc.yaml` 与 `config_test.go` 无 `test-web`/`10092`/`IAM_TEST_WEB_*` 残留，且 `GOWORK=off go test ./...` 全部通过。
- [x] **AC-5（对应 REQ-5）**：`GOWORK=off go mod tidy` 与根目录 `pnpm install` 后，`pnpm-lock.yaml` 不再包含 `app/example/web`、`app/test/web` 条目，且二次执行 `go mod tidy` / `pnpm install --frozen-lockfile` 无新增 diff。
- [x] **AC-6（对应 REQ-6）**：`python3 ./.trellis/scripts/get_context.py --mode packages` 不再列出 `example`、`test`；全仓检索 `app/example`、`app/test`、`examplepb`、`10080`、`10090`、`IAM_TEST_WEB` 在非历史归档文件中无残留引用。
