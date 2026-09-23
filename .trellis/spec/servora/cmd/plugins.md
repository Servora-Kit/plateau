# `protoc` 插件

插件从 Proto descriptor 生成 辅助生成文件，不生成业务 service、biz、data、repository、事务或授权代码。公共注解的 owner 和语义见 [Proto annotations](../proto/annotations.md)。

`protoc-gen-servora-crud` 的入口 [`main.go`](../../../../../servora/cmd/protoc-gen-servora-crud/main.go) 只接受 `target=go|ts`：Go 输出 descriptor、typed field path 和资源名 helper，TypeScript 输出资源名、字段路径和 update-field helper；完整 resource 注解和标准 CRUD 方法在生成期严格校验。它不 import `core/crud`，multi-pattern Create/List 也不能按声明顺序暗选 pattern，详见 [`cmd/protoc-gen-servora-crud/AGENTS.md`](../../../../../servora/cmd/protoc-gen-servora-crud/AGENTS.md)。

`protoc-gen-servora-audit` 用 `ruleplan.Build` 合并 method `rule` 与 service default，只输出 merged mode 为 enabled 的规则，见 [`protoc-gen-servora-audit/main.go`](../../../../../servora/cmd/protoc-gen-servora-audit/main.go)。`protoc-gen-servora-conf` 根据配置注解生成唯一的 `Apply() error`，完成必填检查、默认值和子配置处理，见 [`protoc-gen-servora-conf/main.go`](../../../../../servora/cmd/protoc-gen-servora-conf/main.go)。`protoc-gen-go-errors` 的 Go/TS target 与 HTTP code 校验边界由 [`cmd/protoc-gen-go-errors/AGENTS.md`](../../../../../servora/cmd/protoc-gen-go-errors/AGENTS.md) 定义。

`protoc-gen-go-errors target=ts` 只为显式 Servora error option 的顶层 enum 生成 source-relative `*.errors.ts`：它用 `import type ... from './index.js'` 复用同目录生成 enum，并输出同名 membership object 与 `isXxx` guard。辅助生成文件 不生成 HTTP client、Kratos runtime、message、重试或 UI 行为；两个 target 都拒绝 100..599 以外的 HTTP code，依据 [`generate_ts.go`](../../../../../servora/cmd/protoc-gen-go-errors/generate_ts.go) 与 [`targets_test.go`](../../../../../servora/cmd/protoc-gen-go-errors/targets_test.go)。

插件实现改动先运行本插件 Go 测试，再运行 `just plugin` 与受影响的 Go/TS 生成；CRUD 还需 `just gen`、`just gen-ts`，TypeScript HTTP/error 输出再运行 web typecheck/build。保持错误包含方法或字段路径，便于定位不合法 Proto。
