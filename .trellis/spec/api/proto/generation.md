# 生成流程与产物归属

所有改动从源 Proto 或生成器进入。生成目录可整体重建，不容纳手写 transport、业务 helper 或应用状态。

## 现有流程

根 `just gen` 先执行 `api`（api-go、api-ts），再执行注册服务的 `_gen`（OpenAPI、Wire、Ent）。服务 leaf 的 `api` 则先回根生成 Go，再用服务自己的 TS 模板生成前端产物；根 `gen` 不执行这一步服务级 TS 生成。

| 源／模板 | 输出与维护方 |
| --- | --- |
| buf.yaml 中平台、Audit、Example、IAM 模块 + buf.go.gen.yaml | `api/gen/go`：Proto/gRPC/HTTP/errors/validate/redact、Plateau AuthN/AuthZ、Servora CRUD/audit/conf |
| 同一 workspace + buf.typescript.gen.yaml | `api/gen/ts`：HTTP client、TS errors、TS CRUD sidecar |
| Example 的 api/buf.typescript.gen.yaml | `app/example/web/src/api/generated`：Example 前端实际消费的独立 HTTP client、TS errors、TS CRUD sidecar |
| 各服务 api/buf.openapi.gen.yaml | 服务自己的 OpenAPI assets，例如 Example `internal/assets` |
| 服务 Wire／Ent 源 | 相应服务 cmd/server、internal/data 中的生成输出 |

Go、TS 根模板及 Example leaf TS 模板均为 `clean: true`，只清理各自输出目录；共享 TS 与 leaf TS 互不刷新，修改前确认消费者实际导入哪份产物。旧 `buf.es.gen.yaml` 停用，不把 Protobuf-ES 参考模板当作当前实际链路。

## 包和工具边界

`api/gen/go` 属于仓库根 Go module，import path 继续以 `github.com/Servora-Kit/plateau/api/gen/go` 开头。TS `@plateau/api` 直接通过 wildcard exports 暴露 ts 源：service index、`*.errors`、`*.crud`；当前 tsconfig 是 `noEmit: true`，并不构建 dist。依赖和锁文件由根 pnpm workspace 维护。

Plateau AuthN/AuthZ 插件从当前 checkout 本地安装；Servora 插件由 Just 配置安装对应版本。父级 `go.work` 的源码联调不能证明本机生成器二进制来自同一版本；变更框架生成器时明确实际调用来源，并同时检查生成契约。

`@plateau/api` 消费 `@servora/proto-utils`，手写 transport 属于 [web/client](../../web/client/index.md)。Go/TS 生成目录不设独立 spec package。

当前 TS 模板用 `protoc-gen-typescript-http` 生成 service client，`protoc-gen-go-errors target=ts,paths=source_relative` 生成错误 sidecar，`protoc-gen-servora-crud target=ts,paths=source_relative` 生成 CRUD sidecar。三个职责分开维护，HTTP client 不重复生成错误枚举或 CRUD 元数据。sidecar 的 ESM import、ProtoJSON 类型及 runtime 纯度由 [Servora TS 契约](../../servora/web/proto-utils.md) 维护；应用文案留在消费方，生成器不写业务提示文本。

## 验证

Proto 修改后运行 `just gen` 并审阅生成 diff，再运行 `just lint-proto`、`just api-ts-check` 及受影响消费者的检查。涉及 Example API 时，还需运行根入口 `just service::example::api-ts`（或在该服务 leaf 中运行 `just api-ts`），刷新其独立 TS 输出，并执行 Example Web 的 `pnpm --dir app/example/web type-check`。共享 TS 检查不能证明 leaf 产物已同步。只修改规范时不为证明命令存在而重建产物。OpenAPI、Wire、Ent 都有写文件副作用，不能混入只读调查。

来源：[根 justfile](../../../../justfile)、[service.just](../../../../just/service.just)、[Go 模板](../../../../buf.go.gen.yaml)、[TS 模板](../../../../buf.typescript.gen.yaml)、[TS exports](../../../../api/gen/package.json)、[tsconfig](../../../../api/gen/tsconfig.json)、[Example leaf TS 模板](../../../../app/example/service/api/buf.typescript.gen.yaml)、[Example 实际导入](../../../../app/example/web/src/api/userApi.ts)、[Example OpenAPI 模板](../../../../app/example/service/api/buf.openapi.gen.yaml)。
