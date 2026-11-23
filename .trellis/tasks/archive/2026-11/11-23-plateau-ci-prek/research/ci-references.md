## Summary

- **go-wind-admin**：`.github/workflows` 只有 3 个**窄触发**工作流（README 三语对齐、update-mask allowlist 对齐、OIDC SSO 全链路真依赖冒烟），**没有** Go 单测/lint/CodeQL/release 流水线，没有 dependabot、没有仓库级 pre-commit；本地钩子只存在于 `frontend/admin/react`（husky + lint-staged）与另两个前端的 `lint-staged` 配置。质量门禁是「脚本 + push paths」型，而不是「全量 CI」型。
- **Ech0**：14 个 workflow，按职责域拆得很细（后端测试、CLI 类型漂移、i18n、部署、release、helm、zigcc 交叉编译、hub 运维）。最值得抄的是：**生成物漂移用 `git diff --exit-code` 卡死**（mocks / wire / OpenAPI / CLI client 四类）、**coverage 分 RAW 与 CALIBRATED 两档写 step summary**、**release 前 `verify-version` 把 tag 和 `version.go` 对账**、**容器镜像发布后验证 PID 1 非 root**。缺 lint/安全扫描流水线。
- **Kratos / go-zero / Kitex**：三个框架主 CI 都是「build + test(+race) + coverage + lint」，差异在于 *模块化 lint 矩阵*（Kratos `resolve-modules.sh` 动态矩阵）、*merge_queue 支持*（go-zero）、*Windows/ARM 矩阵*（go-zero、Kitex）、*静态检查下沉到 PR 注释*（go-zero reviewdog、Kitex only-new-issues）。安全只有 CodeQL（Kratos、go-zero），**无人用 govulncheck/trivy/gosec**。
- **prek**：pre-commit 的 Rust 重写，单二进制、配置完全兼容（`.pre-commit-config.yaml` 原样可用）、原生 workspace/monorepo、内置 Rust hooks、`j178/prek-action`（最新 v3.1.1）一行接入 CI。对 plateau（多 Go module + pnpm workspace + just 入口）适配度很高，唯一硬约束是 `just`/`buf`/`pnpm` 这些工具必须先在 runner/本机就绪（prek 只托管 Python/Node/Bun/Go/Rust/Ruby 等 toolchain）。

---

## 1. tx7do/go-wind-admin（本地 clone，已核对上游 main 一致）

### 1.1 文件清单

| 文件 | 内容 |
| --- | --- |
| `.github/workflows/docs-parity.yml` | 三语 README 结构对齐 |
| `.github/workflows/update-mask-allowlist-parity.yml` | 三端前端 update-mask 白名单一致性 |
| `.github/workflows/sso-smoke.yml` | OIDC SSO 全链路真依赖 e2e |
| `backend/.golangci.yml` | golangci-lint v2 配置（排除生成代码） |
| `frontend/admin/react/.husky/{pre-commit,commit-msg}` | husky：`lint-staged` / `commitlint` |
| `frontend/admin/vue-vben/.lintstagedrc.mjs`、`vue-element/package.json#lint-staged`、`vue-vben/package.json#lint-staged` | ESLint/Prettier/Stylelint 暂存文件修复 |
| **不存在** | 根 `Makefile`、`.pre-commit-config.yaml`、`dependabot.yml`、`goreleaser`、CodeQL、Go test/lint CI、任何 release/镜像 workflow |

### 1.2 逐 workflow

| workflow | 触发 | jobs | matrix / 运行时 | 关键 action | 缓存 | concurrency / permissions |
| --- | --- | --- | --- | --- | --- | --- |
| `docs-parity` | `push`/`pull_request` + paths `README.md`,`README.en-US.md`,`README.ja-JP.md` | 1：`readme-parity` → `node scripts/check-readme-parity.mjs` | ubuntu-latest，无 Go | `actions/checkout@v4` | 无 | 无 / 无 |
| `update-mask-allowlist-parity` | push/PR + paths `frontend/admin/{react,vue-element,vue-vben}/src/**`、`scripts/check-update-mask-allowlists.mjs` | 1：`node scripts/check-update-mask-allowlists.mjs` | ubuntu-latest | `actions/checkout@v4` | 无 | 无 / 无 |
| `sso-smoke` | `workflow_dispatch` + push paths（`authentication_sso*.go`、`transactional_mail*.go`、`sso_state_cache.go`、`user_repo.go`、`backend/pkg/mailtext/**`、`backend/scripts/deploy/sso/**`、自身） | 1：`sso-smoke` | ubuntu-latest；`postgres:16-alpine` service；docker 起 `redis:7-alpine`(requirepass)、`quay.io/keycloak/keycloak:26.0 start-dev` | `actions/checkout@v4`、`actions/setup-go@v5`（`go-version-file: backend/go.mod`）、`actions/setup-node@v4`（node 20） | **无**（Go/Node 均未开 cache） | 无 / 无 |

