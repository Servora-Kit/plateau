# servora-kit 工程化优化（长期路线图）

## 目标

按「轻 `servora` 微内核 + 重 `plateau` 业务平台超集」的双层定位重构两仓：先将 `servora` 对齐至 `plateau` 同款 `mise + just + prek + GitHub Actions + Renovate` 工程化门禁并清理遗留 `Makefile` 与 `cmd/svr`，再把 `servora` 中的 AIP CRUD 全栈生态及数据库/缓存 `contrib` 剥离上移至 `plateau`，最后为 `plateau` 打通单一 BSR 源（`buf.build/servora/plateau`）、Go 模块与单一前端 NPM 工具包的自动化发布流水线，使内部衍生微服务与未来下游独立仓库都能直接复用 `plateau` 的安全、CRUD、基础设施及 `iam` / `audit` / `admin` 契约。

> **任务定位**：本任务作为跨仓演进的父级长期路线图（Epic），目标、约束与验收见本文档，架构取舍、目录映射与分阶段无死锁演进方案见 [design.md](design.md)；具体执行步骤清单（`implement.md`）在启动各阶段子任务时再分别编写。

## 现状证据与背景

1. **工具链与 CI 双轨维护**：
   - `plateau` 已落地统一工具链与门禁（`mise.toml`、`go.mod` `tool` 固定 `protoc` 插件、三层 `prek.toml`、含路径分流与 `ci-ok` 聚合门禁的 `.github/workflows/ci.yml`、`security.yml` `govulncheck` 及 `renovate.json5`，见 `11-23-plateau-ci-prek`）。
   - `servora` 仍同时维护 `Makefile`（217 行，含 `@latest` 插件安装）与 `justfile`，缺少 `mise.toml`、`prek.toml`、`govulncheck`、`gen-check` 门禁及 Renovate 配置；`cmd/svr`（811 行）默认模板路径已失效且引入了多余 TUI / SQLite 依赖。
2. **业务平台生态跨仓割裂**：
   - `Actor`、`oidc`（直接关联 `iam.user.v1.User`）、`authn`/`authz`/`audit`、`openfga`、`clickhouse`、`kafka`（含 `transport/server/kafka`）与软删除 mixin 位于 `plateau`；而同属业务开发生态的 AIP `crud`（`core/crud`、`protoc-gen-servora-crud`、`proto-utils/src/crud`）与数据库/缓存适配（`contrib/db/{entgo,gorm,redis}`、`contrib/cache/redis`）却留在底层的 `servora`，导致演进时需频繁跨仓改版。
   - `contrib/k8s`（63 行）被 `servora/core/registry/kubernetes.go` 直接调用，属于微内核服务发现的组成部分，不可随数据库/缓存适配外迁。
3. **Plateau 尚未具备对外交付通道**：
   - `plateau/buf.yaml` 当前拆分为 4 个未命名本地模块，尚未汇总推送至已建好的单一 BSR 仓库 `https://buf.build/servora/plateau`。
   - 前端能力分散在私有包 `web/packages/client` 与 `@servora/proto-utils/crud` 两处，且 `plateau` 缺少类似 `servora` 的 Go Release、BSR 自动推送与 NPM OIDC 自动发布流水线。

## 架构原则与约束

- **双层定位不变量**：
  - **`servora`（Kratos v3 微内核与 Proto 通用传输层）**：仅保留 4 款通用 `protoc` 插件（`conf`、`go-errors`、`redact`、`typescript-http`）、HTTP/gRPC 服务端与客户端 `transport` 封装、`security/tls`、`core/{bootstrap,config,registry}`（含 `contrib/k8s`）、`obs/{logger,tracing,metrics}`，以及仅含 `transport` / `errors` 与内建基础契约的极简 `@servora/proto-utils`。
  - **`plateau`（企业级业务平台超集）**：作为 `servora` 的超集，完整承载 `security/*`（含 `Actor`、AuthN/AuthZ/Audit、OIDC、Session、CAP、JWT、Password）、AIP `crud/*` 全栈生态、统一基础设施层 `infra/*`（整合 `entgo`、`gorm`、`redis`、`cache`、`openfga`、`clickhouse`、`kafka`、`mail`）、`transport/server/kafka`、4 款平台 `protoc` 插件（`crud`、`authn`、`authz`、`audit`）以及 `app/{iam,admin,audit}` 三大核心平台服务。
