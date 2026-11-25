# 开发与运维启动链路治理

## Goal

构建本地开发与生产运维同构、幂等且支持终端与 AI Agent 协同控制的启动及初始化体系：将 OpenFGA 平台服务静态关系元组纳入声明式同步，用 IAM 原生 Go 子命令生成并校验 OIDC 密钥，修复 Just 与 Air 启动缺陷并补齐前端初始化，启用 Traefik 共享动态路由承接本地（`*.localhost:8080`）与线上入口流量，并通过 `process-compose`（含内置 SSE MCP Server）一键编排全栈进程。

## Background

- **OpenFGA 元组同步缺口**：`manifests/scripts/openfga/openfga.{sh,ps1}` 仅提供 `init` 与 `apply` 管理 Store 与授权模型（`manifests/openfga/fga.mod`）。`app/admin/service/internal/biz/seed.go:75-82` 在自举初始管理员时需以 `service:admin` 身份调用 `iam.service/ListUsers`；若环境未预先写入 `service:admin manage_users iam:global` 等静态服务元组，本地与生产启动自举均会报 `PermissionDenied`。同时 CI 集成测试（`app/admin/service/internal/server/audit_integration_test.go:645-654`）依赖零预设元组的干净测试 Store 验证 403 拦截，元组同步必须独立于 `init`/`apply`。
- **IAM 本地与运维密钥生成**：仅 `app/iam/service/configs/local/oidc.yaml:3-4` 读取本地私钥与加密密钥文件，`app/iam/service/internal/oidc/op_key.go:39-119` 要求 RSA 私钥（PKCS#1/PKCS#8 PEM，`>= 2048` bits）与 32 字节加密密钥。`just/service.just:64-66` 已对 `cmd/` 下全部二进制执行统一构建，新增 Go 子命令可同时服务本地初始化与编译后一次性运维产物生成，免维护双份跨平台脚本。
- **Just 与 Air 现存缺陷**：
  - `app/iam/service/.air.toml:5` 误写 `-X main.Name=audit.service`，热重载时会导致 Consul 注册名覆盖 `audit.service` 且审计日志 `service_name` 错乱。
  - `just/service.just:70-75` 的 `run` 与 `dev` 前置依赖 `api openapi`，多进程并发启动时会同时触发根级 `buf generate` 覆盖写 `api/gen/go/` 造成编译竞态。
  - 根 `justfile:37-46` 的 `init` 未安装独立 workspace `app/admin/web` 的依赖，`just/webs.just` 未暴露 `test` 聚合入口。
- **Traefik 网关与公开入口同构需求**：`manifests/traefik/traefik.yaml:29-37` 当前处于零路由空转状态，而 `app/admin/web/apps/web-antd/vite.config.ts:12-34` 打包后为纯静态 `dist/`，生产环境必须依赖网关同域转发 `/v1/*` 至 `admin.service:10010` 才能通过 `security/session/filter.go` 同源校验。Traefik v3 主静态配置不支持环境变量模板，而 File Provider 动态配置文件原生支持 Go Sprig `{{ env "..." }}` 模板；且当前 `app/iam/service/internal/oidc/provider.go:359-366`、`security/jwt/jwks.go:23` 尚未按 RFC 6761 放行 `*.localhost` 回环域名，`app/iam/web/next.config.ts` 尚未配置 `allowedDevOrigins`。
- **多进程顺序依赖与 Agent 协同**：`app/audit/service/configs/local/jwt_authn.yaml:5` 与 `security/authn/jwt/authn.go:80` 在启动时同步拉取 `http://127.0.0.1:10000/keys`，失败即退出，要求进程编排器具备 `depends_on` 与 HTTP 就绪探针。`process-compose`（`v1.124.0`）原生支持 DAG 探针与内置 SSE MCP Server（`expose_control_tools: true`），需显式将 REST API 与 MCP 端口固定至本地基础设施端口段（`18090` / `18091`）以避开 Traefik `:8080`。

## Requirements

