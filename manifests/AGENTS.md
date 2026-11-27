# AGENTS.md - manifests/

<!-- Parent: ../AGENTS.md -->
<!-- Generated: 2026-03-15 | Updated: 2026-11-25 -->

## 目录定位

`manifests/` 保存仓库内实际运行与复用的部署及初始化资产，包含 OpenFGA 授权模型与平台静态服务元组、本地容器网关 Traefik 配置、数据库与模型自动化脚本，以及可观测组件配置。

## 当前结构

```text
manifests/
├── openfga/                          # OpenFGA 模块化授权模型与静态服务元组
│   ├── fga.mod                       # 组合各领域模块的部署入口
│   ├── *.fga                         # 各领域授权模块（iam.fga / admin.fga / audit.fga）
│   ├── *.fga.yaml                    # 引用 fga.mod 的声明式模型测试
│   └── tuples/
│       └── platform-services.yaml    # 平台服务间静态授权关系元组（如 service:admin）
├── scripts/
│   ├── openfga/
│   │   ├── openfga.sh                # Unix OpenFGA store / model / sync-tuples 脚本
│   │   └── openfga.ps1               # Windows PowerShell 等价实现
│   ├── kafka/                        # Compose Kafka 平台 Topic 预建脚本
│   ├── postgres/                     # Compose PostgreSQL 多库与角色初始化脚本
│   └── ci/                           # CI 辅助脚本
├── traefik/
│   ├── traefik.yaml                  # 本地开发网关静态配置（Consul Catalog + File Provider）
│   └── routes.yaml                   # 纯前端进程（iam-web / admin-web）低优先级页面兜底路由
├── otel/                             # OpenTelemetry Collector 配置
├── prometheus/                       # Prometheus 抓取配置
├── loki/                             # Loki 日志存储配置
└── grafana/                          # Grafana 数据源与预置看板
```

## WHERE TO LOOK

| 任务 | 位置 | 说明 |
|------|------|------|
| 本地基础设施容器 | `../docker-compose.yaml` | `just compose-up` |
| 本地全栈一键编排 | `../process-compose.yaml` | `just dev` / `just dev-up` / `just dev-down` |
| Traefik 网关路由 | `traefik/routes.yaml` + 各服务 `bootstrap.yaml` | Go 后端由各自 `registry.consul.tags` 自治声明，`routes.yaml` 仅作纯前端兜底；严禁暴露 `audit.service` 或 `/metrics`、`/healthz`、`/readyz` |
| OpenFGA 模型与单测 | `openfga/*.fga`、`openfga/*.fga.yaml` | `just openfga-model-test` / `just openfga-model-apply` |
| OpenFGA 平台服务元组 | `openfga/tuples/platform-services.yaml` | `just openfga-tuple-sync`（或本地 `just openfga-setup`） |
| OpenFGA 管理脚本 | `scripts/openfga/openfga.{sh,ps1}` | 支持 `init`、`apply`、`sync-tuples` 三种子命令 |
| PostgreSQL 初始化 | `scripts/postgres/create-databases.sh` | 供 `docker-compose.yaml` 中 `postgres-provision` 挂载执行 |
| Kafka Topic 预建 | `scripts/kafka/create-topics.sh` | 供 `docker-compose.yaml` 中 `kafka-provision` 挂载执行（幂等创建 `plateau.audit.events` 等平台 Topic） |

## 关键约定

1. **OpenFGA 三层分离**：
   - `init` / `apply` 仅管理 Store 与授权模型（写入 `.env` 的 `FGA_STORE_ID` / `FGA_MODEL_ID`），不自动写元组，确保集成测试临时 Store 零预设元组隔离。
   - `tuples/platform-services.yaml` 仅声明不含动态用户 UUID 与个人敏感信息的平台级服务关系（如 `service:admin` 对 `iam:global` 和 `audit:global` 的调用权），通过 `just openfga-tuple-sync` 幂等同步（`--on-duplicate ignore`）。
2. **Traefik 本地网关路由（后端 Consul 自治 + 前端文件兜底）**：
   - `traefik/traefik.yaml` 同时启用 `providers.consulCatalog`（`exposedByDefault: false`）与 `providers.file`：Go 微服务（`iam.service`、`admin.service` 及后续业务服务）统一在各自 `configs/{local,docker}/bootstrap.yaml` 的 `registry.consul.tags` 声明 `priority=100` 路由及显式 HTTP 端口（`server.port`），新增后端服务无需改动 `manifests/traefik/`；`traefik/routes.yaml` 仅保留不注册 Consul 的纯前端进程（`iam-web:10002`、`admin-web:10012`，`priority: 10`）兜底。

## 常用命令

```bash
just compose-up
just compose-down
just compose-reset
just openfga-model-validate
just openfga-model-test
just openfga-init
just openfga-model-apply
just openfga-tuple-sync
just openfga-setup
```
