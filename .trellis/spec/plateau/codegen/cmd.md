# 平台安全生成命令

根 `cmd/` 的安全 protoc 插件和 `internal/codegen/` 属于平台代码生成能力。应用启动入口见 [bootstrap](../../service/backend/bootstrap.md)，框架命令见 [servora/cmd](../../servora/cmd/index.md)。

## 插件契约

| 插件 | 输出与入口 |
| --- | --- |
| protoc-gen-plateau-authn | 每个有规则的 Go 输出目录生成 authn_rules.gen.go，公开 AuthnRules() |
| protoc-gen-plateau-authz | 同理生成 authz_rules.gen.go，公开 AuthzRules() |

两者使用 protogen、支持 proto3 optional，经 [ruleplan](ruleplan.md) 合并规则和确定分组。输出由 operation 排序；公开函数每次返回新 map 和克隆规则，调用方改返回值不能污染下一次调用。

插件从当前 checkout 本地安装：`go install ./cmd/protoc-gen-plateau-authn` 和 `go install ./cmd/protoc-gen-plateau-authz`，由根 `just plugin` 纳入工具安装，Buf 经 [generation](../../api/proto/generation.md) 调用。修改本地源码后只运行旧安装二进制不能验证变更。

## 校验归属

AuthN 检查声明 mode。AuthZ 还校验合并后 REQUIRED 规则的 action、resource_type 和唯一目标，沿 RPC input descriptor 校验字段路径。被覆盖声明中的未知枚举也必须检查。

检查合并、未知 mode、字段路径、无规则无输出、冲突 Go package、输出排序与返回值隔离。入口：`go test ./cmd/protoc-gen-plateau-authn ./cmd/protoc-gen-plateau-authz`；同时审阅生成 diff，不手改产物。