1. **R1 — OpenFGA 平台服务元组声明式同步**：
   - 新增 `manifests/openfga/tuples/platform-services.yaml`，声明 `service:admin` 对 `iam:global`（`manage_users`、`expunge_users`）与 `audit:global`（`read_events`）的 3 条平台服务元组。
   - 在 `manifests/scripts/openfga/openfga.sh` 与 `openfga.ps1` 新增幂等子命令 `sync-tuples`（支持 `--tuples-file`、读取环境或 `--env-file` 中的 `FGA_STORE_ID` 与 `FGA_MODEL_ID`、执行 `--on-duplicate ignore` 写入）；保持 `init` 与 `apply` 不自动写元组以兼容 CI 测试隔离。
   - 在根 `justfile` 新增 `openfga-tuple-sync` 与组合命令 `openfga-setup`（串联 `openfga-init` 与 `openfga-tuple-sync`）。
2. **R2 — IAM OIDC 密钥生成 Go 子命令**：
   - 新增 `app/iam/service/cmd/keygen/main.go`，支持 `-conf`（扫描 `oidc.yaml` 解析 `signing_key_path` 与 `crypto_key_path`）及显式 `-signing-key` / `-crypto-key` 覆盖参数，支持 `-force` 强制轮换。
   - 默认幂等策略：目标密钥文件已存在时调用与 `internal/oidc` 同源的校验逻辑验证合法性，合法则跳过不覆盖，损坏或缺失且未传 `-force` 时对损坏文件报错提示、对缺失文件按目录 `0700` / 文件 `0600` 原子写入（临时文件 + rename）生成（RSA 2048-bit PKCS#8 PEM 签名私钥 + 32 字节随机加密密钥）并回读验证。
   - 在 `app/iam/service/justfile` 新增 `keygen` 入口，且 `just service::iam::build` 可在同一 `./bin/` 下产出 `server` 与 `keygen`。
3. **R3 — Just 与 Air 生命周期修正**：
   - 将 `app/iam/service/.air.toml` 中的 `-X main.Name=audit.service` 修正为 `-X main.Name=iam.service`。
   - 移除 `just/service.just` 中 `run` 与 `dev` 的 `api openapi` 前置依赖；确保 `_build` 在多 `cmd/*` 子包场景下跨 Unix/Windows 稳定创建并输出到 `./bin/`。
   - 在 `app/admin/web/justfile` 新增 `install`（`pnpm install --frozen-lockfile`），在 `just/webs.just` 新增 `install` 与 `test` 入口，并将 `app/admin/web` 依赖安装串联进根 `just init`（排在根 workspace `pnpm install` 之后）。
4. **R4 — Traefik 本地网关路由与 `*.plateau.localhost` 默认入口**：
   - 启用 Traefik 双数据源（`manifests/traefik/traefik.yaml`：`:80` 入口、`:8081` 调试面板、`:8082` Prometheus 指标、OTel 追踪、`consulCatalog` + `file`）：Go 后端（`iam.service`、`admin.service`）在各自 `configs/{local,docker}/bootstrap.yaml` 的 `registry.consul.tags` 自治声明高优路由（`priority=100`）与显式 HTTP `server.port`，`manifests/traefik/routes.yaml` 仅保留不注册 Consul 的纯前端低优兜底路由（`iam-web:10002`、`admin-web:10012`，`priority: 10`）；严禁暴露 `audit.service` 或内部 `/metrics`、`/healthz`、`/readyz`。
   - `docker-compose.yaml` 的 `traefik` 依赖 `consul` 健康就绪、挂载 `./manifests/traefik:/etc/traefik:ro`，并配置 `extra_hosts: ["host.docker.internal:host-gateway"]`（前端默认域名与上游由 `routes.yaml` 的 `default` 统一兜底）。
   - 提取共享 `security.IsLoopbackHost` 并在 `app/iam/service/internal/oidc` 与 `security/jwt/jwks.go` 复用，按 RFC 6761 放行 IP 回环字面量、`localhost` 与合法 `*.localhost` 明文回环校验并补齐单测（Next.js 16 与 Vite 原生放行 `*.localhost`，保留原有前端本地代理作为脱离网关单调时的回退能力）。
   - 将 `.env.example` 与 `app/{iam,admin,audit}/service/configs/local/` 中的默认 `IAM_PUBLIC_ORIGIN`、`ADMIN_PUBLIC_ORIGIN`、`ADMIN_OIDC_REDIRECT_URI` 更新为 `http://iam.plateau.localhost:8080` 与 `http://admin.plateau.localhost:8080`（各服务自身监听端口 `10000–10021` 不变）。
