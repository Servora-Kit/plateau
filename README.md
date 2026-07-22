# Servora Platform

简体中文

> 本项目是 [Servora](https://github.com/Servora-Kit/servora) 框架的**示例项目**，提供平台级基础微服务实现。

`servora-platform` 当前包含简单的示例审计（Audit）微服务，后续将持续扩展更多平台级基础服务。

## 包含内容

### 微服务

- **Example 服务**（`app/example/service/`）：Servora CRUD 生态示例服务
  - 很重要，是 servora 的经典用法，也是官方推荐的代码布局

- **Audit 服务**（`app/audit/service/`）：全链路审计日志服务
  - 基于 Kafka 消费审计事件
  - ClickHouse 持久化存储
  - 审计日志查询 API

### 部署

- OpenFGA model：`manifests/openfga/`

## 技术栈

- 框架：[servora](https://github.com/Servora-Kit/servora)
- API：Protobuf + Buf v2（业务 proto 依赖 [buf.build/servora/servora](https://buf.build/servora/servora)）
- DI：Google Wire
- 消息：Kafka（franz-go）
- 存储：ClickHouse（审计日志）
- 授权：OpenFGA

## 项目结构

```text
.
├── api/
│   └── gen/go/                      # Go 生成代码（业务 proto，勿手改）
├── app/
│   └── audit/service/               # Audit 微服务
│       ├── api/protos/              # 审计业务 proto
│       ├── cmd/                     # 服务入口
│       ├── configs/
│       │   ├── local/               # 本地运行配置
│       │   └── docker/              # 容器部署配置
│       └── internal/                # 业务实现（service/biz/data/server）
├── manifests/
│   └── openfga/                     # OpenFGA model 与测试
├── buf.yaml                         # Buf v2 workspace（依赖 buf.build/servora/servora）
├── buf.go.gen.yaml                  # Go 代码生成模板
├── docker-compose.yaml              # 基础设施（Kafka、ClickHouse、Consul 等）
├── docker-compose.apps.yaml         # 应用容器编排
├── make/
│   ├── core.mk                      # 根目录/服务目录共享 Make 逻辑
│   └── extra.mk                     # API/Ent/OpenFGA 等仓库扩展
└── Makefile                         # 项目变量 + include make/core.mk
```

## 快速开始

### 前置要求

- Go 1.26+
- Make
- Docker / Docker Compose

### 安装工具

```bash
make init    # 安装 protoc 插件与 CLI 工具
```

### 生成代码

```bash
make gen     # 统一生成（api + wire）
```

### 启动开发环境

Compose 负责基础设施，应用通过 `make run` 在本机启动：

```bash
# 启动基础设施
make compose.up

# 在服务目录本地启动
cd app/audit/service && make run
```

### 常用命令

```bash
# 代码生成
make gen                    # 统一生成
make api                    # 仅生成 proto Go 代码
make wire                   # 仅生成 Wire

# 质量检查
make lint                   # Go lint
make lint.proto             # Proto lint

# 服务目录（app/audit/service/）
make run                    # 直接运行（读 configs/local/）
make build                  # 编译二进制

# Compose - 基础设施
make compose.up             # 启动基础设施
make compose.stop           # 停止容器，不删除容器
make compose.down           # 移除容器/网络（保留数据卷）
make compose.reset          # 移除容器/网络/数据卷
make compose.ps             # 查看 Compose 服务状态
make compose.logs           # 跟踪 Compose 服务日志

# OpenFGA
make openfga.init           # 初始化 store
make openfga.model.validate # 验证 model
make openfga.model.test     # 测试 model
make openfga.model.apply    # 应用 model 更新
```

## 依赖关系

本项目依赖 servora 核心框架：

- **Go 依赖**：`github.com/Servora-Kit/servora`（基础库）、`github.com/Servora-Kit/servora/api/gen`（框架 proto 生成代码）
- **Proto 依赖**：`buf.build/servora/servora`（框架公共 proto）
- **CLI / 代码生成工具**：`make init` 从 GitHub 安装 `svr`、Servora 代码生成插件与 GoWind `protoc-gen-go-redact`；项目由 Buf 驱动生成，无需安装 `kratos` CLI。

## 质量约束

- 不要手动编辑生成代码：`api/gen/go/`、`wire_gen.go`
- 修改 proto 后执行 `make gen`
- 修改 Wire 依赖图后执行 `make wire`
- 修改 OpenFGA model 后执行 `make openfga.model.apply`
- 提交前执行 `make lint`

## License

MIT，详见 `LICENSE`。
