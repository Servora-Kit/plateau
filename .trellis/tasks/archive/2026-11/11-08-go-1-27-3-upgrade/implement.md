# 实施计划

## 启动条件

- [x] 用户在最终规划摘要之后明确批准本方案。
- [x] 读取当前 Phase 1.4 指引，确认 PRD 收敛与两个 JSONL 清单均含真实规范/研究项，再启动本任务。
- [x] 按修改 package 读取 `trellis-before-dev` 及相关规范正文；保留已有用户变更，不提交 Git。

## 1. 更新版本元数据

- [x] 按 [升级清单](research/upgrade-inventory.md) 更新 Plateau、Servora、servora-transport/server/tcp 三个 `go.mod` 为 `go 1.27.3`。
- [x] 更新父级 `../go.work` 的版本并移除 servora-example 四个成员，仅保留上述三个模块；不修改旧示例仓库中的任何文件。
- [x] 使用目标工具链读取模块/工作区元数据，确认父级版本冲突消失；不运行 `go work sync` 或依赖全量升级。

## 2. 完成代码影响分析

- [x] 在 Plateau、Servora 各执行 `gitnexus analyze --index-only`，不注入上下文文件、不自动提交。
- [x] 对 `hashTemporaryPassword`、`translateRepoError`、`serviceFromSource`、四个待删除私有测试 helper 和 Conf fixture 所属测试函数运行 upstream impact，记录调用方/执行流程/风险。
- [x] HIGH/CRITICAL 先报告，UNKNOWN 继续核实。未完成影响分析不修改函数。
- [x] 使用 gopls/LSP references 查全私有 helper 调用，包含同包其他测试文件；确认 `int32Pointer` 调用的精确类型。
- [x] 对选定包运行 `GOWORK=off GOFLAGS=-mod=readonly go fix -diff -newexpr -errorsastype -stringscutprefix <packages>` 只读预览；diff 的非零退出码与真正加载失败分开处理，未全仓套用建议。

## 3. 精确应用采纳项

- [x] Example 密码哈希成功分支采用 `new(string(hash))`，Kratos 错误匹配采用单个 `errors.AsType` 判断且继续返回原始 `err`。
- [x] 迁移并删除 IAM `stringPtr`、Example integration `stringPointer`、Servora filter 的 `stringPointer`/`int32Pointer`，显式保留 int32 类型。
- [x] Audit `serviceFromSource` 的双斜线分支采用 `strings.CutPrefix`，不改其他分支。
- [x] 同步 Servora Conf 测试两份临时 `go.mod` 版本，不改行为断言或生成代码。
- [x] 仅格式化被修改的手写 Go 文件，不执行全仓格式化或生成。

## 4. 一次性完成检查与实际 smoke

版本元数据与全部语法修改完成后集中运行，不让多个 worker 在编辑过程中重复构建/测试。

### 独立模块检查

统一设置 `GOTOOLCHAIN=local GOWORK=off`；必要时加 `GOFLAGS=-mod=readonly` 防止检查过程中静默调整依赖。

| cwd | 构建/测试 | 其他检查 |
| --- | --- | --- |
| Plateau 根目录 | `go build ./...`；`go test -short -race ./...` | `golangci-lint run ./...` |
| `../servora` | `go build ./...`；`go test -short -race ./...` | `golangci-lint run ./...` |
| `../servora-transport/server/tcp` | `go build ./...`；`go test -short ./...` | 使用原锁定依赖 |

全仓测试必须覆盖以下现有行为，不为语法文本单独增加永久测试：

- Example 用户创建/更新的临时密码处理与服务层 integration 行为。
- IAM 账户测试中所有改写的指针配置。
- Servora CRUD filter 动态描述符的 string/int32 值与过滤行为。
- Audit `TestServiceFromSource` 的新格式、legacy 格式、plain 与空串。
- Servora `cmd/protoc-gen-servora-conf` 的配置执行与跨包生成编译行为。

