# 后端布局

每个服务是根 `go.work` 纳管的独立 Go module，位于 `app/{ServiceName}/service/`。完整目录职责来自 [app 服务结构](../../../../app/AGENTS.md)：

- `api/protos/` 同时放服务领域 Proto 和私有配置，均按所属领域组织；IAM 现有配置源为 [`iam/conf/v1/config.proto`](../../../../app/iam/service/api/protos/iam/conf/v1/config.proto) 与 [`iam/oidc/conf/v1/config.proto`](../../../../app/iam/service/api/protos/iam/oidc/conf/v1/config.proto)。`api/buf.openapi.gen.yaml` 是服务 OpenAPI 配置。
- `cmd/server/` 是启动入口；`configs/local/` 与 `configs/docker/` 分别承载本地和容器配置。
- `internal/assets/` 放 OpenAPI 等内嵌产物；`internal/server`、`service`、`biz`、`data` 承担通用四层。
- Ent 的 schema 与生成目录在 `internal/data/schema`、`internal/data/ent`；`generate.go` 是生成入口，不能手改 `ent/`。

新增应用以 [Example 服务](../../../../app/example/service/) 为起点，保留服务自己的 `go.mod`、`justfile`、配置和 API。根生成流程与 Go、TypeScript、OpenAPI、Wire、Ent 产物所有权遵循 [API 生成规范](../../api/proto/generation.md)；服务 leaf 的生成细节以自身 `justfile` 为准。

## 应用专有模块

不能硬把独立领域塞进四层。像 IAM 现有的 [oidc](../../../../app/iam/service/internal/oidc/)、[authn](../../../../app/iam/service/internal/authn/)、[authz](../../../../app/iam/service/internal/authz/)、[mail](../../../../app/iam/service/internal/mail/) 和 [startup](../../../../app/iam/service/internal/startup/) 与四层同级放在 `internal/`。它们仍须有明确职责和依赖边界；领域规则写入应用规范，不提升为平台共享能力。

## `internal/server` 的生产文件

通常只有 `server.go`、`grpc.go`、`http.go`，按实际服务端职责再增加 `sse.go`、`asynq.go` 等。Example 的 [server.go](../../../../app/example/service/internal/server/server.go)、[http.go](../../../../app/example/service/internal/server/http.go)、[grpc.go](../../../../app/example/service/internal/server/grpc.go) 是现有样式。测试文件可按测试职责同包存在；此约定不要求删除测试。
