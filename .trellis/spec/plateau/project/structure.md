# Plateau 项目结构

适用于新增目录、共享包、应用或生成入口。正文以根 AGENTS 的“目录结构”为基础，并按当前 checkout 核对；目录存在不代表相应服务已经验收。

## 目录与文件归属

- `api/`：平台领域 Proto 与共享生成产物。
  - `protos/`：`api/protos/plateau/**` 下的平台安全与基础设施定义。
  - `gen/go/`：所有 Buf workspace 微服务的 Go Proto 输出。
  - `gen/ts/`：共享 TypeScript HTTP client、error reason 与 CRUD helper 输出。
  - `gen/package.json`：`@plateau/api` 的依赖及 exports；`gen/go/` 属于仓库根 Go module。
- `app/<应用>/service|web`：平台业务端；服务目录与职责见 [layout](../../service/backend/layout.md) 和 [layers](../../service/backend/layers.md)。
- `cmd/protoc-gen-plateau-authn/`、`cmd/protoc-gen-plateau-authz/`：平台安全生成器，从当前 checkout 本地安装。
- `internal/codegen/`：`optionmerge`、`ruleplan`、`plugintest` 共享实现；命令入口和校验见 [codegen](../codegen/index.md)。
- `security/`：`actor.go`、`authn/`、`authz/`、`cap/`、`password/`、`jwt/`、`session/`。公共安全错误源在 `api/protos/plateau/security/errors/v1/`，当前没有手写 `security/errors/`。
- `infra/`：当前包含 `openfga/` 与 `clickhouse/`。Ent 适配属于 Servora 的 `contrib/db/entgo`，不能按历史目录说明补造平台 `infra/entgo` 或 `infra/errors`。
- `web/packages/client/`：平台共享前端通信能力，规则见 [web/client](../../web/client/index.md)。
- `just/`：共享 Just settings、应用注册和 service/web 命令实现；根 `justfile` 是项目入口。
- `manifests/`：scripts、openfga、grafana、prometheus、otel、traefik、loki 等部署资源。
- `docs/adr/`：架构决策；任务研究、迁移差异留在 `.trellis/tasks/`。
- `pnpm-workspace.yaml`、`pnpm-lock.yaml`：纳管 `api/gen`、平台原生 Web、`web/packages/*` 的根依赖 workspace 和共享锁文件；`app/admin/web` 保留 Vben 自己的 workspace 和 lockfile。
- `buf.yaml`：模块、依赖、lint 和 breaking 配置；`buf.go.gen.yaml`、`buf.typescript.gen.yaml`：统一生成模板。`buf.es.gen.yaml` 当前停用，只作参考。
- `go.mod`、`go.sum`：统一管理平台根代码、`api/gen/go` 与三个服务后端。仓库不跟踪 `go.work`；本机父级 `/servora-kit/go.work` 只作为 Plateau、Servora 与其他相邻仓库的可选源码联调层。
- `docker-compose.yaml`：本地基础设施；`docker-compose.apps.yaml`：应用容器编排。
- `.trellis/`：规范、任务和工作记录；应用 spec 按 `spec/<应用>/{index,architecture,frontend,backend}.md` 及实际专题组织，文件按需建立，规则见 [规范索引](../../index.md)。一个应用 package 可以同时包含 Go 与前端源码，不要求等同单个 Go/Node 包。

## 新增目录时检查

先判断属于平台能力、共通使用方式、框架内部还是具体业务；稳定身份契约不能随业务目录迁入 IAM。新增应用须同步实际模块注册和端口登记，已有占位目录不自动登记为已实现 package。

来源：[根 AGENTS](../../../../AGENTS.md)、[API 约定](../../../../api/AGENTS.md)、[go.mod](../../../../go.mod)、[pnpm workspace](../../../../pnpm-workspace.yaml)、[API exports](../../../../api/gen/package.json)。
