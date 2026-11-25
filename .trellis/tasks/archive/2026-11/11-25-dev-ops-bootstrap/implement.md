# 实施计划：开发与运维启动链路治理

## 1. 编辑前强制检查（GitNexus Impact）

在修改任何现有 Go / TS 符号前，必须先通过 GitNexus 执行上游影响分析并记录风险等级（`UNKNOWN` 须辅以文本检索确认）：

- [x] `mcp__gitnexus_impact` 分析 `app/iam/service/internal/oidc/provider.go` 中的 `isLoopbackHost`、`normalizeIssuer`
- [x] `mcp__gitnexus_impact` 分析 `app/iam/service/internal/oidc/op_key.go` 中的 `loadSigningKey`、`loadCryptoKey`
- [x] `mcp__gitnexus_impact` 分析 `security/jwt/jwks.go` 中的 `NewJWKS`

## 2. 有序实施清单

### Step 1：OpenFGA 平台服务元组声明化与脚本扩展（R1）
- [x] 新建 `manifests/openfga/tuples/platform-services.yaml`，声明 `service:admin` 的 3 条平台服务关系元组。
- [x] 扩展 `manifests/scripts/openfga/openfga.sh` 与 `manifests/scripts/openfga/openfga.ps1`，新增 `sync-tuples` 子命令（校验 `FGA_STORE_ID`、`FGA_MODEL_ID` 与元组文件存在性，调用 `fga tuple write ... --on-duplicate ignore`）。
- [x] 在根 `justfile`（Unix / Windows 双平台）新增 `openfga-tuple-sync` 与 `openfga-setup`（`openfga-init` + `openfga-tuple-sync`）。

### Step 2：IAM OIDC 密钥生成子命令 `cmd/keygen`（R2）
- [x] 在 `app/iam/service/internal/oidc/op_key.go` 补充可供 `cmd/keygen` 复用的密钥校验/生成辅助能力（不引入仅为单测存在的冗余私有包装），保持现有 `loadSigningKey` / `loadCryptoKey` 行为不变。
- [x] 新建 `app/iam/service/cmd/keygen/main.go` 及 `main_test.go`：支持 `-conf`、`-signing-key`、`-crypto-key`、`-force`；覆盖首次生成、`0600` 权限校验、合法文件幂等跳过、损坏文件拒绝覆盖、`-force` 强制轮换及 `internal/oidc` 回读兼容性单测。
- [x] 在 `app/iam/service/justfile` 新增 `keygen *args`；核查并加固 `just/service.just` 的 `_build` 在多 `cmd/*` 二进制下对 `./bin/` 目录的跨平台输出。

### Step 3：Just 与 Air 生命周期修复及前端集成（R3）
- [x] 修正 `app/iam/service/.air.toml` 中 `-X main.Name=iam.service`。
- [x] 移除 `just/service.just` 中 `run` 与 `dev` 的 `api openapi` 前置依赖。
- [x] 在 `app/admin/web/justfile` 新增 `install`（Unix / Windows），在 `app/iam/web/justfile` 补齐 `test`（`--if-present`），在 `just/webs.just` 暴露 `install` 与 `test`，并将 `web::install` 串入根 `justfile` 的 `init`。

### Step 4：Traefik 共享动态路由与 RFC 6761 `*.localhost` 入口切换（R4）
- [x] 按 RFC 6761 更新 `app/iam/service/internal/oidc/provider.go`（`isLoopbackHost`）与 `security/jwt/jwks.go`（`NewJWKS` 回环检查），允许合法 `*.localhost` 子域名使用开发态 `http://`，并在对应 `*_test.go` 补充正反例单测（含 `iam.localhost`、`.localhost`、`foo..localhost`、`evil-localhost.com`）。
- [x] 更新 `app/iam/web/next.config.ts`（`allowedDevOrigins: ["iam.localhost", "*.localhost"]`）与 `app/admin/web/apps/web-antd/vite.config.ts`（`server.allowedHosts` / `preview.allowedHosts: [".localhost"]`）。
- [x] 将 `manifests/traefik/traefik.yaml` 拆分为 `manifests/traefik/local.yaml` 与 `manifests/traefik/docker.yaml`，新建共享 `manifests/traefik/dynamic/routes.yaml`（Go Sprig `env` 模板，区分 API/OIDC/Docs 与 Web 路由，不暴露 `/metrics`、`/healthz`、`/readyz` 与 `audit.service`）。
- [x] 更新 `docker-compose.yaml` 中 `traefik` 服务（挂载 `local.yaml` 与 `dynamic/`，添加 `extra_hosts: ["host.docker.internal:host-gateway"]` 及默认路由环境变量）。
- [x] 更新 `.env.example` 与 `app/{iam,admin,audit}/service/configs/local/` 默认公开入口为 `http://iam.localhost:8080` 与 `http://admin.localhost:8080`（含回调 `http://admin.localhost:8080/v1/admin/auth/callback`；检查本地 `.env` 若存在旧 `IAM_PUBLIC_ORIGIN=http://localhost:10002` 提醒或按非破坏原则核对）。