- **单一 BSR 源约束（`buf.build/servora/plateau`）**：
  - 采用**全集中式 Proto 布局**：将 `plateau` 平台公共 Proto（`plateau/{security,crud,infra}/**`）与平台服务全部 Proto（`iam/**`、`audit/**`、`admin/**`，含各服务 `conf`）统一集中至根 `api/protos/` 下（`name: buf.build/servora/plateau`），移除 `app/*/service/api/protos/` 子目录，各服务 OpenAPI 生成模板按路径过滤输入，对外只维护一个 BSR 仓库。
- **单一公开 NPM 工具包约束**：
  - 将原 `@plateau/client`（HTTP / SSE / WebSocket 传输实现）与从 `servora` 迁入的 `crud` TS 辅助工具整合为 `plateau/web/packages/` 下的**唯一公开 NPM 工具包**（归入 `@servora` 组织作用域，如 `@servora/plateau-utils`），通过 `package.json` `exports` 子路径（如 `./client`、`./crud`）加 `"sideEffects": false` 兼顾单包维护便利性与按需 Tree-shaking；`api/gen`（`@plateau/api`）继续作为生成产物包维护。
- **Go 模块与独立校验约束**：
  - `servora` 与 `plateau` 均保持各自单根 `go.mod` 发布；非测试生产包不引入 CGO 驱动依赖（如 `mattn/go-sqlite3` 仅限测试或改为纯 Go 方案），确保外部仓库 `go get github.com/Servora-Kit/plateau` 时 `CGO_ENABLED=0` 可编译。
  - 两仓所有本地钩子与 CI 门禁均在 `GOWORK=off` 下独立验证通过。

## 分阶段路线图与需求清单

### 阶段一：Servora 工程化门禁对齐与遗留清理（子任务：[11-25-servora-ci-prek](../11-25-servora-ci-prek/prd.md)）

- [x] **R1.1 移除 `Makefile` 与废弃 `cmd/svr`**：删除 `servora/Makefile` 与 `servora/cmd/svr`（同步清理 `charmbracelet/huh`、`modernc.org/sqlite`、`gorm.io/gen` 等专属依赖及文档引用），确立 `justfile` 为唯一任务入口。
- [x] **R1.2 工具版本与生成器固定**：在 `servora` 新增 `mise.toml`（对齐 `plateau` 工具版本策略）并在 `go.mod` 通过 `tool` 指令固定 `protoc` 插件，移除 `@latest` 安装脚本。
- [x] **R1.3 本地钩子与 Plateau 同款 CI 门禁**：新增 `servora/prek.toml`；升级 `servora/.github/workflows/ci.yml`（含 `changes` 路径分流、`prek`、`go`、`proto`、`gen`、`web` 及 `ci-ok` 聚合门禁），新增 `security.yml`（`govulncheck`）与 `renovate.json5`，并与既有 `buf-ci.yml`、`release.yml`、`publish-proto-utils.yml` 兼容衔接。

### 阶段二：Servora 瘦身（剥离 CRUD 与数据/缓存适配）

- [ ] **R2.1 剥离 CRUD 与非核心 `contrib`**：从 `servora` 移出 `core/crud`、`conformance/crud`、`cmd/protoc-gen-servora-crud`、`api/protos/servora/{crud,example,contrib}`、`contrib/{db,cache}`（`entgo`、`gorm`、`redis`、`cache/redis`）及 `web/packages/proto-utils/src/crud`；保留 `contrib/k8s`（供 `core/registry` 使用）及 `conf`、`go-errors`、`redact`、`typescript-http` 4 款插件。
- [ ] **R2.2 精简依赖与发版**：清理 `servora/go.mod` 中随上述模块移出的 `ent`、`gorm`、`redis`、`copier`、`aip` 等外部依赖，发布瘦身后的 `servora` Go 版本、`buf.build/servora/servora` 与 `@servora/proto-utils`。

### 阶段三：Plateau 承接 CRUD、基础设施与合并前端工具包

