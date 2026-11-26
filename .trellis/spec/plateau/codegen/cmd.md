# 平台代码生成命令

根 `cmd/` 的安全与 CRUD protoc 插件及 `internal/codegen/`（`optionmerge`、`ruleplan`、`plugintest`、`protoreach`）属于平台代码生成能力。应用启动入口见 [bootstrap](../../service/backend/bootstrap.md)，微内核通用插件见 [servora/cmd](../../servora/cmd/index.md)。

## 插件契约

| 插件 | 输出与入口 |
| --- | --- |
| protoc-gen-plateau-authn | 每个有规则的 Go 输出目录生成 authn_rules.gen.go，公开 AuthnRules() |
| protoc-gen-plateau-authz | 同理生成 authz_rules.gen.go，公开 AuthzRules() |
| protoc-gen-plateau-audit | 同理生成 audit_rules.gen.go，公开 AuditRules() |
| protoc-gen-plateau-crud | `target=go` 生成 `*_crud.pb.go`（descriptor、typed field path、资源名 helper，错误引用 `plateau/crud/v1`）；`target=ts` 生成 `*.crud.ts`（导入 `@servora/plateau-utils/crud`） |

三者使用 protogen、支持 proto3 optional，经 [ruleplan](ruleplan.md) 合并规则和确定分组。输出由 operation 排序；公开函数每次返回新 map 和克隆规则，调用方改返回值不能污染下一次调用。

插件统一在根 `go.mod` `tool` 声明，Buf 经 `go tool` 调用（见 [generation](../../api/proto/generation.md)）。

## 校验归属

AuthN 检查声明 mode；Audit 检查声明 mode 与 category。AuthZ 还校验合并后 REQUIRED 规则的 action、resource_type 和唯一目标，沿 RPC input descriptor 校验字段路径。被覆盖声明中的未知枚举也必须检查。

检查合并、未知枚举、字段路径、资源 pattern 冲突、无规则无输出、冲突 Go package、输出排序与返回值隔离。入口：`go test ./cmd/protoc-gen-plateau-authn ./cmd/protoc-gen-plateau-authz ./cmd/protoc-gen-plateau-audit ./cmd/protoc-gen-plateau-crud`；同时审阅生成 diff，不手改产物。
