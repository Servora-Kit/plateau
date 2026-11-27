# 本地开发与检查入口

仓库使用单一根 Go module；父级工作区只用于本机跨仓联调，仓库门禁须在 `GOWORK=off` 下独立运行。命令从仓库根目录执行，服务级命令从对应 service 目录执行。

## 固定端口

每个应用保留 10 个端口：+0 HTTP、+1 gRPC、+2 Web，其余预留。新增应用使用已登记的空闲段，不重排已有编号；具体分配以[项目约定](../../../../AGENTS.md)为准，不在规范重复业务端口表。

原生服务与容器宿主映射不能占用同一端口，Web dev 与 preview/start 不同时运行。本地默认通过 Traefik（`:8080`）暴露 RFC 6761 回环域名 `http://iam.plateau.localhost:8080` 与 `http://admin.plateau.localhost:8080`（免改 `/etc/hosts`），各应用自身监听端口保持 `10000–10029` 不变；公开入口变更须同步 origin、Web 端口和网关/后端代理地址。

## 跨仓联调

父级 Go/pnpm workspace 仅作为本机源码联调层，不提交到仓库。pnpm 使用最近的 workspace；跨仓命令显式从共同父目录执行，独立第三方 workspace 保持自己的依赖和 lockfile。

本地包链接在安装后生效；exports 指向构建产物的共享包必须先构建或启用 watch，不能把 workspace 注册等同于已可消费。

## 命令及副作用

| 入口 | 用途 |
| --- | --- |
| `just init` | 依次执行 `mise install`、`prek install`、根 `pnpm install --frozen-lockfile` 与 `just web::install`（`app/admin/web`），完成工具链、Git 钩子及全部前端依赖准备 |
| `just dev`／`dev-up`／`dev-down` | 通过 `process-compose`（REST `:18090`、SSE MCP `127.0.0.1:18091`）编排 `compose-up`、`iam-keygen`、`openfga-setup` 与 5 个前后端进程；`dev-down` 仅停止本地应用进程并保留 Docker 容器 |
| `just gen` | 根 API 生成后执行各服务 OpenAPI、Wire、Ent；修改 Proto 后使用（沿用当前 Go 环境，支持父级 `go.work` 联调） |
| `just gen-check` | 在 `GOWORK=off` 下运行 `just gen`，并对生成路径校验 `git diff --exit-code` 与未跟踪文件 |
| `just wire` | 各服务 Wire 装配刷新 |
| `just test` | 在根 module 运行 `go test -race -count=1 ./...` |
| `just lint` | API TS typecheck、Buf lint、从根 module 执行的全仓 Go lint；不等于全仓 Go 测试 |
| `just fmt-proto-check` | `buf format -d --exit-code` 检查 Proto 格式 |
| `just breaking` | 手动运行 `buf breaking --against` 检查破坏性变更（默认对比 `.git#ref=origin/main`，可用 `BUF_BREAKING_AGAINST` 覆盖；`0.x` 阶段仅手动运行，`v1.0.0` 恢复自动拦截） |
| `just api-ts-check` | 共享生成 TS 契约检查 |
| `just service::iam::keygen` | 幂等校验或原子生成本地 OIDC RSA-2048 签名私钥与 32B 加密密钥（`0600`），传 `-force` 强制轮换 |
| `just web::<应用>::dev`／`build`／`lint`／`test` | 应用 Web 开发、构建、lint 与单测（根级聚合 `just web::install`／`build`／`lint`／`test`） |
| `just openfga-model-validate`／`test` | 本地 model 与场景检查 |
| `just openfga-model-apply` | 修改 model 后的应用步骤，会写外部 model／环境配置，不写业务或服务元组 |
| `just openfga-tuple-sync`／`openfga-setup` | 幂等同步 `manifests/openfga/tuples/platform-services.yaml` 平台服务元组；`openfga-setup` 串联 `openfga-init` 与 `openfga-tuple-sync` |

## 工具版本与单一来源