要点：
- `sso-smoke` 是真正的端到端：`/etc/hosts` 映射 postgres/redis → `setup-keycloak.mjs` 初始化 realm/client/user → `go build ./app/admin/service/cmd/server` → 起服务轮询 `/admin/v1/sso/login-info` 到 200 → `sso-e2e.mjs` 验证 discovery/state/code 交换/JWT/state 重放拒绝 → `if: failure()` 转储 `/tmp/backend.log`。**失败可诊断**这点做得好。
- 三个 workflow 都**没有** `concurrency`、`permissions`、版本矩阵、覆盖率、lint。Go 侧唯一质量入口是本地 `make lint`（`golangci-lint run`）/`go vet ./...`，见 `backend/Makefile:71-73` 与 `backend/AGENTS.md`。生成代码一致性检查、proto breaking、安全扫描：**全部缺失**（`backend/api/buf.yaml` 声明了 `breaking: FILE` 但无 CI 调用）。
- 依赖 bot：无。release：无 `goreleaser`，仅 `make docker*` 本地构建（`backend/Makefile:113+`）。

---

## 2. lin-snow/Ech0（远端 main）

### 2.1 文件清单（`.github/workflows/`，14 个）

`test.yml`、`cli-check.yml`、`i18n-guardrails.yml`、`docker-test-image.yml`、`release.yml`、`release-cli.yml`、`release_helm.yml`、`release_zigcc.yml`、`deploy-hub.yml`、`deploy-site.yml`、`deploy-slides.yml`、`hub-health-cleanup.yml`、`hub-register-from-issue.yml`；另有 `.github/actions/setup-musl-toolchain/action.yml`（composite）、`.github/dependabot.yml`。**无** `.pre-commit-config.yaml` / `prek.toml` / `lefthook.yml`（tree 里 grep 确认），本地门禁统一走 `just`（`justfile`、`scripts/check.sh`）。

### 2.2 核心 workflow

| workflow | 触发 | jobs | matrix | 关键 action | 缓存 | concurrency / permissions |
| --- | --- | --- | --- | --- | --- | --- |
| `test.yml` "Backend Tests" | PR + push `main` + dispatch，paths anchor `&backend-paths`：`**.go`,`go.mod`,`go.sum`,`.mockery.yaml`,`justfile`,自身 | 1：`go test (race + coverage)` | ubuntu；Go **1.27.4** 固定 | `actions/checkout@v7`、`actions/setup-go@v7`（`cache-dependency-path: "**/go.sum"`）、`extractions/setup-just@v4`、`actions/upload-artifact@v7` | setup-go 内建 cache（go.sum hash） | `group: backend-tests-${{ github.ref }}`、`cancel-in-progress: true` / `contents: read` |
| `cli-check.yml` | PR + push `main`，paths `apps/cli/**`,`internal/openapi/**`,`skills/ech0/**` | 1：`bun typecheck + test + gen-check` | ubuntu；Bun **1.4.3**（env 固定） | `actions/checkout@v7`、`oven-sh/setup-bun@v2` | 无（bun install 每次） | `cli-checks-${{ github.ref }}` cancel / `contents: read` |
| `release.yml` | `push: tags: v*` | 4：`verify-version` → `build`(matrix) → `prepare-release` + `build-docker` | `goos: linux × goarch: amd64,arm64`；Node 26 + pnpm **12.16.1**；Zig **0.17.1** 固定 | `pnpm/action-setup@v6`、`actions/setup-node@v7`（`cache: pnpm`）、`actions/setup-go@v7`、`mlugg/setup-zig@v2`、本地 composite `./.github/actions/setup-musl-toolchain`、`actions/upload-artifact@v7`、`actions/download-artifact@v8`、`softprops/action-gh-release@v3`、`docker/login-action@v4`、`docker/setup-qemu-action@v4`、`docker/setup-buildx-action@v4`、`docker/build-push-action@v7` | 前端 pnpm cache；musl toolchain 用 `cache/restore|save` 手工键 | 无 concurrency（发布不打断）/ `contents: write`, `packages: write` |
| `release-cli.yml` | `push: tags: cli-v*` + dispatch（可指定 tag/publish） | 2：`build` → `npm` | ubuntu；Bun 1.4.3、Node 26 | `oven-sh/setup-bun@v2`、`actions/upload-artifact@v7`、`actions/download-artifact@v8`、`actions/setup-node@v7` | 无 | `release-cli-${{ tag \|\| ref }}`、`cancel-in-progress: false` / `contents: write`, `id-token: write`（OIDC npm provenance） |
| `release_helm.yml` | `release: published` + dispatch | 1 | ubuntu | `helm/chart-releaser-action@v1.9.0`、`gh` CLI（latest 指针回写、gh-pages worktree 部署） | 无 | 无 / `contents: write`, `packages: write` |
| `docker-test-image.yml` | `workflow_dispatch`（`image_tag`、`push_test_latest`） | 2：`build`(amd64/arm64) → `build-and-push-image` | linux amd64/arm64 | pnpm/setup-node/setup-go v7、本地 musl composite、`docker/*@v4`、`docker/build-push-action@v7` | `cache-from/cache-to: type=gha` | 无 / `contents: read`, `packages: write` |
| `i18n-guardrails` | **仅 `workflow_dispatch`** | 1：`pnpm run i18n:check` | ubuntu；pnpm 12.16.1、Node 26 | `pnpm/action-setup@v6`、`actions/setup-node@v7` | pnpm cache | 无 / `contents: read` |
| `deploy-{hub,site,slides}` | push `main` + 各自 paths | 1 | ubuntu | `pnpm/action-setup@v6`、`actions/setup-node@v7`、`wrangler@4 deploy` | pnpm cache（hub/site） | `deploy-*-${{ github.ref }}` cancel / `contents: read` + `deployments: write` |
| `hub-health-cleanup` | `schedule: '0 0 * * *'` + dispatch | 1（`if: repository_owner == 'lin-snow'`） | ubuntu | `actions/github-script@v9`（探测 /healthz、SSRF 防护 `redirect: 'manual'`、72h 阈值移除） | 无 | `hub-health-cleanup`、`cancel-in-progress: false` / `contents: write` |
| `hub-register-from-issue` | `issues: opened,reopened` + dispatch | 1 | ubuntu | `actions/github-script@v9`、`peter-evans/create-pull-request@v8` | 无 | 无 / `contents/issues/pull-requests: write` |
| `release_zigcc` | **仅 dispatch** | 1 矩阵 | 8 组：linux `{amd64,arm64,riscv64,arm,386,loong64}` + windows `{amd64,arm64}`，Zig `latest` | `mlugg/setup-zig@v2`、`actions/upload-artifact@v7`（3 天保留） | 无 | 无 / `contents: read` |

