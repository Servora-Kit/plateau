# Servora Platform

简体中文

> 本项目是 [Servora](https://github.com/Servora-Kit/servora) 框架的**示例项目**，提供平台级基础微服务实现。

`servora-platform` 当前包含简单的示例审计（Audit）微服务、User CRUD 参考服务及其 Vue/Next Web 入口。

## 包含内容

### 微服务

- **Example 服务**（`app/example/service/`）：Servora CRUD 生态示例服务
  - 是 servora 的经典用法，也是官方推荐的代码布局

- **Audit 服务**（`app/audit/service/`）：全链路审计日志服务
  - 基于 Kafka 消费审计事件
  - ClickHouse 持久化存储
  - 审计日志查询 API

### Web

- **Example Web**（`app/example/web/`）：Vue 请求控制台
- **Test Web**（`app/test/web/`）：Next.js 构建验证入口

### 部署

- OpenFGA model：`manifests/openfga/`

## 技术栈

- 框架：[servora](https://github.com/Servora-Kit/servora)
- API：Protobuf + Buf v2（业务 proto 依赖 [buf.build/servora/servora](https://buf.build/servora/servora)）
- 任务编排：Just 1.57.0+
- DI：Google Wire
- 消息：Kafka（franz-go）
- 存储：ClickHouse（审计日志）
- 授权：OpenFGA

## 项目结构

```text
.
├── api/
│   └── gen/                         # Go 与共享 TypeScript 生成代码
├── app/
│   ├── audit/service/               # Audit 微服务及 leaf justfile
│   ├── example/service/             # Example 微服务及 leaf justfile
│   ├── example/web/                 # Vue 请求控制台及 leaf justfile
│   └── test/web/                    # Next.js Web 入口及 leaf justfile
├── just/
│   ├── settings.just                # 跨平台设置
│   ├── service.just                 # service 共享实现
│   ├── services.just                # service 显式 registry
│   └── webs.just                    # Web 显式 registry
├── manifests/                       # OpenFGA 与部署资源
├── buf.yaml                         # Buf v2 workspace
├── buf.go.gen.yaml                  # Go 代码生成模板
├── buf.typescript.gen.yaml          # TypeScript HTTP 生成模板
├── docker-compose.yaml              # 基础设施编排
└── justfile                         # 平台 root 任务入口
```

## 快速开始

### 前置要求

- Go 1.26+
- Just 1.57.0+
- Node.js 与 pnpm
- Docker / Docker Compose

### 安装工具

```bash
just init    # 安装 protoc 插件、CLI 与 pnpm workspace 依赖
```

### 生成代码

```bash
just gen     # 根目录统一生成 Go、共享 TypeScript HTTP、OpenAPI、Wire 与 Ent
```

### 启动开发环境

Compose 负责基础设施，应用通过对应 service leaf 任务在本机启动：

```bash
# 启动基础设施
just compose-up

# 启动 Audit 服务
just service::audit::run

# 启动 Example 服务
just service::example::run
```

## 常用命令

```bash
# root 全项目
just gen
just build
just lint
just clean

# API 与生成
just api
just api-go
just api-ts
just api-ts-check
just gen-clean
just gen-fresh

# service 类型聚合
just service::gen
just service::build
just service::lint
just service::clean

# Web 类型聚合
just web::build
just web::lint
just web::clean

# service leaf
just service::audit::run
just service::example::run
just service::example::test-integration

# Web leaf
just web::example::dev
just web::example::lint
just web::example::lint-fix
just web::test::build

# Compose
just compose-build
just compose-up
just compose-stop
just compose-down
just compose-reset
just compose-ps
just compose-logs

# OpenFGA
just openfga-init
just openfga-model-validate
just openfga-model-test
just openfga-model-apply
```

## 依赖关系

本项目依赖 servora 核心框架：

- **Go 依赖**：`github.com/Servora-Kit/servora`（基础库）、`github.com/Servora-Kit/servora/api/gen`（框架 proto 生成代码）
- **Proto 依赖**：`buf.build/servora/servora`（框架公共 proto）
- **TypeScript 依赖**：`@servora-platform/api` 是 `api/gen` 提供的共享 workspace 包；root `just api-ts` 为 `buf.yaml` 全部模块生成 HTTP client，service leaf 的同名任务使用服务自有模板
- **CLI / 代码生成工具**：`just init` 从 GitHub 安装 `svr`、Servora 代码生成插件与 GoWind `protoc-gen-go-redact`；项目由 Buf 驱动生成，无需安装 `kratos` CLI

## 质量约束

- 不要手动编辑生成代码：`api/gen/go/`、`api/gen/ts/`、`app/*/web/src/api/generated/`、`wire_gen.go`
- 修改 proto 后在仓库根执行 `just gen`；需要服务自有 client 时，再在对应 service leaf 执行 `just api-ts`
- 修改 Wire 依赖图后执行 `just wire`
- 修改 OpenFGA model 后执行 `just openfga-model-apply`
- 提交前执行 `just lint`
- `just lint` 只读；自动修复使用对应 Web leaf 的 `lint-fix` 或 `format`

## License

MIT，详见 `LICENSE`。
