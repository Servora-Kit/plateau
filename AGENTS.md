# plateau

Servora 平台服务、主要参考应用与产品安全生态；当前包含安全 runtime/codegen、Audit 服务和 Example CRUD 服务。

## 约定

- 根 `go.work` 连接生成模块与各微服务；基础设施 provider 的业务日志由 data/bootstrap 边界决定
- Proto 统一由根 `just gen` 刷新 Go、TypeScript、OpenAPI、Wire 与 Ent
- 修改 OpenFGA model 后运行 `just openfga-model-apply`
- AuthN/AuthZ 代码生成插件从当前 checkout 的 `cmd/` 本地安装

## 本地端口

- 从 `10000` 起，每个应用固定分配 10 个端口：`+0` HTTP、`+1` gRPC、`+2` Web，`+3～+9` 预留；新增应用登记下一个空闲段，不重排已有编号。
- 此约定用于本地监听和 Docker 宿主机映射；容器内部端口、共享中间件端口和生产公开端口不受影响。同一应用的原生运行与容器映射不可同时占用相同端口。
- Web 的 dev 与 preview/start 默认共用 Web 端口，不同时启动。改动 IAM 公开入口时，同步 `IAM_PUBLIC_ORIGIN`、Web 端口和后端代理地址。

| 应用 | 端口段 | HTTP | gRPC | Web |
|---|---|---|---|---|
| IAM | 10000–10009 | 10000 | 10001 | 10002 |
| Audit | 10010–10019 | 10010 | 10011 | 10012（预留） |
| CMS（占位） | 10020–10029 | 10020（预留） | 10021（预留） | 10022（预留） |
| Example | 10030–10039 | 10030 | 10031 | 10032 |
| Test | 10040–10049 | 10040（预留） | 10041（预留） | 10042 |
| Admin（占位） | 10050–10059 | 10050（预留） | 10051（预留） | 10052（预留） |

## 目录结构

- `api/` 平台领域 Proto 与生成产物（详见 [api/AGENTS.md](api/AGENTS.md)）
  - `protos/` 领域 Proto 定义（`api/protos/plateau/**`）
  - `gen/go/` 所有微服务的 Go Proto 生成输出目录
  - `gen/ts/` 所有微服务的 TypeScript Proto 生成输出目录
  - `gen/package.json` 管理共享 TS 包依赖与 exports
- `app/` 平台微服务，均在 `app/{ServiceName}/` 下（服务结构见 [app/AGENTS.md](app/AGENTS.md)）
- `cmd/` 平台级命令工具
  - `protoc-gen-plateau-authz/`、`protoc-gen-plateau-authn/` AuthN/AuthZ 代码生成插件；插件从当前 checkout 的 `cmd/` 本地安装
- `internal/codegen/` 共享代码生成实现
- `security/` 共享安全生态：`actor.go`、`authn/<implementation>`、`authz/<engine>`、`cap/`、`password/`、`jwt/`、`errors/`
- `infra/` 共享基础设施：`openfga/`、`entgo/`、`clickhouse/`、`errors/`
- `just/` 平台共享 Just settings、registry 与 service 实现
- `manifests/` 部署资源文件（`scripts/`、`openfga/`、`grafana/`、`prometheus/`、`otel/`、`traefik/`、`loki/`）
- `docs/adr/` 架构决策记录
- `justfile` 项目级 Just 命令入口
- `pnpm-workspace.yaml` 统一纳管 `api/gen` 与 `app/*/web`
- `pnpm-lock.yaml` Platform workspace 共享依赖锁文件
- `buf.yaml` buf 总配置，依赖以及 lint 规则
- `buf.go.gen.yaml` 项目级统一 Go 生成配置
- `buf.typescript.gen.yaml` 项目级统一 TypeScript HTTP、error reason 与 CRUD helper 生成配置
- `buf.es.gen.yaml` 已停用并全部注释，仅保留作 Protobuf-ES 配置参考
- `go.work` 统一管理各个微服务与 `./api/gen` 的依赖
- `go.mod`、`go.sum` 总依赖管理
- `docker-compose.yaml` 本地基础设施编排；`docker-compose.apps.yaml` 应用容器编排

## 命令

```bash
just init
just gen
just wire
just lint
just api-ts-check
just openfga-model-validate
just openfga-model-test
just openfga-model-apply
just web::iam::dev
just web::iam::build
just web::iam::preview
just web::iam::typecheck
just web::iam::lint
```

`api/gen` 与 `app/*/web` 共用根 pnpm workspace 和 lockfile。新增平台服务参考 `app/example`。