5. **R5 — `process-compose` 全链路编排与 MCP 接入**：
   - 在 `mise.toml` 固定 `"aqua:F1bonacc1/process-compose" = "1.124.0"`。
   - 新增根 `process-compose.yaml`：配置前置一次性节点（`compose-up`、`iam-keygen`、`openfga-setup`，退出时不自动关停 Docker 容器），按依赖图与 `/readyz` / HTTP 探针依次编排 `iam-service`、`audit-service`、`admin-service`（默认 `air` 热重载）及 `iam-web`、`admin-web`；启用内置 `mcp_server`（`host: 127.0.0.1`、`port: 18091`、`transport: sse`、`expose_control_tools: true`）。
   - 在根 `.mcp.json` 注册 `process-compose`（`"type": "sse"`、`"url": "http://127.0.0.1:18091/sse"`），并在根 `justfile` 提供 `dev`（TUI，固定 `18090` REST 端口）、`dev-up`（后台守护）与 `dev-down`（停止编排进程）。
6. **R6 — 文档与规范同步**：
   - 统一整理并更新工作区中的 `README.md`、`docs/iam.md`（原 `docs/iam-security.md` 重命名及测试注释同步）、`app/iam/web/README.md`、`AGENTS.md`、`manifests/AGENTS.md` 以及 `.trellis/spec/` 相关规范（`plateau/project/{development,structure}.md`、`plateau/infra/openfga.md`、`iam/{architecture,oidc}.md`、`admin/frontend.md`），明确说明 RFC 6761 免改 `/etc/hosts` 机制、OpenFGA 三层运维流程与多副本密钥注意事项。

## Acceptance Criteria

- [x] **AC1（对应 R1）**：执行 `just openfga-setup` 可幂等完成 Store/Model 初始化与 `manifests/openfga/tuples/platform-services.yaml` 3 条服务元组写入，重复执行零报错；`just openfga-init` / `openfga-model-apply` 单独执行时仍不写入任何元组。
- [x] **AC2（对应 R2）**：`just service::iam::keygen` 在缺失密钥时生成 `0600` 权限的签名私钥与加密密钥并通过 `internal/oidc` 加载验证；再次执行时幂等跳过且文件内容不变；传 `-force` 时成功轮换；损坏文件在未传 `-force` 时返回非零错误；`just service::iam::build` 在 `./bin/` 下同时产出 `server` 与 `keygen`。
- [x] **AC3（对应 R3）**：`app/iam/service/.air.toml` 构建标识为 `iam.service`；`just/service.just` 的 `run` 与 `dev` 不再触发 `buf generate`；`just init` 完整安装根 workspace 与 `app/admin/web` 依赖，`just web::test` 正常通过。
- [x] **AC4（对应 R4）**：`security.IsLoopbackHost` 单元测试覆盖正反例并通过；Docker Traefik 通过 `consulCatalog` 发现 `iam-api` / `admin-api` 并结合 `routes.yaml` 前端兜底后，宿主机免改 `/etc/hosts` 访问 `http://iam.plateau.localhost:8080` 与 `http://admin.plateau.localhost:8080` 均可正常加载前端页面、转发 API/OIDC 请求，且 `audit.service` 与 `/metrics`、`/healthz`、`/readyz` 不被网关暴露。
- [x] **AC5（对应 R5）**：`process-compose -f process-compose.yaml -p 18090 --dry-run` 校验通过；通过 `just dev-up` 实测全链路拉起后，5 个应用进程全部健康就绪，`http://127.0.0.1:18091/sse` MCP 端点可达，`.mcp.json` 格式合法，`just dev-down` 可干净退出本地应用进程且保留 Docker 中间件。
- [x] **AC6（对应 R6 & 全局门禁）**：`GOWORK=off` 下 `just gen-check`、`just fmt-proto-check`、`just api-ts-check`、`just openfga-model-test`、`just lint`、`just test`、`just web::lint`、`just web::build` 与 `prek` 检查全部通过，文档相对链接全部有效。

## Out of Scope

- Kubernetes Ingress / Gateway API `HTTPRoute` CRD 清单及生产外部密钥控制器（External Secrets / Vault）部署编排。
- OpenFGA 运行时动态业务元组（含用户 UUID 的授权数据）的静态文件管理。
- 跨仓库 `../servora/README.md` 的提交操作（保持其工作区已有修改不破坏，仅在本仓范围内完成闭环）。
