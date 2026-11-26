# `protoc` 插件

插件生成 descriptor 辅助文件，不生成应用 service、biz、data、repository、事务或授权代码。公共注解语义见 [Proto annotations](../proto/annotations.md)。

AIP CRUD 生成插件已迁入 Plateau 为 `cmd/protoc-gen-plateau-crud`（见 [Plateau codegen](../../plateau/codegen/cmd.md)）；`../servora/cmd/` 保留微内核通用插件 `protoc-gen-servora-conf`、`protoc-gen-go-errors`、`protoc-gen-redact` 与 `protoc-gen-typescript-http`。
`protoc-gen-servora-conf` 的 `Apply() error` 契约见 [配置处理](../proto/annotations.md#配置处理契约)。

`protoc-gen-go-errors target=ts` 只为显式 Servora error option 的顶层 enum 生成 source-relative `*.errors.ts`；以 `import type ... from './index.js'` 复用同目录 enum，输出同名 membership object 与 `isXxx` guard，不生成 HTTP client、Kratos runtime、message、重试或 UI 行为。Go/TS target 都拒绝 100..599 以外的 HTTP code。

所有插件均由 `go.mod` 的 `tool` 块固定版本并经 `go tool` 调用，禁止通过 `@latest` 或手动 `go install` 覆盖插件二进制。插件实现改动先运行本插件 Go 测试（`GOWORK=off go test -race ./cmd/<plugin>/...`），再运行 `just gen` 与 `GOWORK=off just gen-check`；涉及 TypeScript HTTP 或 error 输出时还需运行 `just web-check`、`just web-typecheck`、`just web-test`。保持错误包含方法或字段路径，便于定位不合法 Proto。
