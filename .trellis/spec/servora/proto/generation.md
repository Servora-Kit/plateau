# Proto 生成与生成物边界

根 [`buf.yaml`](../../../../../servora/buf.yaml) 以 `api/protos` 为唯一模块输入并启用 FILE breaking 检查；该目录内不能新增独立 `buf.yaml` 或 `buf.lock`。Go 生成由 [`buf.go.gen.yaml`](../../../../../servora/buf.go.gen.yaml) 调用 Go、gRPC、Kratos HTTP/errors、validate、redact、CRUD、audit 和 conf 插件，写入 `api/gen/go`。

`api/gen/go` 是独立 Go module，只由 `just gen`/Buf 维护，禁止手改。日常 `just gen` 不 clean；删除/重命名 Proto 或移除 plugin 时，`just gen-fresh` 才显式清理后重建，规则来自 [`api/AGENTS.md`](../../../../../servora/api/AGENTS.md)。

[`buf.typescript.gen.yaml`](../../../../../servora/buf.typescript.gen.yaml) 会 clean 并把 TypeScript HTTP、error 和 CRUD companion 写入 `web/packages/proto-utils/src/gen`，该目录同样不可手改。生成 shape 变化时，检查 plugin 测试、生成 diff 和下游 conformance/web 测试；Proto 或生成物的发布顺序见 [`../servora/AGENTS.md`](../../../../../servora/AGENTS.md)，本规范不授权发布。
