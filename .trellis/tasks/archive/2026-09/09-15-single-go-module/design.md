# Servora 与 Plateau 单 Go module 技术设计

## 总体顺序

```text
Servora 合并 module
  -> 修复并通过 main CI
  -> 发布统一根版本 v0.9.7
  -> Plateau 消费 v0.9.7
  -> Plateau 合并 module
  -> 更新本机父 workspace
  -> 两仓分别执行 GOWORK=off 验收
```

该顺序避免 Plateau 先进入“自身单 module、仍依赖两个 Servora modules”的过渡状态。一个 Trellis 任务承载全部步骤，但 Servora 发布成功是 Plateau 依赖切换的显式前置条件。

## Servora 目标结构

### 当前

```text
servora/go.mod
└── require github.com/Servora-Kit/servora/api/gen v0.9.5

servora/api/gen/go.mod

CI: go work init . ./api/gen -> build/test/lint
```

### 目标

```text
servora/go.mod                         # 唯一 module
├── api/gen/go/servora/**              # 普通生成 package
├── cmd/**
├── core/**
├── contrib/**
└── ...
```

删除嵌套 `api/gen/go.mod` 后，生成 package 的 import path 仍是根 module path 加相对目录，因此调用方代码不改。`go mod tidy` 自动删除 Servora 对自身 `api/gen` 的 require，并吸收生成 package 所需 runtime 依赖。

## Servora CI 与命令

- `.github/workflows/ci.yml` 移除 lint/test job 中的 `go work init . ./api/gen`。
- workflow 级或 job 级设置 `GOWORK: off`，确保 CI 不可能依赖外部 workspace。
- lint job 继续运行 Just 格式检查与 golangci-lint action；单次根 lint 会包含 `api/gen/go/**`。
- test job 从根执行 `go build ./...` 与 `go test -short -race -coverprofile=coverage.out ./...`，自然覆盖生成 package。
- Justfile 与 Makefile 的 `dep`、`tidy`、`fmt`、`vet`、`test`、`test-all`、`cover`、`lint` 都改为根目录单次操作。
- `ci-lint` 仍是本地 CI parity 入口，但直接设置标准 `GOWORK=off`，不再接收 workspace 参数。
- 删除 `tag-api`/`tag.api`，根发布是唯一 Go module 发布入口。

## Servora 版本与 BSR

- 既有 `api/gen/v*` tags 继续代表历史子 module 版本，不删除、不移动。
- `v0.9.7` 是统一 module 的首个版本；其 module zip 将包含 `api/gen/go/**`，Plateau 只需 require 根 module。
- `buf.yaml` 中的 `buf.build/servora/servora`、Buf CI 和 `buf push` 与 Go module 边界独立。
- 本迁移不改 `.proto` 或生成输出，所以不创建 API 子模块 tag，也不手动执行 BSR push；根 tag 仍会触发既有 Buf CI，自动推送未变的 schema 并维护对应 labels。
- 发布严格遵循“提交进入 main -> 该 SHA 的 main CI 成功 -> 创建根 tag -> 等待 Release workflow”顺序。

## Plateau 目标结构

### 当前

```text
plateau/go.mod
├── require plateau/api/gen@pseudo
├── require servora v0.9.6
├── require servora/api/gen v0.9.5
├── api/gen/go.mod
├── app/audit/service/go.mod
├── app/example/service/go.mod
└── app/iam/service/go.mod

plateau/go.work 覆盖以上 module 与相邻 Servora module
```

### 目标

```text
plateau/go.mod                         # 唯一 module
├── require servora v0.9.7
├── api/gen/go/**
├── app/audit/service/**
├── app/example/service/**
└── app/iam/service/**
```

所有 Plateau import path 也满足根 module path 加相对目录，删除嵌套 `go.mod` 无需修改 Go import。各 `cmd/server` 仍分别编译，单 module 不合并运行进程或部署单元。

