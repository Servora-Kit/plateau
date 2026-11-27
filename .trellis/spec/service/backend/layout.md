# 后端布局

服务位于 `app/*/service/`，共享根 Go module，但各自是独立二进制与部署单元。目录职责见 [app 服务结构](../../../../app/AGENTS.md)：

- 源 Proto（业务接口与私有配置）统一集中在根 `api/protos/plateau/<service>/**`；服务目录不再保留 `app/*/service/api/` 目录，各服务统一经仓库根级唯一 `buf.openapi.gen.yaml` 按 `api/protos/plateau/<service>` 过滤生成 `internal/assets/openapi.yaml`。
- `cmd/server/` 是启动入口；`configs/local/` 与 `configs/docker/` 分别承载本地和容器配置。
- `internal/assets/` 放 OpenAPI 等内嵌产物；`internal/server`、`service`、`biz`、`data` 承担通用四层。
- Ent schema 属于手写持久化模型；Ent/Wire 产物通过根 module 固定版本的 `go tool ent`、`go tool wire` 生成，不手改。

服务保留自己的 `justfile` 与配置，依赖统一由根 `go.mod` 管理。生成入口与产物所有权遵循 [API 生成规范](../../api/proto/generation.md)，服务细节以自身 `justfile` 为准。

## 应用专有模块

独立的应用模块可与四层同级放在 `internal/`，保持明确职责与依赖边界。领域规则留在应用规范，不提升为平台共享能力。

## `internal/server` 的生产文件

以 `server.go`、`grpc.go`、`http.go` 为基本划分，仅按新增 transport 职责扩展文件；测试文件可同包存在。
