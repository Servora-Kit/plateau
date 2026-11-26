# `protoc` 插件

插件生成 descriptor 辅助文件，不生成应用 service、biz、data、repository、事务或授权代码。公共注解语义见 [Proto annotations](../proto/annotations.md)。

`protoc-gen-servora-crud` 只接受 `target=go|ts`：Go 输出 descriptor、typed field path 和资源名 helper（`Parse<Resource>Name`/`Validate()` 失败直接返回纯协议包 `crud/v1` 的 `400 CRUD_ERROR_REASON_INVALID_RESOURCE_NAME`），TS 输出资源名、字段路径和 update-field helper。生成期严格校验 resource 注解与标准 CRUD 方法，不 import `core/crud`；multi-pattern Create/List 不能按声明顺序暗选 pattern。

`protoc-gen-servora-conf` 的 `Apply() error` 契约见 [配置处理](../proto/annotations.md#配置处理契约)。

`protoc-gen-go-errors target=ts` 只为显式 Servora error option 的顶层 enum 生成 source-relative `*.errors.ts`；以 `import type ... from './index.js'` 复用同目录 enum，输出同名 membership object 与 `isXxx` guard，不生成 HTTP client、Kratos runtime、message、重试或 UI 行为。Go/TS target 都拒绝 100..599 以外的 HTTP code。

所有插件均由 `go.mod` 的 `tool` 块固定版本并经 `go tool` 调用，禁止通过 `@latest` 或手动 `go install` 覆盖插件二进制。插件实现改动先运行本插件 Go 测试（`GOWORK=off go test -race ./cmd/<plugin>/...`），再运行 `just gen` 与 `GOWORK=off just gen-check`；涉及 TypeScript HTTP、error 或 CRUD 输出时还需运行 `just web-check`、`just web-typecheck`、`just web-test`。保持错误包含方法或字段路径，便于定位不合法 Proto。
