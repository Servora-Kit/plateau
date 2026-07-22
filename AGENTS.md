# AGENTS.md - servora-platform

<!-- Updated: 2026-07-21 -->

## 项目概览

`servora-platform` 是 Servora 的平台服务与参考应用仓库，当前包含 Audit 微服务和单一 User CRUD 参考应用。

依赖关系：

- Go module 依赖：`github.com/Servora-Kit/servora`、`github.com/Servora-Kit/servora/api/gen`
- Proto BSR 依赖：`buf.build/servora/servora`
- 平台/参考应用生成物：`api/gen/go/` 与 `api/gen/ts/`
- Go module：`app/audit/service`、`app/example/service`、`api/gen`

当前主线事实：

- 所有开发在 `main` 分支进行；`go.work` 仅用于本地联合开发。
- `app/example/service` 是 `example.servora.dev/User` 的可运行 CRUD 黄金路径。
- `app/example/web` 是 Vue 请求控制台，直接调用本地 HTTP facade，不维护第二套 API contract。

## 开发约束

### 提交消息格式

遵循 Servora-Kit 组织统一规范：

```
type(scope): description
```

**允许的 type**：`feat`、`fix`、`refactor`、`docs`、`test`、`chore`

**建议的 scope**：
- `api`：API / Proto
- `app/audit`：Audit 服务
- `manifests`：部署清单
- `infra`：基础设施/部署
- `repo`：仓库治理

## 顶层目录

- `api/gen/`：Go/TypeScript 生成产物
- `app/audit/service/`：Audit 微服务
- `app/example/service/`：User CRUD 参考服务，包含 service/biz/data/Ent/HTTP facade
- `app/example/web/`：Vue 参考客户端
- `manifests/`：平台部署清单与 OpenFGA model

## 关键文件

- `Makefile`：模块、Buf template 与共享 Make 入口
- `make/core.mk`：根目录/服务目录共享 gen / build / lint / run 逻辑
- `make/extra.mk`：api / ent / openfga 扩展逻辑
- `buf.yaml`：Buf v2 workspace，纳管 Audit 与 Example User proto
- `buf.go.gen.yaml`：Go 生成模板，包含 CRUD descriptor/name/field helper
- `buf.typescript.gen.yaml`：TypeScript HTTP/CRUD 生成模板
- `docker-compose.yaml`：平台基础设施；参考服务本地使用 SQLite，不要求 Audit 容器

## 目录约定

### API / Proto

- Audit proto：`app/audit/service/api/protos/`
- Example User proto：`app/example/service/api/protos/`
- 框架公共 proto 通过 BSR 依赖（`buf.build/servora/servora`）
- Go/TypeScript 生成代码输出到 `api/gen/`
- `api/gen/go`、`api/gen/ts`、Ent、Wire 与 OpenAPI 产物禁止手改，也不创建手写 `AGENTS.md`

### Proto 呍名规范

- 目录与 package 逐段对齐，满足 Buf `PACKAGE_DIRECTORY_MATCH`
- Audit 使用 `servora.audit.*`；参考业务资源使用 `example.service.v1`
- `go_package` 必须落到 `github.com/Servora-Kit/servora-platform/api/gen/go/**`

### 服务实现

- DDD 分层：`service -> biz -> data`
- Wire 依赖注入：修改后执行 `make wire`
- CRUD 原始 RPC wrapper、FieldMask 与 filter/order 文本停在 service；业务 scope 与语义归 biz；Ent binding 与 mutation 归 data

## 常用命令

```bash
# 初始化
make init              # 安装工具（protoc 插件 + CLI）

# 代码生成
make gen               # 统一生成（api + wire）
make api               # 仅生成 proto Go 代码
make wire              # 仅生成 Wire

# 质量检查
make lint              # Go lint
make lint.proto        # Proto lint

# Compose (开发工作流)
make compose.up        # 启动 COMPOSE_FILES 指定的 Compose 服务
make compose.build     # 构建所有服务的最新生产镜像 (包含 latest tag)
make compose.stop      # 停止容器，不删除容器
make compose.down      # 移除容器/网络，保留 volumes
make compose.reset     # 移除容器/网络/volumes
make compose.ps        # 查看 Compose 服务状态
make compose.logs      # 跟踪 Compose 服务日志

# 应用容器
COMPOSE_FILES="-f docker-compose.yaml -f docker-compose.apps.yaml" make compose.up

# OpenFGA
make openfga.init             # 初始化 store
make openfga.model.validate   # 验证 model
make openfga.model.test       # 测试 model
make openfga.model.apply      # 应用 model 更新
```

## 维护提示

- 修改 proto 后执行 `make gen`
- 修改 Wire 依赖图后执行 `make wire`
- 不要手改 `api/gen/go/`、`wire_gen.go`
- 修改 OpenFGA model 后执行 `make openfga.model.apply`
- 自定义 protoc 插件通过 `go install github.com/Servora-Kit/servora/cmd/...@latest` 安装
- 新增平台级微服务时，在 `app/<service>/service/` 下创建标准 Kratos 服务结构，并在 `buf.yaml` 中添加对应 proto 模块
