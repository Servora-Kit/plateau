# `example` 与 `test` 服务全仓耦合盘点

## 1. 待移除目录与生成产物（共 144 个 Git 跟踪文件）

| 路径 | 类型 | 说明 |
| --- | --- | --- |
| `app/example/` | 服务源码 + 前端 + `AGENTS.md` | 包含 `service/`、`web/`、`AGENTS.md`；含全仓唯一服务级 `api/buf.typescript.gen.yaml` |
| `app/test/` | 服务源码 + 前端 | 包含 `service/`、`web/`（Next.js 构建验证入口） |
| `api/gen/go/example/`、`api/gen/go/test/` | Go Proto 生成物 | 移除 `buf.yaml` 模块后由 `just gen-fresh` 整体重建清理 |
| `api/gen/ts/example/`、`api/gen/ts/test/` | TS Proto 生成物 | 同上；`api/gen/dist/` 为 0 个 Git 跟踪文件的本地旧残留，也会随 `just gen-fresh` 清除 |
| `.trellis/spec/example/` | 应用规范（6 文件） | `index.md`、`backend.md`、`frontend.md`、`requests.md`、`user-crud.md`、`forms.md` |
| `.trellis/spec/test/` | 应用规范（2 文件） | `index.md`、`frontend.md` |

## 2. 真实代码与配置耦合点

| 文件 | 行号 | 耦合内容 | 处理方式 |
| --- | --- | --- | --- |
| `web/packages/client/test/kratos_stream_fixture.go` | L16, L161-L173 | 导入 `api/gen/go/example/service/v1`，仅用 `examplepb.DeleteUserRequest` 接收 WebSocket JSON 帧字段 `name`/`etag`/`allowMissing` | 改用同文件已导入的 `structpb.Struct` 解析字段，彻底解耦应用域 Proto |
| `app/iam/service/configs/local/oidc.yaml` | L6-L16 | 注释掉的 `test-web` OIDC 客户端（回调 `http://localhost:10092/callback`） | 删除遗留注释块 |
| `app/iam/service/configs/docker/oidc.yaml` | L6-L16 | 同上 | 删除遗留注释块 |
| `app/iam/service/cmd/server/config_test.go` | L20 | `t.Setenv("IAM_TEST_WEB_CLIENT_SECRET", ...)`，无配置项消费 | 删除该行 |

> 保留项确认：`app/iam/service/internal/oidc/provider_test.go:50` 的 `testClientID = "test-web"` 配合内存夹具 `http://localhost:3001/callback`，与 `app/test/web` 无关，保持不动。`github.com/mattn/go-sqlite3` 仍被 `app/iam/service/internal/data/data_test.go`、`app/iam/service/internal/oidc/claims_test.go`、`security/session/postgres_test.go` 使用，继续保留。

## 3. 构建、Buf 与 Just 注册点

| 文件 | 行号 | 内容 | 处理方式 |
| --- | --- | --- | --- |
| `buf.yaml` | L6-L7 | `app/example/service/api/protos`、`app/test/service/api/protos` | 移除两项 |
| `just/services.just` | L5, L7, L13-L14, L28, L30, L32-L34 | `mod example`、`mod test` 及聚合 recipe 引用 | 移除 `example` 与 `test` |
| `just/webs.just` | L4-L5, L8, L10 | `mod example`、`mod test` 及 `build`/`lint` 引用 | 移除 `example` 与 `test` |
| `justfile` | L137 | `compose-build: (_compose-build-image "example") (_compose-build-image "audit")` | 移除 `example`，保留 `audit` |
| `just/service.just` | L16, L46, L48-L50 | `TYPESCRIPT_TEMPLATE` 与服务级 `api-ts`（全仓仅 `app/example` 存在该模板） | 移除死分支，服务级 `api` 仅调用 `_root-api-go` |

## 4. 依赖与工作区检查

- `pnpm-workspace.yaml` 使用通配符 `app/*/web`（排除 `!app/admin/web`），无需改动路径配置，但需在仓库根目录执行 `pnpm install` 刷新 `pnpm-lock.yaml`，移除 `@plateau/example-web`（Vue/Vite/oxlint/vitest）与 `@plateau/test-web`（Next.js/React/Tailwind）专属依赖项。
- `go.mod` / `go.sum` 需在 `GOWORK=off` 下执行 `go mod tidy` 确认是否可裁剪只被 `example`/`test` 使用的依赖。
- 父目录 `../go.work`（`./plateau`）与 `../pnpm-workspace.yaml`（`plateau/app/*/web`）均为通配符且不受本仓库 Git 跟踪，无需修改。

## 5. 文档、规范与注册表更新点

| 文件 | 行号 | 需更新内容 |
| --- | --- | --- |
| `.trellis/config.yaml` | L179-L184 | 移除 `example` 与 `test` 的 package 注册 |
| `.trellis/spec/api/proto/generation.md` | L7 | `(含 iam、admin、example、test、audit)` 改为 `(含 iam、admin、audit)` |
| `AGENTS.md` | L7, L15-L16, L42, L57, L59, L65 | 移除 Example/Test 端口行与 10080/10090 描述；`五个 Go 后端` 改为 `各服务 Go 后端`；移除 `just web::example::dev`/`just web::test::dev`；新增服务参考改为 `app/iam`、`app/admin` 与 `app/AGENTS.md` |
| `app/AGENTS.md` | L12, L14, L35-L37, L70, L79-L80 | 移除 `example`/`test` 目录项；移除服务级 `buf.typescript.gen.yaml` 说明；常用命令示例改为 `iam`/`admin` |
| `api/AGENTS.md` | L14 | 移除 Example 独立 TS 输出与 `just service::example::api-ts` 说明 |

## 6. 范围外发现（独立仓库 / 显式保留）

- `app/audit/`（含 `docker-compose.apps.yaml`、`justfile` `compose-build` 中的 `audit`、`.trellis/spec/audit/`、`infra/clickhouse/`）：按用户指示保留。
- `../servora/docs/crud.md`（L106, L603, L616, L624）：位于独立的 `servora` 仓库，文中引用 `plateau/app/example` 作为黄金路径示例；本任务仅改动 `plateau` 仓库，列为延后跨仓跟进项。
- `cmd/protoc-gen-plateau-authn/main_test.go`、`cmd/protoc-gen-plateau-authz/main_test.go`、`docs/iam-security.md` 与各单元测试中的 `example.com` / `examplev1`：属于标准占位域名与插件单测虚构包名，与 `app/example` 无关，保持不动。
