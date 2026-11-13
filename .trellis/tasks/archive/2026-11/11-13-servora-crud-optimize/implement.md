# 实施计划

需求：[PRD](prd.md) · 设计：[技术设计](design.md)。

## 1. `servora` 框架与生成器改造

- [x] 在 `../servora/core/crud/mapper/mapper.go` 实现单层/多层嵌套 singular Message `WithFieldMapping` 与 `WithFlatten` 构造期校验及按需初始化投影；在 `mapper_test.go` 补充嵌套映射、单层/多层拍平、全空保持 `nil`、部分非空初始化、自定义 converter、同名顶层防冲突及非法路径拒绝单测。
- [x] 在 `../servora/cmd/protoc-gen-servora-crud/generate_go.go` 将 `Parse<Resource>Name` 与 `Validate()` 的错误改为 `crudv1.ErrorCrudErrorReasonInvalidResourceName(...)`；更新 `main_test.go` 与 `resource_name_conformance_test.go` 断言 `crudv1.IsCrudErrorReasonInvalidResourceName(err)`。
- [x] 在 `../servora` 执行 `go install ./cmd/protoc-gen-servora-crud` 更新本机插件二进制，运行 `just gen && just gen-ts && go test ./core/crud/... ./cmd/protoc-gen-servora-crud/... ./contrib/db/entgo/crud/... && just lint-go && just lint-proto`。

## 2. `plateau` 重生成、样板清理与规范同步

- [x] 在 `plateau` 执行 `just api` 重新生成 `api/gen/go/{iam,example}/.../*_crud.pb.go`。
- [x] 修改 `app/iam/service/internal/data/user.go` 使用 1 行 `crudmapper.WithFlatten(userpb.UserFields.Profile)` 替代 `WithPostToDTOHook`。
- [x] 清理 `app/iam/service/internal/service/user.go`、`app/admin/service/internal/service/user.go`（同步移除冗余 `plan` 字段）、`app/example/service/internal/service/user.go` 中的重复资源名解析错误包装。
- [x] 更新 `.trellis/spec/servora/framework/crud.md`、`.trellis/spec/servora/cmd/plugins.md`，并从 `TODO.md` 移除已完成的 `## Servora CRUD 生态` 两条待办。
- [x] 在工作区模式（默认开启 `../go.work`）下运行全量测试与 lint 验证：
  ```bash
  CC=/usr/bin/clang go test -count=1 ./app/iam/service/... ./app/admin/service/... ./app/example/service/...
  just lint
  ```
  （注：因依赖未发版的 `../servora` 本地改动，独立 `GOWORK=off` 门禁留待后续发版 `servora` 并升级 `plateau/go.mod` 时执行。）
