# `protoc` 插件

插件生成 descriptor 辅助文件，不生成应用 service、biz、data、repository、事务或授权代码。公共注解语义见 [Proto annotations](../proto/annotations.md)。

`protoc-gen-servora-crud` 只接受 `target=go|ts`：Go 输出 descriptor、typed field path 和资源名 helper，TS 输出资源名、字段路径和 update-field helper。生成期严格校验 resource 注解与标准 CRUD 方法，不 import `core/crud`；multi-pattern Create/List 不能按声明顺序暗选 pattern。

`protoc-gen-servora-audit` 用 `ruleplan.Build` 合并 method rule 与 service default，只输出 merged mode 为 enabled 的规则。`protoc-gen-servora-conf` 的 `Apply() error` 契约见 [配置处理](../proto/annotations.md#配置处理契约)。

`protoc-gen-go-errors target=ts` 只为显式 Servora error option 的顶层 enum 生成 source-relative `*.errors.ts`；以 `import type ... from './index.js'` 复用同目录 enum，输出同名 membership object 与 `isXxx` guard，不生成 HTTP client、Kratos runtime、message、重试或 UI 行为。Go/TS target 都拒绝 100..599 以外的 HTTP code。

插件实现改动先运行本插件 Go 测试，再运行 `just plugin` 与受影响的 Go/TS 生成；CRUD 还需 `just gen`、`just gen-ts`，TypeScript HTTP/error 输出再运行 web typecheck/build。保持错误包含方法或字段路径，便于定位不合法 Proto。