### 2.3 可复用的具体手法

- **生成物漂移（4 处，都是硬门禁）**
  - `scripts/check.sh` + `justfile`：`wire-check` = `wire` 后 `git diff --exit-code -- internal/di/wire_gen.go`；`mocks-check` = `mocks` 后 `git diff --exit-code -- internal/test/mocks`；`openapi-check` 同理。`test.yml` 里实跑 `just mocks-check`。
  - `cli-check.yml`：`bun run gen:api` 后 `git diff --exit-code -- apps/cli/src/api/generated`，失败信息带 `::error title=Generated client out of date::`。
- **coverage 双口径**：`test.yml` 用 `grep -v -E 'internal/test/mocks/|/wire_gen\.go:'` 生成 `coverage.calibrated.out`，把 RAW/CALIBRATED 两行写进 `$GITHUB_STEP_SUMMARY`，同时上传 `coverage.out` artifact（**不接第三方 codecov**）。
- **release 三方对账**：`verify-version` 从 `internal/version/version.go` 抽 `Version` 与 `v*` tag 比较，不一致直接 `::error title=Version mismatch::`；`release_helm` 再从 tag 反写 `Chart.yaml` 的 `version/appVersion/image` 并校验三者一致；`release-cli` 校验 `cli-v*` 与 `apps/cli/package.json` 一致、`publish_npm`、`--latest=false` 不抢 Latest 指针。
- **发布后验证**：`build-docker` 末尾 `docker run -d` 后轮询 `/proc/1/status` 确认 `Uid: 10001`（entrypoint 用 `su-exec` 降权），失败打 `docker logs` 并 `exit 1`。
- **交叉编译约束写成代码**：`ZIG_VERSION`/`GOAMD64=v1`/`GOARM64=v8.0`/`-mcpu=baseline` 全部 pin，并用 Python 解析 ELF 检查无 `PT_INTERP`、`GNU_PROPERTY_X86_ISA_1_NEEDED` 不高于 baseline，然后 `./ech0 version` 冒烟（注释里写明 v5.9.10 的 SIGILL 事故）。
- **依赖 bot**：`dependabot.yml` 4 个 ecosystem（`gomod /`、`npm /web`、`github-actions /`、`docker /`），每周一 03:00/03:15/03:30/03:45 UTC 错峰，`open-pull-requests-limit: 5`，patch+minor 分组成单 PR。
- 缺失：`golangci-lint` 不在任何 workflow 里跑（只在 `just check` 本地）；无 CodeQL/govulncheck/trivy/SBOM/cosign（`docs/dev/release-process.md` 把这些列进 "Future improvements"）；`i18n-guardrails` 只 dispatch 不进 PR，容易被遗忘。

---

## 3. 主流框架

### 3.1 go-kratos/kratos

| 文件 | 触发 | jobs / 机制 |
| --- | --- | --- |
| `go.yml` | push/PR `main` + dispatch | `build & test`，`matrix.go: [1.25.x, 1.26.x]`；**services**：etcd v3.5.0、consul 1.20、nacos v2.1.0、polaris-standalone |
| `lint.yml` | push(任意)/PR `main`/dispatch | `resolve-modules`（`./hack/resolve-modules.sh` 扫所有 `go.mod` 输出 `matrix.include[].workdir`）→ `lint`，`matrix: fromJson(...)` |
| `codeql-analysis.yml` | push `main,v1.0.x` | CodeQL `go`，init+analyze，无 autobuild |
| `comment-check.yml` | `pull_request_target` opened/synchronize/reopened | 非英文注释检查（用 API 读 diff，不 checkout fork 代码），`pull-requests: write` |
| `gitee-sync.yml` | push `main` + tags `*` | `Yikun/hub-mirror-action@v1.5` 镜像到 Gitee |
| `issue-translator.yml` | issue/issue_comment | `usthe/issues-translate-action@v2.7` |

