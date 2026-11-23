# Implement：Plateau CI 与 prek

## 前置条件

- [x] test/example 移出工作已提交（`9e4b4774`）；`just/services.just`、`buf.yaml` 只剩 admin/audit/iam
- [ ] 用户已安装 Mend Renovate Community App（Renovate Only，Silent 模式）——不影响实施，影响 R5 验收

## 步骤

1. 工具版本
   - [x] 新增 `mise.toml`：buf、golangci-lint、just、fga、prek、node、pnpm、air 固定具体版本（以本机现有版本为起点）
   - [x] `docker-compose.yaml`：把 `openfga/openfga:latest`（migrate 与 run 两处）改为固定 tag（v1.23.0）
2. 生成器固定（D2）
   - [x] `go get -tool` 加入所有 protoc 插件和本仓库的 authn/authz 插件；执行 `go mod tidy` 并确认结果稳定（没有抬高任何运行时依赖）
   - [x] 所有 buf 模板改为 `local: ["go", "tool", ...]`
   - [x] 删除 `just plugin`、`just cli`、`SERVORA_PKG`、`SERVORA_VERSION`；改写 `just init`
   - [x] `GOWORK=off just gen`：迁移后生成目录 0 变化，没有版本头需要提交
3. just recipe
   - [x] `test`、`fmt-proto-check`、`breaking`、`gen-check`（unix/windows 双实现）
4. prek（D3）
   - [x] 根 `prek.toml`（改用 `repo = "builtin"`；排除 `.trellis/`、`.agent(s)/`、`.omp/` 等工具生成的目录）
   - [x] `app/admin/web/prek.toml`；删除 `lefthook.yml`、`.commitlintrc.js`、commitlint-config 包和相关依赖（同步 lockfile）
   - [x] 确认 pygrep 方式的 commit-msg 校验在 prek 下可用（`--negate --multiline`，7 条合法、5 条非法用例符合预期）
   - [x] 对 `--all-files` 跑出的格式问题做一次性修复，作为独立提交（`62e7cea1`）
5. CI
   - [x] `.github/workflows/ci.yml`（changes、prek、go、proto、gen、integration、web、admin-web、ci-ok）
   - [x] `.github/workflows/security.yml`（govulncheck）
   - [x] `renovate.json`
6. 文档与规范
   - [x] `AGENTS.md` 命令段（init/mise/prek/新 recipe）
   - [x] `.trellis/spec/plateau/project/development.md`：工具版本来源、钩子分层、CI 门禁与 `ci-ok`
   - [x] `.trellis/spec/api/proto/generation.md`：插件由 go.mod tool 固定、gen-check

## 本地验证结果（2026-11-23）

- [x] `mise install` 与 `mise.toml` 一致；`GOWORK=off go mod tidy` 稳定
- [x] 生成代码已提交后连续两次 `GOWORK=off just gen-check` 通过
- [x] `prek run --all-files --stage pre-commit` 全部通过；commit-msg 拦截行为符合预期
- [x] `just lint-go`（0 issues）、`just lint-proto`、`just fmt-proto-check`、`just breaking`（对比 HEAD）通过
- [x] `actionlint`（含 shellcheck）、`renovate-config-validator --strict` 通过
- [x] 集成测试本地实跑：PASS 80 / FAIL 0 / SKIP 1（`TestBrowserHarness` 需手动开启），IAM 与 Admin 集成测试实际执行
- [x] 根 Web lint/typecheck/build、admin web lint/check/test:unit/build 本地通过
- [x] 远端 CI 实跑：push main 的 run `45127566826` 全部 job 通过（含 ci-ok）；Security run `45126297753` 通过。前两轮修复了 just 导出空 `FGA_API_URL`、admin prek.toml 格式、3 个 govulncheck 漏洞、Vben 时区用例对 UTC 的假设
- [ ] PR 事件下的路径分流（等第一个 PR 时观察）
- [ ] Renovate Dependency Dashboard：App 已安装（Renovate Only + Silent），仓库 Issues 已于 2026-11-23 开启，等 Renovate 下一次扫描生成面板
- [x] main 分支保护：必过检查 `ci-ok`（GitHub Actions app_id 15368），`strict=false`，管理员可绕过，禁止 force push 与删除

## 验证

| 检查 | 命令 / 方式 |
| --- | --- |
| 工具安装 | `mise install` 后 `mise ls --current` 与 `mise.toml` 一致 |
| 生成稳定 | 干净工作区上连续两次 `GOWORK=off just gen-check` 都通过 |
| 钩子 | `prek install`；`prek run --all-files`；故意制造格式错误、提交不规范的 message，确认被拦截 |
| pre-push | `prek run --hook-stage pre-push --all-files` 通过 |
| workflow 语法 | `actionlint`（用 mise 临时运行）通过 |
| CI 实跑 | 推送到分支开 PR，所有 job 变绿；再改一个 proto 和一个 Web 文件，确认路径分流符合预期；手动制造生成漂移，确认 gen job 失败 |
| 集成测试 | 本地用 compose 按 CI 同样步骤运行一次，确认 IAM/Admin 集成测试确实执行（`-v` 中没有 SKIP） |

## 风险文件 / 回滚点

- `go.mod`/`go.sum`（tool 依赖）、buf 模板、`justfile`：插件迁移和重新生成单独提交，便于单独回滚
- `app/admin/web/package.json`、lockfile：删除 commitlint 和 lefthook 时单独提交

## 后续用户操作（实施完成后）

- 在 GitHub 分支保护规则中把 `ci-ok` 设为必过检查（涉及账号设置，由用户操作或经确认后用 `gh` 执行）