### 工作区联调

- [x] 在共同父目录显式检查 `./plateau/...`、`./servora/...`、`./servora-transport/server/tcp/...`，运行包解析和构建。不能假定一个根 `./...` 会跨越所有 module。
- [x] 确认父级 `go list -m` 仅返回三个保留模块；servora-example 的本地模块不再参与该工作区解析，旧示例的自有工作区和文件保持不变。
- [x] 对保留模块的失败区分原有问题与升级回归并做隔离比较，未升级业务依赖；既有 Servora lint 发现最初单列，后按用户明确追加要求修复并复验通过。

### 实际命令与语义 smoke

- [x] 使用 `GOWORK=off GOTOOLCHAIN=local go run ./cmd/svr --help` 验证 Servora 实际 CLI 可运行。
- [x] 通过仓库外一次性 CodeGeneratorRequest/Go 驱动程序调用当前 Plateau 插件，并编译执行其生成代码，验证 REQUIRED 默认、PUBLIC 覆盖和返回值独立性；未写入现有生成目录。
- [x] 一次性语言探针验证指针类型/独立性、包装与自定义 As、前缀极端输入；实际项目测试验证 bcrypt 哈希、错误原因保留、Audit 投影以及 SQLite CRUD 行为。
- [x] 核对工具链、三个模块、父级工作区成员/版本、CI 版本来源、fixture 与平台/框架 README；旧示例 Dockerfile、镜像与自有工作区均未修改。

## 5. 交付要求

记录所有实际运行命令、模块结果、已存在失败与环境限制；更新任务验收项和已有 README，删除仓库外临时探针/生成产物。全部完成后按 Trellis 检查/结束流程处理，不自动提交、推送、发布或归档未经确认完成的任务。

## 风险与回退点

- 版本声明、工作区成员精简、文档与语法修改可分别回退；不回退无关用户修改。
- 私有 helper 删除依赖完整 references，尤其注意跨测试文件使用与 int32 常量推导。
- 保留现有 map key 预分配和空切片形态；不接受分配更多的短代码。
- 用户已启动 OrbStack；当前范围没有旧示例镜像构建。两个 GitNexus 索引已建立，OpenSSL 临时绕过及永久修复待办已按用户要求写入 `/Users/horonlee/projects/nix-config/TODO.md`。
- 新模块声明的最低 Go 版本提升是用户已要求的兼容变化；其余公开 API、数据与错误行为不变。

## 执行记录（2026-11-09）

### 验证结果

独立模块检查均设置 `GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly`。最终涉及 CGO 的完整检查还以单次命令环境指定下面的 Xcode 编译器和 SDK，未修改全局配置，未关闭 CGO 或竞争检查：

```text
CC=/Applications/Xcode.app/Contents/Developer/Toolchains/XcodeDefault.xctoolchain/usr/bin/clang
CXX=/Applications/Xcode.app/Contents/Developer/Toolchains/XcodeDefault.xctoolchain/usr/bin/clang++
SDKROOT=/Applications/Xcode.app/Contents/Developer/Platforms/MacOSX.platform/Developer/SDKs/MacOSX.sdk
```

