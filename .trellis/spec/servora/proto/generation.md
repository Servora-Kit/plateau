# Proto 生成与产物归属

根 [`buf.yaml`](../../../../../servora/buf.yaml) 以 `api/protos` 为唯一输入并启用 FILE breaking 检查，目录内不新增独立 `buf.yaml`/`buf.lock`。所有外部与仓内 `protoc` 插件统一声明在根 [`go.mod`](../../../../../servora/go.mod) 的 `tool` 块中，[`buf.go.gen.yaml`](../../../../../servora/buf.go.gen.yaml) 与 [`buf.typescript.gen.yaml`](../../../../../servora/buf.typescript.gen.yaml) 均通过 `local: ["go", "tool", "protoc-gen-*"]` 调用。

`api/gen/go` 是根 Go module 内的生成 package，禁止手改。`just gen` 串联 `just gen-go` 与 `just gen-ts`，一次执行同步刷新 Go 与 TypeScript 生成物；日常增量生成不清空 `api/gen/go`，删除/重命名 Proto 或移除插件时使用 `just gen-fresh`（先清理生成目录并引导生成基础 `*.pb.go`，再运行完整 `gen`）。

[`buf.typescript.gen.yaml`](../../../../../servora/buf.typescript.gen.yaml) 开启 `clean: true`，把 TypeScript HTTP、error 和 CRUD companion 写入 `web/packages/proto-utils/src/gen`，该目录同样不可手改；`protoc-gen-typescript-http` 按文件实际需要从 `@servora/proto-utils/transport` 导入传输类型与路径编码函数，不内联重复运行时。

## 生成漂移门禁

`just gen-check`（本地 `pre-push` 钩子与 CI `gen` 任务共用）固定在 `GOWORK=off` 下运行 `just gen`，并对 `api/gen/go` 与 `web/packages/proto-utils/src/gen` 同时校验 `git diff --exit-code` 与未跟踪文件。生成 shape 变化时，同步检查插件测试、生成 diff、下游 conformance 以及 `just web-check` / `just web-typecheck` / `just web-test`；Proto 或生成物的发布顺序见 [`../servora/AGENTS.md`](../../../../../servora/AGENTS.md)，本规范不授权发布。
