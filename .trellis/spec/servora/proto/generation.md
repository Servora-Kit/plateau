# Proto 生成与生成物边界

根 [`buf.yaml`](../../../../../servora/buf.yaml) 以 `api/protos` 为唯一输入并启用 FILE breaking 检查，目录内不新增独立 `buf.yaml`/`buf.lock`。Go 插件由 [`buf.go.gen.yaml`](../../../../../servora/buf.go.gen.yaml) 配置，输出到 `api/gen/go`。

`api/gen/go` 是根 Go module 内的生成 package，禁止手改。日常 `just gen` 不 clean；删除/重命名 Proto 或移除 plugin 时使用 `just gen-fresh` 清理重建。

[`buf.typescript.gen.yaml`](../../../../../servora/buf.typescript.gen.yaml) 会 clean 并把 TypeScript HTTP、error 和 CRUD companion 写入 `web/packages/proto-utils/src/gen`，该目录同样不可手改。生成 shape 变化时，检查 plugin 测试、生成 diff 和下游 conformance/web 测试；Proto 或生成物的发布顺序见 [`../servora/AGENTS.md`](../../../../../servora/AGENTS.md)，本规范不授权发布。