- Go 版本由根 `go.mod` 管理：`go` 指令表示最低支持版本，不为补丁版本抬高，避免强制依赖方和本地 `go.work` 跟着升级；需要标准库安全补丁时用 `toolchain` 指令固定补丁版本，CI 通过 `actions/setup-go` 读取 `go.mod`。
- `buf`、`golangci-lint`、`just`、`fga`（`aqua:openfga/cli`）、`process-compose`（`aqua:F1bonacc1/process-compose`）、`prek`、`node`、`pnpm`、`air` 由根 `mise.toml` 固定版本；本地通过 `mise install`、CI 通过 `jdx/mise-action` 使用同一份版本。
- 所有 protoc 插件（含 Plateau AuthN/AuthZ/Audit/CRUD 与 Servora 插件）及 Wire/Ent 均在根 `go.mod` 的 `tool` 块声明并通过 `go tool` 执行；不再使用 `just plugin`/`just cli` 安装，Servora 插件版本随 `go.mod` 中的 `github.com/Servora-Kit/servora` 同步。生成路径与入口详见 [generation](../../api/proto/generation.md)。
- 依赖与工具版本由 Renovate 跟踪更新。
- 根 justfile 启用了 `set export` 与 `set dotenv-override`：所有 Just 变量都会导出给 recipe 及其子进程，读取外部环境变量的 Just 变量不能与服务配置、测试使用的环境变量同名，否则空默认值会覆盖配置里的 `${VAR:默认值}`；支持自定义 `ENV_FILE` 的 recipe 在嵌套调用 `just` 时须显式透传 `--dotenv-path "{{ ENV_FILE }}"`，防止子 `just` 重新加载默认 `.env` 覆盖目标文件变量。

## 本地钩子分层（prek）

根 `prek.toml` 与子项目 `app/admin/web/prek.toml` 共同构成 workspace 钩子配置，全局排除生成产物与锁文件：

- `pre-commit`：通用文件卫生（内置 Rust 钩子）、`gofmt`、`buf format`、`just --fmt --check --unstable`；`app/admin/web` 仅对暂存文件执行 `oxlint`/`oxfmt`/`eslint`/`stylelint` 自动修复，不跑全量类型检查。
- `pre-push`：运行 `go-build-nocgo`（`CGO_ENABLED=0 GOWORK=off go build ./...`），并在 `GOWORK=off` 下运行 `just lint-go`、`just lint-proto`、`just gen-check`；`app/admin/web` 运行 `pnpm check:type`。`0.x` 阶段已关闭 `buf-breaking` 自动拦截（待 `v1.0.0` 发布前恢复）。push 前工作区应保持干净，临时绕过可使用 `PREK_SKIP`。
- `commit-msg`：校验 Conventional Commits（允许 `feat|fix|docs|style|refactor|perf|test|build|ci|chore|revert`，描述允许中文；放行 `Merge`、`Revert`、`fixup!`、`squash!`）。
- prek 拒绝在钩子配置文件有未暂存改动时提交；修改 `prek.toml` 时先单独提交配置，或与其他改动一起暂存。

## Go module 与 CI 门禁

- 根 `go.mod`、`go.sum` 统一管理共享代码和服务，不在生成目录或应用目录新增 Go module，也不提交 `go.work`。
- 受影响代码须通过 `GOWORK=off` 的构建、lint 和测试；生产代码须保持 `CGO_ENABLED=0` 可编译（`mattn/go-sqlite3` 仅限 `*_test.go` 使用），三处统一强制 `CGO_ENABLED=0` 构建门禁：`just/service.just` `_build` 默认值（支持环境变量覆盖）、`prek.toml` `pre-push` 的 `go-build-nocgo` 与 `.github/workflows/ci.yml` `go` 任务；`just test` 保持默认 CGO 环境以支持 `-race` 与 `go-sqlite3` 测试。根门禁覆盖全仓，服务级检查只提供定向反馈，不能互相替代。
- 依赖变更执行 `GOWORK=off go mod tidy` 并确认结果稳定；父级工作区通过而独立检查失败时，不视为验收通过。
- CI 门禁在 `GOWORK=off` 下按路径分流运行各领域检查，并由聚合检查 `ci-ok` 作为唯一必过检查（`0.x` 阶段 `ci.yml` `proto` 任务关闭自动 breaking 检查，待 `v1.0.0` 恢复）。对外发布由三条工作流分工承载：`buf-ci.yml` 处理 PR 校验（`0.x` 阶段显式 `breaking: false`，待 `v1.0.0` 恢复）与 `main` 分支推送至 BSR `buf.build/servora/plateau`；`release.yml` 监听 `v*` 标签，在校验标签提交位于 `origin/main` 且 `ci.yml == success` 后，串行推送 BSR `v*` 版本标签并创建 GitHub Release；`publish-plateau-utils.yml` 监听 `plateau-utils/v*` 标签并通过 NPM OIDC `--provenance` 发布 `@servora/plateau-utils`。工作流与门禁配置不硬编码服务名或 Web 包名。
- 开发或审查时先选择受影响模块和检查命令；基础设施 provider 的业务日志由 data/bootstrap 边界决定，不在构造函数中重复记录成功或失败。验证记录留在任务或变更说明，不写入规范。

入口：[根命令](../../../../justfile)、[进程编排](../../../../process-compose.yaml)、[工具版本](../../../../mise.toml)、[本地钩子](../../../../prek.toml)、[服务注册](../../../../just/services.just)、[服务命令](../../../../just/service.just)。