## Plateau 依赖与 Just

1. 删除四个嵌套 module 的 `go.mod/go.sum` 与根 `go.work/go.work.sum`。
2. 根 `.gitignore` 添加 `/go.work`、`/go.work.sum`。
3. 根 `go.mod` 切换到 Servora `v0.9.7`，删除 `servora/api/gen` 与 Plateau 自依赖。
4. 在 `GOWORK=off` 下运行根 `go mod tidy`，审查依赖分类和版本；第二次 tidy 验证稳定。
5. 根 `just lint` 组合 API TS、Buf 与一次 `golangci-lint run ./...`。
6. 服务 leaf 保留定向 lint/build，但移除多 module 专用 `gowork` 参数。
7. 根 `go.mod` 用 `tool` 声明固定 Wire 与 Ent，leaf 生成入口通过 `go tool` 执行；测试夹具 replace Plateau 根 module，不再 replace 已删除的 `api/gen` module。

## 本地父 workspace

最终 `/Users/horonlee/projects/go/servora-kit/go.work` 中与两仓相关的条目为：

```go
use (
    ./servora
    ./plateau
    // servora-example 等其他现有 module 保持不变
)
```

删除的条目：

- `./servora/api/gen`
- `./plateau/api/gen`
- `./plateau/app/audit/service`
- `./plateau/app/example/service`
- `./plateau/app/iam/service`

Go 从 Servora 或 Plateau cwd 都会向上发现父 workspace。CI 与可移植性检查显式使用 `GOWORK=off`，因此父文件始终只是本地联调覆盖层。

## 文档同步

Servora 修改：

- `AGENTS.md`
- `api/AGENTS.md`
- 与独立 API module/tag、workspace 或 CI parity 相关的其他命中文档

Plateau 修改：

- `AGENTS.md`
- `api/AGENTS.md`
- `app/AGENTS.md`
- `.trellis/spec/plateau/project/structure.md`
- `.trellis/spec/plateau/project/development.md`
- `.trellis/spec/api/proto/generation.md`
- `.trellis/spec/service/backend/layout.md`

文档必须区分 Go module、Buf workspace、pnpm workspace 和本地跨仓 Go workspace，不能把其中一个的合并描述为另一个也被取消。

## 风险与防护

- Servora module 边界变化必须通过真实 `v0.9.7` 消费验证；仅本地 replace/workspace 通过不足以证明 module zip 包含生成 package。
- 不得在 main CI 成功前创建 tag，也不得因为 Go module 变化而误触发手动 BSR push。
- Servora Justfile 与 Makefile 是两套现存入口，必须保持行为同步。
- 父 workspace 若保留已删除 module 路径，会导致两个仓库的自动 workspace 命令失败，必须在第二仓迁移完成后更新。
- Plateau 根级 lint 覆盖面会扩大，发现的既有问题要与 module 回归分开处理。
- 删除服务级 `go.sum` 会暴露只由 `go run` CLI 使用的传递依赖；用根 module 的 Go tool 声明维护，而不是保留不可由 `tidy` 稳定复现的多份 checksum。
- Go 1.27.0/1.27.1 混合缓存会产生编译器版本不匹配；两仓验证固定同一 `GOROOT`、`PATH` 与 `GOTOOLCHAIN=local`，必要时使用独立 `GOCACHE`。
- Plateau 工作区已有并行修改，所有 patch 和提交选择必须逐文件审查，禁止回退或覆盖他人改动。

## 回滚

- Servora 在 tag 前可恢复 `api/gen/go.mod/go.sum`、根依赖、命令与 CI；tag 后不得移动已发布 tag，必须以后续版本修正。
- Plateau 在未提交状态可恢复嵌套 module/workspace 文件与本任务 patch；恢复时不得覆盖任务开始前已有的 Servora 版本升级和 Admin/Trellis 修改。
- 父 `go.work` 保存任务前内容，仅回滚本任务删除的五个条目。
