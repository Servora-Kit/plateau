# Servora Platform

简体中文

> 本项目是 [Servora](https://github.com/Servora-Kit/servora) 框架的主要业务实践仓库，并拥有 Platform 产品安全生态。

`servora-platform` 当前包含具体 JWT AuthN、OpenFGA AuthZ、JWT/OpenFGA 基础能力、安全 Proto/codegen、Audit 微服务、Example CRUD 服务及其 Web 入口。

## 包含内容

### 微服务

- **Example 服务**（`app/example/service/`）：Servora CRUD 生态示例服务
  - 是 servora 的经典用法，也是官方推荐的代码布局

- **Audit 服务**（`app/audit/service/`）：全链路审计日志服务
  - 基于 Kafka 消费审计事件
  - ClickHouse 持久化存储
  - 审计日志查询 API
  - 不定义或解析 AuthN/AuthZ typed Audit payload；旧安全事件按 generic CloudEvent 保存 raw data

### 公共安全能力

- `security/actor.go`：跨服务共享的最小 Actor 与 context carrier
- `security/authn/jwt`：JWT Bearer 认证、路由策略与 Actor 映射
- `security/authz/openfga`：直接消费 Actor 的 OpenFGA 检查、Batch、List 与路由策略
- `security/jwt`：claims-neutral RS256、KID、Signer 与 Verifier
- `infra/openfga`：Platform config 到官方 OpenFGA SDK Client
- `cmd/protoc-gen-servora-authn`、`cmd/protoc-gen-servora-authz`：由当前 checkout 本地安装的规则插件

安全公共 Proto 位于 `api/protos/platform/**`，生成物位于独立 module `api/gen`；Platform 不发布自己的 BSR module。

### Web

- **Example Web**（`app/example/web/`）：Vue 请求控制台
- **Test Web**（`app/test/web/`）：Next.js 构建验证入口

### 部署

- OpenFGA model：`manifests/openfga/`

## 技术栈

- 框架：[servora](https://github.com/Servora-Kit/servora)
- API：Protobuf + Buf v2；Servora 通用 Proto 依赖 [buf.build/servora/servora](https://buf.build/servora/servora)，Platform 安全 Proto 由本仓本地 module 管理
- 任务编排：Just 1.57.0+
- DI：Google Wire
- 消息：Kafka（franz-go）
- 存储：ClickHouse（审计日志）
- 授权：OpenFGA

## 项目结构

```text
.
├── api/
│   ├── protos/platform/              # Platform AuthN/AuthZ/JWT/OpenFGA Proto
│   └── gen/                          # 独立 Go module 与共享 TypeScript 生成代码
├── app/
│   ├── audit/service/                # generic CloudEvents Audit 微服务
│   ├── example/service/              # Example CRUD 微服务
│   ├── example/web/                  # Vue 请求控制台
│   └── test/web/                     # Next.js 构建验证入口
├── cmd/
│   ├── protoc-gen-servora-authn/     # Platform AuthN 规则插件
│   └── protoc-gen-servora-authz/     # Platform AuthZ 规则插件
├── infra/openfga/                    # 官方 OpenFGA SDK Client 构造
├── security/
│   ├── actor.go                      # 跨服务共享 Actor
│   ├── authn/jwt/                    # 具体 JWT Bearer AuthN
│   ├── authz/openfga/                # 具体 OpenFGA AuthZ
│   └── jwt/                          # claims-neutral JWT primitive
├── just/                             # service/web 跨平台任务模块
├── manifests/                        # OpenFGA、部署资源与管理脚本
├── buf.yaml                          # 本地 Buf v2 workspace modules
├── buf.go.gen.yaml                   # Go 代码生成模板
├── buf.typescript.gen.yaml           # TypeScript 生成模板
├── docker-compose.yaml               # 基础设施编排
└── justfile                          # Platform root 任务入口
```

## 快速开始

### 前置要求

- Go 1.26+
- Just 1.57.0+
- Node.js 与 pnpm
- Docker / Docker Compose
- OpenFGA CLI `fga`；Unix OpenFGA 管理还需 `jq`

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
- **CLI / 代码生成工具**：`just init` 安装 Servora 仍拥有的通用插件，并从当前 checkout 安装 AuthN/AuthZ 插件；OpenFGA 管理由仓库脚本和 `fga` CLI 完成，不安装或调用 `svr`

### 安全与 Audit 边界

- Platform 拥有具体 AuthN/AuthZ 实现、注解 Proto、生成插件、JWT primitive 和 OpenFGA SDK Client 构造。
- Servora 只提供通用 Audit runtime、CloudEvents backend、RPC Audit 注解/plugin 与其他框架 primitive。
- 本次不定义 `platform.authn.*` 或 `platform.authz.*` Audit 事件；IAM 开始后再基于真实身份模型单独设计。
- Audit service 对遗留 `servora.authn.*` / `servora.authz.*` 事件只执行 generic raw-data 存储，不做 typed projection。

## 质量约束

- 不要手动编辑生成代码：`api/gen/go/`、`api/gen/ts/`、`app/*/web/src/api/generated/`、`wire_gen.go`
- 修改 proto 后在仓库根执行 `just gen`；需要服务自有 client 时，再在对应 service leaf 执行 `just api-ts`
- 修改 Wire 依赖图后执行 `just wire`
- 修改 OpenFGA model 后执行 `just openfga-model-apply`
- 提交前执行 `just lint`
- `just lint` 只读；自动修复使用对应 Web leaf 的 `lint-fix` 或 `format`

## License

MIT，详见 `LICENSE`。