关键 action/版本：`actions/checkout@v7`、`actions/setup-go@v7`、`actions/cache@v6`（手工 `~/.cache/go-build` + `~/go/pkg/mod`，key=`hashFiles('**/go.sum')`）、`golangci/golangci-lint-action@v7`（`version: v2.12.2`、`working-directory: ${{ matrix.workdir }}`、**`skip-cache: true`**）、`github/codeql-action/*@v4.37.3`。coverage 用**旧式** `bash <(curl -s https://codecov.io/bash)`（无 pin、无 `fail_ci_if_error`）；`make test-coverage` 遍历所有 module 追加到统一 `coverage.out`（`hack/tools.sh`）。无 concurrency、无 permissions、无 dependabot 的 gomod（**只有 github-actions，monthly**）。
配置文件：`.golangci.yml`（v2，`default: none` + 21 个显式 linter，`gocyclo.min-complexity: 50`、`lll: 160`、`govet.enable: [shadow]`，formatters `gofmt/gofumpt/goimports`）；`buf.yaml` v2（`breaking: FILE`，但 CI **不含** buf 步骤）；`codecov.yml` 忽略 `examples`、`**/*.pb.go`；无 release（库 module）。

### 3.2 zeromicro/go-zero（master）

| 文件 | 触发 | jobs |
| --- | --- | --- |
| `go.yml` | **`merge_group: checks_requested`** + push/PR `master` | `test-rpc-generation`（tools/goctl，装 `protobuf-compiler` + pin 版本 `protoc-gen-go`/`protoc-gen-go-grpc@v1.6.2`，`go test -race ./rpc/...`）；`test-linux`（`go vet -stdmethods=false` → **`go mod tidy` 后 `git status --porcelain` 非空即失败** → `go test -race -coverprofile` → codecov）；`test-win`（`windows-latest`，`go mod verify/download`、`go test`、`goctl` 构建） |
| `reviewdog.yml` | `pull_request` | `reviewdog/action-staticcheck@v1`，`reporter: github-pr-review`、`filter_mode: nofilter`、`fail_level: any`、`staticcheck_flags: -checks=inherit,-SA1019,-SA1029,-SA5008` |
| `codeql-analysis.yml` | merge_group + push/PR master + `cron '18 19 * * 6'` | CodeQL `go`，`github/codeql-action/*@v4.39.3` |
| `release.yml` | `push: tags: tools/goctl/*` | matrix `goos: [linux,windows,darwin] × goarch: [386,amd64,arm64]`（排除 darwin/386），**`zeromicro/go-zero-release-action@master`**（自研 action，`goversion` 显式下载 tarball） |
| `version-check.yml` | tag `tools/goctl/v*` + dispatch | 从 tag 抽版本 vs 源码 `const BuildVersion`，不一致 exit 1 |

`test-linux` 用 `actions/setup-go@v7`（`go-version-file: go.mod` = **go 1.25.0**、`check-latest: true`、`cache: true`）与 `codecov/codecov-action@v7`（`use_oidc: true`、`fail_ci_if_error: true`，`permissions: contents: read, id-token: write`）；`reviewdog` 用 `secrets.github_token`。**无 concurrency**；`release.yml` 的 `@master` 引用是供应链隐患。无根 `.golangci.yml`（404）；`dependabot.yml` 4 条（docker/gh-actions weekly、gomod `/` 与 `/tools/goctl` daily）。无 goreleaser，release 走自研 composite action。

### 3.3 cloudwego/kitex

| 文件 | 触发 | jobs |
| --- | --- | --- |
| `tests.yml` | **`pull_request` only** | `unit-scenario-test`（matrix go `1.21`/`1.26`，`runs-on: [Linux, X64]`，clone 外部 `kitex-tests` 跑 `run.sh`，codecov `flags: integration`）；`benchmark-test`（`go test -bench=. -benchtime=100ms`）；`unit-test-x64`（matrix **1.21…1.26 六个版本**）；`unit-test-arm`（`ubuntu-24.04-arm`，同六版本，`cache: true`）；`codegen-test`（`thriftgo@main`、本地 `tool/cmd/kitex`、跨仓 `replace` 后跑 `codegen_run.sh`）；`windows-test`（`go test -run=^$ ./...` 仅编译检查）；`codecov`（needs 三个单测 job，重跑一次 `-race -coverprofile` 上传，`flags: unit`） |
| `pr-check.yml` | `pull_request` | `compliant`：`apache/skywalking-eyes/header@v0.4.0`（License header）+ `crate-ci/typos@master`；`golangci-lint`：`golangci/golangci-lint-action@v8`（`version: latest`、**`only-new-issues: true`**） |
| `claude.yml` / `claude-review-fork.yml` | `@claude` 提及事件 / dispatch | `anthropics/claude-code-action@v1`（fork PR 走 dispatch 手动触发，`--allowed-tools` 白名单限制为 `gh pr/issue` 只读命令） |

