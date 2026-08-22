# AGENTS.md - app/

<!-- Parent: ../AGENTS.md -->
<!-- Generated: 2026-03-15 | Updated: 2026-08-22 -->

## 目录概览

`app/` 存放可运行服务，每个服务目录都是独立 Go module，并通过根 `go.work` 纳管。当前包含：

- `app/example/service/` + `app/example/web/`：最小 CRUD 参考模板（`example.servora.dev/User`），新增平台级微服务的推荐起点
- `app/iam/service/` + `app/iam/web/`：主要业务实践（账号/会话/OIDC/AuthZ）
- `app/audit/service/`：审计微服务，只消费 generic CloudEvents
- `app/admin/web/`、`app/cms/web/`：前端
- `app/test/web/`：Next.js 构建验证入口

各服务目录可包含：

- `service/`：后端
- `web/`：前端（如有）
- `manifests/`：服务专属补充资源
- `openapi.yaml`：服务 OpenAPI 产物（由 buf 生成）

## 服务结构

后端布局为 `app/{ServiceName}/service/`，标准结构与职责如下：

```text
{ServiceName}/service/         后端（独立 Go module）
├── api/                       本微服务 proto 接口定义和私有配置文件 proto 定义
│   ├── protos/{DomainName}/service/   该微服务各个领域的 Proto API 定义
│   ├── protos/{ConfigName}/conf.proto 该微服务自己的业务配置 proto
│   ├── buf.openapi.gen.yaml   该服务的 OpenAPI buf 生成配置
│   └── buf.typescript.gen.yaml 可选的服务级模板；仅在服务 leaf 执行 `just api-ts` 时使用，
│                               按模板自己的 `out` 生成到服务 Web。仓库根 `buf.typescript.gen.yaml`
│                               为 `buf.yaml` 中全部模块生成共享 HTTP client，两者互不清理
├── cmd/                       启动入口，一般只包含 server/
├── configs/                   运行时配置
│   ├── local/                 本地开发环境（数据库、消息队列指向 127.0.0.1）
│   └── docker/                容器化环境（指向 Docker Compose network 中的服务名）
├── internal/                  业务逻辑（可额外按领域增加子包，如 iam 的 oidc/、authn/、authz/、mail/）
│   ├── assets/                默认包含 protoc-gen-openapi 生成的 openapi.yaml
│   ├── server/                http/grpc 等服务层
│   │   ├── server.go          除了 ProviderSet，一般不能有别的方法
│   │   └── http.go|grpc.go    服务具体实现，后续若有 WebSocket、Asynq 的服务端也可以放这里
│   ├── service/               接口实现层，无任何业务逻辑，不能被 biz 层 import
│   │   ├── service.go         除了 ProviderSet，一般不能有别的方法
│   │   └── xxx.go             某业务的接口适配
│   │                          定义 XxxService 结构体嵌入 xxxv1.UnimplementedXxxServiceServer 实现接口，
│   │                          嵌入 *biz.XxxUsecase 将规范化后的请求交给 biz 层处理
│   ├── biz/                   业务逻辑层，可被 service 层 import，不应 import data 层（不关心存储如何实现）
│   │   ├── biz.go             除了 ProviderSet，一般不能有别的方法
│   │   └── xxx.go             某业务的具体处理逻辑
│   │                          定义 XxxUsecase 结构体表示本领域的具体业务；
│   │                          定义 XxxRepo 接口表示本业务所需要的 data 层方法
│   └── data/                  数据访问层，包含数据库访问逻辑，可以 import biz 层
│       ├── data.go            除了 ProviderSet，有且仅有 NewData 相关初始化逻辑
│       ├── xxx.go             实现 biz 层定义的 XxxRepo 接口，实现具体的数据访问逻辑
│       ├── schema/ + ent/     如用了 Ent ORM：schema 定义表结构，ent 为生成代码
│       └── generate.go        如用了 Ent ORM 框架，生成代码的入口
├── go.mod|go.sum              独立模块
└── justfile                   服务级 Just 命令入口
```

## 关键约定

- 业务分层：`service -> biz <- data`；service 只适配接口，biz 定义 Use Case/Repo，data 实现存储。
- 服务 leaf 的 `just gen` 会执行 `api + openapi + wire + gen-ent`
- 服务 leaf 的 `just build` 会先执行 `just gen`，再编译当前服务
- 服务 leaf 的 `just api` 会回到仓库根目录生成统一 Go API；若存在 `api/buf.typescript.gen.yaml`，再生成当前服务的 TypeScript API
- 服务 leaf 的 `just openapi` 读取本目录 `api/buf.openapi.gen.yaml`
- root `just build` 使用隐藏 build helper，避免生成完成后重复生成

## 常用命令

```bash
just service::iam::run        # IAM 服务
just service::example::run    # 本地 User CRUD HTTP/gRPC 服务
just web::example::dev        # Vite 请求控制台
just gen                       # 生成平台 API/Wire/Ent
just lint                      # 全项目只读质量检查
```

## 配置约定

每个服务的 `configs/` 目录下默认区分两种配置环境：

- `configs/local/`：**本地开发环境**。数据库、消息队列地址指向 `127.0.0.1`。
  - 使用 `just service::<name>::dev` 或 `just service::<name>::run` 启动时，服务读取此目录。
- `configs/docker/`：**容器化环境**。数据库地址指向 Docker Compose network 中的服务名（如 `kafka:9092`）。
  - 当通过 `docker-compose.apps.yaml` 启动容器时，服务读取此目录。

在 `configs/` 中定义的 `.yaml` 在代码中通过 `conf.Bootstrap` 结构体映射，使用 Protobuf 结构定义在各自的 `api/` 目录下。

## 维护提示

- 部署清单以根 `manifests/` 为主；各服务可带 `manifests/` 补充资源
- 新增平台级微服务优先参考 `app/example/service/` 的最小结构，再按需要补齐 `api/`、`justfile` 与 `internal/`