| 对象 | 实际执行 | 结果 |
| --- | --- | --- |
| Plateau | `go build ./...` | 通过 |
| Plateau | `go test -short -race -timeout 5m ./...` | 27 个含测试包通过，68 个包无测试 |
| Plateau | `golangci-lint run --timeout 10m ./...` | 0 issues |
| Servora | `go build ./...` | 通过 |
| Servora | `go test -short -race -timeout 5m ./...` | 45 个含测试包通过，28 个包无测试 |
| Servora | `golangci-lint run --timeout 10m ./...` | 0 issues；用户追加修复后全仓重跑 |
| TCP 独立模块 | `go build ./...`、`go test -short ./...` | 通过 |
| 父级工作区 | 显式 `go build ./plateau/... ./servora/... ./servora-transport/server/tcp/...` | 通过；`go list -m` 仅列出三个保留模块 |
| Example SQLite integration | 设置临时文件 DSN 后，`go test -tags=integration ./app/example/service/internal/service -run '^TestUserReferenceIntegration$' -count=1 -timeout 5m` | 通过，使用真实 SQLite 与当前 CRUD 实现 |
| Servora CLI | `go run ./cmd/svr --help` | 正常显示真实命令帮助 |
| Plateau AuthN 插件 | 子进程运行真实插件，输入含服务默认和方法覆盖的 protobuf request，再编译运行输出代码 | REQUIRED 默认、PUBLIC 覆盖、独立 map/message 均通过 |
| Example 服务二进制 | `go build -o <临时目录>/example-server ./app/example/service/cmd/server`，再运行 `example-server -h` | CGO 链接成功，实际进程正常输出配置参数帮助 |
| etcd 心跳失败恢复 | 临时 Go overlay 编译真实 `Registry.heartBeat`，以受控 Lease/KV 注入首轮续租失败、首次注册失败，再恢复；`go run -race` 运行 20 轮 | 验证每轮两次 Grant、两次 KeepAlive、一次 Put、正确租约和取消退出；不依赖外部 etcd 实例 |

### 既有问题与最小修复

1. **已修复的受影响测试 fixture**：`app/example/service/internal/service/user_integration_test.go:342` 仍以单个 Ent client 调用已要求 Dialer 和 client 的 `data.NewData`。先用 Git HEAD 的原始测试文件和 Go overlay 复现同一编译错误，再补为 `data.NewData(nil, transaction.Client())`；该场景仅执行 CRUD，不使用远程调用。修复后真实 SQLite integration 通过，未修改产品构造函数或行为断言。
2. **用户追加修复的既有 lint 发现**：`../servora/core/registry/etcd.go:211` 原 `case <-errChan:` 被 errcheck 报告错误返回值未检查。曾复制 Git HEAD 的原始 `go.mod`（Go 1.27.1）、`go.sum` 和 `etcd.go` 到临时目录，以 `GOFLAGS='-mod=readonly -modfile=<原始模块> -overlay=<原始源码映射>' golangci-lint run --enable-only errcheck ./core/registry` 复现同一错误。用户随后明确要求修复：用 LSP 将局部通道改名为 `retryChan`，改为容量仍为 1 的 `chan struct{}`；发送端继续判断 `registerErr != nil` 后发送失败通知，接收端仍直接进入下一次重试。未更改成功租约通道、超时、重试次数、取消逻辑，也未添加忽略规则。追加修改后 `go test -short -race ./core/registry`、Servora 全仓 build/竞争测试与全仓 lint 均通过。

### 本机环境处理

- **GitNexus/OpenSSL**：LadybugDB 预编译原生库指向 Homebrew 的 `libssl.3.dylib`/`libcrypto.3.dylib`，本机可用库位于 Nix store。单进程设置 `DYLD_LIBRARY_PATH` 并直接运行实际 JS 入口后，两个 `--index-only --workers 4` 索引成功；未注入上下文文件。详细错误、有效命令、未确认的迁移关联和永久修复验收项已写入用户指定的 nix-config TODO。
- **损坏的 Nacos 模块缓存**：原 `github.com/nacos-group/nacos-sdk-go/v2@v2.3.5` 解压目录缺少 clients、vo 等内容，`go mod verify` 报 `dir has been modified`。原目录完整移动到 `/tmp/nacos-sdk-v2.3.5-cache-backup-83ddd6f6-aa70-4ffe-9b02-1cda2f63b6f7` 保留，再以 `go mod download github.com/nacos-group/nacos-sdk-go/v2@v2.3.5` 从已有校验包恢复同一版本；随后 `go mod verify` 返回 `all modules verified`。没有修改依赖版本或清空模块下载缓存。
- **旧 Go 构建索引**：恢复目录后默认缓存仍产生缺包错误，隔离 `GOCACHE` 时三个目标包均能解析。执行 `go clean -cache` 只清理可重建的构建缓存，之后原缺包错误消失。
- **Nix clang/SDK**：默认编译器 `/etc/profiles/per-user/horonlee/bin/clang` 在服务链接时报告 `ld: library not found for -lresolv`。通过 `xcrun` 找到现有 Xcode clang/SDK，使用上述临时环境后服务实际运行及全仓构建/测试通过。此环境选择仅用于验证，没有对 nix-config 做软件声明或 switch。