`actions/checkout@v4`、`actions/setup-go@v5`（多处 **`cache: false`** 并注释 "false for self-hosted runners"）、`codecov/codecov-action@v5`。**无 concurrency、无 permissions、无 push 触发**（master 无 CI，只有 PR）。`.golangci.yaml` v2：`default: none` + `govet/ineffassign/staticcheck/unconvert/unused`，排除 `kitex_gen`；`.codecov.yml` `threshold: 0.05%`；`.typos.toml` 自定义词典；无 release（库 module）；dependabot 只升 `github.com/cloudwego/*` + `bytedance/sonic`，gomod daily 10:00 Asia/Shanghai 分组。

### 3.4 同组织参考：Servora-Kit/servora（plateau 的直接上游约定）

`ci.yml`：`GOWORK=off`、`just --fmt --check`、`golangci-lint-action@v9`、`go test -short -race -coverprofile` + artifact；`concurrency: ci-${{ github.ref }}` cancel、`contents: read`。
`buf-ci.yml`：paths `**.proto`/`**/buf.yaml`/`**/buf.lock` + `delete`，`bufbuild/buf-action@v1`（`push_disable_create: true`），`pull-requests: write`；`buf.yaml` 的 `breaking: FILE` 由此真正生效。
`release.yml`：tag `v*` 先校验 **commit 在 origin/main 上** 且 **该 sha 的 ci.yml 有 success run**（`gh api .../actions/workflows/ci.yml/runs`）才发版，附 `git log` changelog。
`publish-proto-utils.yml`：`proto-utils/v*` tag，`npm publish --provenance --access public` + `id-token: write`。

---

## 4. 对比表：值得借鉴 / 应避免

| 维度 | 值得借鉴（来源） | 应避免（来源） |
| --- | --- | --- |
| 触发面 | `paths` 锚点精确化：Ech0 `&backend-paths` YAML anchor 复用、Servora buf 只对 proto 触发；`merge_group` 支持 merge queue（go-zero） | 只在 `workflow_dispatch` 跑的门禁（Ech0 i18n）等于没有门禁；Kitex 只有 PR 触发 → master 无基线 |
| 矩阵 | Kratos 动态矩阵 `resolve-modules.sh` 输出 `{workdir}`；Ech0 交叉编译 `include:` 带 `zig_target/output_suffix`；Kitex amd64/arm64 双 runner | 六版本 × 双架构无 `fail-fast: false`（Kitex 未设，默认 true，一个失败全挂）；go-wind-admin 无任何矩阵 |
| action 版本 | 全 pin 到具体 minor（Kratos `golangci-lint-action@v7` + `version: v2.12.2`；Ech0 `ZIG_VERSION: 0.17.1`） | `@master`/`@latest`（go-zero `go-zero-release-action@master`、Kitex `golangci-lint version: latest`、`typos@master`）；Ech0 大量 `@v7/@v8` 超前 major，需靠 dependabot 兜 |
| 缓存 | `setup-go cache-dependency-path: "**/go.sum"`（Ech0 多 module 正确写法）；Kratos 手工缓存 `~/.cache/go-build` + `~/go/pkg/mod` | go-wind-admin 完全无缓存；Kratos `golangci-lint-action skip-cache: true` 与自建缓存并存容易互相打脸 |
| concurrency | Ech0 几乎每个 workflow 都有 `group + cancel-in-progress`，发布类用 `cancel-in-progress: false` | Kratos/go-zero/Kitex/go-wind-admin/Servora **release** 全无 concurrency；go-wind-admin 所有 workflow 均无 |
| permissions | Ech0 逐 workflow 最小化（test=`contents: read`，release-cli=`id-token: write`）；Servora ci=`contents: read` | Kratos/Kitex/go-wind-admin 无 `permissions` 块（默认 token 全权限）；`pull_request_target` + fork 代码是高危组合（Kratos comment-check 明确不 checkout，值得学） |
| 生成物一致性 | Ech0 四处 `git diff --exit-code`（mocks/wire/openapi/CLI client）+ 明确修复命令写进 `::error`；go-zero `go mod tidy` 后 `git status --porcelain` 非空即失败 | go-wind-admin 生成代码入库却无漂移检查；plateau 同理（`api/gen`、`wire_gen.go`、Ent、OpenAPI 全入库） |
| proto | Servora `bufbuild/buf-action@v1` 按路径触发 + PR 注释 + `breaking: FILE` 真正执行 | Kratos/go-wind-admin 声明了 `breaking` 却无 CI 调用；Ech0 无 buf（OpenAPI 由 Go 生成，改用 `just openapi-check`） |
| 安全 | go-zero CodeQL 含 `schedule`（周扫）；Kitex license header + typos；Ech0 hub 脚本显式防 SSRF | 五个项目**全部**没有 govulncheck / trivy / gosec / SBOM / cosign（Ech0 自己列为 future work） |
| 测试与覆盖率 | go-zero `fail_ci_if_error: true` + `use_oidc`；Ech0 RAW/CALIBRATED 双口径 + artifact；Kitex `codecov.yml threshold 0.05%` | Kratos 旧式 `curl codecov.io/bash`（无校验、无失败注入）；go-wind-admin 无 Go 测试 CI |
| 发布 | Ech0 `verify-version` tag↔源码对账 + draft release + 多架构镜像 + 发布后非 root 校验 + helm chart-releaser/OCI + npm OIDC provenance；Servora 「tag 必须对应 main 上绿 CI」；go-zero 全平台二进制矩阵 | go-wind-admin 零发布流水线；Ech0 release 一旦 tag 错了只能重打（`immutable: true`），靠 `workflow_dispatch` 重跑兜底 |
| 可诊断性 | Ech0 失败转储后端日志、`::error title=...::` 带修复指令、`$GITHUB_STEP_SUMMARY` 写结果表 | Kitex 多 job 无 `fail-fast: false`，失败定位成本高 |

