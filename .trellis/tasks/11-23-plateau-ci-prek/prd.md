# 设计 Plateau GitHub CI 与 prek 本地钩子

## Goal

给 plateau 补上 GitHub Actions 质量门禁，让本地钩子（prek）和 CI 共用同一组规则、同一份工具版本；依赖与工具由 Renovate 自动跟踪。微服务框架的日常推进因此有统一、可重复的规范约束，日后接入镜像发布时也不必改动已有门禁。

参考：tx7do/go-wind-admin、lin-snow/Ech0、Servora、Kratos/go-zero/Kitex（调研见 [research/ci-references.md](research/ci-references.md)；技术设计见 [design.md](design.md)，执行清单见 [implement.md](implement.md)）。

## Background（仓库证据）

- 仓库现状：没有 `.github/`、hook 配置、`.golangci.yml`；仓库 `Servora-Kit/plateau` 为 PUBLIC，默认分支 `main`。门禁入口全部在 Just：`just lint`（api-ts-check + buf lint + 根 golangci-lint）、`just gen`（buf Go/TS + 各服务 OpenAPI/Wire/Ent）、`just web::lint`、`just openfga-model-test`（[justfile](../../../justfile)、[just/service.just](../../../just/service.just)）。
- 生成产物入库：`api/gen/{go,ts}`、`wire_gen.go`、Ent、OpenAPI 都已提交，可以做漂移检查。`just plugin` 大多用 `@latest` 安装，Servora 插件靠 `SERVORA_VERSION` 手工同步；Wire/Ent 已由 go.mod `tool` 固定。
- 门禁须在 `GOWORK=off` 下独立运行（[development.md](../../spec/plateau/project/development.md#L34-L39)），Go `1.27.3`。
- Web：根 pnpm workspace（`api/gen`、`app/*/web`、`web/packages/*`）；`app/admin/web` 是 Vben 独立 workspace（build 需要 8GB 内存）。
- 集成测试在缺少 `IAM_TEST_POSTGRES_DSN`/`IAM_TEST_FGA_URL` 等变量或本地 Postgres 不可达时自动 Skip（[postgres_test.go](../../../app/iam/service/internal/data/postgres_test.go#L21-L23)、[fixture_test.go](../../../app/iam/service/internal/server/fixture_test.go#L70-L107)、[integration_test.go](../../../app/admin/service/internal/server/integration_test.go#L211-L221)）；所需依赖与 `docker-compose.yaml` 中的 postgres:18、redis:8.6.1（requirepass）、openfga 一致。
- 本地钩子：没有任何 git hook 生效。Vben 的 [lefthook.yml](../../../app/admin/web/lefthook.yml) 和 `.commitlintrc.js` 位于子目录，从未生效。提交信息习惯为 `type(scope): 中文`。
- 没有服务 Dockerfile；根 `compose-build` 引用的根 `Dockerfile` 不存在。
- Servora CI 约定：`GOWORK=off`、`just --fmt --check`、`go test -race`、按 proto paths 跑 buf-action、tag 必须在 main 且 main CI 已通过才发布。
- 本机工具：prek 0.5.11、just 1.59.0、golangci-lint 2.15.0、buf 1.75.0、fga 0.9.0、node 24、pnpm 12.21.0（都通过 mise 全局安装）。

## Requirements

- **R1 工具版本单一来源**：新增 `mise.toml`，固定 buf、golangci-lint、just、fga、prek、node、pnpm、air；Go 版本以 `go.mod` 为准。本地和 CI 读取同一份版本（D5）。
- **R2 生成器固定**：protoc 插件全部改为 go.mod `tool` 声明，buf 模板通过 `go tool` 调用；删除 `SERVORA_PKG`/`SERVORA_VERSION`/`just plugin`/`just cli`；`just init` 一条命令完成 mise、prek、pnpm 的准备（D2）。
- **R3 prek 三层钩子**：根 `prek.toml` + `app/admin/web/prek.toml`；pre-commit 只做快速检查，pre-push 做重检查，commit-msg 校验格式；删除失效的 lefthook/commitlint（D3）。
- **R4 CI 门禁**：`ci.yml` 包含 changes、prek、go、proto、gen、integration、web、admin-web 和聚合检查 `ci-ok`；按 job 级路径分流；`GOWORK=off`；最小权限；同一 ref 并发时取消旧运行（D1、D4）。
- **R5 依赖自动跟踪**：`renovate.json` 统一升级 gomod（含 tool）、npm（两个 workspace）、GitHub Actions、mise、compose 镜像，分组、每周定时、不自动合并（D5）。
- **R6 漏洞扫描**：`security.yml` 运行 govulncheck，范围是 go.mod/go.sum 变化的 PR 加每周定时（用户 2026-11-23 批准，D6）。
- **R-EXT 可扩展发布**：分支保护只依赖 `ci-ok`；以后新增 docker/release workflow 不用改门禁 job，可以沿用 Servora 的「tag 必须在 main 且 CI 已通过」做法；镜像矩阵从 `app/*/service/Dockerfile` 动态生成。
- **R-DYN 服务集合不写死**：`app/test`、`app/example` 已移出仓库（`9e4b4774`）。workflow、prek、paths 过滤都不写服务名或 Web 包名（Go 用 `./...`，Web 用 `pnpm -r`）。

## Acceptance Criteria

- [ ] 干净 clone 后执行 `just init`，即可装齐固定版本的工具、git 钩子和 pnpm 依赖，不需要再执行 `just plugin`（R1、R2、R3）
- [ ] 仓库里不再出现 `SERVORA_VERSION`、`@latest` 插件安装；升级 go.mod 中的 servora 后，`just gen` 自动使用对应版本的插件（R2）
- [ ] 干净工作区上连续两次 `GOWORK=off just gen-check` 都通过；手动改动一个生成文件后会失败，并指出文件（R2、R4）
- [ ] 暂存一个格式错误的 Go/proto/justfile 文件时，`git commit` 被 pre-commit 拦截；提交不符合 `type(scope): 描述` 的 message 时，被 commit-msg 拦截；`Merge`/`Revert` 提交放行（R3）
- [ ] 只改 admin web 的 TS 文件时，pre-commit 只对暂存文件跑 oxlint/oxfmt/eslint/stylelint，不跑全量类型检查（R3）
- [ ] `prek run --hook-stage pre-push --all-files` 依次执行 golangci-lint、buf lint、buf breaking、gen-check 并全部通过（R3）
- [ ] PR 上 `ci.yml` 全部变绿；只改 Web 时 go/proto/gen/integration 被跳过且 `ci-ok` 仍为绿；只改 proto 时 web 相关 job 被跳过（R4）
- [ ] integration job 日志中 IAM、Admin 集成测试实际执行，没有被 Skip（R4、D4）
- [ ] workflow 通过 `actionlint`（R4）
- [ ] Renovate 在仓库上生成 Dependency Dashboard，并按分组提交第一批 PR（R5，需要用户先安装 App）
- [ ] workflow 和 prek 配置中搜不到 `example`、`test` 等服务名（R-DYN）
- [ ] `AGENTS.md`、`development.md`、`generation.md` 已同步新的命令、版本来源和门禁说明

## Decisions

- D1 范围（2026-11-23）：只做质量门禁；不做 Release、Dockerfile、镜像发布。
- D2 生成器版本（2026-11-23）：protoc 插件改为 go.mod `tool`，Servora 插件跟随 servora 依赖，删除 `SERVORA_VERSION`；buf、golangci-lint 保持二进制。用户原则：能自动跟踪的都自动跟踪，同时方便、易维护。
- D3 prek 三层钩子（2026-11-23）：pre-commit（文件卫生、gofmt、buf format、just fmt；admin 只对暂存文件做 lint 修复）、pre-push（golangci-lint、buf lint、buf breaking、gen-check）、commit-msg（Conventional Commits，描述允许中文）；CI 跑同一份规则。
- D4 集成测试（2026-11-23）：独立 `integration` job，PR 和 main 上 Go/proto/OpenFGA 有改动时运行，作为必过门禁；用仓库 compose 启动依赖。
- D5 依赖 bot（2026-11-23）：Renovate + `mise.toml`；使用 Mend Renovate Community App，选 Renovate Only（免费）和 Silent 模式。
- D6 漏洞扫描（2026-11-23）：加入 `security.yml`，运行 govulncheck。

## Out of Scope

- 服务 Dockerfile、GHCR 镜像发布、tag 触发的 GitHub Release（D1）。
- CodeQL、trivy、gosec；codecov 等第三方覆盖率服务（覆盖率只写 step summary）。
- 修改业务代码、生成器逻辑本身；修正 IAM openapi 输出路径不一致问题。
- `docker-compose.yaml` 中与集成测试无关的 `latest` 镜像固定。
- 迁移 Servora 仓库到 Renovate。
- GitHub 分支保护设置（实施完成后由用户操作或确认后执行）。
