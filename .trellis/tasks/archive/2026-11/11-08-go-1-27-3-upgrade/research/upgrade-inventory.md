# Go 1.27.3 升级清单

## 工作区与模块

父级 `../go.work:1-11` 当前声明 `go 1.27.0`。2026-11-09 用户明确排除不再维护的 servora-example，计划删除其四个 `use` 条目，仅升级下表三个保留模块和父级工作区至 `go 1.27.3`。

| 仓库 | 模块目录 | 当前版本 | 证据 |
| --- | --- | --- | --- |
| Plateau | `.` | 1.27.1 | `go.mod:1-3` |
| Servora | `../servora` | 1.27.1 | `../servora/go.mod:1-3` |
| servora-transport | `../servora-transport/server/tcp` | 1.27.0 | `../servora-transport/server/tcp/go.mod:1-3` |

从父级移除 `./servora-example`、`./servora-example/api/gen`、`./servora-example/app/master/service`、`./servora-example/app/worker/service`。不删除目录、不修改该仓库的任何文件；其自有 `../servora-example/go.work:1-8` 保持 Go 1.26.1 及原四个成员。Plateau 内 `app/example` 不受此范围调整影响。父级工作区不提交到任何仓库，项目约定见 `.trellis/spec/plateau/project/development.md:3,11-15,34-40`。

## 其他有效版本配置

- `../servora/README.md:23-27`：明确框架本身要求 Go 1.27.3+；现有旧示例 quickstart 不改写为已升级的示例要求。
- `../servora-example/Dockerfile:1` 的 `golang:1.26.1-alpine`、`../servora-example/README.md:13` 的 Go 1.26+ 及四模块原版本均属于已排除仓库，保持原样。
- `README.md:15-20`：目前没有 Go 前置要求，可在已有项目介绍后补充 Go 1.27.3+，不新增独立说明文档。
- `../servora/cmd/protoc-gen-servora-conf/main_test.go:153,336`：两个行为回归测试临时创建的 `go.mod` 固定 Go 1.27.1，需要同步到 1.27.3，避免替换根模块后 fixture 声明低于依赖最低要求。
- `../servora/.github/workflows/ci.yml:17-18,26-28,43-49`：已采用 `GOWORK=off` 和 `go-version-file: go.mod`，随模块声明自动选择目标 Go，无需新增硬编码。
- Plateau 没有 `.github` 工作流目录；本次已扫描的 Just、Compose、Air 及 Servora CLI 模板未发现额外 Go 版本固定值。不为这次升级新增 mise 配置或重写现有安装工具入口。

## 已执行的只读核实

- `GOTOOLCHAIN=local go version`：`go version go1.27.3 darwin/arm64`，本机目标工具链已存在。
- `go env GOWORK GOTOOLCHAIN GOPROXY`：工作区为父级 `go.work`；`GOTOOLCHAIN=auto`；代理为 `https://goproxy.cn,direct`。
- `mise current go`：提示未设置 Go 版本；未修改用户全局工具配置。
- `GOTOOLCHAIN=local go list -m`：失败，Plateau、Servora 要求 Go >=1.27.1，但父级 `go.work` 仅声明 1.27.0。该冲突先于包编译发生；升级工作区声明即可解决这一版本冲突，后续还需真实构建验证。
- `golangci-lint version`：2.15.0，使用 go1.27.3 构建。
- 历史检查中 Docker 客户端为 29.8.2，daemon 当时未运行。用户于 2026-11-09 明确说明 OrbStack 已启动，旧运行状态不再作为阻碍；缩小范围后不构建旧示例 master/worker 镜像。
- `gitnexus list`：GitNexus 1.6.12，未找到已索引仓库；两仓也没有本地 `.gitnexus/run.cjs`。代码影响分析必须在实施前以 `gitnexus analyze --index-only` 建立索引后完成，不能将空索引视为无调用影响。

## 移除旧示例的可行性

- 定向搜索保留三个模块的 Go 源码、go.mod 和已知 Just/Compose/Make 入口，未发现 `servora-example` 引用；排除文件中的历史文档介绍不代表代码依赖。
- 使用仓库外临时 Go 1.27.3 工作区，仅保留 Plateau、Servora、servora-transport/server/tcp，执行 `GOWORK=<临时文件> GOTOOLCHAIN=local go list -m` 成功，输出恰好为三个保留模块。临时文件及可能的 sum 文件已删除，真实父级 `go.work` 尚未修改。
- 此检查证明精简后的成员声明可解析，未声称业务构建已通过。移除主模块可能改变共享依赖选版，实施后仍执行独立模块与工作区构建。
- 官方工作区 use 指令及解析测试：[Go workspace example](https://github.com/golang/go/blob/go1.27.3/src/cmd/go/testdata/script/work_sync.txt)、[workspace replacement tests](https://github.com/golang/go/blob/go1.27.3/src/cmd/go/testdata/script/work_replace.txt)，已通过 Context7 核实。

## 验证注意事项

- 三个保留模块分别执行 `GOWORK=off` 独立检查和父级工作区联调，Plateau、Servora 增加竞争检查和 Go lint。
- servora-example 原有 Servora v0.9.0 与旧 `servora/api/gen` 子模块依赖不再参与此次维护，不升级、不迁移、不构建、不测试。
- 仅修改 Go 最低版本不需要全量 `go get -u` 或 `go work sync`。若现有工具要求必要的 module 元数据整理，先检查其实际差异，避免无关依赖版本变更。
- Go 升级不涉及 Proto 契约变化，不运行全量生成来扩大改动。