---

## 5. prek 专项

### 5.1 是什么 / 兼容性

prek 是 pre-commit 的 Rust 重写：**单二进制、无运行时依赖**，磁盘占用与耗时均优于 pre-commit，被 CPython、Airflow、FastAPI、Ruff、Sentry 等采用（README「Who is using prek?」）。官方定位是 **drop-in replacement**。

| 事项 | 结论（证据） | 链接 |
| --- | --- | --- |
| 配置兼容 | `.pre-commit-config.yaml` / `.yml` **原样可用**；也支持原生 `prek.toml`；同目录并存时优先级 `prek.toml` > `.pre-commit-config.yaml` > `.yml` | https://prek.j178.dev/compatibility/ 、https://prek.j178.dev/configuration/ |
| 迁移步骤 | ① 把脚本里的 `pre-commit` 换成 `prek`；② `prek install -f` 重装 Git shim | https://prek.j178.dev/quickstart/ |
| 命令映射 | `install-hooks→prepare-hooks`、`autoupdate→update`、`gc→cache gc`、`clean→cache clean`、`migrate-config→util yaml-to-toml`、`init-templatedir→util init-template-dir`；隐藏但可用 | https://prek.j178.dev/compatibility/ |
| 未实现 | `pre-commit hazmat` | 同上 |
| hook 仓库 | 现有 `.pre-commit-hooks.yaml` 与所有远端 hook 仓库照常工作；`rev` 语义不变 | 同上 |
| 严格双向兼容 | 需要上游也能读时：保持 YAML 且**不用** prek-only 扩展（`update`、`orphan`、`priorities`、`repo: builtin`、`groups`、`priority`、`env`、`shell`、glob 形式的 `files/exclude` 等） | https://prek.j178.dev/compatibility/#if-you-need-strict-upstream-portability |

### 5.2 安装方式

| 方式 | 命令 | 链接 |
| --- | --- | --- |
| 独立脚本 | `curl --proto '=https' -LsSf https://github.com/j178/prek/releases/download/v0.5.11/prek-installer.sh \| sh`（Win 用 `.ps1`）；CI 用 `PREK_UNMANAGED_INSTALL=/dir` 跳过改 PATH/self-update | https://prek.j178.dev/installation/ |
| uv / pip / pipx | `uv tool install prek`、`uvx prek`、`pip install prek`、`pipx install prek` | 同上 |
| Homebrew / mise / Nix / conda / Scoop / Winget / MacPorts | `brew install prek`、`mise use prek`（≥2025.8.11）、`nix profile install nixpkgs#prek`、`conda install conda-forge::prek`、`scoop install main/prek`、`winget install --id j178.Prek`、`sudo port install prek` | 同上 |
| cargo | `cargo binstall prek` / `cargo install --locked prek`（Rust 1.97+） | 同上 |
| npm | `npm i -g @j178/prek` / `pnpm add -D @j178/prek` / `bunx @j178/prek` | 同上 |
| Docker | `ghcr.io/j178/prek:{X.Y.Z,latest,alpine,-alpine3.24}`；scratch 版可直接 `COPY --from=ghcr.io/j178/prek:0.5.11 /prek /usr/local/bin/prek` | https://prek.j178.dev/integrations/ |
| GitHub Actions | `j178/prek-action`（最新 **v3.1.1**，2026-11-16；prek **v0.5.11**，2026-11-16） | https://github.com/j178/prek-action |
| Agent skill | `gh skill install j178/prek prek`（gh ≥ 2.90.0） | https://github.com/j178/prek |

> plateau 现状：本机已通过 mise 装到 0.5.11（PRD 已确认），与最新 release 一致。

### 5.3 GitHub Action 用法（`j178/prek-action`）

```yaml
- uses: actions/checkout@<sha>          # 建议 pin SHA
- uses: j178/prek-action@44924fe9...    # v3.1.0；v3 起不再发布浮动 major/minor tag
```

