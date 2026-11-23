# 本地开发与检查入口

仓库使用单一根 Go module；父级工作区只用于本机跨仓联调，仓库门禁须在 `GOWORK=off` 下独立运行。命令从仓库根目录执行，服务级命令从对应 service 目录执行。

## 固定端口

每个应用保留 10 个端口：+0 HTTP、+1 gRPC、+2 Web，其余预留。新增应用使用已登记的空闲段，不重排已有编号；具体分配以[项目约定](../../../../AGENTS.md)为准，不在规范重复业务端口表。

原生服务与容器宿主映射不能占用同一端口，Web dev 与 preview/start 不同时运行。公开入口变更须同步 origin、Web 端口和后端代理地址。

## 跨仓联调

父级 Go/pnpm workspace 仅作为本机源码联调层，不提交到仓库。pnpm 使用最近的 workspace；跨仓命令显式从共同父目录执行，独立第三方 workspace 保持自己的依赖和 lockfile。

本地包链接在安装后生效；exports 指向构建产物的共享包必须先构建或启用 watch，不能把 workspace 注册等同于已可消费。

## 命令及副作用

| 入口 | 用途 |
| --- | --- |
| `just init` | 依次执行 `mise install`、`prek install`、`pnpm install --frozen-lockfile`，完成工具链、Git 钩子与根 pnpm 依赖准备 |
| `just gen` | 根 API 生成后执行各服务 OpenAPI、Wire、Ent；修改 Proto 后使用（沿用当前 Go 环境，支持父级 `go.work` 联调） |
| `just gen-check` | 在 `GOWORK=off` 下运行 `just gen`，并对生成路径校验 `git diff --exit-code` 与未跟踪文件 |
| `just wire` | 各服务 Wire 装配刷新 |
| `just test` | 在根 module 运行 `go test -race -count=1 ./...` |
| `just lint` | API TS typecheck、Buf lint、从根 module 执行的全仓 Go lint；不等于全仓 Go 测试 |
| `just fmt-proto-check` | `buf format -d --exit-code` 检查 Proto 格式 |
| `just breaking` | `buf breaking --against` 检查破坏性变更（默认对比 `.git#ref=origin/main`，可用 `BUF_BREAKING_AGAINST` 覆盖） |
| `just api-ts-check` | 共享生成 TS 契约检查 |
| `just web::<应用>::dev`／`build`／`lint` | 应用 Web 开发、构建与 lint；其他工具使用所属 package 的 pnpm script |
| `just openfga-model-validate`／`test` | 本地 model 与场景检查 |
| `just openfga-model-apply` | 修改 model 后的应用步骤，会写外部 model／环境配置，需在明确目标环境执行 |

## 工具版本与单一来源

- Go 版本由根 `go.mod` 管理：`go` 指令表示最低支持版本，不为补丁版本抬高，避免强制依赖方和本地 `go.work` 跟着升级；需要标准库安全补丁时用 `toolchain` 指令固定补丁版本，CI 通过 `actions/setup-go` 读取 `go.mod`。
- `buf`、`golangci-lint`、`just`、`fga`（`aqua:openfga/cli`）、`prek`、`node`、`pnpm`、`air` 由根 `mise.toml` 固定版本；本地通过 `mise install`、CI 通过 `jdx/mise-action` 使用同一份版本。
- 所有 protoc 插件（含 Plateau AuthN/AuthZ 与 Servora 插件）及 Wire/Ent 均在根 `go.mod` 的 `tool` 块声明并通过 `go tool` 执行；不再使用 `just plugin`/`just cli` 安装，Servora 插件版本随 `go.mod` 中的 `github.com/Servora-Kit/servora` 同步。生成路径与入口详见 [generation](../../api/proto/generation.md)。
- 依赖与工具版本由 Renovate 跟踪更新。
- 根 justfile 启用了 `set export`，所有 Just 变量都会导出给 recipe 及其子进程；读取外部环境变量的 Just 变量不能与服务配置、测试使用的环境变量同名，否则空默认值会覆盖配置里的 `${VAR:默认值}`。

## 本地钩子分层（prek）

根 `prek.toml` 与子项目 `app/admin/web/prek.toml` 共同构成 workspace 钩子配置，全局排除生成产物与锁文件：

- `pre-commit`：通用文件卫生（内置 Rust 钩子）、`gofmt`、`buf format`、`just --fmt --check --unstable`；`app/admin/web` 仅对暂存文件执行 `oxlint`/`oxfmt`/`eslint`/`stylelint` 自动修复，不跑全量类型检查。
- `pre-push`：在 `GOWORK=off` 下运行 `just lint-go`、`just lint-proto`、`just breaking`、`just gen-check`；`app/admin/web` 运行 `pnpm check:type`。push 前工作区应保持干净，临时绕过可使用 `PREK_SKIP`。
- `commit-msg`：校验 Conventional Commits（允许 `feat|fix|docs|style|refactor|perf|test|build|ci|chore|revert`，描述允许中文；放行 `Merge`、`Revert`、`fixup!`、`squash!`）。
- prek 拒绝在钩子配置文件有未暂存改动时提交；修改 `prek.toml` 时先单独提交配置，或与其他改动一起暂存。

## Go module 与 CI 门禁

- 根 `go.mod`、`go.sum` 统一管理共享代码和服务，不在生成目录或应用目录新增 Go module，也不提交 `go.work`。
- 受影响代码须通过 `GOWORK=off` 的构建、lint 和测试；根门禁覆盖全仓，服务级检查只提供定向反馈，不能互相替代。
- 依赖变更执行 `GOWORK=off go mod tidy` 并确认结果稳定；父级工作区通过而独立检查失败时，不视为验收通过。
- CI 门禁在 `GOWORK=off` 下按路径分流运行各领域检查，并由聚合检查 `ci-ok` 作为唯一必过检查；工作流与门禁配置不硬编码服务名或 Web 包名。
- 开发或审查时先选择受影响模块和检查命令；基础设施 provider 的业务日志由 data/bootstrap 边界决定，不在构造函数中重复记录成功或失败。验证记录留在任务或变更说明，不写入规范。

入口：[根命令](../../../../justfile)、[工具版本](../../../../mise.toml)、[本地钩子](../../../../prek.toml)、[服务注册](../../../../just/services.just)、[服务命令](../../../../just/service.just)。
