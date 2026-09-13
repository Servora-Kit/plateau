# 平台安全生成命令

根 `cmd/` 当前的两个 protoc 插件和 `internal/codegen/` 共同属于平台代码生成能力，不单设 cmd spec package。服务自己的 cmd/server 见 [bootstrap](../../service/backend/bootstrap.md)，母框架命令见 [servora/cmd](../../servora/cmd/index.md)。

## 插件契约

| 插件 | 输出与入口 |
| --- | --- |
| protoc-gen-plateau-authn | 每个有规则的 Go 输出目录生成 authn_rules.gen.go，公开 AuthnRules() |
| protoc-gen-plateau-authz | 同理生成 authz_rules.gen.go，公开 AuthzRules() |

两者使用 protogen、支持 proto3 optional，经 [ruleplan](ruleplan.md) 合并规则和确定分组。输出由 operation 排序；公开函数每次返回新 map 和克隆规则，调用方改返回值不能污染下一次调用。

插件从当前 checkout 本地安装：`go install ./cmd/protoc-gen-plateau-authn` 和 `go install ./cmd/protoc-gen-plateau-authz`，由根 `just plugin` 纳入工具安装，Buf 经 [generation](../../api/proto/generation.md) 调用。修改本地源码后只运行旧安装二进制不能验证变更。

## 校验归属

AuthN 检查声明 mode。AuthZ 除检查 mode，还验证合并后 REQUIRED 规则的 action、resource_type 和唯一目标，沿 RPC input descriptor 校验字段路径。未知枚举即使出现在被覆盖声明中也不能跳过检查。命令源码含实际生成和领域校验，不把它描述成只解析参数的薄入口。

测试入口：`go test ./cmd/protoc-gen-plateau-authn ./cmd/protoc-gen-plateau-authz`。检查合并、未知 mode、字段路径、无规则无输出、冲突 Go package、输出排序与返回值隔离。测试通过后仍需在实际 Proto 变更任务中检查生成 diff，不手改产物。

依据：[AuthN main](../../../../cmd/protoc-gen-plateau-authn/main.go)、[AuthZ main](../../../../cmd/protoc-gen-plateau-authz/main.go)、[AuthN tests](../../../../cmd/protoc-gen-plateau-authn/main_test.go)、[AuthZ tests](../../../../cmd/protoc-gen-plateau-authz/main_test.go)。