- 固定调用形态：`prek run --show-diff-on-failure --color=always <extra-args>`，`extra-args` 默认 `--all-files`。
- Inputs：`extra-args`、`prek-version`（`0.3.x`/`<=1.0.0`/`latest`/`latest-known`，默认 `latest`）、`sha256`（覆盖内置校验表）、`install-only`、`working-directory`、`show-verbose-logs`（默认 true）、`cache`（默认 true）。
- Outputs：`prek-version`、`cache-hit`。
- 要求仓库有 `prek.toml` 或 `.pre-commit-config.yaml|yml`。
- `install-only: true` 后可自行 `prek run --all-files`，便于与 `just`/`pnpm install`/`buf` 组合；对 fork PR 建议 `pull_request` 而非 `pull_request_target`。
- 官方 CI 文档：https://prek.j178.dev/ci/ —— 默认 `prek run --all-files`；大仓可 `prek run --from-ref origin/main --to-ref HEAD`（需足够 fetch-depth）；`PREK_HOME` 缓存要慎（依赖 prek 版本/平台/配置/rev，官方建议先不开缓存）；`autofix.ci` 有现成配方（`continue-on-error` → 二次 `prek run` 验证 → `autofix-ci/action`）。

### 5.4 workspace / monorepo 支持

- **无需 manifest、无需 opt-in**：从 cwd 向上找第一个 config 作为 workspace root（不越过 Git root），再向下**递归发现**所有子项目 config（可继续嵌套）。参见 https://prek.j178.dev/monorepos/ 。
- 每个目录一份 config；**父项目过滤器不关闭子项目**，子项目也不覆盖父项目；目录内有多个 config 时按上面的优先级取一个。
- `orphan = true`（prek-only）：把该子树从所有祖先 hook 的文件范围里摘出去，即使该项目的 hook 被 skip 也不回流。
- 执行顺序：**由深到浅**（`src/backend/` → `src/` → root），**同深度可并发**（受 `PREK_CONCURRENT_HOOKS` 限制）。
- 选择器：`prek run frontend/`、`prek run frontend:check-json`、`prek run --skip frontend/`、`PREK_SKIP=frontend/,backend:format`；`--skip` > `PREK_SKIP` > `SKIP`。
- `-C frontend` 改 cwd 再发现；`--config` 退化为单 config 模式（**默认关闭 workspace 发现**，hook 从 Git root 跑、路径按仓库相对）。
- `.prekignore`（gitignore 语法）排除发现；`.gitignore`/submodule/隐藏目录默认排除。
- 安装：只在 root 跑一次 `prek install`，按 root config 的 `default_install_hook_types`（**不合并子项目配置**）。
- 参考：https://prek.j178.dev/reference/workspace/

### 5.5 内置 Rust hooks（`repo: builtin`，离线零依赖）

`trailing-whitespace`、`end-of-file-fixer`、`check-added-large-files`、`check-case-conflict`、`check-illegal-windows-names`、`file-contents-sorter`、`requirements-txt-fixer`、`fix-byte-order-marker`、`forbid-new-submodules`、`check-{json,json5,jsonc,toml,yaml,xml}`、`pretty-format-json`（仅 builtin）、`check-vcs-permalinks`、`deny-filename-pattern`/`deny-pattern`/`replace-pattern`/`require-filename-pattern`/`require-pattern`、`mixed-line-ending`、`normalize-indentation`、`check-symlinks`/`destroyed-symlinks`、`check-merge-conflict`、`detect-private-key`、`no-commit-to-branch`、`check-signed-commit`、`check-commit-trailers`、`check-shebang-scripts-are-executable`、`check-executables-have-shebangs`。

- **Automatic fast path**：配置仍写 `https://github.com/pre-commit/pre-commit-hooks`（`rev` 只用于 manifest/fallback），但实际执行走 Rust 实现，**配置对上游 pre-commit 依旧合法**；`PREK_NO_FAST_PATH=1` 或给 hook 指定 `language: python` 可退回原实现。
- `repo: builtin` 不允许 `entry`、`language` 必须为 `system`（或省略）。
- `prek util list-builtins` 枚举当前版本可用项。参考：https://prek.j178.dev/built-in-hooks/ 、https://prek.j178.dev/reference/built-in-hooks/

### 5.6 与 plateau 直接相关的 3 个能力

| 能力 | 用法 | 参考 |
| --- | --- | --- |
| Go 语言 hook 托管 | `language: golang` 会在隔离 `GOPATH` 里 `go install ./...`，`language_version` 可从 `go.mod` 的 `go`/`toolchain` 推断（`1.22`、`>=1.20, <1.23`） | https://prek.j178.dev/reference/language-support/#golang |
| 用 mise 装 golangci-lint（不用污染系统） | `language: mise` + `additional_dependencies: ["aqua:golangci/golangci-lint@2"]` + `entry: golangci-lint run --fast-only ./...`、`pass_filenames: false` | https://prek.j178.dev/reference/language-support/#mise |
| 本地 hook + shell | `repo: local` + `language: system`；**默认不走 shell**（`|`/`&&`/重定向无效），复杂逻辑写成脚本，或显式 `shell: bash` | https://prek.j178.dev/local-hooks/ |

其它实用项：`groups: [ci|lint|format]` 配 `--group/--require-group/--no-group` 做「本地只跑快档、CI 跑全档」（https://prek.j178.dev/reference/configuration/#groups）；`priority` 同值并发（https://prek.j178.dev/reference/configuration/#priority）；`PREK_HOME`（默认 `~/.cache/prek`）、`PREK_CONCURRENT_HOOKS/BATCHES`、`PREK_NO_CONCURRENCY`、`PREK_QUIET`、`PREK_DOWNLOAD_CHECKSUM_POLICY`（https://prek.j178.dev/reference/environment-variables/）；`prek update --check`（`--dry-run --exit-code`）可在 CI 里卡住过期 `rev`，并支持 `--cooldown-days` 与 impostor-commit 检测。

