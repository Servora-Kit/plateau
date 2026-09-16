# 本地开发与检查入口

本规范记录现有开发入口。Plateau 仓库只使用根 Go module；本机父级 `/servora-kit/go.work` 可用于跨仓源码联调，但所有仓库级门禁必须能在 `GOWORK=off` 下独立运行。命令均从 Plateau 根目录运行；在采用 RTK 的环境中加 `rtk proxy` 前缀。

## 固定端口

从 10000 起，每个应用保留 10 个端口：+0 HTTP、+1 gRPC、+2 Web，+3～+9 预留。新增应用分配下一个空闲段，不重排旧编号。

| 应用 | HTTP | gRPC | Web |
| --- | --- | --- | --- |
| IAM | 10000 | 10001 | 10002 |
| Audit | 10010 | 10011 | 10012（预留） |
| CMS（占位） | 10020（预留） | 10021（预留） | 10022（预留） |
| Example | 10030 | 10031 | 10032 |
| Test | 10040（预留） | 10041（预留） | 10042 |
| Admin（占位） | 10050（预留） | 10051（预留） | 10052（预留） |

约定作用于本地监听和 Docker 宿主映射；容器内部、共享中间件和生产公开端口不受约束。相同应用的原生运行与容器映射不可同时占用同一端口，Web dev 与 preview/start 共用端口也不能同时启动。修改 IAM 公开入口须同步 `IAM_PUBLIC_ORIGIN`、Web 端口和后端代理地址。

## 命令及副作用

| 入口 | 用途 |
| --- | --- |
| `just init` | 安装 CLI／插件及冻结锁文件的 pnpm 依赖，会产生本地安装副作用 |
| `just gen` | 根 API 生成后执行各服务 OpenAPI、Wire、Ent；修改 Proto 后使用 |
| `just wire` | 各服务 Wire 装配刷新 |
| `just lint` | API TS typecheck、Buf lint、从根 module 执行的全仓 Go lint；不等于全仓 Go 测试 |
| `just api-ts-check` | 共享生成 TS 契约检查 |
| `just web::<应用>::dev`／`build`／`lint` | Example、IAM、Test、Admin 统一的 Web 开发、构建与 lint 入口；其他工具按所属 package 的 pnpm script 执行 |
| `just openfga-model-validate`／`test` | 本地 model 与场景检查 |
| `just openfga-model-apply` | 修改 model 后的应用步骤，会写外部 model／环境配置，需在明确目标环境执行 |

根插件安装中 Plateau AuthN/AuthZ 必须来自当前 checkout。Servora 工具版本由当前 Just 变量配置；不能因为父级 workspace 引用源码就假定已安装的插件也已同步。服务 Wire/Ent 生成器由根 `go.mod` 的 `tool` 声明固定，并通过 `go tool` 执行。生成路径与入口详见 [generation](../../api/proto/generation.md)。

开发或审查时先选择受影响模块和检查命令；记录实际执行与未执行项。基础设施 provider 的业务日志由 data/bootstrap 边界决定，不在构造函数中重复记录成功或失败。

## 场景：维护单 Go module 边界

### 1. 范围／触发

修改根 Go 依赖、`api/gen/go`、任一服务后端、Go 生成器或相关 Just 入口时，必须同时证明仓库可独立构建，并保留服务作为独立二进制和部署单元的边界。

### 2. 命令签名

```bash
GOWORK=off go mod tidy
GOWORK=off go list ./...
GOWORK=off go build ./...
GOWORK=off just lint
GOWORK=off just service::lint
GOWORK=off just service::_build
```

Wire 入口固定为 `go tool wire ./cmd/server`；Ent 由服务 `generate.go` 中的 `go tool ent generate ...` 调用。版本只在根 `go.mod` 的 `tool` block 中维护。

### 3. 契约

- 仓库只跟踪根 `go.mod`、`go.sum`，不得在 `api/gen` 或 `app/*/service` 新建嵌套 Go module，也不得提交仓库级 `go.work`。
- `GOWORK=off` 是可移植性门禁；未设置时允许 Go 自动发现本机父级 `/servora-kit/go.work` 进行跨仓源码联调。
- 根 lint 覆盖根共享包、生成 package 与全部服务；服务 leaf lint/build 只提供定向反馈，不能替代根门禁。

### 4. 校验与错误矩阵

| 条件 | 判定 |
| --- | --- |
| `GOWORK=off` 无法解析 Plateau 或 Servora package | 根依赖或 module 边界错误，禁止提交 |
| 父 workspace 模式通过、`GOWORK=off` 失败 | 依赖了本机源码覆盖，禁止视为验收通过 |
| 根门禁通过、leaf build/lint 失败 | 服务入口回归，必须修复或记录为迁移前基线 |
| `go mod tidy` 二次执行仍修改 `go.mod/go.sum` | 依赖状态不稳定，禁止提交 |

### 5. Good／Base／Bad

- Good：无父 workspace 的干净 checkout 仅凭根 module 完成 list、build、lint 和测试。
- Base：本机从 Plateau cwd 自动发现父 `go.work`，用于同时调试 Plateau 与 Servora 当前源码。
- Bad：为修复 leaf 命令重新增加服务 `go.mod`，或只报告父 workspace 模式成功。

### 6. 必需测试

- 两次执行 `GOWORK=off go mod tidy`，断言第二次无 diff。
- 执行根 list、build、lint 与短测试，断言没有 module/workspace 解析错误。
- 执行 Audit、Example、IAM 的 leaf build/lint，断言仍从根 module 解析依赖。
- 运行受影响生成入口并审阅生成 diff，断言 Wire/Ent 工具版本来自根 `go.mod`。

### 7. Wrong vs Correct

```bash
# Wrong：结果可能被父 workspace 悄悄覆盖
go build ./...

# Correct：先证明仓库自身完整，再按需验证父 workspace 联调
GOWORK=off go build ./...
go build ./...
```

来源：[根命令](../../../../justfile)、[服务注册](../../../../just/services.just)、[服务命令实现](../../../../just/service.just)、[根 AGENTS](../../../../AGENTS.md)。
