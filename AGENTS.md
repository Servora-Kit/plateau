# AGENTS.md - servora-platform

<!-- Generated: 2026-03-25 | Updated: 2026-04-12 -->

## 项目概览

`servora-platform` 是 [Servora](https://github.com/Servora-Kit/servora) 框架的**示例项目**，提供平台级基础微服务。当前包含 Audit（审计）微服务，后续将持续扩展。

依赖关系：
- Go module 依赖：`github.com/Servora-Kit/servora`、`github.com/Servora-Kit/servora/api/gen`
- Proto BSR 依赖：`buf.build/servora/servora`
- 业务 proto 发布到：`buf.build/servora/servora-platform`
- Go module 路径：
  - `github.com/Servora-Kit/servora-platform/app/audit/service`
  - `github.com/Servora-Kit/servora-platform/api/gen`

当前主线事实：
- 所有开发在 `main` 分支进行
- `go.work` 已 gitignore，仅用于仓库内部多模块联合与顶层跨仓库开发
- 无前端应用

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

- `api/`：生成代码产物
  - `gen/go/`：Go 生成代码（业务 proto）
- `app/`：微服务实现
  - `audit/service/`：Audit 微服务
    - `api/protos/`：审计业务 proto
    - `cmd/`：服务入口
    - `configs/`：配置文件
    - `internal/`：业务实现（service/biz/data/server）
- `manifests/`：部署清单
  - `openfga/`：OpenFGA model 与测试

## 关键文件

- `Makefile`：项目变量 + `include make/core.mk`
- `make/core.mk`：根目录/服务目录共享 Make 逻辑（gen / openapi / wire / build / lint / compose / run / dev）
- `make/extra.mk`：仓库扩展 Make 逻辑（api / ent / openfga）
- `buf.yaml`：Buf v2 workspace，包含 `app/audit/service/api/protos`（名为 `buf.build/servora/servora-platform`）；依赖 `buf.build/servora/servora`
- `buf.go.gen.yaml`：Go 代码生成模板（含 servora 自定义插件：authz、mapper、audit）
- `docker-compose.yaml`：**仅基础设施**（Kafka、ClickHouse）
- `docker-compose.apps.yaml`：**生产镜像部署**（audit 服务生产镜像，可通过 `COMPOSE_FILES` 显式启用）
- `.env.example`：环境变量模板

## 目录约定

### API / Proto
- Audit 业务 proto：`app/audit/service/api/protos/`
- 框架公共 proto 通过 BSR 依赖（`buf.build/servora/servora`），不在本仓库存放
- Go 生成代码输出到 `api/gen/go/`

### Proto 命名规范
- `package` 以 `servora.` 开头，携带版本后缀
- 目录与 `package` 逐段对齐（Buf `PACKAGE_DIRECTORY_MATCH`）
- `go_package` 落到 `github.com/Servora-Kit/servora-platform/api/gen/go/servora/**`

### 服务实现
- DDD 分层：`service -> biz -> data`
- Wire 依赖注入：修改后执行 `make wire`

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