---

## 6. Files（证据锚点）

**本地**
- `/Users/horonlee/projects/go/tx7do/go-wind-admin/.github/workflows/sso-smoke.yml`（全链路 e2e 模板）
- `/Users/horonlee/projects/go/tx7do/go-wind-admin/.github/workflows/docs-parity.yml`、`update-mask-allowlist-parity.yml`
- `/Users/horonlee/projects/go/tx7do/go-wind-admin/backend/.golangci.yml`（生成代码排除写法）
- `/Users/horonlee/projects/go/tx7do/go-wind-admin/backend/Makefile:71-73`（`lint`/`vet` 仅本地）
- `/Users/horonlee/projects/go/tx7do/go-wind-admin/frontend/admin/react/.husky/{pre-commit,commit-msg}`、`vue-vben/.lintstagedrc.mjs`
- `/Users/horonlee/projects/go/servora-kit/servora/.github/workflows/{ci,buf-ci,release,publish-proto-utils}.yml`（同组织约定）
- `/Users/horonlee/projects/go/servora-kit/plateau/{justfile,just/services.just,just/service.just,just/webs.just,buf.yaml}`（待接入 CI 的入口；无 `.github/`、无 `.golangci.yml`、无 hook 配置）
- `/Users/horonlee/projects/go/servora-kit/plateau/.trellis/tasks/11-23-plateau-ci-prek/prd.md`（约束：`GOWORK=off`、生成物入库、`just` 为唯一入口、预置 `plugin` 多为 `@latest` → 漂移不可复现）

**远端（已读原文）**
- Ech0：`.../lin-snow/Ech0/main/.github/workflows/{test,cli-check,release,release-cli,release_helm,release_zigcc,docker-test-image,i18n-guardrails,deploy-*,hub-*}.yml`、`.github/actions/setup-musl-toolchain/action.yml`、`.github/dependabot.yml`、`.golangci.yaml`、`justfile`、`scripts/check.sh`、`docs/dev/release-process.md`
- Kratos：`.github/workflows/{go,lint,codeql-analysis,comment-check,gitee-sync,issue-translator}.yml`、`hack/{resolve-modules.sh,tools.sh}`、`Makefile`、`.golangci.yml`、`buf.yaml`、`codecov.yml`、`.github/dependabot.yml`
- go-zero：`.github/workflows/{go,reviewdog,codeql-analysis,release,version-check}.yml`、`.github/dependabot.yml`
- Kitex：`.github/workflows/{tests,pr-check,claude,claude-review-fork}.yml`、`.golangci.yaml`、`.codecov.yml`、`.typos.toml`、`.github/dependabot.yml`
- prek：`j178/prek` README、https://prek.j178.dev/{installation,quickstart,configuration,compatibility,running-hooks,ci,monorepos,built-in-hooks,local-hooks}/ 、`/reference/{configuration,cli,workspace,language-support,built-in-hooks,environment-variables}/`、`j178/prek-action` README 与 release v3.1.1

---

## 7. 对 plateau 的直接结论（供主智能体收敛方案）

1. **必学**：Ech0 的 `git diff --exit-code` 漂移门禁（plateau 有 `api/gen/{go,ts}`、5 个 `wire_gen.go`、Ent、OpenAPI，是同一类风险）；Servora 的「tag 必须落在 main 且有绿色 CI」；`GOWORK=off` 与 `just --fmt --check`；Ech0 的 `concurrency` + 最小 `permissions` 双写。
2. **必须改造后再用**：Ech0/本地 go-wind-admin 的 action 版本（`@v7/@v8`）不可照抄，plateau 应 pin 到实际存在且 `-action` README 支持的具体版本；`just plugin` 里的 `@latest` 会让漂移检查不可复现 —— 需先把 buf/golangci-lint/protoc 插件与 `SERVORA_VERSION` 一样固定，或在 CI 里用 env 覆盖（`BUF_VERSION`、`GOLANGCI_LINT_VERSION`、`PROTOC_GEN_GO_VERSION`… 已在 justfile 中预留 `env()` 钩子）。
3. **prek 落地形态**：根 `prek.toml`（YAML 兼容无收益，因为无既有配置）+ `repo: local` 包住 `just` 子集（如 `just lint-go`/`just lint-proto`/`just api-ts-check`/`just --fmt --check`），用 `groups = ["fast"]`/`["ci"]` 区分 pre-commit 与 `prek run --all-files --group ci`（CI 用 `j178/prek-action` `install-only: true` 后显式跑，便于先 `pnpm install`/装 buf）；`shell` 显式开启或全部落到 `just` 单条命令避免 shell 语法；是否启用 workspace 模式：admin web 是独立 pnpm workspace，可只给它一份 config 并在根 config 里 `exclude` 掉（或 `orphan`），避免根 hook 重复扫描 Vben 产物。