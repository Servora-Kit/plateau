# Plateau 项目结构

适用于新增目录、共享包、应用或生成入口。正文以根 AGENTS 的“目录结构”为基础，并按当前 checkout 核对；目录存在不代表相应服务已经验收。

## 目录与文件归属

- `api/`：平台领域 Proto 与共享生成产物。
  - `protos/`：`api/protos/plateau/**` 下的平台 CRUD、安全与基础设施定义。
  - `gen/go/`：所有 Buf workspace 微服务的 Go Proto 输出。
  - `gen/ts/`：共享 TypeScript HTTP client、error reason 与 CRUD helper 输出。
  - `gen/package.json`：`@plateau/api` 的依赖及 exports；`gen/go/` 属于仓库根 Go module。
- `app/<应用>/service|web`：平台业务端；服务目录与职责见 [layout](../../service/backend/layout.md) 和 [layers](../../service/backend/layers.md)。
- `cmd/protoc-gen-plateau-authn/`、`cmd/protoc-gen-plateau-authz/`、`cmd/protoc-gen-plateau-audit/`、`cmd/protoc-gen-plateau-crud/`：平台安全与 CRUD 生成器，在根 `go.mod` `tool` 声明并由 Buf 调用。
- `crud/`：AIP CRUD 运行时、`crud/mapper` 读投影及 `crud/testdata/` 跨 Go/TS 一致性向量与 `examplepb` 测试夹具。
- `internal/codegen/`：`optionmerge`、`ruleplan`、`plugintest`、`protoreach` 共享实现；命令入口和校验见 [codegen](../codegen/index.md)。
- `security/`：`actor.go`、`identity.go`、`loopback.go`、`rules.go`、`authn/`、`authz/`、`audit/`、`oidc/`、`cap/`、`password/`、`jwt/`、`session/`。公共安全错误源在 `api/protos/plateau/security/errors/v1/`，当前没有手写 `security/errors/`。
- `infra/`：共享基础设施（`openfga/`、`clickhouse/`、`kafka/`、`redis/`、`cache/`、`entgo/` 通用 driver/tracing/crud/mixin、`gorm/`）。
- `transport/server/kafka/`：与 `servora/transport/server/{http,grpc}` 同构的非泛型 Kafka 入站批消费 `transport.Server`。
- `web/packages/plateau-utils/`：单一公开前端工具包 `@servora/plateau-utils`（`client`、`crud`、`error-reasons/*`），规则见 [plateau-utils](../../web/plateau-utils/index.md)。
- `just/`：共享 Just settings、应用注册和 service/web 命令实现；根 `justfile` 是项目入口。
- `manifests/`：scripts（含 `openfga.{sh,ps1}` 与 postgres 初始化）、openfga（模型与 `tuples/platform-services.yaml` 平台服务元组）、traefik（`traefik.yaml` 启用 Consul Catalog + File，`routes.yaml` 仅作纯前端兜底，Go 后端路由归各自 `bootstrap.yaml` 的 `registry.consul.tags`）、grafana、prometheus、otel、loki 等部署资源。
- `docs/adr/`：架构决策；任务研究、迁移差异留在 `.trellis/tasks/`。
- `pnpm-workspace.yaml`、`pnpm-lock.yaml`：根依赖 workspace 和共享锁文件；独立第三方 workspace 保留自己的依赖边界。
- `buf.yaml`：模块、依赖、lint 和 breaking 配置；`buf.go.gen.yaml`、`buf.typescript.gen.yaml`：统一生成模板。`buf.es.gen.yaml` 当前停用，只作参考。
- `go.mod`、`go.sum`：统一管理共享代码与所有服务后端；父级 `go.work` 仅用于本机跨仓联调，仓库不跟踪它。
- `docker-compose.yaml`：本地基础设施编排。
- `process-compose.yaml`：本地全栈开发进程拓扑、健康探针与内置 SSE MCP Server 配置；项目级 MCP 客户端配置位于 `.mcp.json`。
- `.trellis/`：规范、任务和工作记录；应用 spec 按 `spec/<应用>/{index,architecture,frontend,backend}.md` 及实际专题组织，文件按需建立，规则见 [规范索引](../../index.md)。一个应用 package 可以同时包含 Go 与前端源码，不要求等同单个 Go/Node 包。

## 新增目录时检查

先判断属于平台、框架、共通使用方式还是应用专有职责。新增应用同步模块注册和端口登记，目录存在不等于能力已实现。

来源：[根 AGENTS](../../../../AGENTS.md)、[API 约定](../../../../api/AGENTS.md)、[go.mod](../../../../go.mod)、[pnpm workspace](../../../../pnpm-workspace.yaml)、[API exports](../../../../api/gen/package.json)。
