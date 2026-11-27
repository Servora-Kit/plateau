# 生成流程与产物归属

所有改动从源 Proto 或生成器进入。生成目录可整体重建，不容纳手写 transport、业务 helper 或应用状态。

## 现有流程

根 `just gen` 先执行 `api`（`api-go`、`api-ts`），再执行注册服务的 `_gen`（OpenAPI、Wire、Ent）。根 `api/protos/` 作为单一模块（`buf.build/servora/plateau`）承载全部 29 个 `plateau.*.*.v1` 契约（含共享基础设施/安全/CRUD 与 `iam`、`admin`、`audit` 服务及配置），统一生成至共享目录，不再为单个服务重复维护独立的 Proto 目录、`api/buf.openapi.gen.yaml` 或 `buf.typescript.gen.yaml` 产物。

| 源／模板 | 输出与维护方 |
| --- | --- |
| 根 `buf.yaml` 单一模块 `api/protos` + `buf.go.gen.yaml` | `api/gen/go`：统一 Go 生成产物 |
| 同一模块 + `buf.typescript.gen.yaml` | `api/gen/ts`（`@plateau/api` 全量客户端/sidecar）与 `web/packages/plateau-utils/src/gen`（通过插件级 `types` 过滤生成 `@servora/plateau-utils/error-reasons/*` 公共错误枚举） |
| 根级唯一 `buf.openapi.gen.yaml`（`out: internal/assets`，`just/service.just` 以 `--path api/protos/plateau/<service> -o app/<service>/service` 调用） | 全部 3 个服务（`iam`、`admin`、`audit`）各自的 `internal/assets/openapi.yaml`（均纳入版本控制并由 `just gen-check` 校验） |
| 服务 Wire／Ent 源 | 相应服务 `cmd/server`、`internal/data` 中的生成输出 |

生成模板只清理自身输出目录；停用的参考模板（如 `buf.es.gen.yaml`）不属于实际生成链。

## 包和工具边界

`api/gen/go` 属于仓库根 Go module，import path 继续以 `github.com/Servora-Kit/plateau/api/gen/go` 开头。TS `@plateau/api` 直接通过 wildcard exports 暴露 ts 源：service index、`*.errors`、`*.crud`；当前 tsconfig 是 `noEmit: true`，并不构建 dist。依赖和锁文件由根 pnpm workspace 维护。

所有 protoc 插件（含 Plateau AuthN/AuthZ/Audit/CRUD 与 Servora 通用插件）及 Wire/Ent 均由根 `go.mod` 的 `tool` 声明固定，buf 模板通过 `local: ["go", "tool", "protoc-gen-*"]` 调用；Servora 插件版本随 `go.mod` 中的 `github.com/Servora-Kit/servora` 自动同步，不再单独安装插件或维护 `SERVORA_VERSION`。父级 `go.work` 仅用于本机跨仓源码联调，`just gen` 沿用当前 Go 环境；`just gen-check`（pre-push 与 CI）固定在 `GOWORK=off` 下执行，用于验证仓库独立可复现性。

`@plateau/api` 消费 `@servora/proto-utils` 与 `@servora/plateau-utils/crud`，手写 transport 与平台公共错误导出属于 [web/plateau-utils](../../web/plateau-utils/index.md)。Go/TS 生成目录不设独立 spec package。

当前 TS 模板用 `protoc-gen-typescript-http` 生成 service client，`protoc-gen-go-errors target=ts,paths=source_relative` 生成错误 sidecar，`protoc-gen-plateau-crud target=ts,paths=source_relative` 生成 CRUD sidecar。三个职责分开维护，HTTP client 不重复生成错误枚举或 CRUD 元数据。sidecar 的 ESM import、ProtoJSON 类型及 runtime 纯度由 [Servora TS 契约](../../servora/web/proto-utils.md) 维护；应用文案留在消费方，生成器不写业务提示文本。

## 验证

Proto 修改后运行生成（`just gen`）、生成一致性校验（`just gen-check`）、Buf lint（`just lint-proto`）、Proto 格式检查（`just fmt-proto-check`）、API TS 检查（`just api-ts-check`）及受影响消费者检查，按需手动执行破坏性变更检查（`just breaking`；`0.x` 阶段关闭自动拦截，待 `v1.0.0` 恢复）。`just gen-check` 在 `GOWORK=off` 下重新生成并对生成路径（`api/gen/{go,ts}`、`web/packages/plateau-utils/src/gen`、`**/wire_gen.go`、`app/*/service/internal/data/ent`、`app/*/service/**/openapi.yaml`）检查 `git diff --exit-code` 与未跟踪文件。共享检查不能替代应用检查。只读调查不执行有写文件副作用的生成命令。

入口：[根 justfile](../../../../justfile)、[服务命令](../../../../just/service.just)、[Go 模板](../../../../buf.go.gen.yaml)、[TS 模板](../../../../buf.typescript.gen.yaml)、[OpenAPI 模板](../../../../buf.openapi.gen.yaml)、[go.mod](../../../../go.mod)。