### Step 5：`process-compose` 全链路编排与 MCP 接入（R5）
- [x] 在 `mise.toml` 新增 `"aqua:F1bonacc1/process-compose" = "1.124.0"` 并执行 `mise install` 安装验证。
- [x] 新建根 `process-compose.yaml`（前置节点 `compose-up`、`iam-keygen`、`openfga-setup`；常驻节点 `iam-service`、`audit-service`、`admin-service`、`iam-web`、`admin-web`；配置 `/readyz` / HTTP 探针与 `mcp_server` `127.0.0.1:18091` SSE + `expose_control_tools: true`）。
- [x] 更新根 `.mcp.json` 注册 `process-compose` SSE MCP Server，在根 `justfile` 新增 `dev`、`dev-up`、`dev-down`。

### Step 6：文档与 `.trellis/spec/` 规范同步（R6）
- [x] 统筹更新工作区现有的 `README.md`、`docs/iam.md`、`app/iam/web/README.md`、`AGENTS.md`、`manifests/AGENTS.md`（更新快速开始为 `just init` + `just dev`，补充 Traefik `*.localhost:8080` 架构、OpenFGA 三层运维说明、`cmd/keygen` 用法及 `process-compose` MCP 说明）。
- [x] 同步更新 `.trellis/spec/plateau/project/{development,structure}.md`、`.trellis/spec/plateau/infra/openfga.md`、`.trellis/spec/iam/{architecture,oidc}.md`、`.trellis/spec/admin/frontend.md`。

## 3. 验证与门禁命令

1. **工具链与配置静态检查**：
   ```bash
   mise install
   just --fmt --check --unstable
   process-compose -f process-compose.yaml -p 18090 --dry-run
   just openfga-model-test
   ```
2. **代码构建、单测与门禁检查（`GOWORK=off`）**：
   ```bash
   GOWORK=off just gen-check
   GOWORK=off just fmt-proto-check
   GOWORK=off just api-ts-check
   GOWORK=off just lint
   GOWORK=off just test
   GOWORK=off just build
   just web::lint
   just web::test
   just web::build
   ```
3. **真实全栈运行与网关/MCP 端到端联调**：
   - 运行 `just service::iam::keygen` 连续两次，验证首次创建 `0600` 密钥、二次幂等跳过且 `mtime`/哈希不变。
   - 运行 `just dev-up`（或测试环境验证 `openfga-setup` + `traefik` + 5 个进程），确认：
     - `process-compose process list -p 18090` 中一次性节点 `Completed`、5 个常驻进程全部 `Running (Healthy)`；
     - `curl -fsS http://127.0.0.1:18091/sse`（带超时读取首包 `endpoint` 事件）确认 MCP SSE Server 正常响应；
     - `curl -fsS http://iam.localhost:8080/.well-known/openid-configuration` 返回 `"issuer":"http://iam.localhost:8080"`；
     - `curl -fsSI http://iam.localhost:8080/login/` 与 `curl -fsSI http://admin.localhost:8080/` 均经 Traefik 正常返回前端响应；
     - 执行 `just dev-down` 干净停止本地 5 个进程且 Docker 容器保持运行。
4. **提交前卫生与图变更检查**：
   ```bash
   prek run --all-files --stage pre-commit --skip app/admin/web/
   python3 ./.trellis/scripts/task.py validate 11-25-dev-ops-bootstrap
   node .gitnexus/run.cjs detect-changes --scope all --repo .
   ```

## 4. 高风险文件与回滚点

- **`justfile` / `just/service.just`**：因 `just/settings.just` 启用了 `set export`，严禁定义与微服务配置环境变量同名的顶层空默认值变量（如 `FGA_STORE_ID`、`IAM_PUBLIC_ORIGIN`）；改动后通过 `just --fmt --check --unstable` 与 `GOWORK=off go test ./app/*/service/cmd/server` 验证配置扫描不受污染。
- **`app/iam/service/internal/oidc/provider.go` & `security/jwt/jwks.go`**：回环白名单仅放行 `localhost`、`127.0.0.1`、`::1` 与合法 `*.localhost`（必须以 `.localhost` 结尾且前导标签非空，防止 `evil-localhost.com` 绕过 HTTPS 强制检查）。
- **本地 `.env` 文件**：若开发者本地已有未入库的 `.env` 写死了旧 `IAM_PUBLIC_ORIGIN=http://localhost:10002`，`just` 的 `dotenv-override` 会优先生效；联调验证时需检查本地 `.env` 是否存在过期覆盖项（不静默篡改用户无关自定义项，仅对齐或提示）。

## 5. `task.py start` 前置确认项

- [x] `prd.md` 已完成收敛且无未决阻塞问题
- [x] `design.md` 与 `implement.md` 已完备
- [x] `implement.jsonl` 与 `check.jsonl` 已注入真实规范上下文并通过 `task.py validate`
- [x] 用户审阅并明确批准最终规划摘要后，再执行 `task.py start` 进入实施阶段