- [ ] **R3.1 承接 AIP CRUD 全栈生态**：在 `plateau` 落位 `crud/`（含 `conformance/crud`）、`api/protos/plateau/crud/v1` 与 `cmd/protoc-gen-plateau-crud`（复用 `internal/codegen`），更新本仓资源 Proto 与生成模板。
- [ ] **R3.2 统一 `infra/` 基础设施与双 ORM 支持**：将迁入的 `entgo`（与既有软删除 mixin 合并）、`gorm`、`redis`、`cache/redis` 及对应配置 Proto 整合进 `plateau/infra/`，保留完整的 Ent 与 GORM CRUD 适配能力以支撑后续多样化微服务需求。
- [ ] **R3.3 整合单一前端工具包**：将 `web/packages/client` 与迁入的 `crud` TS 模块合并为单个公开 NPM 工具包（划分 `./client` 与 `./crud` 子路径导出），切换 `api/gen`、`app/iam/web` 与 `app/admin/web` 引用。

### 阶段四：Plateau 单一 BSR、Go 包与 NPM 自动化发布闭环

- [ ] **R4.1 全集中式 `api/protos/` 与单一 BSR 模块 `buf.build/servora/plateau`**：将 `app/{iam,admin,audit}/service/api/protos/**`（含 `conf`）全部迁入根 `api/protos/` 并声明单一模块 `name: buf.build/servora/plateau`，移除服务子目录下的 `api/protos/` 并调整各服务 OpenAPI 生成配置路径。
- [ ] **R4.2 落地自动化发布工作流与文档同步**：参照 `servora` 为 `plateau` 新增 `buf-ci.yml`（推送至 `buf.build/servora/plateau`）、`release.yml`（校验 `main` `ci-ok` 通过后发布 Go Release 与 BSR 版本标签）及前端工具包 NPM OIDC 自动发布工作流；清理生产代码路径中的 CGO 依赖，更新两仓 `README.md`、`AGENTS.md` 与 `.trellis/spec/`。

## 验收标准

- [ ] `servora` 已移除 `Makefile`、`cmd/svr`、`crud` 及 `contrib/{db,cache}`，仅保留 4 款通用 `protoc` 插件（`conf`、`go-errors`、`redact`、`typescript-http`）、`core`（含 `k8s` 注册发现）、`obs`、`transport/{http,grpc}`、`security/tls` 与极简 `@servora/proto-utils`，且具备与 `plateau` 一致的 `mise + prek + ci-ok + govulncheck + Renovate` 门禁（R1.1–R2.2）。
- [ ] `plateau` 完整承载 `security`、AIP `crud`、`infra`（含 `entgo`、`gorm`、`redis`、`cache`、`openfga`、`clickhouse`、`kafka`、`mail`）、`transport/server/kafka` 及 4 款平台 `protoc` 插件（`crud`、`authn`、`authz`、`audit`）（R3.1–R3.2）。
- [ ] `plateau` 前端基础能力收敛为单一公开 NPM 工具包（含 `./client` 与 `./crud`），配置完成基于 Git tag 的 NPM OIDC 自动发布流水线（R3.3、R4.2）。
- [ ] `plateau` 全部公共与服务 `.proto` 统一归属单一模块 `buf.build/servora/plateau` 并配置自动推送；外部最小服务仅依赖 `github.com/Servora-Kit/plateau`、`buf.build/servora/plateau` 与该单一 NPM 包即可在 `CGO_ENABLED=0` 下完成构建并调用 `iam` / `audit` / `admin` 契约（R4.1–R4.2）。
- [ ] 在 `GOWORK=off` 且无父级 `pnpm-workspace.yaml` 条件下，`servora` 与 `plateau` 各自独立执行全量门禁与集成测试全部通过，IAM / Admin / Audit 前后端全链路行为零回归（R1–R4）。

## 范围外事项

- **Plateau 服务生成脚手架与 GitHub Template**：`new service` / `new api` 脚手架 CLI 及衍生垂类业务仓（CMS / OA / IoT 等），在本路线图完成后另立专项任务推进。
- **既有平台服务业务逻辑变更**：不改动 `app/iam`、`app/admin`、`app/audit` 的现有业务功能、数据库表结构或 OpenFGA 业务规则语义。
- **容器镜像发布流水线**：不在本路线图中新增微服务 Docker 镜像构建推送（GHCR）或 Helm Chart 发布。
