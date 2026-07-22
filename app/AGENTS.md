# AGENTS.md - app/

<!-- Parent: ../AGENTS.md -->
<!-- Generated: 2026-03-15 | Updated: 2026-04-12 -->

## 目录概览

`app/` 存放可运行服务。当前包含：
- `app/audit/service/`：审计微服务
- `app/example/service/`：唯一公开 User CRUD 参考服务
- `app/example/web/`：参考服务的 Vue 请求控制台（非 CI E2E 测试应用）
每个服务目录都是独立 Go module，并通过根 `go.work` 纳管。

## 服务共同结构

```text
app/{service}/service/
├── api/         # 服务私有 proto、TS/OpenAPI 生成模板
├── cmd/         # 启动入口
├── configs/     # 配置
├── internal/    # 实现代码
├── Makefile     # include ../../../make/core.mk
└── go.mod       # 独立模块
```

各服务目录可包含：
- `web/`：前端（如有）
- `manifests/`：服务专属补充资源
- `openapi.yaml`：服务 OpenAPI 产物（由 buf 生成）

## 关键约定

- 服务目录中的 `make gen` 会执行 `api + openapi + wire + gen.ent`
- 服务目录中的 `make build` 会先执行 `make gen`，再编译当前服务
- 服务目录中的 `make api` 会回到仓库根目录跑 `make api-go`
- 若存在 `api/buf.typescript.gen.yaml`，服务级 `make api` 会额外生成 TypeScript 客户端
- 服务级 `make openapi` 读取本目录 `api/buf.openapi.gen.yaml`

## 常用命令

```bash
cd app/audit/service && make run
cd app/example/service && make run     # 本地 User CRUD HTTP/gRPC 服务
cd app/example/web && pnpm dev        # Vite 请求控制台
make gen                                # 生成平台 API/Wire/Ent
make lint && make lint.proto
```

## 配置约定

每个服务的 `configs/` 目录下默认区分两种配置环境：
- `configs/local/`：**本地开发环境**。数据库、消息队列地址指向 `127.0.0.1`。
  - 使用 `make dev` 或 `make run` 启动时，服务读取此目录。
- `configs/docker/`：**容器化环境**。数据库地址指向 Docker Compose network 中的服务名（如 `kafka:9092`）。
  - 当通过 `docker-compose.apps.yaml` 启动容器时，服务读取此目录。

在 `configs/` 中定义的 `.yaml` 在代码中通过 `conf.Bootstrap` 结构体映射，使用 Protobuf 结构定义在各自的 `api/` 目录下。

## 维护提示

- 部署清单以根 `manifests/` 为主；各服务可带 `manifests/` 补充资源
- 若新增服务，优先参考 `app/audit/service/` 的基本结构，再按需要补齐 `api/` 与 `internal/`