### 影响分析与范围核对

- Plateau 索引绑定提交 `ff6d8ed`：27,635 nodes、63,568 edges；Servora 绑定 `9144069`：8,125 nodes、20,806 edges。索引器报告过调用候选和流程枚举裁剪，未声称图覆盖完整。
- Plateau 的密码哈希和 source 投影为 LOW；IAM stringPtr 为 MEDIUM；integration stringPointer 为 LOW。translateRepoError 的 UNKNOWN 通过 LSP/精确搜索确认 13 处调用。其他被修改的测试入口按测试框架入口处理，未因空调用图删除测试。
- Servora 目标的 UNKNOWN 以 LSP 和精确搜索补充核实：stringPointer 有 19 处调用、int32Pointer 有 3 处调用；同包 lifecycle/mask 测试调用全部迁移。没有 HIGH/CRITICAL 返回。
- integration 标签下的引用通过 `GOFLAGS=-tags=integration gopls references -d` 补齐，未遗漏测试文件。
- 四个 helper 已删除，无兼容 wrapper；语法优化限于批准的三类改写，另按用户追加要求修复 etcd 失败通知通道；未批量修改生成物、排序/预分配结构或外部指针 helper。
- 沿用现有项目规范，无新的公共约定需要写入 `.trellis/spec`。README 的平台/框架最低 Go 要求已同步；Servora 对旧示例的介绍没有被改写成旧仓库已升级。
- etcd 追加修改前的 upstream impact 为 LOW，仅解析到 `Register` 直接调用和一条 Register → RegisterWithKV 流程；LSP 确认局部通道三个引用点，改名由语言服务器完成。

### 交付状态

用户已明确授权提交、归档和 push。交付包括 Plateau、Servora、servora-transport 与 nix-config 的本任务改动；nix-config 的 `home/common/editor/zed/settings.json` 为非本任务改动，不纳入提交。父级本机 `go.work` 不提交。该任务没有关联 PR，使用 Trellis 的 `--skip-branch-validation` 处理直接 main 归档；不打标签或发布版本。临时驱动、overlay、SQLite 数据库和测试构建产物已移除；为保护原有缓存内容，Nacos 缓存备份保留。

提交前 `detect-changes --scope all`：Plateau 为 LOW（7 文件、13 符号），Servora 为 MEDIUM（7 文件、8 符号），后者涉及 Register → RegisterWithKV 流程，已由心跳故障注入、竞争检查及全仓 lint 验证。TCP 仅修改 `go.mod` 的 Go 声明，nix-config 仅提交 TODO 记录，不包含源代码符号修改。

### 业务提交

| 仓库 | 提交 | 内容 |
| --- | --- | --- |
| Plateau | `3f87fe124579306dab20a7514f5977e38317948e` | Go 1.27.3、语法精简、集成 fixture 参数及 README |
| Servora | `599252632613439266bf08b6c7524d23a1a838d8` | Go 1.27.3、测试 helper、Conf fixture 及 README |
| Servora | `265559baa8eb3534629a861e5f21f29847093d1b` | etcd 心跳失败重试信号 |
| servora-transport | `cb9d326a477dc56384cdff185a87a6e68e44f630` | TCP 模块 Go 1.27.3 |
| nix-config | `945e282a5ff1ea9cfb5b9a24cf26e1f41b305951` | GitNexus/OpenSSL 问题与临时绕过记录 |
